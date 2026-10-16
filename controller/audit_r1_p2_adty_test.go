package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 审查 P2-6：管理员不能给自己建/改企业协议价；root 可以。
func TestEnterprisePricingSelfForbiddenForAdmin(t *testing.T) {
	database := modelManagementDB(t, "sqlite", "")
	require.NoError(t, database.AutoMigrate(&model.EnterprisePriceRule{}))
	require.NoError(t, model.EnsureEnterprisePriceSchema(database))
	bob := model.User{Username: "bob26", Password: "unused-password", Role: common.RoleAdminUser, AffCode: "p26b"}
	require.NoError(t, database.Create(&bob).Error)

	body := `{"user_id":` + strconv.Itoa(bob.Id) + `,"discount":0.1}`
	rec := adtyCall(AdminCreateEnterprisePricing, http.MethodPost, "/x", body, bob.Id, common.RoleAdminUser, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "self_pricing_forbidden")

	rec = adtyCall(AdminCreateEnterprisePricing, http.MethodPost, "/x", body, 1, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data struct {
			Id int64 `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	rec = adtyCall(AdminUpdateEnterprisePricing, http.MethodPut, "/x", `{"discount":0.1}`, bob.Id, common.RoleAdminUser,
		gin.Params{{Key: "id", Value: strconv.FormatInt(out.Data.Id, 10)}})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// 审查 P2-8：重复兑换提示「已被使用」。
func TestRedeemUsedCodeMessage(t *testing.T) {
	database := modelManagementDB(t, "sqlite", "")
	require.NoError(t, database.AutoMigrate(&model.Redemption{}, &model.UserBonusWallet{}, &model.LedgerIdempotency{}, &model.Log{}))
	require.NoError(t, model.EnsureLedgerSchema(database))
	u := model.User{Username: "carol28", Password: "unused-password", AffCode: "p28c"}
	require.NoError(t, database.Create(&u).Error)
	r := model.Redemption{Key: "p28usedcode000000000000000000000", Status: common.RedemptionCodeStatusUsed, Quota: 10, Name: "x"}
	require.NoError(t, database.Create(&r).Error)
	rec := adtyCall(TopUp, http.MethodPost, "/api/user/topup", `{"key":"`+r.Key+`"}`, u.Id, common.RoleCommonUser, nil)
	require.Contains(t, rec.Body.String(), "This redemption code has already been used", rec.Body.String())
	rec = adtyCall(TopUp, http.MethodPost, "/api/user/topup", `{"key":"doesnotexist"}`, u.Id, common.RoleCommonUser, nil)
	require.Contains(t, rec.Body.String(), "Redemption failed, please try again later")
}

// 审查 P2-9：档位赠送金额两位小数，无浮点尾巴。
func TestTopupTiersBonusAmountRounded(t *testing.T) {
	modelManagementDB(t, "sqlite", "")
	require.NoError(t, model.DB.AutoMigrate(&model.EpayInstance{}))
	rec := adtyCall(GetTopupTiers, http.MethodGet, "/api/topup/tiers", "", 0, 0, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data struct {
			Tiers []struct {
				BonusAmount float64 `json:"bonus_amount"`
			} `json:"tiers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	require.NotEmpty(t, out.Data.Tiers, rec.Body.String())
	for _, tr := range out.Data.Tiers {
		s := strconv.FormatFloat(tr.BonusAmount, 'f', -1, 64)
		require.LessOrEqual(t, len(s), len(strconv.FormatFloat(tr.BonusAmount, 'f', 2, 64)), s)
	}
}
