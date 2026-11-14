package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func setupAccountDeletionPrecheck(t *testing.T) (*model.User, service.AuthIdentity) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	opened, err := model.OpenAdtyTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	db := opened.DB
	db.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.TwoFA{}, &model.TwoFABackupCode{},
		&model.PasskeyCredential{}, &model.AuthFlow{}, &model.UserOAuthBinding{},
		&model.Option{}, &model.UserAccessToken{}, &model.ExternalIdentityClaim{},
		&model.AdtySite{}, &model.AuditIntent{}, &model.AuditLog{}, &model.Token{},
		&model.TopUp{}, &model.AdtyTopupOrder{}, &model.AdtyBalanceLedger{},
		&model.BillingOperation{}, &model.AdtyBillingHold{}, &model.AdtyManualRefund{},
		&model.AdtySiteWithdrawal{}, &model.AdtySiteInviteWithdrawal{},
		&model.ReferralWithdrawal{}, &model.AdtyStaffBalanceRequest{},
		&model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{},
		&model.AdtyTeamKey{},
	))
	previousRedis, previousSecret := common.RedisEnabled, common.SessionSecret
	previousEncryption := common.PasswordLoginEncryptionEnabled
	common.RedisEnabled = false
	common.SessionSecret = "account-deletion-precheck-secret"
	common.PasswordLoginEncryptionEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled, common.SessionSecret = previousRedis, previousSecret
		common.PasswordLoginEncryptionEnabled = previousEncryption
	})
	password, err := common.Password2Hash("enrollment-password")
	require.NoError(t, err)
	user := &model.User{Username: "precheck-user", Password: password, Email: "precheck-user@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, model.PublishUserAuthCache(user.Id))
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "precheck-test")
	require.NoError(t, err)
	identity, err := service.ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	return user, identity
}

func TestDeleteSelfRefusesUnfinishedPaymentWithoutConsumingTheCode(t *testing.T) {
	user, identity := setupAccountDeletionPrecheck(t)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: user.Id, TradeNo: "self-pending", Status: common.TopUpStatusPending, Amount: 8}).Error)
	require.NoError(t, model.DB.Create(&model.AdtyBalanceLedger{UserId: user.Id, DeltaQuota: 1, BalanceBefore: 0, BalanceAfter: 1, Type: model.AdtyLedgerTypeTopUp, RefId: "historical-self"}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: user.Id, TradeNo: "self-historical", Status: common.TopUpStatusSuccess, Amount: 3, CompleteTime: time.Now().Unix()}).Error)

	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}
	body := accountDeletionBody(t, user)
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	blocked := securityEnrollmentRequest("DELETE", "/api/user/self", body, proof, identity, DeleteSelf)
	assert.Contains(t, blocked.Body.String(), `"success":false`, blocked.Body.String())
	assert.Contains(t, blocked.Body.String(), model.ErrUserDeleteUnfinishedPayment.Error(), blocked.Body.String())

	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid)
	assert.Equal(t, "precheck-user", stored.Username)
	assert.Equal(t, "precheck-user@example.com", stored.Email)

	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	stillBlocked := securityEnrollmentRequest("DELETE", "/api/user/self", body, proof, identity, DeleteSelf)
	assert.Contains(t, stillBlocked.Body.String(), model.ErrUserDeleteUnfinishedPayment.Error(), stillBlocked.Body.String())

	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "self-pending").Update("status", common.TopUpStatusExpired).Error)
	proof = issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	ok := securityEnrollmentRequest("DELETE", "/api/user/self", body, proof, identity, DeleteSelf)
	assert.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	_, err = model.GetUserById(user.Id, false)
	require.Error(t, err)
	var historical model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "self-historical").First(&historical).Error)
	assert.Equal(t, common.TopUpStatusSuccess, historical.Status)
	var ledger model.AdtyBalanceLedger
	require.NoError(t, model.DB.Where("ref_id = ?", "historical-self").First(&ledger).Error)
	assert.EqualValues(t, 1, ledger.DeltaQuota)
}

func TestDeleteSelfRefusesActiveStaffAndRestoreDoesNotSoftDelete(t *testing.T) {
	user, identity := setupAccountDeletionPrecheck(t)
	require.NoError(t, model.DB.Model(user).Update("staff_site_id", 4).Error)
	require.NoError(t, model.DB.Create(&model.AdtySiteStaff{
		SiteId: 4, OwnerUserId: 1, UserId: user.Id, Username: user.Username,
		DisplayName: "Staff", Role: "staff", Status: "enabled", Version: 1,
	}).Error)
	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}
	body := accountDeletionBody(t, user)
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	blocked := securityEnrollmentRequest("DELETE", "/api/user/self", body, proof, identity, DeleteSelf)
	assert.Contains(t, blocked.Body.String(), model.ErrUserDeleteActiveStaff.Error(), blocked.Body.String())
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid)
	assert.Equal(t, 4, stored.StaffSiteId)
}

func TestOfficialTeamCloseStillRunsAfterUnfinishedTeamTopupIsCleared(t *testing.T) {
	user, _ := setupAccountDeletionPrecheck(t)
	require.NotNil(t, model.AdtyBeforeUserDeleteHook)
	team := model.AdtyTeam{LeaderUserId: user.Id, Name: "Official", Status: model.AdtyTeamStatusActive, SharedQuota: 120000}
	require.NoError(t, model.DB.Create(&team).Error)
	member := &model.User{Username: "team-member", Password: "hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, model.DB.Create(member).Error)
	require.NoError(t, model.DB.Create(&model.AdtyTeamMember{TeamId: team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: user.Id, TradeNo: "official-team-pending", Status: common.TopUpStatusPending, Amount: 2}).Error)
	require.NoError(t, model.DB.Create(&model.AdtyTopupOrder{TradeNo: "official-team-pending", UserId: user.Id, TeamId: team.Id, FaceUSD: decimal.NewFromInt(2)}).Error)

	_, err := user.Delete()
	require.ErrorIs(t, err, model.ErrUserDeleteUnfinishedTeamFinance)
	var storedTeam model.AdtyTeam
	require.NoError(t, model.DB.First(&storedTeam, team.Id).Error)
	assert.Equal(t, model.AdtyTeamStatusActive, storedTeam.Status)
	var storedMember model.AdtyTeamMember
	require.NoError(t, model.DB.Where("team_id = ? AND user_id = ?", team.Id, member.Id).First(&storedMember).Error)
	assert.Equal(t, model.AdtyTeamMemberActive, storedMember.Status)

	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "official-team-pending").Update("status", model.TopUpStatusCanceled).Error)
	_, err = user.Delete()
	require.NoError(t, err)
	require.NoError(t, model.DB.First(&storedTeam, team.Id).Error)
	assert.Equal(t, model.AdtyTeamStatusClosed, storedTeam.Status)
	assert.EqualValues(t, 120000, storedTeam.SharedQuota)
	require.NoError(t, model.DB.Where("team_id = ? AND user_id = ?", team.Id, member.Id).First(&storedMember).Error)
	assert.Equal(t, model.AdtyTeamMemberLeft, storedMember.Status)
}

func TestDeleteSelfStillRejectsRoot(t *testing.T) {
	user, identity := setupAccountDeletionPrecheck(t)
	require.NoError(t, model.DB.Model(user).Update("role", common.RoleRootUser).Error)
	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountDelete}
	_, err := service.GetVerificationRequirements(identity, operation.Scope)
	assert.ErrorIs(t, err, service.ErrVerificationForbidden)
	_, err = model.DeleteUserForSession(model.AuthSessionIdentity{
		UserID: identity.UserID, SessionID: identity.SessionID,
		UserAuthVersion: identity.UserAuthVersion, SessionVersion: identity.SessionVersion,
	})
	require.ErrorIs(t, err, model.ErrCannotDeleteRootUser)
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.False(t, stored.DeletedAt.Valid)
}

func TestAdminHardDeleteCannotBypassUnfinishedRefund(t *testing.T) {
	_, _ = setupAccountDeletionPrecheck(t)
	target := &model.User{Username: "admin-target", Password: "hash", Email: "admin-target@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, model.DB.Create(target).Error)
	require.NoError(t, model.DB.Create(&model.AdtyManualRefund{
		RefundId: "admin-block", TradeNo: "admin-trade", UserId: target.Id,
		Status: model.AdtyManualRefundStatusApproved, RequestUSD: decimal.NewFromInt(1), RequestQuota: 500000,
	}).Error)
	_, err := model.HardDeleteUserById(target.Id)
	require.ErrorIs(t, err, model.ErrUserDeleteUnfinishedRefund)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, target.Id).Error)
	assert.False(t, stored.DeletedAt.Valid)
	assert.Equal(t, "admin-target", stored.Username)
}
