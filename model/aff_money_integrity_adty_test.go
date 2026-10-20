package model

import (
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAffMoneyIntegrity(t *testing.T) {
	runAffMoneyIntegrity(t)
}

func TestAffMoneyIntegrityPostgres(t *testing.T) {
	if os.Getenv("ADTY_TEST_PG") != "1" {
		t.Skip("set ADTY_TEST_PG=1 and TEST_POSTGRES_DSN to run the postgres path")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	require.NotEmpty(t, dsn)
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	ip := net.ParseIP(parsed.Hostname())
	require.NotNil(t, ip)
	require.True(t, ip.IsLoopback(), "database tests only permit loopback instances")

	name := "adty_aff_" + strings.ToLower(common.GetRandomString(12))
	require.True(t, strings.HasPrefix(name, "adty_aff_"))
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, adminSQL.Close())
	t.Cleanup(func() { dropIsolatedAffDatabase(t, dsn, name) })

	parsed.Path = "/" + name
	isolated, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	require.NoError(t, err)
	t.Logf("isolated database: %s", name)
	t.Cleanup(func() {
		if sqlDB, err := isolated.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	origDB, origLog := DB, LOG_DB
	origMain, origLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		DB = origDB
		LOG_DB = origLog
		common.SetDatabaseTypes(origMain, origLogType)
	})
	DB = isolated
	LOG_DB = isolated
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	require.NoError(t, DB.AutoMigrate(
		&User{}, &UserBonusWallet{}, &AffCommission{}, &AffWithdrawal{},
		&TopupOrder{}, &TopupRefund{}, &LedgerEntry{}, &LedgerIdempotency{},
		&Checkin{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{},
	))
	require.NoError(t, EnsureLedgerSchema(DB))
	runAffMoneyIntegrity(t)
}

func dropIsolatedAffDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	if !strings.HasPrefix(name, "adty_aff_") || name == "postgres" || strings.ContainsAny(name, "\"'; ") {
		t.Errorf("refusing to drop database %s", name)
		return
	}
	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		t.Errorf("reopen admin database: %v", err)
		return
	}
	sqlDB, err := admin.DB()
	if err != nil {
		t.Errorf("admin sql database: %v", err)
		return
	}
	defer sqlDB.Close()
	if err := admin.Exec("DROP DATABASE " + name).Error; err != nil {
		t.Errorf("drop isolated database %s: %v", name, err)
	}
}

func runAffMoneyIntegrity(t *testing.T) {
	t.Helper()
	l2l3l4Cleanup(t)
	require.NoError(t, DB.AutoMigrate(&Checkin{}, &SubscriptionPlan{}, &SubscriptionOrder{}, &UserSubscription{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM aff_withdrawals")
		DB.Exec("DELETE FROM checkins")
		DB.Exec("DELETE FROM subscription_orders")
		DB.Exec("DELETE FROM user_subscriptions")
		DB.Exec("DELETE FROM subscription_plans")
	})

	t.Run("insert persists inviter and drops cyclic code", func(t *testing.T) {
		inviter := l2l3l4User(t, 0, 0, 0)
		invitee := &User{
			Username:    "bind-" + common.GetRandomString(6),
			Password:    "password-123456",
			Email:       "bind-" + common.GetRandomString(6) + "@example.com",
			DisplayName: "bind",
		}
		require.NoError(t, invitee.Insert(inviter.Id))
		var stored int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Select("inviter_id").Scan(&stored).Error)
		require.Equal(t, inviter.Id, stored)

		got, err := ResolveSignupInviter("missing-" + common.GetRandomString(4))
		require.NoError(t, err)
		require.Equal(t, 0, got)

		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("inviter_id", invitee.Id).Error)
		got, err = ResolveSignupInviter(inviter.AffCode)
		require.NoError(t, err)
		require.Equal(t, 0, got)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("inviter_id", 0).Error)
		got, err = ResolveSignupInviter(inviter.AffCode)
		require.NoError(t, err)
		require.Equal(t, inviter.Id, got)
	})

	t.Run("aff code collision retries without rehashing password", func(t *testing.T) {
		existing := l2l3l4User(t, 0, 0, 0)
		fresh := &User{
			Username:    "retry-" + common.GetRandomString(6),
			Password:    "password-123456",
			Email:       "retry-" + common.GetRandomString(6) + "@example.com",
			DisplayName: "retry",
			AffCode:     existing.AffCode,
		}
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return fresh.InsertWithTx(tx, 0)
		}))
		require.NotEqual(t, existing.AffCode, fresh.AffCode)
		require.NotEmpty(t, fresh.AffCode)
		require.True(t, common.ValidatePasswordAndHash("password-123456", fresh.Password))

		blank := l2l3l4User(t, 0, 0, 0)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", blank.Id).Update("aff_code", "").Error)
		code, err := EnsureUserAffCode(blank.Id)
		require.NoError(t, err)
		require.NotEmpty(t, code)
		var stored string
		require.NoError(t, DB.Model(&User{}).Where("id = ?", blank.Id).Select("aff_code").Scan(&stored).Error)
		require.Equal(t, code, stored)
	})

	t.Run("transfer credits paid ledger and does not save the whole row", func(t *testing.T) {
		user := l2l3l4User(t, 1000, int(common.QuotaPerUnit), 0)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("display_name", "keep-me").Error)
		user.DisplayName = "stale"
		require.NoError(t, user.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
		var after User
		require.NoError(t, DB.First(&after, user.Id).Error)
		require.Equal(t, 0, after.AffQuota)
		require.Equal(t, 1000+int(common.QuotaPerUnit), after.Quota)
		require.Equal(t, "keep-me", after.DisplayName)
		var affN, paidN int64
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND account = ? AND kind = ?", user.Id, "aff", "aff_transfer").Count(&affN).Error)
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND account = ? AND kind = ?", user.Id, "paid", "aff_transfer").Count(&paidN).Error)
		require.Equal(t, int64(1), affN)
		require.Equal(t, int64(1), paidN)
	})

	t.Run("refund reclaims unpaid withdrawal and does not claw unrelated paid", func(t *testing.T) {
		hist := l2l3l4User(t, int(99*common.QuotaPerUnit), 0, 0)
		invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, 0)
		order := l2l3l4Order(t, invitee.Id, "CLAW-WD-"+common.GetRandomString(6), 10)
		commQuota := int64(5 * common.QuotaPerUnit)
		require.NoError(t, DB.Create(&AffCommission{
			InviterId: hist.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo, Source: "topup",
			PaidAmountUSD: 10, RateSnapshot: 0.5, CommissionQuota: commQuota,
			Status: AffCommissionStatusAvailable, UnfreezeAt: time.Now().Add(-time.Hour),
		}).Error)
		wd := AffWithdrawal{
			UserId: hist.Id, AmountQuota: commQuota, NetQuota: commQuota,
			IdemKey: "wd-" + common.GetRandomString(8), Status: AffWithdrawStatusPending,
		}
		require.NoError(t, DB.Create(&wd).Error)

		res, err := RefundTopupOrder(order.OrderNo, 10, "claw-wd", "claw-wd-"+common.GetRandomString(6), 7)
		require.NoError(t, err)
		require.Equal(t, commQuota, res.AffClawed)
		var after User
		require.NoError(t, DB.Select("quota", "aff_quota").First(&after, hist.Id).Error)
		require.Equal(t, int(99*common.QuotaPerUnit), after.Quota)
		require.Equal(t, 0, after.AffQuota)
		var status string
		require.NoError(t, DB.Model(&AffWithdrawal{}).Where("id = ?", wd.Id).Select("status").Scan(&status).Error)
		require.Equal(t, AffWithdrawStatusRejected, status)
		var paidClaw int64
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind = ?", hist.Id, "aff_refund_claw_paid").Count(&paidClaw).Error)
		require.Equal(t, int64(0), paidClaw)
	})

	t.Run("refund claws transferred paid once and leaves unrelated paid", func(t *testing.T) {
		hist := l2l3l4User(t, 1000, int(common.QuotaPerUnit), 0)
		require.NoError(t, hist.TransferAffQuotaToQuota(int(common.QuotaPerUnit)))
		invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, 0)
		order := l2l3l4Order(t, invitee.Id, "CLAW-XFER-"+common.GetRandomString(6), 10)
		commQuota := int64(common.QuotaPerUnit)
		require.NoError(t, DB.Create(&AffCommission{
			InviterId: hist.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo, Source: "topup",
			PaidAmountUSD: 10, RateSnapshot: 0.1, CommissionQuota: commQuota,
			Status: AffCommissionStatusAvailable, UnfreezeAt: time.Now().Add(-time.Hour),
		}).Error)
		_, err := RefundTopupOrder(order.OrderNo, 10, "claw-xfer", "claw-xfer-"+common.GetRandomString(6), 7)
		require.NoError(t, err)
		var after User
		require.NoError(t, DB.Select("quota", "aff_quota").First(&after, hist.Id).Error)
		require.Equal(t, 1000, after.Quota)
		require.Equal(t, 0, after.AffQuota)
		var paidClaw, affClaw int64
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind = ?", hist.Id, "aff_refund_claw_paid").Count(&paidClaw).Error)
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind = ?", hist.Id, "aff_refund_claw").Count(&affClaw).Error)
		require.Equal(t, int64(1), paidClaw)
		require.Equal(t, int64(0), affClaw)
	})

	t.Run("shortfall stays on aff and does not touch unrelated paid", func(t *testing.T) {
		hist := l2l3l4User(t, int(40*common.QuotaPerUnit), 0, 0)
		invitee := l2l3l4User(t, int(10*common.QuotaPerUnit), 0, 0)
		order := l2l3l4Order(t, invitee.Id, "CLAW-NEG-"+common.GetRandomString(6), 10)
		commQuota := int64(5 * common.QuotaPerUnit)
		require.NoError(t, DB.Create(&AffCommission{
			InviterId: hist.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo, Source: "topup",
			PaidAmountUSD: 10, RateSnapshot: 0.5, CommissionQuota: commQuota,
			Status: AffCommissionStatusAvailable, UnfreezeAt: time.Now().Add(-time.Hour),
		}).Error)
		_, err := RefundTopupOrder(order.OrderNo, 10, "claw-neg", "claw-neg-"+common.GetRandomString(6), 7)
		require.NoError(t, err)
		var after User
		require.NoError(t, DB.Select("quota", "aff_quota").First(&after, hist.Id).Error)
		require.Equal(t, int(40*common.QuotaPerUnit), after.Quota)
		require.Equal(t, int(-commQuota), after.AffQuota)
	})

	t.Run("void reclaims pending withdrawal but refuses pay_unknown", func(t *testing.T) {
		pendingUser := l2l3l4User(t, 0, 0, 0)
		pending := AffCommission{
			InviterId: pendingUser.Id, InviteeId: pendingUser.Id + 1, OrderNo: "VOID-P-" + common.GetRandomString(6),
			Source: "topup", CommissionQuota: 5 * int64(common.QuotaPerUnit), Status: AffCommissionStatusAvailable,
		}
		require.NoError(t, DB.Create(&pending).Error)
		wd := AffWithdrawal{
			UserId: pendingUser.Id, AmountQuota: pending.CommissionQuota, NetQuota: pending.CommissionQuota,
			IdemKey: "void-wd-" + common.GetRandomString(8), Status: AffWithdrawStatusApproved,
		}
		require.NoError(t, DB.Create(&wd).Error)
		out, err := VoidAffCommission(pending.Id, 3)
		require.NoError(t, err)
		require.Equal(t, AffCommissionStatusVoided, out.Status)
		var aff int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", pendingUser.Id).Select("aff_quota").Scan(&aff).Error)
		require.Equal(t, 0, aff)
		var status string
		require.NoError(t, DB.Model(&AffWithdrawal{}).Where("id = ?", wd.Id).Select("status").Scan(&status).Error)
		require.Equal(t, AffWithdrawStatusRejected, status)

		unknownUser := l2l3l4User(t, int(20*common.QuotaPerUnit), 0, 0)
		unknown := AffCommission{
			InviterId: unknownUser.Id, InviteeId: unknownUser.Id + 1, OrderNo: "VOID-U-" + common.GetRandomString(6),
			Source: "topup", CommissionQuota: 5 * int64(common.QuotaPerUnit), Status: AffCommissionStatusAvailable,
		}
		require.NoError(t, DB.Create(&unknown).Error)
		unknownWD := AffWithdrawal{
			UserId: unknownUser.Id, AmountQuota: unknown.CommissionQuota, NetQuota: unknown.CommissionQuota,
			IdemKey: "void-unk-" + common.GetRandomString(8), Status: AffWithdrawStatusPayUnknown,
		}
		require.NoError(t, DB.Create(&unknownWD).Error)
		_, err = VoidAffCommission(unknown.Id, 3)
		require.ErrorIs(t, err, ErrAffCommissionAlreadyWithdrawn)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", unknownUser.Id).Select("aff_quota").Scan(&aff).Error)
		require.Equal(t, 0, aff)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", unknownUser.Id).Select("quota").Scan(&aff).Error)
		require.Equal(t, int(20*common.QuotaPerUnit), aff)
		require.NoError(t, DB.Model(&AffWithdrawal{}).Where("id = ?", unknownWD.Id).Select("status").Scan(&status).Error)
		require.Equal(t, AffWithdrawStatusPayUnknown, status)
	})

	t.Run("release drains past the single page cap", func(t *testing.T) {
		inviter := l2l3l4User(t, 0, 0, 0)
		const n = 201
		rows := make([]AffCommission, 0, n)
		for i := 0; i < n; i++ {
			rows = append(rows, AffCommission{
				InviterId: inviter.Id, InviteeId: inviter.Id + 1,
				OrderNo: "REL-" + common.GetRandomString(10), Source: "topup",
				CommissionQuota: 1, Status: AffCommissionStatusFrozen, UnfreezeAt: time.Now().Add(-time.Minute),
			})
		}
		require.NoError(t, DB.Create(&rows).Error)
		one, err := ReleaseDueAffCommissions(inviter.Id)
		require.NoError(t, err)
		require.Equal(t, 200, one)
		rest, err := ReleaseAllDueAffCommissions()
		require.NoError(t, err)
		require.GreaterOrEqual(t, rest, 1)
		var left int64
		require.NoError(t, DB.Model(&AffCommission{}).Where("inviter_id = ? AND status = ?", inviter.Id, AffCommissionStatusFrozen).Count(&left).Error)
		require.Equal(t, int64(0), left)
		var aff int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Select("aff_quota").Scan(&aff).Error)
		require.Equal(t, n, aff)
	})

	t.Run("check-in credits bonus and does not inflate paid", func(t *testing.T) {
		setting := operation_setting.GetCheckinSetting()
		prev := *setting
		setting.Enabled = true
		setting.MinQuota = 1000
		setting.MaxQuota = 1000
		t.Cleanup(func() { *setting = prev })
		user := l2l3l4User(t, 500, 0, 0)
		got, err := UserCheckin(user.Id)
		require.NoError(t, err)
		require.Equal(t, 1000, got.QuotaAwarded)
		var paid int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select("quota").Scan(&paid).Error)
		require.Equal(t, 500, paid)
		bonus, err := GetUserBonusBalance(user.Id)
		require.NoError(t, err)
		require.Equal(t, int64(1000), bonus)
		var kind string
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND ref_type = ?", user.Id, "checkin").Select("kind").Scan(&kind).Error)
		require.Equal(t, "checkin_bonus", kind)
	})

	t.Run("subscription balance purchase spends bonus before paid", func(t *testing.T) {
		user := l2l3l4User(t, 999, 0, 0)
		priceQuota, err := calcSubscriptionBalanceQuota(1)
		require.NoError(t, err)
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			_, err := AdjustQuota(tx, user.Id, "bonus", int64(priceQuota), "test bonus", "test", "sub-bonus", "", 0, "test_bonus")
			return err
		}))
		plan := SubscriptionPlan{Title: "day", PriceAmount: 1, Currency: "USD", DurationUnit: SubscriptionDurationDay, DurationValue: 1, Enabled: true}
		require.NoError(t, DB.Create(&plan).Error)
		require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
		var paid int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select("quota").Scan(&paid).Error)
		require.Equal(t, 999, paid)
		bonus, err := GetUserBonusBalance(user.Id)
		require.NoError(t, err)
		require.Equal(t, int64(0), bonus)
	})

	t.Run("subscription compensate skips balance purchase", func(t *testing.T) {
		if GetAffSubscriptionCommissionRate() <= 0 {
			t.Skip("subscription commission rate is disabled")
		}
		inviter := l2l3l4User(t, 0, 0, 0)
		invitee := l2l3l4User(t, 0, 0, inviter.Id)
		direct := SubscriptionOrder{
			UserId: invitee.Id, Money: 10, TradeNo: "SUB-DIR-" + common.GetRandomString(8),
			PaymentMethod: "stripe", PaymentProvider: "stripe", Status: common.TopUpStatusSuccess,
			CreateTime: time.Now().Unix(), CompleteTime: time.Now().Unix(),
		}
		balance := SubscriptionOrder{
			UserId: invitee.Id, Money: 10, TradeNo: "SUB-BAL-" + common.GetRandomString(8),
			PaymentMethod: PaymentMethodBalance, PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess,
			CreateTime: time.Now().Unix(), CompleteTime: time.Now().Unix(),
		}
		require.NoError(t, DB.Create(&direct).Error)
		require.NoError(t, DB.Create(&balance).Error)
		n, err := compensateMissingSubscriptionAffCommissions(20)
		require.NoError(t, err)
		require.GreaterOrEqual(t, n, 1)
		var directN, balanceN int64
		require.NoError(t, DB.Model(&AffCommission{}).Where("order_no = ?", direct.TradeNo).Count(&directN).Error)
		require.NoError(t, DB.Model(&AffCommission{}).Where("order_no = ?", balance.TradeNo).Count(&balanceN).Error)
		require.Equal(t, int64(1), directN)
		require.Equal(t, int64(0), balanceN)
	})

	t.Run("signup invitee bonus lands on bonus wallet", func(t *testing.T) {
		prevInvitee := common.QuotaForInvitee
		prevInviter := common.QuotaForInviter
		common.QuotaForInvitee = 2500
		common.QuotaForInviter = 800
		t.Cleanup(func() {
			common.QuotaForInvitee = prevInvitee
			common.QuotaForInviter = prevInviter
		})
		inviter := l2l3l4User(t, 100, 0, 0)
		invitee := &User{
			Username:    "invitee-" + common.GetRandomString(8),
			Password:    "password-123456",
			DisplayName: "invitee",
			Email:       common.GetRandomString(8) + "@example.com",
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
		}
		require.NoError(t, invitee.Insert(inviter.Id))
		var paid int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Select("quota").Scan(&paid).Error)
		require.Equal(t, common.QuotaForNewUser, paid)
		bonus, err := GetUserBonusBalance(invitee.Id)
		require.NoError(t, err)
		require.Equal(t, int64(2500), bonus)
		var kind string
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind = ?", invitee.Id, "signup_invite_bonus").Select("kind").Scan(&kind).Error)
		require.Equal(t, "signup_invite_bonus", kind)
		require.NoError(t, creditSignupInviteeBonus(invitee.Id, common.QuotaForInvitee))
		bonus, err = GetUserBonusBalance(invitee.Id)
		require.NoError(t, err)
		require.Equal(t, int64(2500), bonus, "invitee bonus must be idempotent")

		var affQuota, affCount int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Select("aff_quota").Scan(&affQuota).Error)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Select("aff_count").Scan(&affCount).Error)
		require.Equal(t, 800, affQuota)
		require.Equal(t, 1, affCount)
		var affKind string
		require.NoError(t, DB.Model(&LedgerEntry{}).Where("user_id = ? AND kind = ?", inviter.Id, "signup_inviter_bonus").Select("kind").Scan(&affKind).Error)
		require.Equal(t, "signup_inviter_bonus", affKind)
	})
}
