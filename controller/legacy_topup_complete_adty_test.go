package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// 审查重审 P0-1：legacy POST /api/user/topup/complete 仅 root；admin → 403，root 可补单。
func TestLegacyAdminCompleteTopUpRootOnly(t *testing.T) {
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.TopUp{}, &model.User{}, &model.Log{}))

	root := model.User{Username: "legroot", Password: "unused-password", Role: common.RoleRootUser, AffCode: "legr", Quota: 0}
	alice := model.User{Username: "legalice", Password: "unused-password", Role: common.RoleCommonUser, AffCode: "lega", Quota: 1000}
	bob := model.User{Username: "legbob", Password: "unused-password", Role: common.RoleAdminUser, AffCode: "legb", Quota: 0}
	require.NoError(t, database.Create(&root).Error)
	require.NoError(t, database.Create(&alice).Error)
	require.NoError(t, database.Create(&bob).Error)

	tradeNo := "USR2NOLEGACYTEST001"
	top := &model.TopUp{
		UserId:          alice.Id,
		Amount:          10,
		Money:           70.7,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, top.Insert())

	body := `{"trade_no":"` + tradeNo + `"}`

	rec := adtyCall(AdminCompleteTopUp, http.MethodPost, "/api/user/topup/complete", body, bob.Id, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "AUTH_INSUFFICIENT_PRIVILEGE")

	var before model.User
	require.NoError(t, database.First(&before, alice.Id).Error)

	rec = adtyCall(AdminCompleteTopUp, http.MethodPost, "/api/user/topup/complete", body, root.Id, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var after model.User
	require.NoError(t, database.First(&after, alice.Id).Error)
	require.Greater(t, after.Quota, before.Quota)

	var done model.TopUp
	require.NoError(t, database.Where("trade_no = ?", tradeNo).First(&done).Error)
	require.Equal(t, common.TopUpStatusSuccess, done.Status)
}

// 审查重审 P2-2：legacy POST /api/user/pay 返回 410，引导二开 /api/topup/order。
func TestRequestEpayReturnsGone(t *testing.T) {
	adtyTestDB(t)
	rec := adtyCall(RequestEpay, http.MethodPost, "/api/user/pay", `{"amount":10,"payment_method":"alipay"}`, 2, common.RoleCommonUser, nil)
	require.Equal(t, http.StatusGone, rec.Code, rec.Body.String())
	var out struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.False(t, out.Success)
	require.Equal(t, "LEGACY_PAY_REMOVED", out.Code)
}
