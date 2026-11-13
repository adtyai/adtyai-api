package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type topUpProofFixture struct {
	db     *gorm.DB
	admin  model.User
	root   model.User
	buyer  model.User
	adminI service.AuthIdentity
	rootI  service.AuthIdentity
}

func setupTopUpProofFixture(t *testing.T) *topUpProofFixture {
	t.Helper()
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.AuditLog{}, &model.Log{}, &model.TopUp{}, &model.TwoFA{}, &model.PasskeyCredential{}))
	require.NoError(t, model.MigrateAdtyTables())
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })

	password, err := common.Password2Hash("enrollment-password")
	require.NoError(t, err)
	admin := model.User{Username: "topup-proof-admin", Password: password, Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	root := model.User{Username: "topup-proof-root", Password: password, Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	buyer := model.User{Username: "topup-proof-buyer", Password: password, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&buyer).Error)
	require.NoError(t, model.PublishUserAuthCache(admin.Id))
	require.NoError(t, model.PublishUserAuthCache(root.Id))

	adminBundle, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "topup-proof-admin")
	require.NoError(t, err)
	adminI, err := service.ParseAccessToken(adminBundle.AccessToken)
	require.NoError(t, err)
	rootBundle, err := service.CreateLoginSession(root.Id, "password", "127.0.0.1", "topup-proof-root")
	require.NoError(t, err)
	rootI, err := service.ParseAccessToken(rootBundle.AccessToken)
	require.NoError(t, err)
	return &topUpProofFixture{db: db, admin: admin, root: root, buyer: buyer, adminI: adminI, rootI: rootI}
}

func seedProofOrder(t *testing.T, userID int, tradeNo string) {
	t.Helper()
	require.NoError(t, model.CreateAdtyTopUp(
		&model.TopUp{UserId: userID, Amount: 100, TradeNo: tradeNo, PaymentMethod: "usdt.trc20", PaymentProvider: model.PaymentProviderAdtyBEpusdt, CreateTime: 1, Status: common.TopUpStatusPending},
		&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: userID, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20",
			FaceUSD: decimal.NewFromInt(100), BonusUSD: decimal.RequireFromString("2.5"), CreditQuota: 51_250_000,
			PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", TopupGroupRatio: decimal.NewFromInt(1), ExpireAt: common.GetTimestamp() + 900},
	))
}

func issueTopUpProof(t *testing.T, identity service.AuthIdentity, method string, operation service.VerificationOperation) string {
	t.Helper()
	binding, err := service.BindVerificationOperation(operation)
	require.NoError(t, err)
	proof, _, err := service.IssueSecurityProof(identity, method, binding)
	require.NoError(t, err)
	return proof
}

func performTopUpAdminRequest(t *testing.T, method, path, body, proof string, identity service.AuthIdentity, role int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if body == "" {
		c.Request = httptest.NewRequest(method, path, http.NoBody)
	} else {
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	if proof != "" {
		c.Request.Header.Set("X-Security-Proof", proof)
	}
	if tradeNo := c.Request.URL.Path; true {
		parts := strings.Split(strings.Trim(tradeNo, "/"), "/")
		for i, part := range parts {
			if part == "topup" && i+1 < len(parts) {
				c.Params = gin.Params{{Key: "trade_no", Value: parts[i+1]}}
				break
			}
		}
	}
	c.Set("id", identity.UserID)
	c.Set("role", role)
	c.Set("username", fmt.Sprintf("user-%d", identity.UserID))
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
	handler(c)
	return recorder
}

func TestManualCompleteAdtyTopUpRequiresBoundSecurityProof(t *testing.T) {
	f := setupTopUpProofFixture(t)
	seedProofOrder(t, f.buyer.Id, "EPU-ctrl-complete")
	path := "/api/adty/admin/topup/EPU-ctrl-complete/complete"
	body := `{"proof":"0xabc","note":"checked"}`
	op := service.AdtyTopUpManualCompleteOperation("EPU-ctrl-complete", "0xabc", "checked")

	missing := performTopUpAdminRequest(t, http.MethodPost, path, body, "", f.adminI, common.RoleAdminUser, ManualCompleteAdtyTopUp)
	assert.Equal(t, http.StatusForbidden, missing.Code)
	assert.Contains(t, missing.Body.String(), "SECURITY_PROOF_REQUIRED")
	var order model.TopUp
	require.NoError(t, f.db.Where("trade_no = ?", "EPU-ctrl-complete").First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	var buyer model.User
	require.NoError(t, f.db.First(&buyer, f.buyer.Id).Error)
	assert.Zero(t, buyer.Quota)

	mismatch := performTopUpAdminRequest(t, http.MethodPost, path, body, issueTopUpProof(t, f.adminI, service.VerificationMethodPassword, service.AdtyTopUpManualCompleteOperation("EPU-other", "0xabc", "checked")), f.adminI, common.RoleAdminUser, ManualCompleteAdtyTopUp)
	assert.Contains(t, mismatch.Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH")
	require.NoError(t, f.db.Where("trade_no = ?", "EPU-ctrl-complete").First(&order).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)

	proof := issueTopUpProof(t, f.adminI, service.VerificationMethodPassword, op)
	ok := performTopUpAdminRequest(t, http.MethodPost, path, body, proof, f.adminI, common.RoleAdminUser, ManualCompleteAdtyTopUp)
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	require.NoError(t, f.db.First(&buyer, f.buyer.Id).Error)
	assert.Equal(t, 51_250_000, buyer.Quota)
	replay := performTopUpAdminRequest(t, http.MethodPost, path, body, proof, f.adminI, common.RoleAdminUser, ManualCompleteAdtyTopUp)
	assert.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED")
}

func TestRefundAdtyTopUpRequiresBoundSecurityProof(t *testing.T) {
	f := setupTopUpProofFixture(t)
	seedProofOrder(t, f.buyer.Id, "EPU-ctrl-refund")
	complete := performTopUpAdminRequest(t, http.MethodPost, "/api/adty/admin/topup/EPU-ctrl-refund/complete", `{"proof":"0xabc","note":"checked"}`,
		issueTopUpProof(t, f.adminI, service.VerificationMethodPassword, service.AdtyTopUpManualCompleteOperation("EPU-ctrl-refund", "0xabc", "checked")),
		f.adminI, common.RoleAdminUser, ManualCompleteAdtyTopUp)
	require.Contains(t, complete.Body.String(), `"success":true`, complete.Body.String())

	preview := performTopUpAdminRequest(t, http.MethodGet, "/api/adty/admin/topup/EPU-ctrl-refund/refund-preview", "", "", f.adminI, common.RoleAdminUser, PreviewAdtyTopUpRefund)
	require.Contains(t, preview.Body.String(), `"success":true`, preview.Body.String())
	require.Contains(t, preview.Body.String(), `"refund_quota":50000000`)

	path := "/api/adty/admin/topup/EPU-ctrl-refund/refund"
	body := `{"reason":"duplicate","idempotency_key":"ctrl-refund-1","refund_usd":"100"}`
	op := service.AdtyTopUpRefundOperation("EPU-ctrl-refund", "duplicate", "100", nil)
	missing := performTopUpAdminRequest(t, http.MethodPost, path, body, "", f.adminI, common.RoleAdminUser, RefundAdtyTopUp)
	assert.Equal(t, http.StatusForbidden, missing.Code)
	assert.Contains(t, missing.Body.String(), "SECURITY_PROOF_REQUIRED")
	var buyer model.User
	require.NoError(t, f.db.First(&buyer, f.buyer.Id).Error)
	assert.Equal(t, 51_250_000, buyer.Quota)

	wrongAmount := performTopUpAdminRequest(t, http.MethodPost, path, body, issueTopUpProof(t, f.adminI, service.VerificationMethodPassword, service.AdtyTopUpRefundOperation("EPU-ctrl-refund", "duplicate", "50", nil)), f.adminI, common.RoleAdminUser, RefundAdtyTopUp)
	assert.Contains(t, wrongAmount.Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH")
	require.NoError(t, f.db.First(&buyer, f.buyer.Id).Error)
	assert.Equal(t, 51_250_000, buyer.Quota)

	proof := issueTopUpProof(t, f.adminI, service.VerificationMethodPassword, op)
	refund := performTopUpAdminRequest(t, http.MethodPost, path, body, proof, f.adminI, common.RoleAdminUser, RefundAdtyTopUp)
	require.Contains(t, refund.Body.String(), `"success":true`, refund.Body.String())
	assert.Contains(t, refund.Body.String(), `"refund_id":"ctrl-refund-1"`)
	assert.Contains(t, refund.Body.String(), `"request_quota":50000000`)
	assert.Contains(t, refund.Body.String(), `"status":"submitted"`)
	require.NoError(t, f.db.First(&buyer, f.buyer.Id).Error)
	assert.Equal(t, 51_250_000, buyer.Quota, "apply occupies the cap but does not debit")
	replay := performTopUpAdminRequest(t, http.MethodPost, path, body, proof, f.adminI, common.RoleAdminUser, RefundAdtyTopUp)
	assert.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED")
}

func TestRootManualCompleteUsesRootExemptProofAndWritesAudit(t *testing.T) {
	f := setupTopUpProofFixture(t)
	seedProofOrder(t, f.buyer.Id, "EPU-ctrl-root")
	path := "/api/adty/admin/topup/EPU-ctrl-root/complete"
	body := `{"proof":"0xabc","note":"checked"}`
	requirements, err := service.GetVerificationRequirements(f.rootI, service.VerificationScopeAdtyTopUpManualComplete)
	require.NoError(t, err)
	require.Len(t, requirements.Methods, 1)
	assert.Equal(t, service.VerificationMethodRootExempt, requirements.Methods[0].Method)

	ok := performTopUpAdminRequest(t, http.MethodPost, path, body, issueTopUpProof(t, f.rootI, service.VerificationMethodRootExempt, service.AdtyTopUpManualCompleteOperation("EPU-ctrl-root", "0xabc", "checked")), f.rootI, common.RoleRootUser, ManualCompleteAdtyTopUp)
	require.Contains(t, ok.Body.String(), `"success":true`, ok.Body.String())
	var audit model.AuditLog
	require.NoError(t, f.db.Where("action = ?", "adty.topup.manual_complete").Order("id desc").First(&audit).Error)
	assert.Equal(t, f.root.Id, audit.UserId)
	assert.Contains(t, audit.Content, "EPU-ctrl-root")
}
