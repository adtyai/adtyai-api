package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 公告发布到 TG 队列：勾了才入队，不勾不入队；入队后能真正发出去。
//
// 修前 `QueueAdtyPublicTelegram` 零生产调用方，`RetryAdtyTelegram` 里
// `!row.Public` 又把公开行排除在发送之外，而 `AdtyTelegramSender` 全仓没有
// 任何赋值——三处合起来，公开公告这条路整条是死的。
func TestAnnouncementQueuesTelegramOnlyWhenAsked(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	root := ticketTestUser(t, db, "announce-root", 100)
	// bot token 必须真的能被 OpenAdtyPurposeSecret 打开，否则重试任务会停在
	// 「bot token unavailable」，测不到发送分支。cipher 依赖 CRYPTO_SECRET，
	// 测试里给一个 32 字节以上的值（生产由部署注入）。
	t.Setenv("CRYPTO_SECRET", "adty-announcement-telegram-test-secret")
	cipher, err := model.SealAdtyPurposeSecret("telegram:"+model.AdtyTelegramPurposeAdmin, "123456789:token")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.AdtyTelegramBot{
		Purpose: model.AdtyTelegramPurposeAdmin, BotId: 1, Username: "root_bot",
		TokenCipher: cipher, Enabled: true,
	}).Error)

	var sent []string
	previous := AdtyTelegramSender
	AdtyTelegramSender = func(token, chat, text string) error {
		sent = append(sent, chat+"|"+text)
		return nil
	}
	t.Cleanup(func() { AdtyTelegramSender = previous })

	// 不勾：只发站内，队列里一行都不该有。
	_, err = SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "Maintenance", Content: "Tonight 02:00", Kind: "maintenance",
		Audience: "all_users",
	}, false)
	require.NoError(t, err)
	assert.Zero(t, countTelegramDeliveries(t, db), "没勾同步就不该入队")
	assert.EqualValues(t, 0, telegramAtOf(t, db, "Maintenance", "Tonight 02:00"), "没推过就不能留推送标记")

	// 勾了：入队一条，频道落库。
	_, err = SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "Maintenance", Content: "Tomorrow 02:00", Kind: "maintenance",
		Audience: "all_users", SyncTelegram: true, TelegramChannel: "@adty_ops",
	}, false)
	require.NoError(t, err)

	var row model.AdtyTelegramDelivery
	require.NoError(t, db.Where("public = ?", true).First(&row).Error)
	assert.Equal(t, "@adty_ops", row.Channel, "目标频道必须落库，否则入队后不知道发到哪")
	assert.Equal(t, model.AdtyTelegramPurposeAdmin, row.Purpose)
	assert.Equal(t, model.AdtyTelegramDeliveryWait, row.Status)

	// 发送：Public 行必须走频道分支，而不是被 !row.Public 排掉。
	n, err := RetryAdtyTelegram()
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, sent, 1)
	assert.Equal(t, "@adty_ops", sent[0][:len("@adty_ops")])

	var after model.AdtyTelegramDelivery
	require.NoError(t, db.First(&after, row.Id).Error)
	assert.Equal(t, model.AdtyTelegramDeliverySent, after.Status, "公开公告必须真的发出去")
}

// 同一段文字发到两个频道是两条投递；只哈希 body 会让第二条被唯一索引丢掉。
func TestPublicTelegramDedupesPerChannel(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	_, err := SaveAdtyNotificationPublication(ticketTestUser(t, db, "announce-dedupe", 100).Id, nil, 0, AdtyNotificationInput{
		Title: "Same text", Content: "Same body", Kind: "system", Audience: "all_users",
	}, false)
	require.NoError(t, err)

	body := "Same text\n\nSame body"
	require.NoError(t, QueueAdtyPublicTelegram("@channel_a", model.AdtyTelegramPurposeAdmin, body))
	require.NoError(t, QueueAdtyPublicTelegram("@channel_b", model.AdtyTelegramPurposeAdmin, body))
	assert.EqualValues(t, 2, countTelegramDeliveries(t, db), "不同频道是两条，不能因为正文相同被去重")

	// 同频道同正文仍然去重（唯一索引兜底）。
	err = QueueAdtyPublicTelegram("@channel_a", model.AdtyTelegramPurposeAdmin, body)
	assert.Error(t, err, "同频道同正文应撞唯一索引")
}

// 公告正文里的凭据要被脱敏后入队，而不是整条拒掉。
func TestPublicTelegramRedactsInsteadOfRejecting(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	require.NoError(t, QueueAdtyPublicTelegram("@ops", model.AdtyTelegramPurposeAdmin,
		"key rotation: api_key=sk-abcdefgh12345678"))

	var row model.AdtyTelegramDelivery
	require.NoError(t, db.Where("channel = ?", "@ops").First(&row).Error)
	assert.NotContains(t, row.Body, "sk-abcdefgh12345678")
	assert.Contains(t, row.Body, "[redacted]", "脱敏要留可见痕迹，管理员才知道系统动过")

	// 私有词仍然拒——脱敏解决不了「这段内容不该进公开频道」。
	assert.Error(t, QueueAdtyPublicTelegram("@ops", model.AdtyTelegramPurposeAdmin, "订单 123 已退款"))
}

func telegramAtOf(t *testing.T, db *gorm.DB, title, content string) int64 {
	t.Helper()
	var row model.AdtyNotificationPublication
	require.NoError(t, db.Where("title = ? AND content = ?", title, content).First(&row).Error)
	return row.TelegramAt
}

func countTelegramDeliveries(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).Count(&n).Error)
	return n
}

// 同一条公告改三次只能推一次。
//
// 「新建才推、编辑不推」按操作类型分流的写法看起来等价，但公告先发布、后勾上
// 同步时就永远没机会推；这条测的正是「按推过没有分流」：勾上之后仍能推一次，
// 之后无论改多少遍都不再推。
func TestAnnouncementPushesTelegramOnceOnly(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	root := ticketTestUser(t, db, "announce-once", 100)
	t.Setenv("CRYPTO_SECRET", "adty-announcement-telegram-test-secret")
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

	// 先发布，不勾同步。
	row, err := SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "Rules", Content: "First version", Kind: "rules", Audience: "all_users",
	}, false)
	require.NoError(t, err)
	require.Zero(t, telegramAtOf(t, db, "Rules", "First version"))
	assert.Zero(t, countTelegramDeliveries(t, db))

	// 后来勾上同步：仍然能推一次（这正是「按操作类型分流」做不到的）。
	_, err = SaveAdtyNotificationPublication(root.Id, nil, row.Id, AdtyNotificationInput{
		Title: "Rules", Content: "First version", Kind: "rules", Audience: "all_users",
		Revision: row.Revision, SyncTelegram: true, TelegramChannel: "@adty_ops",
	}, false)
	require.NoError(t, err)
	assert.EqualValues(t, 1, countTelegramDeliveries(t, db), "后勾上同步也要推一次")
	assert.NotZero(t, telegramAtOf(t, db, "Rules", "First version"))

	// 再改两遍（改错别字）：一条都不该新增。
	var current model.AdtyNotificationPublication
	require.NoError(t, db.Where("id = ?", row.Id).First(&current).Error)
	for _, body := range []string{"Second version", "Third version"} {
		_, err = SaveAdtyNotificationPublication(root.Id, nil, row.Id, AdtyNotificationInput{
			Title: "Rules", Content: body, Kind: "rules", Audience: "all_users",
			Revision: current.Revision, SyncTelegram: true, TelegramChannel: "@adty_ops",
		}, false)
		require.NoError(t, err)
		require.NoError(t, db.Where("id = ?", row.Id).First(&current).Error)
	}
	assert.EqualValues(t, 1, countTelegramDeliveries(t, db), "推过的公告再改不重推")
	assert.Zero(t, sent, "还没到发送时机（Retry 未跑）")

	_, err = RetryAdtyTelegram()
	require.NoError(t, err)
	assert.Equal(t, 1, sent, "队列里只有一条")
}

// 并发保存同一公告时只有一个能抢到推送权。
//
// 条件写在 SQL 里（telegram_at = 0）而不是先读后写；应用层 check-then-write
// 在这里会漏——两个请求都能读到 0。把 claim 改成先读后写，这条会红。
func TestAnnouncementTelegramSlotIsClaimedOnceUnderConcurrency(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	root := ticketTestUser(t, db, "announce-race", 100)
	row, err := SaveAdtyNotificationPublication(root.Id, nil, 0, AdtyNotificationInput{
		Title: "Race", Content: "Body", Kind: "system", Audience: "all_users",
	}, false)
	require.NoError(t, err)

	claims := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		go func(at int64) {
			won, err := model.ClaimAdtyNotificationTelegramSlot(row.Id, at)
			if err != nil {
				claims <- false
				return
			}
			claims <- won
		}(int64(1000 + i))
	}
	winners := 0
	for i := 0; i < 8; i++ {
		if <-claims {
			winners++
		}
	}
	assert.Equal(t, 1, winners, "8 个并发保存只能有一个拿到推送权")
}
