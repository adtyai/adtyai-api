package controller

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegisterHidesDatabaseError(t *testing.T) {
	database := adtyTestDB(t)
	prevQuota := common.QuotaForNewUser
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	common.QuotaForNewUser = 100
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	t.Cleanup(func() {
		common.QuotaForNewUser = prevQuota
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		require.NoError(t, model.EnsureLedgerSchema(database))
	})
	require.NoError(t, database.Exec(`DROP TABLE IF EXISTS ledger_entries`).Error)

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-hide","password":"password-123456"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Equal(t, 500, rec.Code, body)
	require.Contains(t, body, `"code":"internal_error"`)
	lower := strings.ToLower(body)
	require.NotContains(t, lower, "sql")
	require.NotContains(t, lower, "ledger")
	require.NotContains(t, lower, "no such table")
}

func TestRegisterRejectsHiddenDefaultIdentity(t *testing.T) {
	database := adtyTestDB(t)
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	prevQuota := common.QuotaForNewUser
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	t.Cleanup(func() {
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		common.QuotaForNewUser = prevQuota
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"VIP"}`))

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-hidden","password":"password-123456"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Contains(t, body, "No public billing group is available for new users")
	require.NotContains(t, body, "SQL")
	require.NotContains(t, body, "default")

	var count int64
	require.NoError(t, database.Model(&model.User{}).Where("username = ?", "reg-hidden").Count(&count).Error)
	require.Zero(t, count)
}

func TestRegisterHidesInviterDatabaseError(t *testing.T) {
	database := adtyTestDB(t)
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	prevQuota := common.QuotaForNewUser
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	previous := lookupSignupAffCode
	t.Cleanup(func() {
		lookupSignupAffCode = previous
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		common.QuotaForNewUser = prevQuota
	})
	lookupSignupAffCode = func(string) (int, error) {
		return 0, errors.New("SQL logic error: no such table: users (1)")
	}

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-aff","password":"password-123456","aff_code":"WXINVITE"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Equal(t, 500, rec.Code, body)
	require.Contains(t, body, `"code":"internal_error"`)
	lower := strings.ToLower(body)
	require.NotContains(t, lower, "sql")
	require.NotContains(t, lower, "no such table")
	require.NotContains(t, lower, "aff_code")

	var count int64
	require.NoError(t, database.Model(&model.User{}).Where("username = ?", "reg-aff").Count(&count).Error)
	require.Zero(t, count)
}

func TestRegisterHidesInviteCycleDatabaseError(t *testing.T) {
	database := adtyTestDB(t)
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	prevQuota := common.QuotaForNewUser
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	previousLookup := lookupSignupAffCode
	previousCycle := assertSignupInviteCycle
	t.Cleanup(func() {
		lookupSignupAffCode = previousLookup
		assertSignupInviteCycle = previousCycle
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		common.QuotaForNewUser = prevQuota
	})
	lookupSignupAffCode = func(string) (int, error) {
		return 7, nil
	}
	assertSignupInviteCycle = func(int, int) error {
		return errors.New("SQL logic error: no such table: users (1)")
	}

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-cycle","password":"password-123456","aff_code":"WXINVITE"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Equal(t, 500, rec.Code, body)
	require.Contains(t, body, `"code":"internal_error"`)
	lower := strings.ToLower(body)
	require.NotContains(t, lower, "sql")
	require.NotContains(t, lower, "invalid invitation code")

	var count int64
	require.NoError(t, database.Model(&model.User{}).Where("username = ?", "reg-cycle").Count(&count).Error)
	require.Zero(t, count)
}

func TestRegisterRejectsInviteCycle(t *testing.T) {
	database := adtyTestDB(t)
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	prevQuota := common.QuotaForNewUser
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	previousLookup := lookupSignupAffCode
	previousCycle := assertSignupInviteCycle
	t.Cleanup(func() {
		lookupSignupAffCode = previousLookup
		assertSignupInviteCycle = previousCycle
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		common.QuotaForNewUser = prevQuota
	})
	lookupSignupAffCode = func(string) (int, error) {
		return 7, nil
	}
	assertSignupInviteCycle = func(int, int) error {
		return model.ErrAffInviteCycle
	}

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-loop","password":"password-123456","aff_code":"WXINVITE"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Contains(t, body, "Invalid invitation code")
	require.NotEqual(t, 500, rec.Code, body)
	var count int64
	require.NoError(t, database.Model(&model.User{}).Where("username = ?", "reg-loop").Count(&count).Error)
	require.Zero(t, count)
}

func TestRegisterIgnoresUnknownAffiliateCode(t *testing.T) {
	database := adtyTestDB(t)
	prevRegister := common.RegisterEnabled
	prevPassword := common.PasswordRegisterEnabled
	prevEmail := common.EmailVerificationEnabled
	prevQuota := common.QuotaForNewUser
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	t.Cleanup(func() {
		common.RegisterEnabled = prevRegister
		common.PasswordRegisterEnabled = prevPassword
		common.EmailVerificationEnabled = prevEmail
		common.QuotaForNewUser = prevQuota
	})
	lookupSignupAffCode = func(code string) (int, error) {
		require.Equal(t, "MISSING", code)
		return 0, gorm.ErrRecordNotFound
	}
	t.Cleanup(func() { lookupSignupAffCode = model.GetUserIdByAffCode })

	rec := adtyCall(Register, "POST", "/api/user/register",
		`{"username":"reg-miss","password":"password-123456","aff_code":"MISSING"}`, 0, 0, nil)
	body := rec.Body.String()
	require.Equal(t, 200, rec.Code, body)
	require.Contains(t, body, `"success":true`)

	var created model.User
	require.NoError(t, database.Where("username = ?", "reg-miss").First(&created).Error)
	require.Zero(t, created.InviterId)
}

func TestCreateUserHidesPolicyReloadError(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.CasbinRule{}))
	prevQuota := common.QuotaForNewUser
	common.QuotaForNewUser = 0
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		common.QuotaForNewUser = prevQuota
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))

	rec := adtyCall(CreateUser, "POST", "/api/user/",
		`{"username":"policy-hide","password":"password-123456","role":1}`, 1, common.RoleRootUser, nil)
	body := rec.Body.String()
	require.Equal(t, 500, rec.Code, body)
	require.Contains(t, body, `"code":"internal_error"`)
	lower := strings.ToLower(body)
	require.NotContains(t, lower, "enforcer")
	require.NotContains(t, lower, "sql")

	var created model.User
	require.NoError(t, database.Where("username = ?", "policy-hide").First(&created).Error)
	require.Equal(t, "default", created.Group)
}
