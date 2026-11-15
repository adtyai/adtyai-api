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

func countTelegramDeliveries(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).Count(&n).Error)
	return n
}
