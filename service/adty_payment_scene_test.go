package service

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPaymentSceneDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := prepareReferralDialectDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&model.AuditIntent{}))
			oldGateway := *setting.GetBEpusdtSetting()
			*setting.GetBEpusdtSetting() = setting.BEpusdtSetting{Enabled: true, BaseURL: "https://scene-gateway.example", Secret: "synthetic-scene-secret", TradeTypes: []string{"usdt.trc20"}}
			t.Cleanup(func() { *setting.GetBEpusdtSetting() = oldGateway })
			root := referralUser(t, db, "scene-root")
			require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
			buyer := referralUser(t, db, "scene-customer")
			config, err := GetAdtyPaymentScenes()
			require.NoError(t, err)
			scene := config.Scenes[0]
			scene.Key, scene.Name = "restricted", "Original checkout"
			scene.Providers, scene.Methods, scene.Currencies = []string{"bepusdt"}, []string{"usdt.trc20"}, []string{"USDT"}
			scene.SiteScope, scene.UserTypes = "main", []string{"main_customer"}
			scene.MinUSD, scene.MaxUSD = decimal.NewFromInt(50), decimal.NewFromInt(100)
			_, err = SaveAdtyPaymentScenes(buyer.Id, 1, []model.AdtyPaymentScene{scene}, model.AuditLog{})
			require.Error(t, err, "the service also rejects a non-Root actor")
			saved, err := SaveAdtyPaymentScenes(root.Id, 1, []model.AdtyPaymentScene{scene}, model.AuditLog{})
			require.NoError(t, err)
			require.EqualValues(t, 2, saved.Version)
			_, err = SaveAdtyPaymentScenes(root.Id, 1, []model.AdtyPaymentScene{}, model.AuditLog{})
			require.ErrorIs(t, err, model.ErrAdtyPaymentSceneConflict)
			views, err := AdtyPaymentScenesForUser(buyer.Id)
			require.NoError(t, err)
			require.Len(t, views, 1)
			require.Len(t, views[0].Methods, 1)
			assert.Equal(t, "restricted", views[0].Key)
			assert.Equal(t, "usdt.trc20", views[0].Methods[0].Id)

			for _, invalid := range []struct {
				name, key, currency string
				amount              int64
				site, staff, role   int
			}{
				{"missing default", "", "USDT", 100, 0, 0, 1},
				{"unknown scene", "unknown", "USDT", 100, 0, 0, 1},
				{"below minimum", "restricted", "USDT", 49, 0, 0, 1},
				{"above maximum", "restricted", "USDT", 101, 0, 0, 1},
				{"wrong currency", "restricted", "CNY", 100, 0, 0, 1},
				{"SQL site moved", "restricted", "USDT", 100, 7, 0, 1},
				{"SQL staff identity", "restricted", "USDT", 100, 0, 7, 1},
				{"SQL role changed", "restricted", "USDT", 100, 0, 0, common.RoleRootUser},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					require.NoError(t, db.Model(&model.User{}).Where("id = ?", buyer.Id).Updates(map[string]any{"site_id": invalid.site, "staff_site_id": invalid.staff, "role": invalid.role}).Error)
					top, ext := paymentSceneOrder(buyer.Id, invalid.name)
					ext.FaceUSD, ext.PayCurrency = decimal.NewFromInt(invalid.amount), invalid.currency
					require.Error(t, CreateAdtyPaymentOrder(top, ext, invalid.key))
					var count int64
					require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", top.TradeNo).Count(&count).Error)
					assert.Zero(t, count)
				})
			}
			require.NoError(t, db.Model(buyer).Updates(map[string]any{"site_id": 0, "staff_site_id": 0, "role": 1}).Error)
			top, ext := paymentSceneOrder(buyer.Id, "frozen-checkout")
			require.NoError(t, CreateAdtyPaymentOrder(top, ext, "restricted"))
			assert.Equal(t, "restricted", ext.PaymentScene)
			assert.EqualValues(t, 2, ext.PaymentSceneVersion)
			assert.Contains(t, ext.PaymentSceneSnapshot, `"name":"Original checkout"`)
			assert.NotContains(t, ext.PaymentSceneSnapshot, "synthetic-scene-secret")

			// Inject the actual audit-storage boundary failure: the policy CAS
			// must roll back along with the missing durable audit evidence.
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("scene-audit-failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "audit_intents" {
					tx.AddError(errors.New("synthetic audit outage"))
				}
			}))
			scene.Enabled = false
			_, err = SaveAdtyPaymentScenes(root.Id, 2, []model.AdtyPaymentScene{scene}, model.AuditLog{})
			require.Error(t, err)
			require.NoError(t, db.Callback().Create().Remove("scene-audit-failure"))
			config, err = GetAdtyPaymentScenes()
			require.NoError(t, err)
			assert.EqualValues(t, 2, config.Version)
			assert.True(t, config.Scenes[0].Enabled)

			var group sync.WaitGroup
			results := make(chan error, 2)
			for range 2 {
				group.Go(func() {
					_, saveErr := SaveAdtyPaymentScenes(root.Id, 2, []model.AdtyPaymentScene{scene}, model.AuditLog{})
					results <- saveErr
				})
			}
			group.Wait()
			close(results)
			successes, conflicts := 0, 0
			for saveErr := range results {
				if saveErr == nil {
					successes++
				} else {
					require.ErrorIs(t, saveErr, model.ErrAdtyPaymentSceneConflict)
					conflicts++
				}
			}
			assert.Equal(t, 1, successes)
			assert.Equal(t, 1, conflicts)
			denied, deniedExt := paymentSceneOrder(buyer.Id, "disabled-checkout")
			require.Error(t, CreateAdtyPaymentOrder(denied, deniedExt, "restricted"))
			views, err = AdtyPaymentScenesForUser(buyer.Id)
			require.NoError(t, err)
			assert.Empty(t, views)
			facts := model.AdtyPaymentFacts{Provider: "bepusdt", Method: "usdt.trc20", PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100), PaidCurrency: "USDT", Source: "callback"}
			for _, wantAlready := range []bool{false, true} {
				already, creditErr := model.CompleteAdtyTopUp(top.TradeNo, facts, "")
				require.NoError(t, creditErr)
				assert.Equal(t, wantAlready, already)
			}
			stored, err := model.GetAdtyTopupOrder(top.TradeNo)
			require.NoError(t, err)
			assert.Equal(t, ext.PaymentSceneSnapshot, stored.PaymentSceneSnapshot)
			var current model.User
			require.NoError(t, db.First(&current, buyer.Id).Error)
			assert.Equal(t, 50000000, current.Quota+current.QuotaPending)
			var intents int64
			require.NoError(t, db.Model(&model.AuditIntent{}).Count(&intents).Error)
			assert.EqualValues(t, 2, intents, "failed or conflicted writes do not emit an audit")
		})
	}
}

func paymentSceneOrder(userID int, ref string) (*model.TopUp, *model.AdtyTopupOrder) {
	trade := fmt.Sprintf("scene-%d-%s", userID, ref)
	top := &model.TopUp{UserId: userID, TradeNo: trade, PaymentProvider: "bepusdt", PaymentMethod: "usdt.trc20", Amount: 100, Money: 100, Status: common.TopUpStatusPending}
	ext := &model.AdtyTopupOrder{UserId: userID, TradeNo: trade, Provider: "bepusdt", Method: "usdt.trc20", FaceUSD: decimal.NewFromInt(100), PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", CreditQuota: 50000000}
	return top, ext
}

func TestPaymentSceneEmptyRestrictionsStayArrays(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	require.NoError(t, db.AutoMigrate(&model.AuditIntent{}))
	root := referralUser(t, db, "scene-empty-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	scene := model.AdtyPaymentScene{Key: "open", Name: "Open checkout", Business: "wallet_topup", Enabled: true, Providers: []string{"bepusdt"}, SiteScope: "all"}
	_, err := SaveAdtyPaymentScenes(root.Id, 1, []model.AdtyPaymentScene{scene}, model.AuditLog{})
	require.NoError(t, err)
	stored, err := GetAdtyPaymentScenes()
	require.NoError(t, err)
	require.Len(t, stored.Scenes, 1)
	assert.NotNil(t, stored.Scenes[0].Methods, "Root editor requires arrays even when the API omitted an optional restriction")
	assert.NotNil(t, stored.Scenes[0].ChannelIDs)
	assert.NotNil(t, stored.Scenes[0].Currencies)
	assert.NotNil(t, stored.Scenes[0].SiteIDs)
	assert.NotNil(t, stored.Scenes[0].UserTypes)
}
