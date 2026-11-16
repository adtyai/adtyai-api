package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/setting/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm/clause"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Subscriptions, redemption codes and upstream invite rewards were deleted in
// tasks 1.2-1.5 (docs/adty/deleted-features.md). A new database must not get
// their tables and columns back from an upstream merge. The full migration
// runs on SQLite because the deleted subscription_plans DDL only ran there.
func TestAdtyDeletedSchemaAbsent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = oldDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, migrateDB())

	require.True(t, db.Migrator().HasTable("users"))
	for _, table := range []string{
		"subscription_plans",
		"subscription_orders",
		"user_subscriptions",
		"subscription_pre_consume_records",
		"redemptions",
	} {
		assert.Falsef(t, db.Migrator().HasTable(table), "deleted table %s is created again", table)
	}
	for _, column := range []string{"aff_code", "aff_count", "aff_quota", "aff_history", "inviter_id"} {
		assert.Falsef(t, db.Migrator().HasColumn("users", column), "deleted column users.%s is created again", column)
	}
}

func TestAdtyDeletedOptionKeysAbsent(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMapRWMutex.Unlock()
	// InitOptionMap ends with loadOptionsFromDatabase, which also rebuilds the
	// request policy snapshot from the new map.
	oldRequestPolicy := requestPolicySnapshot.Load()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
		requestPolicySnapshot.Store(oldRequestPolicy)
	})

	InitOptionMap()

	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	require.NotEmpty(t, common.OptionMap)
	for _, key := range []string{"QuotaForInviter", "QuotaForInvitee", "TopUpLink", "WaffoSubscriptionReturnUrl"} {
		assert.NotContainsf(t, common.OptionMap, key, "deleted option %s is published again", key)
	}
}

// Test-only schema fixtures copied from 0826fdf, before marketing removal.
type legacyAdtyMarketingContact struct {
	UserId          int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Email           string `json:"email" gorm:"type:varchar(255);index"`
	Consent         bool   `json:"consent" gorm:"not null"`
	ConsentAt       int64  `json:"consent_at" gorm:"not null"`
	ConsentIpHash   string `json:"-" gorm:"type:varchar(64)"`
	ConsentLang     string `json:"consent_lang" gorm:"type:varchar(16)"`
	ConsentVersion  string `json:"consent_version" gorm:"type:varchar(32)"`
	ConsentSource   string `json:"consent_source" gorm:"type:varchar(16)"` // register | profile
	Unsubscribed    bool   `json:"unsubscribed" gorm:"not null"`
	UnsubscribedAt  int64  `json:"unsubscribed_at" gorm:"not null"`
	Bounced         bool   `json:"bounced" gorm:"not null"`
	Complained      bool   `json:"complained" gorm:"not null"`
	ResendContactId string `json:"-" gorm:"type:varchar(64);index"`
	SyncedEmail     string `json:"-" gorm:"type:varchar(255)"` // Resend 上这个联系人的邮箱；本地邮箱变了要删掉重建
	Lang            string `json:"lang" gorm:"type:varchar(16)"`
	SiteId          int    `json:"site_id" gorm:"index;not null"`
	SyncedAt        int64  `json:"synced_at" gorm:"not null"`
	UpdatedAt       int64  `json:"updated_at" gorm:"index;not null;autoUpdateTime:false"`
}

func (legacyAdtyMarketingContact) TableName() string { return "adty_marketing_contacts" }

type legacyAdtyMarketingWebhookEvent struct {
	Id          string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	Type        string `json:"type" gorm:"type:varchar(48);index:idx_adty_mkt_event_broadcast,priority:2"`
	BroadcastId string `json:"broadcast_id" gorm:"type:varchar(64);index:idx_adty_mkt_event_broadcast,priority:1"`
	EmailId     string `json:"email_id" gorm:"type:varchar(64)"`
	Email       string `json:"email" gorm:"type:varchar(255)"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
}

func (legacyAdtyMarketingWebhookEvent) TableName() string { return "adty_marketing_webhook_events" }

type legacyAdtyMarketingCampaign struct {
	Id          int          `json:"id"`
	Name        string       `json:"name" gorm:"type:varchar(128);not null"`
	IsTemplate  bool         `json:"is_template" gorm:"not null;index"`
	Status      string       `json:"status" gorm:"type:varchar(16);not null;index"`
	Subjects    AdtyLangText `json:"subjects" gorm:"type:text;not null"`
	Bodies      AdtyLangText `json:"bodies" gorm:"type:text;not null"` // HTML
	Audience    string       `json:"audience" gorm:"type:text;not null"`
	ScheduledAt int64        `json:"scheduled_at" gorm:"not null"` // 0 立即发
	Recipients  int          `json:"recipients" gorm:"not null"`
	Error       string       `json:"error" gorm:"type:varchar(255)"`
	CreatedBy   int          `json:"created_by" gorm:"not null"`
	SentBy      int          `json:"sent_by" gorm:"not null"`
	CreatedAt   int64        `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64        `json:"updated_at" gorm:"autoUpdateTime"`
	SentAt      int64        `json:"sent_at" gorm:"not null"`
}

func (legacyAdtyMarketingCampaign) TableName() string { return "adty_marketing_campaigns" }

type legacyAdtyMarketingCampaignSend struct {
	Id          int    `json:"id"`
	CampaignId  int    `json:"campaign_id" gorm:"index;not null"`
	SiteId      int    `json:"site_id" gorm:"not null"`
	Lang        string `json:"lang" gorm:"type:varchar(16);not null"`
	SegmentId   string `json:"segment_id" gorm:"type:varchar(64)"`
	BroadcastId string `json:"broadcast_id" gorm:"type:varchar(64);index"`
	Recipients  int    `json:"recipients" gorm:"not null"`
	Status      string `json:"status" gorm:"type:varchar(16);not null"`
	Error       string `json:"error" gorm:"type:varchar(255)"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (legacyAdtyMarketingCampaignSend) TableName() string { return "adty_marketing_campaign_sends" }

type legacyAdtyMarketingCampaignRecipient struct {
	CampaignId int    `json:"campaign_id" gorm:"primaryKey;autoIncrement:false"`
	UserId     int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	SendId     int    `json:"send_id" gorm:"index;not null"`
	Email      string `json:"email" gorm:"type:varchar(255);not null"`
}

func (legacyAdtyMarketingCampaignRecipient) TableName() string {
	return "adty_marketing_campaign_recipients"
}

type legacyAdtyMarketingTriggerSend struct {
	Rule      string `json:"rule" gorm:"primaryKey;type:varchar(32)"`
	UserId    int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Status    string `json:"status" gorm:"type:varchar(16);not null"`
	EmailId   string `json:"email_id" gorm:"type:varchar(64)"`
	CreatedAt int64  `json:"created_at" gorm:"not null;index"`
}

func (legacyAdtyMarketingTriggerSend) TableName() string { return "adty_marketing_trigger_sends" }

type legacyAdtySignupSource struct {
	UserId        int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	InviterUserId int    `json:"inviter_user_id" gorm:"index;not null"` // P6 推广人，0 表示没有
	SiteId        int    `json:"site_id" gorm:"index;not null"`         // P11 站点，0 是主站
	UtmSource     string `json:"utm_source" gorm:"type:varchar(64);index;not null"`
	UtmMedium     string `json:"utm_medium" gorm:"type:varchar(64);not null"`
	UtmCampaign   string `json:"utm_campaign" gorm:"type:varchar(64);not null"`
	LandingPath   string `json:"landing_path" gorm:"type:varchar(255);not null"`
	ReferrerHost  string `json:"referrer_host" gorm:"type:varchar(255);index;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"index;not null"`
}

func (legacyAdtySignupSource) TableName() string { return "adty_signup_sources" }

// 优惠码表在 2026-11 删除优惠码残留后不再新建；升级时旧表和旧行原样保留。
type legacyAdtyPromoCode struct {
	Id        int    `gorm:"primaryKey"`
	Code      string `gorm:"type:varchar(64);uniqueIndex"`
	Value     string `gorm:"type:varchar(32)"`
	SiteId    int
	CreatedAt int64
}

func (legacyAdtyPromoCode) TableName() string { return "adty_promo_codes" }

type legacyAdtyPromoRedemption struct {
	Id        int `gorm:"primaryKey"`
	CodeId    int
	UserId    int
	TradeNo   string `gorm:"type:varchar(64);uniqueIndex"`
	CreatedAt int64
}

func (legacyAdtyPromoRedemption) TableName() string { return "adty_promo_redemptions" }

func TestMarketingRemovalDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%v", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				require.NoError(t, db.AutoMigrate(&Option{}))
				oldPolicy := requestPolicySnapshot.Load()
				common.OptionMapRWMutex.Lock()
				oldOptions := common.OptionMap
				common.OptionMap = map[string]string{}
				common.OptionMapRWMutex.Unlock()
				t.Cleanup(func() {
					requestPolicySnapshot.Store(oldPolicy)
					common.OptionMapRWMutex.Lock()
					common.OptionMap = oldOptions
					common.OptionMapRWMutex.Unlock()
				})
				legacy := []any{
					&legacyAdtyMarketingContact{UserId: 91, Email: "history@example.test", Consent: true, Unsubscribed: true, Bounced: true, Complained: true, SiteId: 4, UpdatedAt: 1234},
					&legacyAdtyMarketingWebhookEvent{Id: "history-event", Type: "email.complained", BroadcastId: "history-broadcast", Email: "history@example.test", CreatedAt: 1234},
					&legacyAdtyMarketingCampaign{Id: 7, Name: "Historical scheduled campaign", Status: "scheduled", Subjects: AdtyLangText{"en": "Historical subject"}, Bodies: AdtyLangText{"en": "<p>Historical body</p>"}, Audience: "{\"include_site_customers\":true}", ScheduledAt: 4102444800, Recipients: 1, CreatedBy: 1, CreatedAt: 1234, UpdatedAt: 1234},
					&legacyAdtyMarketingCampaignSend{Id: 8, CampaignId: 7, SiteId: 4, Lang: "en", BroadcastId: "history-broadcast", Recipients: 1, Status: "sending", CreatedAt: 1234},
					&legacyAdtyMarketingCampaignRecipient{CampaignId: 7, UserId: 91, SendId: 8, Email: "history@example.test"},
					&legacyAdtyMarketingTriggerSend{Rule: "winback", UserId: 91, Status: "sending", EmailId: "history-email", CreatedAt: 1234},
					&legacyAdtySignupSource{UserId: 91, InviterUserId: 10, SiteId: 4, UtmSource: "historical", UtmCampaign: "archive", LandingPath: "/old-landing", ReferrerHost: "example.test", CreatedAt: 1234},
					&legacyAdtyPromoCode{Id: 1, Code: "HISTORY10", Value: "10", SiteId: 4, CreatedAt: 1234},
					&legacyAdtyPromoRedemption{Id: 1, CodeId: 1, UserId: 91, TradeNo: "history-order", CreatedAt: 1234},
				}
				before := map[string][]map[string]any{}
				if upgrade {
					require.NoError(t, db.AutoMigrate(legacy...))
					require.NoError(t, db.AutoMigrate(&Option{}))
					for _, row := range legacy {
						require.NoError(t, db.Create(row).Error)
					}
					require.NoError(t, db.Create(&[]Option{
						{Key: "adty_marketing_setting.promo_enabled", Value: "true"},
						{Key: "adty_marketing_setting.enabled", Value: "true"},
						{Key: "adty_marketing_setting.triggers_enabled", Value: "true"},
					}).Error)
					for _, row := range legacy {
						table := row.(interface{ TableName() string }).TableName()
						var rows []map[string]any
						require.NoError(t, db.Table(table).Find(&rows).Error)
						before[table] = rows
					}
				}
				for range 2 {
					// The changed startup entry point is shared by every dialect. Full startup
					// is also covered by TestAdtyDeletedSchemaAbsent and the PostgreSQL browser gate.
					require.NoError(t, MigrateAdtyTables())
					// Historical marketing option rows, including the old promo switch, load without error.
					loadOptionsFromDatabase()
					for _, row := range legacy {
						table := row.(interface{ TableName() string }).TableName()
						assert.Equal(t, upgrade, db.Migrator().HasTable(table), table)
						if upgrade {
							var after []map[string]any
							require.NoError(t, db.Table(table).Find(&after).Error)
							assert.Equal(t, before[table], after, "historical data changed: %s", table)
						}
					}
					for key := range config.GlobalConfig.ExportAllConfigs() {
						assert.Falsef(t, strings.HasPrefix(key, "adty_marketing_setting."), "retired setting registered again: %s", key)
					}
				}
			})
		}
	}
}

func isolatedMigrationTestDatabase(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	if dialect == "postgres" {
		if os.Getenv("ADTY_TEST_PG_DSN") == "" {
			t.Skip("ADTY_TEST_PG_DSN is not configured")
		}
		opened, err := OpenAdtyTestDatabase()
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, opened.Close()) })
		var version string
		require.NoError(t, opened.DB.Raw("SELECT version()").Scan(&version).Error)
		t.Logf("postgres version: %s", version)
		return opened.DB
	}
	var driver gorm.Dialector
	if dialect == "sqlite" {
		driver = sqlite.Open(filepath.Join(t.TempDir(), "marketing-removal.db"))
	} else {
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		cfg, err := mysqldriver.ParseDSN(dsn)
		require.NoError(t, err)
		admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		adminSQL, err := admin.DB()
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, adminSQL.Close()) })
		cfg.DBName = fmt.Sprintf("codex_adty_mkt_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE DATABASE ?", clause.Table{Name: cfg.DBName}).Error)
		t.Cleanup(func() { assert.NoError(t, admin.Exec("DROP DATABASE ?", clause.Table{Name: cfg.DBName}).Error) })
		driver = mysqlMigrationDialector{mysql.Dialector{Config: &mysql.Config{DSN: cfg.FormatDSN()}}}
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLOG := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
	initCol()
	t.Cleanup(func() {
		assert.NoError(t, sqlDB.Close())
		DB, LOG_DB = oldDB, oldLOG
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
	})
	query := "SELECT version()"
	if dialect == "sqlite" {
		query = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(query).Scan(&version).Error)
	t.Logf("%s version: %s", dialect, version)
	return db
}
