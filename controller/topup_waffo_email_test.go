package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	waffo "github.com/waffo-com/waffo-go"
	"github.com/waffo-com/waffo-go/core"
	"github.com/waffo-com/waffo-go/types/order"
)

func TestWaffoPayUsesAccountEmail(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	require.NoError(t, model.MigrateAdtyTables())
	empty := &model.User{Username: "waffo-no-mail", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Email: "  "}
	buyer := &model.User{Username: "waffo-buyer", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Email: "Buyer@Example.com"}
	require.NoError(t, db.Create(empty).Error)
	require.NoError(t, db.Create(buyer).Error)

	previousDisplay := operation_setting.GetGeneralSetting().QuotaDisplayType
	previousEnabled := setting.WaffoEnabled
	previousSandbox := setting.WaffoSandbox
	previousAPI := setting.WaffoSandboxApiKey
	previousPrivate := setting.WaffoSandboxPrivateKey
	previousPublic := setting.WaffoSandboxPublicCert
	previousMin := setting.WaffoMinTopUp
	previousPrice := setting.WaffoUnitPrice
	previousRedis := common.RedisEnabled
	previousCreate := waffoCreateOrder
	privateKey, publicKey := waffoTestKeyPair(t)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = "waffo_test_f077"
	setting.WaffoSandboxPrivateKey = privateKey
	setting.WaffoSandboxPublicCert = publicKey
	setting.WaffoMinTopUp = 1
	setting.WaffoUnitPrice = 1
	common.RedisEnabled = false
	var createCalls int
	waffoCreateOrder = func(_ context.Context, _ *waffo.Waffo, params *order.CreateOrderParams) (*core.ApiResponse[order.CreateOrderData], error) {
		createCalls++
		_ = params.UserInfo.UserEmail
		return nil, http.ErrServerClosed
	}
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplay
		setting.WaffoEnabled = previousEnabled
		setting.WaffoSandbox = previousSandbox
		setting.WaffoSandboxApiKey = previousAPI
		setting.WaffoSandboxPrivateKey = previousPrivate
		setting.WaffoSandboxPublicCert = previousPublic
		setting.WaffoMinTopUp = previousMin
		setting.WaffoUnitPrice = previousPrice
		common.RedisEnabled = previousRedis
		waffoCreateOrder = previousCreate
	})

	missing := callWaffo(t, RequestWaffoPay, empty.Id, `{"amount":2}`)
	require.Contains(t, missing.Body.String(), "not available")
	require.Zero(t, createCalls)
	var orders int64
	require.NoError(t, db.Model(&model.TopUp{}).Where("user_id = ?", empty.Id).Count(&orders).Error)
	require.Zero(t, orders)

	paid := callWaffo(t, RequestWaffoPay, buyer.Id, `{"amount":2}`)
	require.Contains(t, paid.Body.String(), "not available")
	require.Zero(t, createCalls)
	require.NoError(t, db.Model(&model.TopUp{}).Where("user_id = ?", buyer.Id).Count(&orders).Error)
	require.Zero(t, orders)
}
