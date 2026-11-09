package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type paymentSceneLegacyOrder struct {
	Id          int64
	TradeNo     string `gorm:"uniqueIndex;type:varchar(64);not null"`
	UserId      int
	Provider    string          `gorm:"type:varchar(32)"`
	Method      string          `gorm:"type:varchar(32)"`
	FaceUSD     decimal.Decimal `gorm:"column:face_usd;type:decimal(20,8);not null;default:0"`
	CreditQuota int64
	PayAmount   decimal.Decimal `gorm:"type:decimal(20,8);not null;default:0"`
	PayCurrency string          `gorm:"type:varchar(16)"`
}

func (paymentSceneLegacyOrder) TableName() string { return "adty_topup_orders" }

func TestPaymentSceneMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				if upgrade {
					require.NoError(t, db.AutoMigrate(&User{}, &paymentSceneLegacyOrder{}))
					require.NoError(t, db.Create(&User{Id: 101, Username: "scene-history", Quota: 123456}).Error)
					require.NoError(t, db.Create(&paymentSceneLegacyOrder{TradeNo: "historical-checkout", UserId: 101, Provider: "bepusdt", Method: "usdt.trc20", FaceUSD: decimal.NewFromInt(2), PayAmount: decimal.NewFromInt(2), PayCurrency: "USDT", CreditQuota: 1000000}).Error)
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				config, err := GetAdtyPaymentSceneConfig(db, false)
				require.NoError(t, err)
				assert.EqualValues(t, 1, config.Version)
				var scenes []AdtyPaymentScene
				require.NoError(t, common.UnmarshalJsonStr(config.Payload, &scenes))
				require.Len(t, scenes, 1)
				assert.Equal(t, "wallet_topup", scenes[0].Key)
				assert.Len(t, scenes[0].Providers, 6)
				require.NoError(t, db.Model(config).Updates(map[string]any{"version": 8, "payload": "[]"}).Error)
				for range 2 {
					require.NoError(t, migrateDB())
				}
				config, err = GetAdtyPaymentSceneConfig(db, false)
				require.NoError(t, err)
				assert.EqualValues(t, 8, config.Version)
				assert.Equal(t, "[]", config.Payload, "startup cannot overwrite Root's policy")
				if upgrade {
					order, err := GetAdtyTopupOrder("historical-checkout")
					require.NoError(t, err)
					require.NotNil(t, order)
					assert.Empty(t, order.PaymentScene)
					assert.Empty(t, order.PaymentSceneSnapshot)
					assert.Zero(t, order.PaymentSceneVersion)
					assert.EqualValues(t, 1000000, order.CreditQuota)
					assert.Equal(t, "2", order.FaceUSD.String())
					var buyer User
					require.NoError(t, db.First(&buyer, 101).Error)
					assert.Equal(t, 123456, buyer.Quota)
					assert.Error(t, db.Create(&AdtyTopupOrder{TradeNo: order.TradeNo}).Error, "migration must retain order uniqueness")
				}
			})
		}
	}
}
