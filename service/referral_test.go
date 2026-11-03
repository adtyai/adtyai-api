package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareReferralDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := adtyTestDB(t)
	oldRedis, oldPerUnit, oldSecret := common.RedisEnabled, common.QuotaPerUnit, common.SessionSecret
	common.RedisEnabled, common.QuotaPerUnit, common.SessionSecret = false, 500000, "referral-test-secret"
	saved := *adty_setting.GetReferralSetting()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.RedisEnabled, common.QuotaPerUnit, common.SessionSecret = oldRedis, oldPerUnit, oldSecret
		*adty_setting.GetReferralSetting() = saved
		common.OptionMap = oldMap
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Option{}, &model.AuditLog{}))
	require.NoError(t, model.MigrateAdtyTables())
	// 和钱有关的设置没有内置值（约定 I），测试里设好：比例 10%、最低提现 $20、USDT 手续费 2。
	cfg := adty_setting.GetReferralSetting()
	cfg.Enabled = true
	cfg.DefaultRate, cfg.MinWithdrawUSD, cfg.USDTWithdrawFee = new(10.0), new(20.0), new(2.0)
	return db
}

func referralUser(t *testing.T, db *gorm.DB, name string) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com"}
	require.NoError(t, db.Create(user).Error)
	return user
}

// inviterAndInvitee 建一个推广员和一个绑定了它的用户。
func inviterAndInvitee(t *testing.T, db *gorm.DB, prefix string) (*model.ReferralAffiliate, *model.User) {
	t.Helper()
	inviter := referralUser(t, db, prefix+"-inviter")
	affiliate, err := Referral.EnsureAffiliate(inviter.Id)
	require.NoError(t, err)
	invitee := referralUser(t, db, prefix+"-invitee")
	_, err = Referral.BindInvitee(invitee.Id, affiliate.InviteCode, "")
	require.NoError(t, err)
	return affiliate, invitee
}

// paidTopUp 建一笔 $100、加赠 2.5% 的订单并入账（快照由下单钩子决定）。
func paidTopUp(t *testing.T, invitee *model.User, tradeNo string) {
	t.Helper()
	affiliateID, rate := adtyReferralSnapshot(invitee.Id)
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: invitee.Id, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: invitee.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
			FaceUSD: decimal.NewFromInt(100), BonusRate: decimal.RequireFromString("2.5"), BonusUSD: decimal.RequireFromString("2.5"),
			CreditQuota: 51_250_000, PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1),
			ExpireAt: time.Now().Unix() + 900, ReferralAffiliateId: affiliateID, ReferralRate: rate},
	))
	_, err := model.CompleteAdtyTopUp(tradeNo, model.AdtyPaymentFacts{
		Provider: model.PaymentProviderAdtyBEpusdt, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100),
		PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback,
	}, "")
	require.NoError(t, err)
}

func account(t *testing.T, db *gorm.DB, affiliateID int) model.ReferralCommissionAccount {
	t.Helper()
	var acc model.ReferralCommissionAccount
	require.NoError(t, db.Where("affiliate_id = ?", affiliateID).First(&acc).Error)
	return acc
}

func commissionFor(t *testing.T, db *gorm.DB, tradeNo string) model.ReferralCommission {
	t.Helper()
	var c model.ReferralCommission
	require.NoError(t, db.Where("source_trade_no = ?", tradeNo).First(&c).Error)
	return c
}

func TestAdtyReferralSettingsDefaults(t *testing.T) {
	cfg := adty_setting.GetReferralSetting()
	assert.False(t, cfg.Enabled, "off until the admin sets a rate and turns it on")
	assert.Nil(t, cfg.DefaultRate, "no built-in rate")
	assert.Nil(t, cfg.MinWithdrawUSD, "no built-in minimum")
	assert.Nil(t, cfg.USDTWithdrawFee, "no built-in fee")
	assert.Equal(t, 30, cfg.CookieTTLDays)
	assert.Equal(t, 7, cfg.FreezeDays)
	assert.Equal(t, 30, cfg.CancelWindowMinutes)
	assert.Equal(t, "", cfg.WithdrawCNYRate)
	assert.Equal(t, 3, cfg.BatchRegisterThreshold)
}

func TestReferralMoneySettingsRoundTripNull(t *testing.T) {
	prepareReferralDB(t)
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{"adty_referral_setting.default_rate": "12.5"}))
	cfg := adty_setting.GetReferralSetting()
	require.NotNil(t, cfg.DefaultRate)
	assert.EqualValues(t, 12.5, *cfg.DefaultRate)
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{"adty_referral_setting.default_rate": "null"}))
	assert.Nil(t, cfg.DefaultRate, "null clears the rate back to unset")
}

func TestNoCommissionWhenRateUnset(t *testing.T) {
	db := prepareReferralDB(t)
	adty_setting.GetReferralSetting().DefaultRate = nil
	_, invitee := inviterAndInvitee(t, db, "norate")
	paidTopUp(t, invitee, "EPU-norate")
	var jobs int64
	require.NoError(t, db.Model(&model.ReferralCommissionJob{}).Where("source_trade_no = ?", "EPU-norate").Count(&jobs).Error)
	assert.Zero(t, jobs, "no snapshot, so the credit inserts no commission job")
}

func TestNoCommissionAfterAffiliateOpensSite(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "owner")
	paidTopUp(t, invitee, "EPU-owner-before")
	require.NoError(t, db.Model(affiliate).Update("site_owner_since", time.Now().Unix()).Error)
	paidTopUp(t, invitee, "EPU-owner-after")
	_, err := RunReferralSettlementOnce(time.Now().Unix(), 50)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, commissionFor(t, db, "EPU-owner-before").CommissionAmount, 1e-9, "orders before the site opened still earn")
	var count int64
	require.NoError(t, db.Model(&model.ReferralCommission{}).Where("source_trade_no = ?", "EPU-owner-after").Count(&count).Error)
	assert.Zero(t, count)

	newcomer := referralUser(t, db, "owner-newcomer")
	_, err = Referral.BindInvitee(newcomer.Id, affiliate.InviteCode, "")
	assert.True(t, errors.Is(err, ErrReferralCodeInvalid), "a site owner's link no longer binds new users")
}

func TestCommissionOnFaceExcludingBonusAndSnapshot(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "face")
	paidTopUp(t, invitee, "EPU-face-1")
	// 下单之后改比例，不影响已经下的单。
	require.NoError(t, db.Model(affiliate).Update("rate_override", 30.0).Error)
	_, err := RunReferralSettlementOnce(time.Now().Unix(), 50)
	require.NoError(t, err)
	c := commissionFor(t, db, "EPU-face-1")
	assert.InDelta(t, 10.0, c.CommissionAmount, 1e-9, "10% of the $100 face value, bonus excluded")
	assert.InDelta(t, 10.0, c.Rate, 1e-9)
	assert.Equal(t, model.ReferralCommissionStatusPending, c.Status)
	assert.Greater(t, c.SettleAt, time.Now().Unix()+6*86400)
	assert.InDelta(t, 10.0, account(t, db, affiliate.Id).PendingAmount, 1e-9)

	require.NoError(t, ProcessAdtyTopUpCommission("EPU-face-1"))
	var count int64
	require.NoError(t, db.Model(&model.ReferralCommission{}).Where("source_trade_no = ?", "EPU-face-1").Count(&count).Error)
	assert.EqualValues(t, 1, count, "one commission per trade number")
}

func TestCommissionHookDoesNotAbortCreditTx(t *testing.T) {
	db := prepareReferralDB(t)
	_, invitee := inviterAndInvitee(t, db, "hook")
	require.NoError(t, db.Create(&model.ReferralCommissionJob{SourceType: model.ReferralSourceTopup, SourceTradeNo: "EPU-hook", Status: model.ReferralCommissionJobStatusPending}).Error)
	paidTopUp(t, invitee, "EPU-hook")
	var user model.User
	require.NoError(t, db.First(&user, invitee.Id).Error)
	assert.Equal(t, 51_250_000, user.Quota, "a duplicate job row does not roll back the credit")
}

func TestReferralSettlementRetriesAfterTwentyFailures(t *testing.T) {
	db := prepareReferralDB(t)
	_, invitee := inviterAndInvitee(t, db, "retry20")
	paidTopUp(t, invitee, "EPU-retry-20")
	require.NoError(t, db.Model(&model.ReferralCommissionJob{}).Where("source_trade_no = ?", "EPU-retry-20").
		Updates(map[string]any{"status": model.ReferralCommissionJobStatusFailed, "attempt_count": 20}).Error)
	_, err := RunReferralSettlementOnce(time.Now().Unix(), 50)
	require.NoError(t, err)
	var job model.ReferralCommissionJob
	require.NoError(t, db.Where("source_trade_no = ?", "EPU-retry-20").First(&job).Error)
	assert.Equal(t, model.ReferralCommissionJobStatusSucceeded, job.Status)
	assert.Equal(t, model.ReferralCommissionStatusPending, commissionFor(t, db, "EPU-retry-20").Status)
}

func TestSettlementAfterFreezeAndRetries(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "settle")
	paidTopUp(t, invitee, "EPU-settle")
	now := time.Now().Unix()
	_, err := RunReferralSettlementOnce(now, 50)
	require.NoError(t, err)
	assert.Equal(t, model.ReferralCommissionStatusPending, commissionFor(t, db, "EPU-settle").Status)

	settled, err := RunReferralSettlementOnce(now+8*86400, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, settled)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 0, acc.PendingAmount, 1e-9)
	assert.InDelta(t, 10, acc.AvailableAmount, 1e-9)

	// 停结算的推广员跳过；失败的计佣任务下一轮重试。
	frozen, invitee2 := inviterAndInvitee(t, db, "frozen")
	require.NoError(t, db.Model(frozen).Update("settlement_enabled", false).Error)
	paidTopUp(t, invitee2, "EPU-frozen")
	require.NoError(t, db.Model(&model.ReferralCommissionJob{}).Where("source_trade_no = ?", "EPU-frozen").Update("status", model.ReferralCommissionJobStatusFailed).Error)
	_, err = RunReferralSettlementOnce(now+8*86400, 50)
	require.NoError(t, err)
	assert.Equal(t, model.ReferralCommissionStatusPending, commissionFor(t, db, "EPU-frozen").Status, "failed job retried; frozen affiliate not settled")
}

// 到期列表已经扫出来之后才冻结，这一轮也不能把佣金转成可提现。
func TestSettlementRechecksFreezeAfterScan(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "late-freeze")
	paidTopUp(t, invitee, "EPU-late-freeze")
	now := time.Now().Unix()
	_, err := RunReferralSettlementOnce(now, 50)
	require.NoError(t, err)
	commission := commissionFor(t, db, "EPU-late-freeze")
	require.Equal(t, model.ReferralCommissionStatusPending, commission.Status)

	hold := db.Begin()
	require.NoError(t, hold.Error)
	var locked model.ReferralCommission
	require.NoError(t, model.AdtyLockForUpdate(hold).Where("id = ?", commission.Id).First(&locked).Error)
	t.Cleanup(func() {
		if hold.Error == nil {
			_ = hold.Rollback().Error
		}
	})

	done := make(chan struct{})
	var settled int
	var settleErr error
	go func() {
		defer close(done)
		settled, settleErr = RunReferralSettlementOnce(now+8*86400, 50)
	}()

	require.Eventually(t, func() bool {
		var waiting int
		err := db.Raw(`SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active' AND wait_event_type = 'Lock' AND query LIKE '%referral_commissions%'`).Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 3*time.Second, 20*time.Millisecond)

	require.NoError(t, db.Model(affiliate).Update("settlement_enabled", false).Error)
	require.NoError(t, hold.Commit().Error)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("settlement did not finish")
	}
	require.NoError(t, settleErr)
	assert.Equal(t, 0, settled)
	assert.Equal(t, model.ReferralCommissionStatusPending, commissionFor(t, db, "EPU-late-freeze").Status)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 10, acc.PendingAmount, 1e-9)
	assert.InDelta(t, 0, acc.AvailableAmount, 1e-9)
}

func TestRefundReversals(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "refund")
	now := time.Now().Unix()

	paidTopUp(t, invitee, "EPU-void")
	_, err := RunReferralSettlementOnce(now, 50)
	require.NoError(t, err)
	_, err = refundAdtyTopUp(t, "EPU-void", "chargeback", 1, "k-void")
	require.NoError(t, err)
	assert.Equal(t, model.ReferralCommissionStatusVoid, commissionFor(t, db, "EPU-void").Status)
	assert.InDelta(t, 0, account(t, db, affiliate.Id).PendingAmount, 1e-9)

	paidTopUp(t, invitee, "EPU-claw")
	_, err = RunReferralSettlementOnce(now+8*86400, 50)
	require.NoError(t, err)
	// 先把可提现的 10 全转进余额，再退款：可提现变 -10。
	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: affiliate.UserId, Amount: decimal.NewFromInt(10), AccountType: model.ReferralWithdrawToBalance, IdempotencyKey: "w-claw"})
	require.NoError(t, err)
	_, err = refundAdtyTopUp(t, "EPU-claw", "chargeback", 1, "k-claw")
	require.NoError(t, err)
	assert.Equal(t, model.ReferralCommissionStatusClawedBack, commissionFor(t, db, "EPU-claw").Status)
	assert.InDelta(t, -10, account(t, db, affiliate.Id).AvailableAmount, 1e-9, "claw-back may go negative")

	// 同步不能把 clawed_back 改回去。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return syncCommissionStatusTx(tx, commissionFor(t, db, "EPU-claw").Id) }))
	assert.Equal(t, model.ReferralCommissionStatusClawedBack, commissionFor(t, db, "EPU-claw").Status)

	// 之后的新佣金结算时先抵扣负数。
	paidTopUp(t, invitee, "EPU-next")
	_, err = RunReferralSettlementOnce(now+20*86400, 50)
	require.NoError(t, err)
	assert.InDelta(t, 0, account(t, db, affiliate.Id).AvailableAmount, 1e-9)

	// 退款早于计佣：计佣任务标 skipped。
	paidTopUp(t, invitee, "EPU-early")
	_, err = refundAdtyTopUp(t, "EPU-early", "chargeback", 1, "k-early")
	require.NoError(t, err)
	var job model.ReferralCommissionJob
	require.NoError(t, db.Where("source_trade_no = ?", "EPU-early").First(&job).Error)
	assert.Equal(t, model.ReferralCommissionJobStatusSkipped, job.Status)
}

// 订单 $100 加赠 $2.5：本金 F = $100，赠金 G = $2.5，佣金 $10。退款前的余额决定比例 P ÷ F：
// 余额 $72.5 → 扣回 $72.5、退本金 $70，比例 0.7；余额 $2.5 → 只扣回赠金，比例 0。
func TestRefundClawsBackCommissionByRefundedShare(t *testing.T) {
	db := prepareReferralDB(t)
	now := time.Now().Unix()
	setBalance := func(user *model.User, amount string) {
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", perUnit(amount)).Error)
	}
	partialLedger := func(commissionID int) model.ReferralCommissionLedger {
		var entry model.ReferralCommissionLedger
		require.NoError(t, db.Where("external_ref_id = ?", fmt.Sprintf("clawback_partial:%d", commissionID)).First(&entry).Error)
		return entry
	}

	t.Run("pending commission at 0.7", func(t *testing.T) {
		affiliate, invitee := inviterAndInvitee(t, db, "part-pending")
		paidTopUp(t, invitee, "EPU-part-pending")
		_, err := RunReferralSettlementOnce(now, 50)
		require.NoError(t, err)
		setBalance(invitee, "72.5")
		result, err := refundAdtyTopUp(t, "EPU-part-pending", "chargeback", 1, "k-part-pending")
		require.NoError(t, err)
		require.EqualValues(t, perUnit("70"), result.RefundQuota)
		c := commissionFor(t, db, "EPU-part-pending")
		assert.Equal(t, model.ReferralCommissionStatusPending, c.Status, "a partial claw-back keeps the status")
		assert.InDelta(t, 3, c.CommissionAmount, 1e-9, "$10 − $10 × 0.7")
		assert.InDelta(t, 3, account(t, db, affiliate.Id).PendingAmount, 1e-9)
		entry := partialLedger(c.Id)
		assert.Equal(t, "commission_clawback_partial", entry.Type)
		assert.InDelta(t, -7, entry.DeltaPending, 1e-9)

		_, err = RunReferralSettlementOnce(now+8*86400, 50)
		require.NoError(t, err)
		acc := account(t, db, affiliate.Id)
		assert.InDelta(t, 0, acc.PendingAmount, 1e-9)
		assert.InDelta(t, 3, acc.AvailableAmount, 1e-9, "only the kept part settles")
	})

	t.Run("available commission at 0.7", func(t *testing.T) {
		affiliate, invitee := inviterAndInvitee(t, db, "part-avail")
		paidTopUp(t, invitee, "EPU-part-avail")
		_, err := RunReferralSettlementOnce(now+8*86400, 50)
		require.NoError(t, err)
		setBalance(invitee, "72.5")
		_, err = refundAdtyTopUp(t, "EPU-part-avail", "chargeback", 1, "k-part-avail")
		require.NoError(t, err)
		c := commissionFor(t, db, "EPU-part-avail")
		assert.Equal(t, model.ReferralCommissionStatusAvailable, c.Status)
		assert.InDelta(t, 3, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
		assert.InDelta(t, -7, partialLedger(c.Id).DeltaAvailable, 1e-9)
	})

	t.Run("ratio 0 keeps the commission", func(t *testing.T) {
		affiliate, invitee := inviterAndInvitee(t, db, "part-zero")
		paidTopUp(t, invitee, "EPU-part-zero")
		_, err := RunReferralSettlementOnce(now, 50)
		require.NoError(t, err)
		setBalance(invitee, "2.5")
		result, err := refundAdtyTopUp(t, "EPU-part-zero", "chargeback", 1, "k-part-zero")
		require.NoError(t, err)
		require.Zero(t, result.RefundQuota)
		c := commissionFor(t, db, "EPU-part-zero")
		assert.Equal(t, model.ReferralCommissionStatusPending, c.Status)
		assert.InDelta(t, 10, c.CommissionAmount, 1e-9)
		assert.InDelta(t, 10, account(t, db, affiliate.Id).PendingAmount, 1e-9)
		var entries int64
		require.NoError(t, db.Model(&model.ReferralCommissionLedger{}).Where("commission_id = ? AND type LIKE ?", c.Id, "commission_clawback%").Count(&entries).Error)
		assert.Zero(t, entries)
	})

	// 退款早于计佣：计佣任务照常跑，只按没退的本金计佣。
	for _, tc := range []struct {
		name, balance string
		want          float64
	}{{"0.7", "72.5", 3}, {"0", "2.5", 10}} {
		t.Run("pending job at "+tc.name, func(t *testing.T) {
			affiliate, invitee := inviterAndInvitee(t, db, "part-job-"+tc.name)
			tradeNo := "EPU-part-job-" + tc.name
			paidTopUp(t, invitee, tradeNo)
			setBalance(invitee, tc.balance)
			_, err := refundAdtyTopUp(t, tradeNo, "chargeback", 1, "k-part-job-"+tc.name)
			require.NoError(t, err)
			var job model.ReferralCommissionJob
			require.NoError(t, db.Where("source_trade_no = ?", tradeNo).First(&job).Error)
			assert.Equal(t, model.ReferralCommissionJobStatusPending, job.Status)

			_, err = RunReferralSettlementOnce(now, 50)
			require.NoError(t, err)
			c := commissionFor(t, db, tradeNo)
			assert.InDelta(t, tc.want, c.CommissionAmount, 1e-9)
			assert.InDelta(t, tc.want, account(t, db, affiliate.Id).PendingAmount, 1e-9)
		})
	}
}

func TestSelfInviteSkippedWithReason(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "selfinv")
	require.NoError(t, db.Model(&model.ReferralBinding{}).Where("invitee_user_id = ?", invitee.Id).Update("risk_flag", model.ReferralRiskSelfInvite).Error)
	paidTopUp(t, invitee, "EPU-self")
	ext, err := model.GetAdtyTopupOrder("EPU-self")
	require.NoError(t, err)
	assert.Zero(t, ext.ReferralAffiliateId, "no snapshot for a self-invite binding")
	_, err = RunReferralSettlementOnce(time.Now().Unix(), 50)
	require.NoError(t, err)
	assert.InDelta(t, 0, account(t, db, affiliate.Id).PendingAmount, 1e-9)
}

func availableAffiliate(t *testing.T, db *gorm.DB, prefix string, available float64) *model.ReferralAffiliate {
	t.Helper()
	user := referralUser(t, db, prefix)
	affiliate, err := Referral.EnsureAffiliate(user.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ReferralCommissionAccount{}).Where("affiliate_id = ?", affiliate.Id).Update("available_amount", available).Error)
	return affiliate
}

func TestWithdrawToBalanceInstantAndIdempotent(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate := availableAffiliate(t, db, "tobal", 50)
	input := ReferralWithdrawalCreateInput{UserId: affiliate.UserId, Amount: decimal.NewFromInt(30), AccountType: model.ReferralWithdrawToBalance, IdempotencyKey: "tobal-1"}
	w, err := Referral.CreateWithdrawal(input)
	require.NoError(t, err)
	assert.Equal(t, model.ReferralWithdrawalStatusPaid, w.Status)
	var user model.User
	require.NoError(t, db.First(&user, affiliate.UserId).Error)
	assert.Equal(t, 15_000_000, user.Quota)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 20, acc.AvailableAmount, 1e-9)
	assert.InDelta(t, 30, acc.WithdrawnAmount, 1e-9)
	var ledgers, items int64
	db.Model(&model.AdtyBalanceLedger{}).Where("user_id = ? AND type = ?", affiliate.UserId, model.AdtyLedgerTypeCommissionTransfer).Count(&ledgers)
	db.Model(&model.ReferralWithdrawalItem{}).Where("withdrawal_id = ? AND status = ?", w.Id, model.ReferralWithdrawalItemStatusWithdrawn).Count(&items)
	assert.EqualValues(t, 1, ledgers)
	assert.EqualValues(t, 1, items)

	again, err := Referral.CreateWithdrawal(input)
	require.NoError(t, err)
	assert.Equal(t, w.Id, again.Id)
	require.NoError(t, db.First(&user, affiliate.UserId).Error)
	assert.Equal(t, 15_000_000, user.Quota)
}

func TestWithdrawUSDTRules(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate := availableAffiliate(t, db, "usdtw", 500)
	base := ReferralWithdrawalCreateInput{UserId: affiliate.UserId, AccountType: model.ReferralWithdrawToUSDT}
	try := func(amount string, network, address, key string) (*model.ReferralWithdrawal, error) {
		in := base
		in.Amount, in.AccountNetwork, in.AccountNo, in.IdempotencyKey = decimal.RequireFromString(amount), network, address, key
		return Referral.CreateWithdrawal(in)
	}
	tron := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	evm := "0x52908400098527886E0F7030069857D2E4169EE7"
	_, err := try("50", "BEP20", tron, "u1")
	require.Error(t, err, "T address on BEP20")
	_, err = try("50", "TRC20", evm, "u2")
	require.Error(t, err, "0x address on TRC20")
	_, err = try("19.99", "TRC20", tron, "u3")
	require.Error(t, err, "below the minimum")
	cfg := adty_setting.GetReferralSetting()
	fee := cfg.USDTWithdrawFee
	cfg.USDTWithdrawFee = nil
	_, err = try("50", "TRC20", tron, "u5")
	assert.True(t, errors.Is(err, ErrReferralWithdrawUnavailable), "no USDT withdrawals until the fee is set")
	cfg.USDTWithdrawFee = fee
	minimum := cfg.MinWithdrawUSD
	cfg.MinWithdrawUSD = nil
	_, err = try("50", "TRC20", tron, "u6")
	assert.True(t, errors.Is(err, ErrReferralWithdrawUnavailable), "no USDT withdrawals until the minimum is set")
	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{
		UserId: affiliate.UserId, AccountType: model.ReferralWithdrawToAlipay, Amount: decimal.NewFromInt(50),
		AccountName: "x", QRImageURL: "/q.png", IdempotencyKey: "u7",
	})
	assert.True(t, errors.Is(err, ErrReferralWithdrawUnavailable), "no CNY withdrawals until the minimum is set")
	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{
		UserId: affiliate.UserId, AccountType: model.ReferralWithdrawToBalance, Amount: decimal.NewFromInt(10), IdempotencyKey: "u8",
	})
	assert.NoError(t, err, "balance transfers do not need the minimum")
	cfg.MinWithdrawUSD = minimum
	w, err := try("150", "POLYGON", evm, "u4")
	require.NoError(t, err)
	assert.InDelta(t, 148, w.NetAmount, 1e-9)
	assert.InDelta(t, 2, w.FeeAmount, 1e-9)
	assert.InDelta(t, 150, account(t, db, affiliate.Id).FrozenAmount, 1e-9)
}

func setCNYRate(t *testing.T, rate string) {
	t.Helper()
	require.NoError(t, model.UpdateOptionsBulk(map[string]string{
		adty_setting.ReferralKeyWithdrawCNYRate:          rate,
		adty_setting.ReferralKeyWithdrawCNYRateUpdatedAt: "1700000000",
	}))
}

func TestWithdrawCNY(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate := availableAffiliate(t, db, "cnyw", 500)
	input := ReferralWithdrawalCreateInput{UserId: affiliate.UserId, Amount: decimal.NewFromInt(100), AccountType: model.ReferralWithdrawToAlipay,
		AccountName: "Zhang San", QRImageURL: "/api/referral-assets/x.png", ExpectedCNYRate: decimal.RequireFromString("7.1"), IdempotencyKey: "c1"}

	_, err := Referral.CreateWithdrawal(input)
	require.ErrorIs(t, err, ErrReferralRateUnset, "no row yet")
	setCNYRate(t, "0")
	_, err = Referral.CreateWithdrawal(input)
	require.ErrorIs(t, err, ErrReferralRateUnset, "zero is unset")

	setCNYRate(t, "7.1")
	common.OptionMap[adty_setting.ReferralKeyWithdrawCNYRate] = "7.0" // a stale in-memory value
	input.ExpectedCNYRate = decimal.RequireFromString("7.0")
	_, err = Referral.CreateWithdrawal(input)
	var changed *RateChangedError
	require.ErrorAs(t, err, &changed, "the rate is read from the database")
	assert.Equal(t, "7.1", changed.Change.Rate)
	assert.Equal(t, "710.00", changed.Change.CNYAmount)

	input.ExpectedCNYRate = decimal.RequireFromString("7.1")
	w, err := Referral.CreateWithdrawal(input)
	require.NoError(t, err)
	assert.Equal(t, "7.1", w.CNYRate)
	assert.Equal(t, "710.00", w.CNYAmount)
	assert.InDelta(t, 0, w.FeeAmount, 1e-9, "no fee for CNY withdrawals")
	setCNYRate(t, "7.3")
	var stored model.ReferralWithdrawal
	require.NoError(t, db.First(&stored, w.Id).Error)
	assert.Equal(t, "710.00", stored.CNYAmount, "later rate changes do not touch submitted withdrawals")
}

func TestReferralSettingsSaveKeepsRateUnset(t *testing.T) {
	db := prepareReferralDB(t)
	require.NoError(t, Referral.UpdateWithdrawCNYRate("", 1, "", ""))
	require.NoError(t, Referral.UpdateWithdrawCNYRate("0", 1, "", ""))
	_, ok, err := model.ReadOptionFromDB(adty_setting.ReferralKeyWithdrawCNYRate)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, Referral.UpdateWithdrawCNYRate("7.2", 1, "", ""))
	var audits int64
	db.Model(&model.ReferralAdminAuditLog{}).Where("action = ?", "withdraw_cny_rate").Count(&audits)
	assert.EqualValues(t, 1, audits)
	require.NoError(t, Referral.UpdateWithdrawCNYRate("7.2", 1, "", ""))
	db.Model(&model.ReferralAdminAuditLog{}).Where("action = ?", "withdraw_cny_rate").Count(&audits)
	assert.EqualValues(t, 1, audits, "an unchanged rate is not written again")
}

func TestConcurrentWithdrawalsCannotOverdraw(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate := availableAffiliate(t, db, "race", 50)
	tron := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := range 2 {
		wg.Go(func() {
			_, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: affiliate.UserId, Amount: decimal.NewFromInt(30),
				AccountType: model.ReferralWithdrawToUSDT, AccountNetwork: "TRC20", AccountNo: tron, IdempotencyKey: "race-" + string(rune('a'+i))})
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 1, ok)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 20, acc.AvailableAmount, 1e-9)
	assert.InDelta(t, 30, acc.FrozenAmount, 1e-9)
}

// 退款已经锁住佣金时，打款还不能先占住佣金账户，否则两边互相等待。
func TestPayoutDoesNotDeadlockWithRefundLock(t *testing.T) {
	db := prepareReferralDB(t)
	minUSD, fee := 1.0, 0.0
	cfg := adty_setting.GetReferralSetting()
	cfg.MinWithdrawUSD, cfg.USDTWithdrawFee = &minUSD, &fee
	affiliate, invitee := inviterAndInvitee(t, db, "lock-order")
	paidTopUp(t, invitee, "EPU-lock-order")
	now := time.Now().Unix()
	_, err := RunReferralSettlementOnce(now+8*86400, 50)
	require.NoError(t, err)
	commission := commissionFor(t, db, "EPU-lock-order")
	tron := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	withdrawal, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{
		UserId: affiliate.UserId, Amount: decimal.NewFromInt(10),
		AccountType: model.ReferralWithdrawToUSDT, AccountNetwork: "TRC20", AccountNo: tron, IdempotencyKey: "lock-order",
	})
	require.NoError(t, err)
	require.NoError(t, Referral.ReviewWithdrawal(withdrawal.Id, 1, true, "", "", ""))

	hold := db.Begin()
	require.NoError(t, hold.Error)
	var locked model.ReferralCommission
	require.NoError(t, model.AdtyLockForUpdate(hold).Where("id = ?", commission.Id).First(&locked).Error)
	t.Cleanup(func() { _ = hold.Rollback().Error })

	done := make(chan struct{})
	var payErr error
	go func() {
		defer close(done)
		payErr = Referral.MarkWithdrawalPaid(withdrawal.Id, 1, "0xhash", "", "", "", "")
	}()
	require.Eventually(t, func() bool {
		var waiting int
		err := db.Raw(`SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active' AND wait_event_type = 'Lock' AND query LIKE '%referral_commissions%'`).Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 3*time.Second, 20*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, hold.WithContext(ctx).Exec("SELECT id FROM referral_commission_accounts WHERE affiliate_id = ? FOR UPDATE", affiliate.Id).Error)
	require.NoError(t, hold.Commit().Error)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("payout did not finish")
	}
	require.NoError(t, payErr)
	var stored model.ReferralWithdrawal
	require.NoError(t, db.First(&stored, withdrawal.Id).Error)
	assert.Equal(t, model.ReferralWithdrawalStatusPaid, stored.Status)
}

func TestPendingWithdrawalCannotBeMarkedPaid(t *testing.T) {
	db := prepareReferralDB(t)
	affiliate := availableAffiliate(t, db, "pay-pending", 100)
	tron := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	withdrawal, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{
		UserId: affiliate.UserId, Amount: decimal.NewFromInt(40),
		AccountType: model.ReferralWithdrawToUSDT, AccountNetwork: "TRC20", AccountNo: tron, IdempotencyKey: "pay-pending",
	})
	require.NoError(t, err)
	require.Equal(t, model.ReferralWithdrawalStatusPending, withdrawal.Status)

	require.Error(t, Referral.MarkWithdrawalPaid(withdrawal.Id, 1, "0xhash", "", "", "", ""))

	var stored model.ReferralWithdrawal
	require.NoError(t, db.First(&stored, withdrawal.Id).Error)
	assert.Equal(t, model.ReferralWithdrawalStatusPending, stored.Status)
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 60, acc.AvailableAmount, 1e-9)
	assert.InDelta(t, 40, acc.FrozenAmount, 1e-9)
	assert.InDelta(t, 0, acc.WithdrawnAmount, 1e-9)
}

func TestCancelWindowAndReview(t *testing.T) {
	db := prepareReferralDB(t)
	sent := 0
	oldEmail := sendReferralWithdrawalEmail
	sendReferralWithdrawalEmail = func(int, *model.ReferralWithdrawal, bool, string) { sent++ }
	t.Cleanup(func() { sendReferralWithdrawalEmail = oldEmail })
	affiliate := availableAffiliate(t, db, "cancel", 200)
	tron := "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	create := func(key string) *model.ReferralWithdrawal {
		w, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: affiliate.UserId, Amount: decimal.NewFromInt(50),
			AccountType: model.ReferralWithdrawToUSDT, AccountNetwork: "TRC20", AccountNo: tron, IdempotencyKey: key})
		require.NoError(t, err)
		return w
	}
	w1 := create("cw1")
	require.NoError(t, Referral.CancelWithdrawal(w1.Id, affiliate.UserId, w1.SubmittedAt+60))
	w2 := create("cw2")
	require.Error(t, Referral.CancelWithdrawal(w2.Id, affiliate.UserId, w2.SubmittedAt+31*60), "after the 30 minute window")
	assert.InDelta(t, 150, account(t, db, affiliate.Id).AvailableAmount, 1e-9)

	require.NoError(t, Referral.ReviewWithdrawal(w2.Id, 1, false, "bad address", "", ""))
	assert.InDelta(t, 200, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
	w3 := create("cw3")
	require.NoError(t, Referral.ReviewWithdrawal(w3.Id, 1, true, "", "", ""))
	require.Error(t, Referral.MarkWithdrawalPaid(w3.Id, 1, "", "", "", "", ""), "USDT payouts need the transaction hash")
	require.NoError(t, Referral.MarkWithdrawalPaid(w3.Id, 1, "0xhash", "", "", "", ""))
	acc := account(t, db, affiliate.Id)
	assert.InDelta(t, 50, acc.WithdrawnAmount, 1e-9)
	assert.InDelta(t, 0, acc.FrozenAmount, 1e-9)
	assert.Equal(t, 2, sent, "reject and paid emails")
}

func TestReferralCookieAndBinding(t *testing.T) {
	db := prepareReferralDB(t)
	first := referralUser(t, db, "cookie-a")
	second := referralUser(t, db, "cookie-b")
	a, err := Referral.EnsureAffiliate(first.Id)
	require.NoError(t, err)
	b, err := Referral.EnsureAffiliate(second.Id)
	require.NoError(t, err)
	assert.Len(t, a.InviteCode, 8)

	value, err := BuildReferralCookie(a.InviteCode, time.Now())
	require.NoError(t, err)
	code, err := ParseReferralCookie(value, time.Now())
	require.NoError(t, err)
	assert.Equal(t, a.InviteCode, code)
	_, err = ParseReferralCookie(value+"x", time.Now())
	require.Error(t, err, "tampered")
	_, err = ParseReferralCookie(value, time.Now().Add(31*24*time.Hour))
	require.Error(t, err, "past the cookie TTL")

	gin.SetMode(gin.TestMode)
	register := func(name, cookieCode string) *model.User {
		user := referralUser(t, db, name)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/user/register", nil)
		if cookieCode != "" {
			v, _ := BuildReferralCookie(cookieCode, time.Now())
			c.Request.Header.Set("Cookie", AdtyReferralCookie+"="+v)
		}
		AdtyAfterRegister(c, user)
		return user
	}
	// 最后一次点击生效：cookie 被 b 覆盖后注册，绑到 b。
	invitee := register("cookie-new", b.InviteCode)
	var binding model.ReferralBinding
	require.NoError(t, db.Where("invitee_user_id = ?", invitee.Id).First(&binding).Error)
	assert.Equal(t, b.Id, binding.AffiliateId)
	_, err = Referral.EnsureAffiliate(invitee.Id)
	require.NoError(t, err)
	var count int64
	db.Model(&model.ReferralAffiliate{}).Where("user_id = ?", invitee.Id).Count(&count)
	assert.EqualValues(t, 1, count, "registration created the affiliate")
	_, err = Referral.BindInvitee(invitee.Id, a.InviteCode, "")
	require.ErrorIs(t, err, ErrReferralAlreadyBound, "binding is locked after registration")

	_, err = Referral.BindInvitee(first.Id, a.InviteCode, "")
	require.ErrorIs(t, err, ErrReferralSelfOrCycle)
	register("cookie-bad", "")
	_, err = Referral.BindInvitee(second.Id, "NOPE1234", "")
	require.True(t, errors.Is(err, ErrReferralCodeInvalid))
}

func TestPartnerApprovalSetsRate(t *testing.T) {
	db := prepareReferralDB(t)
	user := referralUser(t, db, "partner")
	require.NoError(t, Referral.ApplyPartner(user.Id, "I run a blog"))
	require.NoError(t, Referral.ReviewPartner(user.Id, 1, true, decimal.NewFromInt(18), "", "", ""))
	affiliate, err := Referral.EnsureAffiliate(user.Id)
	require.NoError(t, err)
	assert.Equal(t, model.ReferralPartnerStatusApproved, affiliate.PartnerStatus)
	rate, ok := AffiliateRate(affiliate)
	require.True(t, ok)
	assert.Equal(t, "18", rate.String())
	require.Error(t, Referral.ReviewPartner(user.Id, 1, true, decimal.NewFromInt(20), "", "", ""))
}

func TestPartnerReviewRequiresPendingAndBoundedRate(t *testing.T) {
	db := prepareReferralDB(t)
	user := referralUser(t, db, "partner-bounds")
	require.NoError(t, Referral.ApplyPartner(user.Id, "note"))
	require.Error(t, Referral.ReviewPartner(user.Id, 1, true, decimal.NewFromInt(14), "", "", ""))
	require.Error(t, Referral.ReviewPartner(user.Id, 1, true, decimal.NewFromInt(21), "", "", ""))

	var affiliate model.ReferralAffiliate
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&affiliate).Error)
	assert.Equal(t, model.ReferralPartnerStatusPending, affiliate.PartnerStatus)
	assert.Nil(t, affiliate.RateOverride)

	nine := 9.0
	require.NoError(t, db.Model(&affiliate).Update("rate_override", nine).Error)
	require.NoError(t, Referral.ReviewPartner(user.Id, 1, false, decimal.Zero, "no", "", ""))
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&affiliate).Error)
	assert.Equal(t, model.ReferralPartnerStatusRejected, affiliate.PartnerStatus)
	assert.Nil(t, affiliate.RateOverride)
	require.Error(t, Referral.ReviewPartner(user.Id, 1, false, decimal.Zero, "again", "", ""))

	require.NoError(t, Referral.ApplyPartner(user.Id, "again"))
	require.NoError(t, Referral.ReviewPartner(user.Id, 1, true, decimal.NewFromInt(20), "", "", ""))
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&affiliate).Error)
	require.NotNil(t, affiliate.RateOverride)
	assert.Equal(t, "20", decimal.NewFromFloat(*affiliate.RateOverride).String())
}

func TestReferralRisk(t *testing.T) {
	db := prepareReferralDB(t)
	gin.SetMode(gin.TestMode)
	ctx := func(ip, device string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.RemoteAddr = ip + ":1234"
		if device != "" {
			c.Request.Header.Set("Cookie", AdtyDeviceCookie+"="+device)
		}
		return c
	}
	withCookie := func(c *gin.Context, code string) {
		v, _ := BuildReferralCookie(code, time.Now())
		c.Request.Header.Add("Cookie", AdtyReferralCookie+"="+v)
	}
	inviter := referralUser(t, db, "risk-inviter")
	affiliate, err := Referral.EnsureAffiliate(inviter.Id)
	require.NoError(t, err)
	AdtyRecordLoginFingerprint(ctx("10.0.0.1", "dev-inviter"), inviter.Id)
	var prints int64
	db.Model(&model.AdtyUserFingerprint{}).Where("user_id = ? AND kind = ?", inviter.Id, "login").Count(&prints)
	assert.EqualValues(t, 1, prints, "login records a fingerprint")

	registerFrom := func(name, ip, device string) model.ReferralBinding {
		user := referralUser(t, db, name)
		c := ctx(ip, device)
		withCookie(c, affiliate.InviteCode)
		AdtyAfterRegister(c, user)
		var binding model.ReferralBinding
		require.NoError(t, db.Where("invitee_user_id = ?", user.Id).First(&binding).Error)
		return binding
	}
	assert.Equal(t, model.ReferralRiskSelfInvite, registerFrom("risk-device", "10.9.9.9", "dev-inviter").RiskFlag, "same device")
	assert.Equal(t, model.ReferralRiskSelfInvite, registerFrom("risk-ip", "10.0.0.1", "dev-other").RiskFlag, "same IP")
	assert.Empty(t, registerFrom("risk-clean", "10.5.5.5", "dev-clean").RiskFlag, "different IP and device")

	adty_setting.GetReferralSetting().BatchRegisterThreshold = 0
	for i := range 3 {
		registerFrom("batch-off-"+string(rune('a'+i)), "10.7.7.7", "")
	}
	var stored model.ReferralAffiliate
	require.NoError(t, db.First(&stored, affiliate.Id).Error)
	assert.True(t, stored.SettlementEnabled, "threshold 0 turns the check off")

	adty_setting.GetReferralSetting().BatchRegisterThreshold = 3
	for i := range 3 {
		registerFrom("batch-on-"+string(rune('a'+i)), "10.8.8.8", "")
	}
	require.NoError(t, db.First(&stored, affiliate.Id).Error)
	assert.False(t, stored.SettlementEnabled, "three sign-ups from one IP freeze settlement")
	assert.Equal(t, model.ReferralRiskBatchRegister, stored.RiskReason)
}
