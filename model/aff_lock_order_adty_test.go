package model

import (
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAffRefundLiftAndTransferReleasePostgres(t *testing.T) {
	withIsolatedAffPostgres(t, func() {
		t.Run("net", TestRefundAfterNetWrittenCommissionClawsFullOrderProportion)
		t.Run("scale", TestRefundAfterSelfInviteScaleClawsFullOrderProportion)
		t.Run("transfer", TestTransferReleasesDueCommissionBeforeDebit)
		t.Run("preview", TestPreviewTopupRefundUsesCreditBeforePaid)
	})
}

func TestAffSelfInviteApproveRefundDoesNotDeadlockPostgres(t *testing.T) {
	withIsolatedAffPostgres(t, func() {
		assertOppositeLockOrderDeadlocks(t)

		for i := 0; i < 8; i++ {
			inviter := l2l3l4User(t, 0, 0, 0)
			invitee := l2l3l4User(t, int(100*common.QuotaPerUnit), 0, inviter.Id)
			order := l2l3l4Order(t, invitee.Id, "lock-"+common.GetRandomString(8), 100)
			comm := &AffCommission{
				InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
				Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
				CommissionQuota: 1000, FreezeDaysSnap: 0, Status: AffCommissionStatusBlocked,
				UnfreezeAt: time.Now(),
			}
			require.NoError(t, DB.Create(comm).Error)
			review := &RiskReview{
				Kind: RiskReviewKindAffSelfInvite, UserId: invitee.Id, OrderNo: order.OrderNo,
				AmountUSD: 100, Status: RiskReviewStatusPending, Reason: "self",
			}
			require.NoError(t, DB.Create(review).Error)

			start := make(chan struct{})
			var approveErr, refundErr error
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, approveErr = ApproveRiskReview(review.Id, 1)
			}()
			go func() {
				defer wg.Done()
				<-start
				_, refundErr = RefundTopupOrder(order.OrderNo, 100, "lock order", "req-"+order.OrderNo, 1)
			}()
			close(start)
			wg.Wait()

			require.False(t, isPostgresDeadlock(approveErr), "approve deadlock: %v", approveErr)
			require.False(t, isPostgresDeadlock(refundErr), "refund deadlock: %v", refundErr)
			require.Equal(t, 0, reloadAffQuota(t, inviter.Id))
		}
	})
}

func withIsolatedAffPostgres(t *testing.T, fn func()) {
	t.Helper()
	if os.Getenv("ADTY_TEST_PG") != "1" {
		t.Skip("set ADTY_TEST_PG=1 and TEST_POSTGRES_DSN to run the postgres lock-order path")
	}
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	require.NotEmpty(t, dsn)
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	ip := net.ParseIP(parsed.Hostname())
	require.NotNil(t, ip)
	require.True(t, ip.IsLoopback(), "database tests only permit loopback instances")

	name := "adty_aff_" + strings.ToLower(common.GetRandomString(12))
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
		&User{}, &UserBonusWallet{}, &AffCommission{}, &RiskReview{},
		&TopupOrder{}, &TopupRefund{}, &LedgerEntry{}, &LedgerIdempotency{},
	))
	require.NoError(t, EnsureLedgerSchema(DB))
	l2l3l4Cleanup(t)
	fn()
}

func assertOppositeLockOrderDeadlocks(t *testing.T) {
	t.Helper()
	inviter := l2l3l4User(t, 0, 0, 0)
	invitee := l2l3l4User(t, 0, 0, inviter.Id)
	order := l2l3l4Order(t, invitee.Id, "probe-"+common.GetRandomString(8), 100)
	comm := &AffCommission{
		InviterId: inviter.Id, InviteeId: invitee.Id, OrderNo: order.OrderNo,
		Source: AffCommissionSourceTopup, PaidAmountUSD: 100, RateSnapshot: 0.1,
		CommissionQuota: 1000, Status: AffCommissionStatusBlocked, UnfreezeAt: time.Now(),
	}
	require.NoError(t, DB.Create(comm).Error)

	tx1 := DB.Begin()
	tx2 := DB.Begin()
	require.NoError(t, tx1.Error)
	require.NoError(t, tx2.Error)
	defer tx1.Rollback()
	defer tx2.Rollback()
	require.NoError(t, tx1.Exec("SET LOCAL lock_timeout = '3s'").Error)
	require.NoError(t, tx2.Exec("SET LOCAL lock_timeout = '3s'").Error)
	require.NoError(t, tx1.Exec("SET LOCAL deadlock_timeout = '100ms'").Error)
	require.NoError(t, tx2.Exec("SET LOCAL deadlock_timeout = '100ms'").Error)

	var locked AffCommission
	require.NoError(t, tx1.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", comm.Id).First(&locked).Error)
	var lockedOrder TopupOrder
	require.NoError(t, tx2.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", order.OrderNo).First(&lockedOrder).Error)

	errCh := make(chan error, 2)
	go func() {
		var row TopupOrder
		errCh <- tx1.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", order.OrderNo).First(&row).Error
	}()
	go func() {
		var row AffCommission
		errCh <- tx2.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", comm.Id).First(&row).Error
	}()
	first := <-errCh
	second := <-errCh
	require.True(t, isPostgresDeadlock(first) || isPostgresDeadlock(second), "expected opposite lock order to deadlock, got %v / %v", first, second)
}

func isPostgresDeadlock(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadlock")
}
