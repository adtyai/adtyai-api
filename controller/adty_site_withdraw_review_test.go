package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

// withdrawRejectFixture 是一条待审核的站点提现：站点收益 50 已冻结 20，钱包余 30。
type withdrawRejectFixture struct {
	db       *gorm.DB
	site     model.AdtySite
	withdraw model.AdtySiteWithdrawal
}

func prepareWithdrawRejectFixture(t *testing.T, name string) withdrawRejectFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}))
	require.NoError(t, db.AutoMigrate(model.AdtySiteTables()...))
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousOptions })

	owner := &model.User{
		Username: name + "-owner", Password: "hashed-password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Email: name + "-owner@example.com",
	}
	require.NoError(t, db.Create(owner).Error)
	site := model.AdtySite{
		OwnerUserId: owner.Id, Subdomain: name, RootDomain: "sites.example.com",
		Status: model.AdtySiteStatusActive, AgreementVer: service.AdtyCurrentSiteAgreementVersion(),
	}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, service.SaveAdtySiteWithdrawRules(service.AdtySiteWithdrawRules{
		MinUSD: "10", FeeUSD: "1", AlipayCNYRate: "7.20", USDTUSDRate: "1",
	}, owner.Id))
	_, err := service.ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("50"), service.AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: name + "-earn",
	})
	require.NoError(t, err)
	row, err := service.CreateAdtySiteWithdrawal(service.AdtySiteWithdrawInput{
		SiteID: site.Id, OwnerID: owner.Id, AmountUSD: decimal.RequireFromString("20"),
		Method: "alipay", AccountName: "Ada", AccountNo: "ada@example.com",
		ExpectedRate: decimal.RequireFromString("7.20"), IdempotencyKey: name + "-wd",
	})
	require.NoError(t, err)
	require.Equal(t, model.AdtySiteWithdrawPending, row.Status)
	return withdrawRejectFixture{db: db, site: site, withdraw: *row}
}

func (f withdrawRejectFixture) earnings(t *testing.T) decimal.Decimal {
	t.Helper()
	var wallet model.AdtySiteWallet
	require.NoError(t, f.db.Where("site_id = ? AND kind = ?", f.site.Id, model.AdtySiteWalletEarnings).First(&wallet).Error)
	return wallet.Balance
}

func (f withdrawRejectFixture) reload(t *testing.T) model.AdtySiteWithdrawal {
	t.Helper()
	var row model.AdtySiteWithdrawal
	require.NoError(t, f.db.First(&row, f.withdraw.Id).Error)
	return row
}

func postWithdrawReview(t *testing.T, action string, id int64, body string) (bool, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/adty/admin/site-withdrawals/"+strconv.FormatInt(id, 10)+"/"+action, strings.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(id, 10)}}
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	switch action {
	case "reject":
		PostAdtySiteWithdrawalReject(c)
	case "paying":
		PostAdtySiteWithdrawalPaying(c)
	default:
		PostAdtySiteWithdrawalPaid(c)
	}
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response.Success, response.Message
}

// 不可解析的请求体不能带着空 reason 走下去：那是一笔真实的资金退回。
func TestSiteWithdrawalRejectRefusesUnreadableBody(t *testing.T) {
	f := prepareWithdrawRejectFixture(t, "wd-reject-body")

	ok, message := postWithdrawReview(t, "reject", f.withdraw.Id, `{"reason":123}`)
	assert.False(t, ok, "type mismatch must not reach the service")
	assert.Contains(t, message, "Invalid parameters")
	assert.Equal(t, model.AdtySiteWithdrawPending, f.reload(t).Status)
	assert.True(t, f.earnings(t).Equal(decimal.RequireFromString("30")), "frozen earnings must stay frozen")

	ok, message = postWithdrawReview(t, "reject", f.withdraw.Id, `{"reason":`)
	assert.False(t, ok, "truncated json must not reach the service")
	assert.Contains(t, message, "Invalid parameters")
	assert.Equal(t, model.AdtySiteWithdrawPending, f.reload(t).Status)
	assert.True(t, f.earnings(t).Equal(decimal.RequireFromString("30")))
}

// 驳回必须留下可审计的理由；空白 reason 与缺失 reason 一样被拒绝。
func TestSiteWithdrawalRejectRequiresReason(t *testing.T) {
	f := prepareWithdrawRejectFixture(t, "wd-reject-reason")

	for _, body := range []string{`{}`, `{"reason":""}`, `{"reason":"   "}`} {
		ok, message := postWithdrawReview(t, "reject", f.withdraw.Id, body)
		assert.False(t, ok, body)
		assert.Contains(t, message, "Reject reason is required", body)
		assert.Equal(t, model.AdtySiteWithdrawPending, f.reload(t).Status, body)
		assert.True(t, f.earnings(t).Equal(decimal.RequireFromString("30")), body)
	}

	ok, message := postWithdrawReview(t, "reject", f.withdraw.Id, `{"reason":"  账户信息不符  "}`)
	require.True(t, ok, message)
	row := f.reload(t)
	assert.Equal(t, model.AdtySiteWithdrawRejected, row.Status)
	assert.Equal(t, "账户信息不符", row.ProofNote, "the audit reason must be stored trimmed")
	assert.True(t, f.earnings(t).Equal(decimal.RequireFromString("50")), "reject releases the frozen amount once")
}

// paying 不带请求体是合法的：收紧解码不能把没有参数的动作用 400 挡掉。
func TestSiteWithdrawalPayingWithoutBodyStillWorks(t *testing.T) {
	f := prepareWithdrawRejectFixture(t, "wd-paying-nobody")

	ok, message := postWithdrawReview(t, "paying", f.withdraw.Id, "")
	require.True(t, ok, message)
	assert.Equal(t, model.AdtySiteWithdrawPaying, f.reload(t).Status)
	assert.True(t, f.earnings(t).Equal(decimal.RequireFromString("30")), "paying keeps the freeze")
}
