package service

import (
	"testing"

	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// announcementTestDB 开一个真 PG schema 并建齐二开表。
//
// service 包自己的 `adtyTestDB` 只建 TestMain 那份固定清单，公告表不在里面；
// 逐个测试自己 AutoMigrate 会漏，所以这里显式跑一遍完整迁移。
func announcementTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := prepareReferralDialectDB(t, "postgres")
	require.NoError(t, model.MigrateAdtyTablesOn(db))
	return db
}

const legacyAnnouncementsFixture = `[
  {"content":"站点将于 02:00 维护","publishDate":"2026-11-01T02:00:00Z","type":"warning","extra":"请提前保存"},
  {"content":"欢迎使用","publishDate":"2026-10-20T08:30:00Z"},
  {"content":"已恢复","publishDate":"2026-11-10T08:00:00Z","type":"success","extra":"感谢等待"}
]`

// 迁移把每条旧公告落成 IssuerSiteId=0 / Audience=all_users 的一条发布记录，
// 正文逐字节保留；重复执行不再多插一行。
func TestMigrateLegacyAnnouncementsIsIdempotentPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	created, err := MigrateLegacyAnnouncements(legacyAnnouncementsFixture)
	require.NoError(t, err)
	assert.Equal(t, 3, created)

	var rows []model.AdtyNotificationPublication
	require.NoError(t, db.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)

	for _, row := range rows {
		assert.Zero(t, row.IssuerSiteId, "旧公告都是平台级：发布域是主站")
		assert.Equal(t, "all_users", row.Audience)
		assert.Equal(t, "published", row.Status)
	}
	// 正文逐字节一致——这是唯一不会被「看起来对」蒙混过去的断言。
	// `extra` 是正文旁的短备注，不是标题：按原文顺序接在正文后面，Title 留空。
	// 放进 Title 会让前端把一条公告拆成两段，而 varchar(120) 还会把备注截掉。
	assert.Equal(t, "站点将于 02:00 维护\n\n请提前保存", rows[0].Content)
	assert.Equal(t, "欢迎使用", rows[1].Content, "没有备注的条目原样保留")
	assert.Equal(t, "已恢复\n\n感谢等待", rows[2].Content)
	assert.Empty(t, rows[0].Title, "旧结构没有标题，迁移不该造一个")
	// 颜色语义存在 Business 里，回滚时能还原。
	assert.Equal(t, "legacy:type=warning", rows[0].Business)
	assert.Equal(t, "legacy:type=success", rows[2].Business)
	assert.Empty(t, rows[1].Business, "default 类型不写标记")
	// 发布时间从 RFC3339 转成 unix，不能都变成「迁移那一刻」。
	// 期望值从同一个字面量解析出来，不写死数字——写死的数字我算错过一次，
	// 而那种错会被读成「迁移逻辑错了」。
	wantPublish, err := time.Parse(time.RFC3339, "2026-11-01T02:00:00Z")
	require.NoError(t, err)
	assert.EqualValues(t, wantPublish.Unix(), rows[0].PublishedAt)

	// 再跑一次：一行都不许多。
	created, err = MigrateLegacyAnnouncements(legacyAnnouncementsFixture)
	require.NoError(t, err)
	assert.Zero(t, created, "重复迁移必须插入 0 行")

	var count int64
	require.NoError(t, db.Model(&model.AdtyNotificationPublication{}).Count(&count).Error)
	assert.EqualValues(t, 3, count, "重跑后行数不变")
}

// 坏 JSON 不该让启动失败，但不能静默：返回错误由调用方记日志。
func TestMigrateLegacyAnnouncementsRejectsBrokenJSON(t *testing.T) {
	announcementTestDB(t)
	created, err := MigrateLegacyAnnouncements(`[{"content":`)
	assert.Error(t, err)
	assert.Zero(t, created)
}

// 空 option 是常态（没配过公告），不该报错也不该插行。
func TestMigrateLegacyAnnouncementsSkipsEmptyOption(t *testing.T) {
	db := announcementTestDB(t)
	for _, raw := range []string{"", "   ", "[]"} {
		created, err := MigrateLegacyAnnouncements(raw)
		require.NoError(t, err)
		assert.Zero(t, created)
	}
	var count int64
	require.NoError(t, db.Model(&model.AdtyNotificationPublication{}).Count(&count).Error)
	assert.Zero(t, count)
}

// 正文为空的条目跳过——旧校验会拦住它，但库里的历史数据不受校验保护。
func TestMigrateLegacyAnnouncementsSkipsBlankContent(t *testing.T) {
	db := announcementTestDB(t)
	created, err := MigrateLegacyAnnouncements(`[{"content":"   ","publishDate":"2026-11-01T02:00:00Z"}]`)
	require.NoError(t, err)
	assert.Zero(t, created)
	var count int64
	require.NoError(t, db.Model(&model.AdtyNotificationPublication{}).Count(&count).Error)
	assert.Zero(t, count)
}

// 公开面板按站点读：主站看不到分站定向公告，分站看得到自己的那条和全平台的。
func TestLegacyAnnouncementsVisiblePerSitePG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	_, err := MigrateLegacyAnnouncements(legacyAnnouncementsFixture)
	require.NoError(t, err)

	// 再插一条只给 7 号站的公告和一条给所有分站的。
	for _, row := range []model.AdtyNotificationPublication{
		{IssuerSiteId: 7, Audience: "site", TargetSiteId: 7, Kind: "platform", Title: "只给七号站",
			Content: "只给七号站", Status: "published", Revision: 1, ReadKey: "k-site-7", PublishedAt: 3},
		{IssuerSiteId: 0, Audience: "all_sites", Kind: "platform", Title: "所有分站",
			Content: "所有分站", Status: "published", Revision: 1, ReadKey: "k-all-sites", PublishedAt: 4},
	} {
		_, err := model.ImportLegacyAnnouncement(&row)
		require.NoError(t, err)
	}

	main, err := model.ListAdtyNotificationsForSite(0, 50)
	require.NoError(t, err)
	mainContents := contentsOf(main)
	assert.Contains(t, mainContents, "欢迎使用")
	assert.Contains(t, mainContents, "所有分站")
	assert.NotContains(t, mainContents, "只给七号站", "主站不能看到分站定向公告")

	seven, err := model.ListAdtyNotificationsForSite(7, 50)
	require.NoError(t, err)
	sevenContents := contentsOf(seven)
	assert.Contains(t, sevenContents, "只给七号站")
	// 旧公告是 all_users，也就是「主站公告」：分站看不到。这正是老板要的形态，
	// 也是这次迁移不会污染分站的原因（迁移前它们就已经只有主站可见）。
	assert.NotContains(t, sevenContents, "欢迎使用", "主站公告不同步到分站")
	assert.Contains(t, sevenContents, "所有分站", "all_sites 是平台级，分站可见")

	eight, err := model.ListAdtyNotificationsForSite(8, 50)
	require.NoError(t, err)
	assert.NotContains(t, contentsOf(eight), "只给七号站", "别的站看不到七号站的公告")
}

func contentsOf(rows []model.AdtyNotificationPublication) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Content)
	}
	return out
}

// 迁移之后，收件箱里每一条旧公告**只出现一次**。
//
// 这条是这次并入里最容易漏的地方：`adtyCurrentNotificationInbox` 原来同时读
// 旧 option 和 publications，迁移把同样的内容搬进新表之后，主站收件箱就会把
// 每条旧公告显示两遍，而且两份的 read_key 不一样——已读一份，另一份还是未读，
// 未读角标也就翻倍。删掉旧 option 那段读之后，这里必须只有一份。
func TestNotificationInboxShowsMigratedAnnouncementOncePG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	setting := console_setting.GetConsoleSetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	setting.AnnouncementsEnabled = true
	setting.Announcements = `[{"content":"只此一条","publishDate":"2026-11-01T02:00:00Z","type":"warning"}]`

	created, err := MigrateLegacyAnnouncements(setting.Announcements)
	require.NoError(t, err)
	require.Equal(t, 1, created)

	user := ticketTestUser(t, db, "inbox-once", 1)
	inbox, err := GetAdtyNotificationInbox(user.Id, nil, 0)
	require.NoError(t, err)

	count := 0
	for _, item := range inbox.Announcements {
		if item["content"] == "只此一条" {
			count++
		}
	}
	assert.Equal(t, 1, count, "旧 option 与新表同时读会让同一条公告出现两次")
	assert.EqualValues(t, 1, inbox.UnreadPublications)
}

// 数组重排不能让公告重插一遍。
//
// 旧面板就是按数组编辑的：删掉第 2 条，第 3 条就变成第 2 条。如果 ReadKey 把
// 数组下标算进哈希，后面每一条都会算出新键、全部重插——公告在面板上直接重复。
//
// 这条钉的是「ReadKey 不随位置变化」。把下标加回哈希，它立刻红。
func TestMigrateLegacyAnnouncementsIsStableAcrossReorderPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	created, err := MigrateLegacyAnnouncements(legacyAnnouncementsFixture)
	require.NoError(t, err)
	require.Equal(t, 3, created)

	// 删掉中间那条（顺序全变），再迁一次。
	reordered := `[
  {"content":"站点将于 02:00 维护","publishDate":"2026-11-01T02:00:00Z","type":"warning","extra":"请提前保存"},
  {"content":"已恢复","publishDate":"2026-11-10T08:00:00Z","type":"success","extra":"感谢等待"}
]`
	created, err = MigrateLegacyAnnouncements(reordered)
	require.NoError(t, err)
	assert.Zero(t, created, "重排不产生新行：ReadKey 不该随数组位置变化")

	var count int64
	require.NoError(t, db.Model(&model.AdtyNotificationPublication{}).Count(&count).Error)
	assert.EqualValues(t, 3, count, "删掉的那条仍在库里（迁移不是同步，不删数据）")

	// 重复启动也不插：内容没变就撞唯一索引。
	for range 3 {
		created, err = MigrateLegacyAnnouncements(legacyAnnouncementsFixture)
		require.NoError(t, err)
		assert.Zero(t, created, "内容没变就一次都不该再插")
	}
}

// 旧面板里复制一条、连日期都没改：两条都要留下，不能第二条被静默丢掉。
func TestMigrateLegacyAnnouncementsKeepsTrueDuplicatesPG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	raw := `[
  {"content":"重复的那条","publishDate":"2026-11-01T02:00:00Z"},
  {"content":"重复的那条","publishDate":"2026-11-01T02:00:00Z"}
]`
	created, err := MigrateLegacyAnnouncements(raw)
	require.NoError(t, err)
	assert.Equal(t, 2, created, "真重复的两条都留着：序号是「同样内容里的第几条」")

	var count int64
	require.NoError(t, db.Model(&model.AdtyNotificationPublication{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}
