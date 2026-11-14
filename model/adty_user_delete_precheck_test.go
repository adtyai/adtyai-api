package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func precheckUserDeleteTables(t *testing.T, db interface{ AutoMigrate(...any) error }) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&User{}, &UserSession{}, &Token{}, &UserAccessToken{}, &ExternalIdentityClaim{}, &AdtySite{},
		&TwoFA{}, &TwoFABackupCode{}, &PasskeyCredential{}, &AuthFlow{}, &UserOAuthBinding{},
		&TopUp{}, &AdtyTopupOrder{}, &AdtyBalanceLedger{}, &BillingOperation{},
		&AdtyBillingHold{}, &AdtyManualRefund{}, &AdtySiteWithdrawal{},
		&AdtySiteInviteWithdrawal{}, &ReferralWithdrawal{}, &AdtyStaffBalanceRequest{},
		&AdtySiteStaff{}, &AdtyTeam{}, &AdtyTeamMember{}, &AdtyTeamKey{},
		&AdtyVoucher{}, &AdtySiteEarningDaily{}, &PaymentOrphanEvent{},
	))
}

func newPrecheckUser(t *testing.T, name string) *User {
	t.Helper()
	user := &User{
		Username: name, Password: "hash", Email: name + "@example.com",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func newPrecheckSession(t *testing.T, user *User) AuthSessionIdentity {
	t.Helper()
	now := time.Now().Unix()
	session := UserSession{
		SID: fmt.Sprintf("sid-%d", user.Id), UserID: user.Id, Version: 1,
		UserAuthVersion: user.AuthVersion, Status: UserSessionStatusActive,
		RefreshHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		LoginMethod: "password", CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, DB.Create(&session).Error)
	return AuthSessionIdentity{
		UserID: user.Id, SessionID: session.SID,
		UserAuthVersion: user.AuthVersion, SessionVersion: session.Version,
	}
}

func assertUserNotDeleted(t *testing.T, user *User) {
	t.Helper()
	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.False(t, stored.DeletedAt.Valid)
	assert.Equal(t, user.Username, stored.Username)
	assert.Equal(t, user.Email, stored.Email)
}

func TestUserDeletePreconditionsRefuseUnfinishedWorkAndLeaveAccountIntact(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)

	cases := []struct {
		name  string
		want  error
		seed  func(*User)
		clear func(*User)
	}{
		{
			name: "pending top-up",
			want: ErrUserDeleteUnfinishedPayment,
			seed: func(user *User) {
				require.NoError(t, db.Create(&TopUp{UserId: user.Id, TradeNo: "pay-" + user.Username, Status: common.TopUpStatusPending, Amount: 10}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&TopUp{}).Where("user_id = ?", user.Id).Update("status", common.TopUpStatusExpired).Error)
			},
		},
		{
			name: "in-flight billing reservation",
			want: ErrUserDeleteInFlightBilling,
			seed: func(user *User) {
				require.NoError(t, db.Create(&BillingOperation{OperationKey: "bill-" + user.Username, UserId: user.Id, Kind: "request", State: BillingReserved}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&BillingOperation{}).Where("user_id = ?", user.Id).Update("state", BillingSettled).Error)
			},
		},
		{
			name: "open shortfall hold",
			want: ErrUserDeleteOpenBillingHold,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtyBillingHold{UserId: user.Id, RequestId: "hold-" + user.Username, ShortfallQuota: 5, Status: AdtyBillingHoldOpen}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyBillingHold{}).Where("user_id = ?", user.Id).Update("status", "resolved").Error)
			},
		},
		{
			name: "submitted manual refund",
			want: ErrUserDeleteUnfinishedRefund,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtyManualRefund{
					RefundId: "rf-" + user.Username, TradeNo: "tn-" + user.Username, UserId: user.Id,
					Status: AdtyManualRefundStatusSubmitted, RequestUSD: decimal.NewFromInt(1), RequestQuota: 500000,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyManualRefund{}).Where("user_id = ?", user.Id).Update("status", AdtyManualRefundStatusTerminated).Error)
			},
		},
		{
			name: "unknown payout occupancy",
			want: ErrUserDeleteUnfinishedRefund,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtyManualRefund{
					RefundId: "rfu-" + user.Username, TradeNo: "tnu-" + user.Username, UserId: user.Id,
					Status: AdtyManualRefundStatusApproved, PayoutStatus: AdtyManualRefundPayoutUnknown,
					RequestUSD: decimal.NewFromInt(1), RequestQuota: 500000,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyManualRefund{}).Where("user_id = ?", user.Id).Updates(map[string]any{
					"status": AdtyManualRefundStatusCompleted, "payout_status": AdtyManualRefundPayoutSuccess,
				}).Error)
			},
		},
		{
			name: "pending site withdrawal",
			want: ErrUserDeleteUnfinishedWithdraw,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtySiteWithdrawal{
					OwnerUserId: user.Id, AmountUSD: decimal.NewFromInt(1), FeeUSD: decimal.Zero, NetUSD: decimal.NewFromInt(1),
					Status: AdtySiteWithdrawPending, IdempotencyKey: "wd-" + user.Username,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtySiteWithdrawal{}).Where("owner_user_id = ?", user.Id).Update("status", AdtySiteWithdrawCancelled).Error)
			},
		},
		{
			name: "pending staff balance request",
			want: ErrUserDeletePendingBalanceReq,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtyStaffBalanceRequest{
					StaffUserId: user.Id, CustomerUserId: user.Id + 1000, Direction: AdtyStaffBalanceIncrease,
					AmountUSD: decimal.NewFromInt(1), Status: AdtyStaffBalancePending,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyStaffBalanceRequest{}).Where("staff_user_id = ?", user.Id).Update("status", AdtyStaffBalanceRejected).Error)
			},
		},
		{
			name: "active staff identity",
			want: ErrUserDeleteActiveStaff,
			seed: func(user *User) {
				require.NoError(t, db.Model(user).Update("staff_site_id", 9).Error)
				user.StaffSiteId = 9
				require.NoError(t, db.Create(&AdtySiteStaff{
					SiteId: 9, OwnerUserId: 1, UserId: user.Id, Username: "staff-user",
					DisplayName: "Staff", Role: "staff", Status: "enabled", Version: 1,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtySiteStaff{}).Where("user_id = ?", user.Id).Update("status", "deleted").Error)
			},
		},
		{
			name: "issued pending voucher",
			want: ErrUserDeleteUnfinishedVoucher,
			seed: func(user *User) {
				require.NoError(t, db.Create(&AdtyVoucher{
					CodeHash: "hash-iss-" + user.Username, CodeTail: "abcd1234",
					Purpose: AdtyVoucherPurposeSiteCustomer, Status: AdtyVoucherStatusPending,
					AmountUSD: decimal.NewFromInt(10), Quota: 5_000_000,
					SourceKind: AdtyVoucherSourceEarnings, IssuerId: user.Id,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyVoucher{}).Where("issuer_id = ?", user.Id).Update("status", AdtyVoucherStatusCancelled).Error)
			},
		},
		{
			name: "bound frozen margin voucher on a closed site",
			want: ErrUserDeleteUnfinishedVoucher,
			seed: func(user *User) {
				site := AdtySite{OwnerUserId: user.Id, Subdomain: "vouch-" + user.Username, Status: AdtySiteStatusClosed}
				require.NoError(t, db.Create(&site).Error)
				require.NoError(t, db.Create(&AdtyVoucher{
					CodeHash: "hash-bound-" + user.Username, CodeTail: "efgh5678",
					Purpose: AdtyVoucherPurposeSiteMargin, Status: AdtyVoucherStatusFrozen,
					AmountUSD: decimal.NewFromInt(20), Quota: 10_000_000,
					SourceKind: AdtyVoucherSourcePlatform, TargetSiteId: site.Id, IssuerId: 1,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtyVoucher{}).Where("code_hash = ?", "hash-bound-"+user.Username).Update("status", AdtyVoucherStatusCancelled).Error)
			},
		},
		{
			name: "pending site earning after the site closed",
			want: ErrUserDeleteUnsettledSiteEarning,
			seed: func(user *User) {
				site := AdtySite{OwnerUserId: user.Id, Subdomain: "earn-" + user.Username, Status: AdtySiteStatusClosed}
				require.NoError(t, db.Create(&site).Error)
				require.NoError(t, db.Create(&AdtySiteEarningDaily{
					SiteId: site.Id, SiteOwnerUserId: user.Id, UserId: user.Id,
					Day: "2026-11-01", Kind: AdtySiteEarningUsage, Earning: decimal.NewFromInt(3),
					Status: AdtySiteEarningPending,
				}).Error)
			},
			clear: func(user *User) {
				require.NoError(t, db.Model(&AdtySiteEarningDaily{}).Where("site_owner_user_id = ?", user.Id).Update("status", AdtySiteEarningAvailable).Error)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := newPrecheckUser(t, "u-"+fmt.Sprintf("%d", time.Now().UnixNano()))
			tc.seed(user)

			_, err := user.Delete()
			require.ErrorIs(t, err, tc.want)
			assertUserNotDeleted(t, user)

			_, err = user.HardDelete()
			require.ErrorIs(t, err, tc.want)
			assertUserNotDeleted(t, user)

			tc.clear(user)
			_, err = user.Delete()
			require.NoError(t, err)
			var stored User
			require.Error(t, DB.First(&stored, user.Id).Error)
			require.NoError(t, DB.Unscoped().First(&stored, user.Id).Error)
			assert.True(t, stored.DeletedAt.Valid)
		})
	}
}

func TestUserDeleteKeepsHistoricalOrdersAndLedgersAndRevokesLiveAccess(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)
	user := newPrecheckUser(t, "keep-books")
	require.NoError(t, db.Model(user).Updates(map[string]any{"display_name": "Keep Books", "github_id": "keep-books-gh"}).Error)
	identity := newPrecheckSession(t, user)
	require.NoError(t, db.AutoMigrate(&Task{}, &AuditLog{}))
	require.NoError(t, db.Create(&TopUp{UserId: user.Id, TradeNo: "hist-keep-books", Status: common.TopUpStatusSuccess, Amount: 25, CompleteTime: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{TradeNo: "hist-keep-books", UserId: user.Id, FaceUSD: decimal.NewFromInt(25), CreditQuota: 12_500_000}).Error)
	require.NoError(t, db.Create(&AdtyBalanceLedger{UserId: user.Id, DeltaQuota: 12_500_000, BalanceBefore: 0, BalanceAfter: 12_500_000, Type: AdtyLedgerTypeTopUp, RefType: "topup", RefId: "hist-keep-books"}).Error)
	require.NoError(t, db.Create(&Task{UserId: user.Id, TaskID: "artifact-keep-books", Status: TaskStatusSuccess, Action: "image"}).Error)
	require.NoError(t, LOG_DB.Create(&AuditLog{EventId: "audit-keep-books", UserId: user.Id, Username: "keep-books", Category: AuditCategoryOperation, Action: "user.login", Content: "keep-audit", Success: true, Result: AuditResultSuccess, CreatedAt: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&AdtyManualRefund{
		RefundId: "done-keep-books", TradeNo: "hist-keep-books", UserId: user.Id,
		Status: AdtyManualRefundStatusCompleted, RequestUSD: decimal.NewFromInt(1), RequestQuota: 500000,
	}).Error)
	token := Token{UserId: user.Id, Key: "live-keep-books", Status: common.TokenStatusEnabled, Name: "live", ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)

	_, err := DeleteUserForSession(identity)
	require.NoError(t, err)

	var topUp TopUp
	require.NoError(t, db.Where("trade_no = ?", "hist-keep-books").First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	var ledger AdtyBalanceLedger
	require.NoError(t, db.Where("ref_id = ?", "hist-keep-books").First(&ledger).Error)
	assert.EqualValues(t, 12_500_000, ledger.DeltaQuota)
	var refund AdtyManualRefund
	require.NoError(t, db.Where("refund_id = ?", "done-keep-books").First(&refund).Error)
	assert.Equal(t, AdtyManualRefundStatusCompleted, refund.Status)
	var storedToken Token
	require.NoError(t, db.Unscoped().First(&storedToken, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, storedToken.Status)
	assert.Equal(t, "live-keep-books", storedToken.Key)
	var session UserSession
	require.NoError(t, db.Where("sid = ?", identity.SessionID).First(&session).Error)
	assert.Equal(t, UserSessionStatusRevoked, session.Status)
	var stored User
	require.NoError(t, db.Unscoped().First(&stored, user.Id).Error)
	assert.True(t, stored.DeletedAt.Valid)
	assert.Empty(t, stored.Email)
	assert.Empty(t, stored.DisplayName)
	assert.Empty(t, stored.GitHubId)
	assert.NotEqual(t, "keep-books", stored.Username)
	assert.Equal(t, user.Id, stored.Id)
	assert.Equal(t, user.SiteId, stored.SiteId)
	assert.Equal(t, common.UserStatusEnabled, stored.Status)
	var tasks, audits int64
	require.NoError(t, db.Model(&Task{}).Where("task_id = ?", "artifact-keep-books").Count(&tasks).Error)
	require.NoError(t, LOG_DB.Model(&AuditLog{}).Where("event_id = ?", "audit-keep-books").Count(&audits).Error)
	assert.EqualValues(t, 1, tasks)
	assert.EqualValues(t, 1, audits)
	deletedStatus := -1
	listed, total, err := SearchUsers(fmt.Sprintf("%d", user.Id), "", nil, &deletedStatus, 0, 20, 0, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	found := false
	for _, item := range listed {
		if item.Id == user.Id {
			found = true
			assert.True(t, item.DeletedAt.Valid)
		}
	}
	assert.True(t, found, "cancelled account must remain listable as 已注销")
}

func TestUserDeleteStillRejectsRootAndOpenSiteOwners(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)

	root := newPrecheckUser(t, "root-block")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	root.Role = common.RoleRootUser
	identity := newPrecheckSession(t, root)
	_, err := DeleteUserForSession(identity)
	require.ErrorIs(t, err, ErrCannotDeleteRootUser)
	assertUserNotDeleted(t, root)

	owner := newPrecheckUser(t, "site-owner-block")
	site := AdtySite{OwnerUserId: owner.Id, Subdomain: "owner-block", Status: AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	_, err = owner.Delete()
	require.ErrorIs(t, err, ErrSiteOwnerCannotDelete)
	_, err = owner.HardDelete()
	require.ErrorIs(t, err, ErrSiteOwnerCannotDelete)
	assertUserNotDeleted(t, owner)
}

func TestUserDeleteRefusesTeamLeaderWithPendingTeamTopupThenClosesWhenClear(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)
	closed := false
	previous := AdtyBeforeUserDeleteHook
	AdtyBeforeUserDeleteHook = func(tx *gorm.DB, userId int) error {
		closed = true
		return tx.Model(&AdtyTeam{}).Where("leader_user_id = ? AND status = ?", userId, AdtyTeamStatusActive).
			Updates(map[string]any{"status": AdtyTeamStatusClosed, "closed_at": time.Now().Unix()}).Error
	}
	t.Cleanup(func() { AdtyBeforeUserDeleteHook = previous })

	leader := newPrecheckUser(t, "team-leader-block")
	team := AdtyTeam{LeaderUserId: leader.Id, Name: "Ops", Status: AdtyTeamStatusActive, SharedQuota: 300000}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&TopUp{UserId: leader.Id, TradeNo: "TTEAM-pending", Status: common.TopUpStatusPending, Amount: 5}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{TradeNo: "TTEAM-pending", UserId: leader.Id, TeamId: team.Id, FaceUSD: decimal.NewFromInt(5)}).Error)

	closed = false
	_, err := leader.Delete()
	require.ErrorIs(t, err, ErrUserDeleteUnfinishedTeamFinance)
	assert.False(t, closed)
	assertUserNotDeleted(t, leader)
	var storedTeam AdtyTeam
	require.NoError(t, db.First(&storedTeam, team.Id).Error)
	assert.Equal(t, AdtyTeamStatusActive, storedTeam.Status)
	assert.EqualValues(t, 300000, storedTeam.SharedQuota)

	require.NoError(t, db.Model(&TopUp{}).Where("trade_no = ?", "TTEAM-pending").Update("status", TopUpStatusCanceled).Error)
	_, err = leader.Delete()
	require.NoError(t, err)
	assert.True(t, closed)
	require.NoError(t, db.First(&storedTeam, team.Id).Error)
	assert.Equal(t, AdtyTeamStatusClosed, storedTeam.Status)
	assert.EqualValues(t, 300000, storedTeam.SharedQuota)
	var topUp TopUp
	require.NoError(t, db.Where("trade_no = ?", "TTEAM-pending").First(&topUp).Error)
	assert.Equal(t, TopUpStatusCanceled, topUp.Status)
}

func TestHardDeleteClosesTeamWhenLeaderHasNoUnfinishedFinance(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)
	previous := AdtyBeforeUserDeleteHook
	AdtyBeforeUserDeleteHook = func(tx *gorm.DB, userId int) error {
		return tx.Model(&AdtyTeam{}).Where("leader_user_id = ? AND status = ?", userId, AdtyTeamStatusActive).
			Updates(map[string]any{"status": AdtyTeamStatusClosed, "closed_at": time.Now().Unix()}).Error
	}
	t.Cleanup(func() { AdtyBeforeUserDeleteHook = previous })

	leader := newPrecheckUser(t, "hard-del-leader")
	team := AdtyTeam{LeaderUserId: leader.Id, Name: "HardOps", Status: AdtyTeamStatusActive, SharedQuota: 150000}
	require.NoError(t, db.Create(&team).Error)

	_, err := leader.HardDelete()
	require.NoError(t, err)
	var storedTeam AdtyTeam
	require.NoError(t, db.First(&storedTeam, team.Id).Error)
	assert.Equal(t, AdtyTeamStatusClosed, storedTeam.Status)
	assert.EqualValues(t, 150000, storedTeam.SharedQuota)
	var stored User
	require.Error(t, db.Unscoped().First(&stored, leader.Id).Error)
}

func TestUserDeleteDoesNotInventPaymentOrphanAttribution(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)
	user := newPrecheckUser(t, "orphan-unmapped")
	require.NoError(t, db.Create(&PaymentOrphanEvent{
		Provider: "epusdt", EventID: "orphan-" + user.Username, EventType: "paid",
		ReferenceID: "no-order", Status: PaymentOrphanStatusPendingReview,
	}).Error)
	identity := newPrecheckSession(t, user)
	_, err := DeleteUserForSession(identity)
	require.NoError(t, err, "an orphan without user_id or an order is not invented as this account's unfinished work")
	var stored User
	require.NoError(t, db.Unscoped().First(&stored, user.Id).Error)
	assert.True(t, stored.DeletedAt.Valid)
	var orphan PaymentOrphanEvent
	require.NoError(t, db.Where("event_id = ?", "orphan-"+user.Username).First(&orphan).Error)
	assert.Equal(t, PaymentOrphanStatusPendingReview, orphan.Status)
}

func seedPrecheckUserOrder(t *testing.T, db *gorm.DB, user *User, tradeNo, status, provider, gatewayID, paidTxHash string) {
	t.Helper()
	require.NoError(t, db.Create(&TopUp{
		UserId: user.Id, TradeNo: tradeNo, Status: status, Amount: 10, PaymentProvider: provider,
	}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{
		TradeNo: tradeNo, UserId: user.Id, Provider: provider,
		GatewayTradeId: gatewayID, PaidTxHash: paidTxHash, FaceUSD: decimal.NewFromInt(10),
	}).Error)
}

func assertPaymentOrphanUntouched(t *testing.T, db *gorm.DB, eventID, status, referenceID, sessionID string) {
	t.Helper()
	var orphan PaymentOrphanEvent
	require.NoError(t, db.Where("event_id = ?", eventID).First(&orphan).Error)
	assert.Equal(t, status, orphan.Status)
	assert.Equal(t, referenceID, orphan.ReferenceID)
	assert.Equal(t, sessionID, orphan.SessionID)
	assert.False(t, db.Migrator().HasColumn(&PaymentOrphanEvent{}, "user_id"), "must not invent a user_id column")
}

func TestUserDeleteRefusesSafelyAttributedPaymentOrphan(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)

	for _, status := range []string{common.TopUpStatusFailed, common.TopUpStatusExpired, TopUpStatusCanceled} {
		t.Run(status, func(t *testing.T) {
			user := newPrecheckUser(t, "orphan-ref-"+status)
			tradeNo := "pay-ref-" + user.Username
			eventID := "orphan-ref-" + user.Username
			seedPrecheckUserOrder(t, db, user, tradeNo, status, PaymentProviderAdtyBEpusdt, "gw-"+tradeNo, "")
			require.NoError(t, db.Create(&PaymentOrphanEvent{
				Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
				ReferenceID: tradeNo, SessionID: "gw-" + tradeNo, Status: PaymentOrphanStatusPendingReview,
			}).Error)

			identity := newPrecheckSession(t, user)
			_, err := DeleteUserForSession(identity)
			require.ErrorIs(t, err, ErrUserDeleteUnfinishedPayment)
			assertUserNotDeleted(t, user)
			assertPaymentOrphanUntouched(t, db, eventID, PaymentOrphanStatusPendingReview, tradeNo, "gw-"+tradeNo)

			_, err = user.HardDelete()
			require.ErrorIs(t, err, ErrUserDeleteUnfinishedPayment)
			assertUserNotDeleted(t, user)
			assertPaymentOrphanUntouched(t, db, eventID, PaymentOrphanStatusPendingReview, tradeNo, "gw-"+tradeNo)
		})
	}
}

func TestUserDeleteRefusesSessionAttributedPaymentOrphan(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)

	cases := []struct {
		name    string
		gateway string
		txHash  string
		session string
	}{
		{name: "gateway-trade-id", gateway: "gw-unique-session", session: "gw-unique-session"},
		{name: "paid-tx-hash", txHash: "0xunique-session", session: "0xunique-session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := newPrecheckUser(t, "orphan-sid-"+tc.name)
			tradeNo := "pay-sid-" + user.Username
			eventID := "orphan-sid-" + user.Username
			seedPrecheckUserOrder(t, db, user, tradeNo, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, tc.gateway, tc.txHash)
			require.NoError(t, db.Create(&PaymentOrphanEvent{
				Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
				SessionID: tc.session, Status: PaymentOrphanStatusPendingReview,
			}).Error)

			_, err := DeleteUserForSession(newPrecheckSession(t, user))
			require.ErrorIs(t, err, ErrUserDeleteUnfinishedPayment)
			assertUserNotDeleted(t, user)
			assertPaymentOrphanUntouched(t, db, eventID, PaymentOrphanStatusPendingReview, "", tc.session)
		})
	}
}

func TestUserDeleteIgnoresUnsafePaymentOrphanAttribution(t *testing.T) {
	db := adtyTestDB(t)
	precheckUserDeleteTables(t, db)

	cases := []struct {
		name string
		seed func(user *User) (eventID, status, referenceID, sessionID string)
	}{
		{
			name: "reference is this user id but no order",
			seed: func(user *User) (string, string, string, string) {
				eventID := "orphan-userid-" + user.Username
				reference := fmt.Sprintf("%d", user.Id)
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: "wallet", EventID: eventID, EventType: "wallet_cache_sync",
					ReferenceID: reference, Status: PaymentOrphanStatusPendingReview,
				}).Error)
				return eventID, PaymentOrphanStatusPendingReview, reference, ""
			},
		},
		{
			name: "reference belongs to another user",
			seed: func(user *User) (string, string, string, string) {
				other := newPrecheckUser(t, "orphan-other-"+user.Username)
				tradeNo := "pay-other-" + user.Username
				seedPrecheckUserOrder(t, db, other, tradeNo, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, "gw-"+tradeNo, "")
				eventID := "orphan-other-" + user.Username
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
					ReferenceID: tradeNo, Status: PaymentOrphanStatusPendingReview,
				}).Error)
				return eventID, PaymentOrphanStatusPendingReview, tradeNo, ""
			},
		},
		{
			name: "session id matches two orders",
			seed: func(user *User) (string, string, string, string) {
				other := newPrecheckUser(t, "orphan-amb-"+user.Username)
				shared := "gw-ambiguous-" + user.Username
				seedPrecheckUserOrder(t, db, user, "pay-amb-a-"+user.Username, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, shared, "")
				seedPrecheckUserOrder(t, db, other, "pay-amb-b-"+user.Username, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, shared, "")
				eventID := "orphan-amb-" + user.Username
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
					SessionID: shared, Status: PaymentOrphanStatusPendingReview,
				}).Error)
				return eventID, PaymentOrphanStatusPendingReview, "", shared
			},
		},
		{
			name: "resolved orphan is not unfinished work",
			seed: func(user *User) (string, string, string, string) {
				tradeNo := "pay-credited-" + user.Username
				seedPrecheckUserOrder(t, db, user, tradeNo, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, "gw-"+tradeNo, "")
				eventID := "orphan-credited-" + user.Username
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
					ReferenceID: tradeNo, Status: PaymentOrphanStatusCredited,
				}).Error)
				return eventID, PaymentOrphanStatusCredited, tradeNo, ""
			},
		},
		{
			name: "completed order is not a pending order",
			seed: func(user *User) (string, string, string, string) {
				tradeNo := "pay-success-" + user.Username
				seedPrecheckUserOrder(t, db, user, tradeNo, common.TopUpStatusSuccess, PaymentProviderAdtyBEpusdt, "gw-"+tradeNo, "0x"+tradeNo)
				eventID := "orphan-success-" + user.Username
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "duplicate_payment",
					ReferenceID: tradeNo, SessionID: "0x" + tradeNo, Status: PaymentOrphanStatusPendingReview,
				}).Error)
				return eventID, PaymentOrphanStatusPendingReview, tradeNo, "0x" + tradeNo
			},
		},
		{
			name: "expired order without an attributable orphan",
			seed: func(user *User) (string, string, string, string) {
				tradeNo := "pay-expired-clear-" + user.Username
				seedPrecheckUserOrder(t, db, user, tradeNo, common.TopUpStatusExpired, PaymentProviderAdtyBEpusdt, "gw-"+tradeNo, "")
				eventID := "orphan-clear-" + user.Username
				require.NoError(t, db.Create(&PaymentOrphanEvent{
					Provider: PaymentProviderAdtyBEpusdt, EventID: eventID, EventType: "payment_callback",
					ReferenceID: "missing-" + user.Username, SessionID: "unrelated-" + user.Username,
					Status: PaymentOrphanStatusPendingReview,
				}).Error)
				return eventID, PaymentOrphanStatusPendingReview, "missing-" + user.Username, "unrelated-" + user.Username
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := newPrecheckUser(t, "orphan-unsafe-"+fmt.Sprintf("%d", time.Now().UnixNano()))
			eventID, status, referenceID, sessionID := tc.seed(user)
			_, err := DeleteUserForSession(newPrecheckSession(t, user))
			require.NoError(t, err)
			var stored User
			require.NoError(t, db.Unscoped().First(&stored, user.Id).Error)
			assert.True(t, stored.DeletedAt.Valid)
			assertPaymentOrphanUntouched(t, db, eventID, status, referenceID, sessionID)
		})
	}
}
