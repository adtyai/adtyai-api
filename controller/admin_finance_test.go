package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeAuditProviderCapabilitiesDatabaseMatrix(t *testing.T) {
	providers := []string{model.PaymentProviderAdtyBEpusdt, model.PaymentProviderAdtyEpay, model.PaymentProviderStripe,
		model.PaymentProviderCreem, model.PaymentProviderWaffo, model.PaymentProviderWaffoPancake}
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("database DSN not configured")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.AdtyTopupOrder{}))
			user := model.User{Username: "audit-provider-buyer", Role: common.RoleCommonUser}
			require.NoError(t, db.Create(&user).Error)
			for _, provider := range providers {
				for _, status := range []string{common.TopUpStatusPending, common.TopUpStatusSuccess} {
					tradeNo := provider + "-" + status
					require.NoError(t, db.Create(&model.TopUp{TradeNo: tradeNo, UserId: user.Id, PaymentProvider: provider, PaymentMethod: provider, Status: status}).Error)
					require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: tradeNo, UserId: user.Id, Provider: provider, Method: provider,
						FaceUSD: decimal.NewFromInt(2), PayAmount: decimal.NewFromInt(2), PayCurrency: "USD", CreditQuota: 1000000}).Error)
				}
			}
			for _, order := range []model.TopUp{
				{TradeNo: "missing-details", PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending},
				{TradeNo: "legacy-payment", PaymentProvider: "legacy-payment", Status: common.TopUpStatusPending},
				{TradeNo: "replaced-order", PaymentProvider: model.PaymentProviderCreem, Status: model.TopUpStatusReplaced},
			} {
				order.UserId = user.Id
				require.NoError(t, db.Create(&order).Error)
			}
			require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: "replaced-order", UserId: user.Id, Provider: model.PaymentProviderCreem}).Error)
			request := func(path string, handler gin.HandlerFunc, target any) {
				t.Helper()
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, path, nil)
				c.Set("role", common.RoleRootUser)
				handler(c)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), target))
			}
			var summary struct {
				Success bool
				Data    struct {
					Providers    []string `json:"provider_option"`
					Capabilities []struct {
						Provider   string `json:"provider"`
						Query      bool   `json:"query"`
						Complete   bool   `json:"complete"`
						Refund     bool   `json:"refund"`
						RefundMode string `json:"refund_mode"`
					} `json:"provider_capabilities"`
				}
			}
			request("/summary", GetRechargeAuditSummary, &summary)
			require.True(t, summary.Success)
			assert.ElementsMatch(t, providers, summary.Data.Providers)
			require.Len(t, summary.Data.Capabilities, 6)
			for _, capability := range summary.Data.Capabilities {
				assert.Equal(t, capability.Provider != model.PaymentProviderAdtyBEpusdt, capability.Query, capability.Provider)
				assert.True(t, capability.Complete, capability.Provider)
				assert.True(t, capability.Refund, capability.Provider)
				assert.Equal(t, "offline", capability.RefundMode, "ledger refunds do not refund the provider automatically")
			}
			for _, provider := range append(providers, "") {
				var listed struct {
					Success bool
					Data    struct {
						Total int
						Items []struct {
							TradeNo  string `json:"trade_no"`
							Provider string `json:"payment_provider"`
							Status   string
							Actions  struct{ Complete, Refund, Query bool } `json:"actions"`
						}
					}
				}
				request("/orders?page_size=50&provider="+provider, GetRechargeAudit, &listed)
				require.True(t, listed.Success)
				if provider == "" {
					assert.Equal(t, 15, listed.Data.Total)
				}
				for _, order := range listed.Data.Items {
					if provider != "" {
						assert.Equal(t, provider, order.Provider)
					}
					if order.TradeNo == "missing-details" || order.TradeNo == "legacy-payment" || order.TradeNo == "replaced-order" {
						assert.False(t, order.Actions.Complete, order.TradeNo)
						assert.False(t, order.Actions.Refund, order.TradeNo)
						assert.False(t, order.Actions.Query, order.TradeNo)
						continue
					}
					assert.Equal(t, order.Status == common.TopUpStatusPending, order.Actions.Complete, fmt.Sprint(order))
					assert.Equal(t, order.Status == common.TopUpStatusSuccess, order.Actions.Refund, fmt.Sprint(order))
					assert.Equal(t, order.Status == common.TopUpStatusPending && order.Provider != model.PaymentProviderAdtyBEpusdt, order.Actions.Query, fmt.Sprint(order))
				}
			}
		})
	}
}
