package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func adtyCall(handler gin.HandlerFunc, method, path, body string, uid, role int, params gin.Params) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("id", uid)
	c.Set("role", role)
	handler(c)
	return rec
}

func seedCompleteFixture(t *testing.T) (rootID, aliceID int) {
	t.Helper()
	database := adtyTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.TopupOrder{}, &model.UserBonusWallet{}, &model.LedgerIdempotency{}, &model.Log{}, &model.AffCommission{}, &model.EpayInstance{}))
	require.NoError(t, model.EnsureLedgerSchema(database))
	root := model.User{Username: "root1", Password: "unused-password", Role: common.RoleRootUser, AffCode: "p11r"}
	alice := model.User{Username: "alice1", Password: "unused-password", AffCode: "p11a"}
	require.NoError(t, database.Create(&root).Error)
	require.NoError(t, database.Create(&alice).Error)
	orders := []model.TopupOrder{
		{OrderNo: "P11-PLACEHOLDER", UserId: alice.Id, AmountUSD: 10, CreditAmountUSD: 10, PaymentMethod: "x", PaymentProvider: model.TopupOrderProviderPlaceholder, Status: model.TopupOrderStatusPending, RefundStatus: "none"},
		{OrderNo: "P11-EPAY", UserId: alice.Id, AmountUSD: 10, CreditAmountUSD: 10, PaymentMethod: "alipay", PaymentProvider: "epay", Status: model.TopupOrderStatusPending, RefundStatus: "none"},
		{OrderNo: "P11-SELF", UserId: root.Id, AmountUSD: 10, CreditAmountUSD: 10, PaymentMethod: "alipay", PaymentProvider: "epay", Status: model.TopupOrderStatusPending, RefundStatus: "none"},
	}
	for i := range orders {
		require.NoError(t, database.Create(&orders[i]).Error)
	}
	return root.Id, alice.Id
}

// 审查 P1-1：人工补单需凭证、需真实拉起过支付、禁止自补，流水记真实 operator_id。
func TestAdminCompleteTopupRequiresEvidenceAndRecordsOperator(t *testing.T) {
	rootID, aliceID := seedCompleteFixture(t)
	p := func(no string) gin.Params { return gin.Params{{Key: "no", Value: no}} }

	rec := adtyCall(AdminCompleteAdtyTopup, http.MethodPost, "/x", `{"confirm":true}`, rootID, common.RoleRootUser, p("P11-EPAY"))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "trade_no_required")

	rec = adtyCall(AdminCompleteAdtyTopup, http.MethodPost, "/x", `{"trade_no":"GW-1"}`, rootID, common.RoleRootUser, p("P11-PLACEHOLDER"))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "no_payment_attempt")

	rec = adtyCall(AdminCompleteAdtyTopup, http.MethodPost, "/x", `{"trade_no":"GW-2"}`, rootID, common.RoleRootUser, p("P11-SELF"))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "self_complete_forbidden")

	rec = adtyCall(AdminCompleteAdtyTopup, http.MethodPost, "/x", `{"trade_no":"GW-3"}`, rootID, common.RoleRootUser, p("P11-EPAY"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var entries []model.LedgerEntry
	require.NoError(t, model.DB.Where("user_id = ? AND ref_id = ?", aliceID, "P11-EPAY").Find(&entries).Error)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		require.Equal(t, rootID, e.OperatorId, e.Kind)
	}

	// 交易号被占用后不能再给别的订单用
	rec = adtyCall(AdminEpayManualComplete, http.MethodPost, "/x", `{"order_no":"P11-SELF","trade_no":"GW-3"}`, aliceID+1000, common.RoleRootUser, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

// 审查 P1-2：下单只接受 alipay/wxpay/usdt。
func TestCreateTopupOrderRejectsUnknownPaymentMethod(t *testing.T) {
	_, aliceID := seedCompleteFixture(t)
	for _, body := range []string{`{"amount":10,"payment_method":"foo"}`, `{"amount":10}`} {
		rec := adtyCall(CreateTopupOrder, http.MethodPost, "/api/topup/order", body, aliceID, common.RoleCommonUser, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "invalid_payment_method")
	}
	var n int64
	require.NoError(t, model.DB.Model(&model.TopupOrder{}).Where("user_id = ? AND payment_method IN ?", aliceID, []string{"foo", ""}).Count(&n).Error)
	require.Zero(t, n)
}

// adtyTestDB 默认 sqlite；ADTY_TEST_PG=1 且设置 TEST_POSTGRES_DSN（回环）时在 PostgreSQL 隔离库上跑。
func adtyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("ADTY_TEST_PG") == "1" && os.Getenv("TEST_POSTGRES_DSN") != "" {
		return modelManagementDB(t, "postgres", os.Getenv("TEST_POSTGRES_DSN"))
	}
	return modelManagementDB(t, "sqlite", "")
}
