package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 公开公告的投递目标必须真的落在表里：channel 以前只参与 EventKey 哈希、
// 不落库，于是「同一段文字发到两个频道」算同一个 EventKey，被唯一索引丢掉一条；
// 而重试任务也没有任何办法知道该发到哪。
//
// 这条在真 PG 上跑两遍迁移（模拟重复启动），确认：
//  1. 全新库上迁移会建出 channel 列；
//  2. 第二次启动不报错、也不把已有数据清掉。
func TestAdtyTelegramDeliveryChannelColumnPG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	require.True(t, db.Migrator().HasColumn(&AdtyTelegramDelivery{}, "channel"))

	require.NoError(t, db.Create(&AdtyTelegramDelivery{
		EventKey: "public:seed", Purpose: AdtyTelegramPurposeAdmin, Body: "hello",
		Channel: "@ops", Status: AdtyTelegramDeliveryWait, Public: true,
	}).Error)

	// 重复启动：迁移再跑一次，列还在、行还在、值还在。
	require.NoError(t, MigrateAdtyTablesOn(db), "重复启动不应失败")
	require.True(t, db.Migrator().HasColumn(&AdtyTelegramDelivery{}, "channel"))

	var row AdtyTelegramDelivery
	require.NoError(t, db.Where("event_key = ?", "public:seed").First(&row).Error)
	assert.Equal(t, "@ops", row.Channel, "重复迁移不能丢已有的频道")
}

// 公告表新增 telegram_channel / telegram_at 两列，同样要在全新库上建出来，
// 且重复启动既不改已有值、也不把 telegram_at 抹回 0（抹回去等于允许重推）。
func TestAdtyNotificationTelegramColumnsPG(t *testing.T) {
	db := adtyTestDBWithSchema(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	require.True(t, db.Migrator().HasColumn(&AdtyNotificationPublication{}, "telegram_channel"))
	require.True(t, db.Migrator().HasColumn(&AdtyNotificationPublication{}, "telegram_at"))

	require.NoError(t, db.Create(&AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: 1, PublisherName: "root", Audience: "all_users",
		Kind: "system", Title: "Seed", Content: "Body", Status: "published",
		ReadKey: "seed-key", PublishedAt: 1, Revision: 1,
		TelegramChannel: "@ops", TelegramAt: 1700000000,
	}).Error)

	require.NoError(t, MigrateAdtyTablesOn(db), "重复启动不应失败")

	var row AdtyNotificationPublication
	require.NoError(t, db.Where("read_key = ?", "seed-key").First(&row).Error)
	assert.Equal(t, "@ops", row.TelegramChannel, "重复迁移不能丢频道")
	assert.EqualValues(t, 1700000000, row.TelegramAt, "重复迁移不能把已推标记抹回 0")
}
