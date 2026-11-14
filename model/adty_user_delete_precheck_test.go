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
		&TopUp{}, &AdtyTopupOrder{}, &AdtyBalanceLedger{}, &BillingOperation{},
		&AdtyBillingHold{}, &AdtyManualRefund{}, &AdtySiteWithdrawal{},
		&AdtySiteInviteWithdrawal{}, &ReferralWithdrawal{}, &AdtyStaffBalanceRequest{},
		&AdtySiteStaff{}, &AdtyTeam{}, &AdtyTeamMember{}, &AdtyTeamKey{},
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
	identity := newPrecheckSession(t, user)
	require.NoError(t, db.Create(&TopUp{UserId: user.Id, TradeNo: "hist-keep-books", Status: common.TopUpStatusSuccess, Amount: 25, CompleteTime: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&AdtyTopupOrder{TradeNo: "hist-keep-books", UserId: user.Id, FaceUSD: decimal.NewFromInt(25), CreditQuota: 12_500_000}).Error)
	require.NoError(t, db.Create(&AdtyBalanceLedger{UserId: user.Id, DeltaQuota: 12_500_000, BalanceBefore: 0, BalanceAfter: 12_500_000, Type: AdtyLedgerTypeTopUp, RefType: "topup", RefId: "hist-keep-books"}).Error)
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
	assert.NotEqual(t, "keep-books", stored.Username)
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
