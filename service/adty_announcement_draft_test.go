package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 草稿就是「先存着、还没发」：它不出现在任何读者面前，也不推 TG。
//
// 没有草稿的话，站长写一条公告只能一次成型——旧面板的「自选发布时间」其实
// 顶掉了这个位置（把时间设到未来），而发布时间被去掉之后那个位置就空了。
func TestAnnouncementDraftIsInvisibleUntilPublishedPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.NotZero(t, version, "live server_version_num must be recorded as evidence")
	t.Logf("evidence only (not a pass/fail condition): server_version_num=%d", version)

	t.Setenv("CRYPTO_SECRET", "adty-announcement-draft-test-secret")
	cipher, err := model.SealAdtyPurposeSecret("telegram:"+model.AdtyTelegramPurposeAdmin, "123456789:token")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.AdtyTelegramBot{
		Purpose: model.AdtyTelegramPurposeAdmin, BotId: 1, Username: "root_bot",
		TokenCipher: cipher, Enabled: true,
	}).Error)
	var sent int
	previous := AdtyTelegramSender
	AdtyTelegramSender = func(token, chat, text string) error { sent++; return nil }
	t.Cleanup(func() { AdtyTelegramSender = previous })

	root := ticketTestUser(t, db, "draft-root", 100)

	// 存草稿时可以是空的——「先建个壳」正是草稿的用途。
	draft, err := SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Kind: "platform", Audience: "all_users", Draft: true,
	}, false)
	require.NoError(t, err)
	require.Equal(t, "draft", draft.Status)
	require.Zero(t, draft.PublishedAt, "草稿没有发布时间")

	// 读者看不到它。
	public, err := model.ListAdtyNotificationsForSite(0, 50)
	require.NoError(t, err)
	assert.Empty(t, public, "草稿不该出现在公开面板")

	user := ticketTestUser(t, db, "draft-reader", 1)
	inbox, err := GetAdtyNotificationInbox(user.Id, nil, 0)
	require.NoError(t, err)
	assert.Empty(t, inbox.Announcements, "草稿不该出现在收件箱")
	assert.Zero(t, inbox.UnreadPublications, "草稿不该计入未读")
	assert.Zero(t, countTelegramDeliveries(t, db), "草稿不该推 TG")

	// 补完内容，再存一次草稿：仍然不发。
	filled, err := SaveAdtyNotificationPublication(root.Id, nil, draft.Id, AdtyNotificationInput{
		Title: "维护", Content: "内容", Kind: "platform", Audience: "all_users",
		Revision: draft.Revision, Draft: true,
	}, false)
	require.NoError(t, err)
	assert.Equal(t, "draft", filled.Status)
	assert.Zero(t, filled.PublishedAt)
	assert.Zero(t, countTelegramDeliveries(t, db), "改草稿不该推")

	// 发布：状态翻转、发布时间落现在、TG 入队一条。
	published, err := SaveAdtyNotificationPublication(root.Id, nil, draft.Id, AdtyNotificationInput{
		Title: "维护", Content: "内容", Kind: "platform", Audience: "all_users",
		Revision: filled.Revision, SyncTelegram: true, TelegramChannel: "@adty_ops",
	}, false)
	require.NoError(t, err)
	assert.Equal(t, "published", published.Status)
	assert.NotZero(t, published.PublishedAt, "发布时才有发布时间")
	assert.EqualValues(t, 1, countTelegramDeliveries(t, db), "发布才推一次")

	public, err = model.ListAdtyNotificationsForSite(0, 50)
	require.NoError(t, err)
	assert.Len(t, public, 1, "发布之后读者才看得到")

	// 再改一遍：状态仍是 published、发布时间不变（不能把老公告顶到最新）、不重推。
	earlier := published.PublishedAt
	edited, err := SaveAdtyNotificationPublication(root.Id, nil, draft.Id, AdtyNotificationInput{
		Title: "维护", Content: "改过的内容", Kind: "platform", Audience: "all_users",
		Revision: published.Revision,
	}, false)
	require.NoError(t, err)
	assert.Equal(t, "published", edited.Status)
	assert.EqualValues(t, earlier, edited.PublishedAt, "编辑不刷新发布时间")
	assert.EqualValues(t, 1, countTelegramDeliveries(t, db), "推过的公告编辑不重推")
}

// 已发布的不能退回草稿：那是「悄无声息地把它从所有人眼前拿走」，而撤回是显式
// 且有历史的动作。这条挡住的是「用草稿状态当隐藏开关」。
func TestPublishedAnnouncementCannotBecomeDraftPG(t *testing.T) {
	announcementTestDB(t)
	root := ticketTestUser(t, model.DB, "draft-reject-root", 100)

	published, err := SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "已发布", Content: "正文", Kind: "system", Audience: "all_users",
	}, false)
	require.NoError(t, err)
	require.Equal(t, "published", published.Status)

	_, err = SaveAdtyNotificationPublication(root.Id, nil, published.Id, AdtyNotificationInput{
		Title: "已发布", Content: "正文", Kind: "system", Audience: "all_users",
		Revision: published.Revision, Draft: true,
	}, false)
	require.Error(t, err, "退回草稿必须被拒")
	assert.Contains(t, err.Error(), "drafts")

	// 撤回仍然可用：那才是「拿下来」的正确动作。
	withdrawn, err := SaveAdtyNotificationPublication(root.Id, nil, published.Id, AdtyNotificationInput{
		Revision: published.Revision,
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "withdrawn", withdrawn.Status)
}

// 撤回的公告不能再编辑（它是历史记录，不是编辑对象）。
func TestWithdrawnAnnouncementIsNotEditablePG(t *testing.T) {
	announcementTestDB(t)
	root := ticketTestUser(t, model.DB, "draft-withdrawn-root", 100)

	row, err := SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "要撤回的", Content: "正文", Kind: "system", Audience: "all_users",
	}, false)
	require.NoError(t, err)
	withdrawn, err := SaveAdtyNotificationPublication(root.Id, nil, row.Id, AdtyNotificationInput{
		Revision: row.Revision,
	}, true)
	require.NoError(t, err)

	_, err = SaveAdtyNotificationPublication(root.Id, nil, row.Id, AdtyNotificationInput{
		Title: "改", Content: "改", Kind: "system", Audience: "all_users",
		Revision: withdrawn.Revision,
	}, false)
	require.Error(t, err)
}
