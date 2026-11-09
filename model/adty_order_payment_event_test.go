package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrderPaymentHistoryDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				db := isolatedMigrationTestDatabase(t, dialect)
				oldRedis := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = oldRedis })
				if upgrade {
					require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &AdtyTopupOrder{}))
					require.NoError(t, db.Create(&User{Id: 101, Username: "historical-payer", Quota: 1234}).Error)
					seedAdtyOrder(t, 101, "historic-order", PaymentProviderAdtyBEpusdt, common.TopUpStatusSuccess)
				}
				for range 2 {
					require.NoError(t, migrateDB())
				}
				payer := seedAdtyTopUpUser(t, db, "event-payer", 0)
				ext := seedAdtyOrder(t, payer.Id, "event-order", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
				require.NoError(t, db.Model(ext).Update("site_id", 7).Error)
				require.NoError(t, db.Model(payer).Update("site_id", 9).Error)
				facts := usdtFacts(ext.TradeNo, "50.01")
				facts.Payload = `{"secret":"must-never-enter-event"}`
				done, err := CompleteAdtyTopUp(ext.TradeNo, facts, "")
				require.NoError(t, err)
				assert.False(t, done)
				facts.Source = AdtyPaymentSourceQuery
				done, err = CompleteAdtyTopUp(ext.TradeNo, facts, "")
				require.NoError(t, err)
				assert.True(t, done)
				var events []struct {
					TradeNo, Source, Outcome, PaidCurrency string
					SiteId, UserId                         int
				}
				require.NoError(t, db.Table("adty_order_payment_events").Order("id").Find(&events).Error)
				require.Len(t, events, 2)
				assert.Equal(t, "credited", events[0].Outcome)
				assert.Equal(t, "callback", events[0].Source)
				assert.Equal(t, "replayed", events[1].Outcome)
				assert.Equal(t, "query", events[1].Source)
				assert.Equal(t, 7, events[0].SiteId, "order ownership remains frozen after the buyer moves")
				assert.Equal(t, payer.Id, events[0].UserId)
				assert.Equal(t, "USDT", events[0].PaidCurrency)
				assert.Equal(t, int(ext.CreditQuota), userQuota(t, db, payer.Id), "payment retries must not credit twice")
				for range 2 {
					require.NoError(t, migrateDB())
				}
				var count int64
				require.NoError(t, db.Table("adty_order_payment_events").Count(&count).Error)
				assert.Equal(t, int64(2), count)
				if upgrade {
					assert.Equal(t, 1234, userQuota(t, db, 101))
					assert.Equal(t, common.TopUpStatusSuccess, topUpStatus(t, db, "historic-order"))
				}
			})
		}
	}
}

func TestOrderPaymentEventRollback(t *testing.T) {
	db := prepareAdtyTopUpDB(t)
	payer := seedAdtyTopUpUser(t, db, "history-rollback", 0)
	ext := seedAdtyOrder(t, payer.Id, "history-rollback", PaymentProviderAdtyBEpusdt, common.TopUpStatusPending)
	failure := errors.New("history insert failed")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("reject-payment-history", func(tx *gorm.DB) {
		if tx.Statement.Table == "adty_order_payment_events" {
			tx.AddError(failure)
		}
	}))
	_, err := CompleteAdtyTopUp(ext.TradeNo, usdtFacts(ext.TradeNo, "50.01"), "")
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 0, userQuota(t, db, payer.Id))
	assert.Equal(t, common.TopUpStatusPending, topUpStatus(t, db, ext.TradeNo))
	var count int64
	require.NoError(t, db.Model(&AdtyBalanceLedger{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Callback().Create().Remove("reject-payment-history"))
	_, err = CompleteAdtyTopUp(ext.TradeNo, usdtFacts(ext.TradeNo, "50.01"), "")
	require.NoError(t, err)
	var event AdtyOrderPaymentEvent
	require.NoError(t, db.First(&event).Error)
	require.ErrorIs(t, db.Model(&event).Update("outcome", "changed").Error, ErrAdtyPaymentEventImmutable)
	require.ErrorIs(t, db.Delete(&event).Error, ErrAdtyPaymentEventImmutable)
}
