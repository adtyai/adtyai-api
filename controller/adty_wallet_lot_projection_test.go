package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWalletLotProjectionController(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	opened, err := model.OpenAdtyTestDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	opened.DB.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.AdtyFundBatch{}, &model.AdtySite{}, &model.Option{}))
}

func createProjectionUser(t *testing.T, name string, quota int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com", Quota: quota}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func getSelfJSON(t *testing.T, user *model.User) (success bool, body string, data map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	c.Set("id", user.Id)
	c.Set("role", user.Role)
	GetSelf(c)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response.Success, recorder.Body.String(), response.Data
}

func lotProjectionFrom(t *testing.T, data map[string]any) service.WalletLotProjection {
	t.Helper()
	raw, ok := data["wallet_lot_projection"]
	require.True(t, ok, "wallet_lot_projection must be present and must not be the only balance field")
	encoded, err := common.Marshal(raw)
	require.NoError(t, err)
	var proj service.WalletLotProjection
	require.NoError(t, common.Unmarshal(encoded, &proj))
	return proj
}

func TestGetSelfReturnsSeparatePrincipalAndBonusRemaining(t *testing.T) {
	setupWalletLotProjectionController(t)
	user := createProjectionUser(t, "self-lots", 1_250_000)
	require.NoError(t, model.DB.Create([]model.AdtyFundBatch{
		{UserId: user.Id, Kind: "principal", Source: "a", Original: 800000, Remaining: 500000},
		{UserId: user.Id, Kind: "principal", Source: "b", Original: 500000, Remaining: 500000},
		{UserId: user.Id, Kind: "bonus", Source: "a", Original: 250000, Remaining: 250000},
	}).Error)

	ok, body, data := getSelfJSON(t, user)
	require.True(t, ok, body)
	require.Contains(t, body, `"quota"`)
	require.Contains(t, body, `"wallet_lot_projection"`)
	assert.NotContains(t, body, "quota_pending")
	assert.EqualValues(t, 1_250_000, data["quota"])
	proj := lotProjectionFrom(t, data)
	assert.EqualValues(t, 1_000_000, proj.PrincipalQuota)
	assert.EqualValues(t, 250_000, proj.BonusQuota)
	assert.EqualValues(t, 1_250_000, proj.TotalQuota)
	assert.True(t, proj.PrincipalUSD.Equal(decimal.NewFromInt(2)))
	assert.True(t, proj.BonusUSD.Equal(decimal.RequireFromString("0.5")))
	assert.True(t, proj.TotalUSD.Equal(decimal.RequireFromString("2.5")))
	assert.Equal(t, service.WalletLotProjectionSourceBatches, proj.Source)

	sum, err := model.SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	assert.Equal(t, sum.Principal, proj.PrincipalQuota)
	assert.Equal(t, sum.Bonus, proj.BonusQuota)
}

func TestGetSelfPrincipalOnlyBonusOnlyAndZeroBonus(t *testing.T) {
	setupWalletLotProjectionController(t)

	principalOnly := createProjectionUser(t, "self-principal", 500_000)
	require.NoError(t, model.DB.Create(&model.AdtyFundBatch{
		UserId: principalOnly.Id, Kind: "principal", Source: "legacy", Original: 500000, Remaining: 500000,
	}).Error)
	ok, body, data := getSelfJSON(t, principalOnly)
	require.True(t, ok, body)
	proj := lotProjectionFrom(t, data)
	assert.EqualValues(t, 500_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
	assert.True(t, proj.BonusUSD.IsZero())

	bonusOnly := createProjectionUser(t, "self-bonus", 250_000)
	require.NoError(t, model.DB.Create(&model.AdtyFundBatch{
		UserId: bonusOnly.Id, Kind: "bonus", Source: "gift", Original: 250000, Remaining: 250000,
	}).Error)
	ok, body, data = getSelfJSON(t, bonusOnly)
	require.True(t, ok, body)
	proj = lotProjectionFrom(t, data)
	assert.EqualValues(t, 0, proj.PrincipalQuota)
	assert.EqualValues(t, 250_000, proj.BonusQuota)
	assert.True(t, proj.PrincipalUSD.IsZero())
	assert.True(t, proj.BonusUSD.Equal(decimal.RequireFromString("0.5")))

	zeroBonus := createProjectionUser(t, "self-zero-bonus", 100_000)
	require.NoError(t, model.DB.Create([]model.AdtyFundBatch{
		{UserId: zeroBonus.Id, Kind: "principal", Source: "paid", Original: 100000, Remaining: 100000},
		{UserId: zeroBonus.Id, Kind: "bonus", Source: "paid", Original: 50, Remaining: 0},
	}).Error)
	ok, body, data = getSelfJSON(t, zeroBonus)
	require.True(t, ok, body)
	proj = lotProjectionFrom(t, data)
	assert.EqualValues(t, 100_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
}

func TestGetSelfDoesNotExposeAnotherUsersLotSplit(t *testing.T) {
	setupWalletLotProjectionController(t)
	alice := createProjectionUser(t, "self-alice", 1_000_000)
	bob := createProjectionUser(t, "self-bob", 2_500_000)
	require.NoError(t, model.DB.Create([]model.AdtyFundBatch{
		{UserId: alice.Id, Kind: "principal", Source: "a", Original: 1000000, Remaining: 1000000},
		{UserId: bob.Id, Kind: "bonus", Source: "b", Original: 2500000, Remaining: 2500000},
	}).Error)

	ok, body, data := getSelfJSON(t, alice)
	require.True(t, ok, body)
	proj := lotProjectionFrom(t, data)
	assert.EqualValues(t, 1_000_000, proj.PrincipalQuota)
	assert.EqualValues(t, 0, proj.BonusQuota)
	assert.True(t, proj.PrincipalUSD.Equal(decimal.NewFromInt(2)))
	assert.NotContains(t, body, "2.5")
	assert.NotContains(t, body, "2500000")

	ok, body, data = getSelfJSON(t, bob)
	require.True(t, ok, body)
	proj = lotProjectionFrom(t, data)
	assert.EqualValues(t, 0, proj.PrincipalQuota)
	assert.EqualValues(t, 2_500_000, proj.BonusQuota)
	assert.True(t, proj.BonusUSD.Equal(decimal.RequireFromString("5")))
	assert.NotContains(t, body, `"principal_quota":1000000`)
}

func TestGetSelfLotProjectionLeavesSpendAndCreditUnchanged(t *testing.T) {
	setupWalletLotProjectionController(t)
	user := createProjectionUser(t, "self-spend", 90)
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.CreateAdtyFundBatchesTx(tx, user.Id, "credit-1", 50, 40)
	}))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.ConsumeAdtyFundBatchesTx(tx, user.Id, 45, 90)
	}))
	ok, body, data := getSelfJSON(t, user)
	require.True(t, ok, body)
	proj := lotProjectionFrom(t, data)
	assert.EqualValues(t, 5, proj.PrincipalQuota)
	assert.EqualValues(t, 40, proj.BonusQuota)
	sum, err := model.SumAdtyFundBatchRemainingByKind(user.Id)
	require.NoError(t, err)
	assert.Equal(t, sum.Principal, proj.PrincipalQuota)
	assert.Equal(t, sum.Bonus, proj.BonusQuota)
}
