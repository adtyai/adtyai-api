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
			require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.AdtyTopupOrder{}, &model.AdtyEpayChannel{}))
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

func TestRechargeAuditSiteOwnershipDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("database DSN not configured")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.AdtyTopupOrder{}, &model.AdtySite{}, &model.AdtyEpayChannel{}))
			for _, site := range []model.AdtySite{
				{Id: 7, Subdomain: "north", RootDomain: "sites.test", Brand: `{"name":"North Studio"}`},
				{Id: 8, Subdomain: "south", RootDomain: "sites.test", Brand: `{"name":"South Studio"}`},
			} {
				require.NoError(t, db.Create(&site).Error)
			}
			user := model.User{Username: "moved-order-buyer", Role: common.RoleCommonUser, SiteId: 7}
			require.NoError(t, db.Create(&user).Error)
			for i, fixture := range []struct {
				trade  string
				siteID int
				face   int64
			}{
				{"main-order", 0, 2}, {"north-original", 7, 3}, {"south-current", 8, 5},
				{"legacy-unknown", -1, 7}, {"historical-site", 99, 11},
			} {
				require.NoError(t, db.Create(&model.TopUp{TradeNo: fixture.trade, UserId: user.Id,
					PaymentProvider: model.PaymentProviderStripe, PaymentMethod: "stripe", Status: common.TopUpStatusSuccess,
					Amount: fixture.face, Money: float64(fixture.face), CreateTime: int64(i + 1), CompleteTime: int64(i + 10)}).Error)
				if fixture.siteID >= 0 {
					require.NoError(t, db.Create(&model.AdtyTopupOrder{TradeNo: fixture.trade, UserId: user.Id, SiteId: fixture.siteID,
						Provider: model.PaymentProviderStripe, FaceUSD: decimal.NewFromInt(fixture.face), BonusUSD: decimal.RequireFromString("0.5")}).Error)
				}
			}
			require.NoError(t, db.Model(&user).Update("site_id", 8).Error)
			type order struct {
				TradeNo   string `json:"trade_no"`
				SiteID    *int   `json:"site_id"`
				SiteName  string `json:"site_name"`
				OrderType string `json:"order_type"`
			}
			var listed struct {
				Success bool
				Data    struct {
					Total int
					Items []order
				}
			}
			modelManagementRequest(t, GetRechargeAudit, http.MethodGet, "/orders", nil, &listed)
			require.True(t, listed.Success)
			require.Equal(t, 5, listed.Data.Total)
			byTrade := map[string]order{}
			for _, item := range listed.Data.Items {
				byTrade[item.TradeNo] = item
				assert.Equal(t, "topup", item.OrderType)
			}
			for trade, expected := range map[string]int{"main-order": 0, "north-original": 7, "south-current": 8, "historical-site": 99} {
				assert.Equal(t, &expected, byTrade[trade].SiteID, trade)
			}
			assert.Equal(t, common.SystemName, byTrade["main-order"].SiteName)
			assert.Equal(t, "North Studio", byTrade["north-original"].SiteName)
			assert.Equal(t, "South Studio", byTrade["south-current"].SiteName)
			assert.Empty(t, byTrade["historical-site"].SiteName)
			assert.Nil(t, byTrade["legacy-unknown"].SiteID, "legacy ownership must not be inferred from the buyer's current site")
			assert.Empty(t, byTrade["legacy-unknown"].SiteName)
			for _, fixture := range []struct{ path, trade string }{
				{"/orders?site_id=0", "main-order"}, {"/orders?site_id=7&order_type=topup&status=success&keyword=original", "north-original"},
				{"/orders?site_id=99", "historical-site"},
			} {
				modelManagementRequest(t, GetRechargeAudit, http.MethodGet, fixture.path, nil, &listed)
				require.True(t, listed.Success, fixture.path)
				require.Equal(t, 1, listed.Data.Total, fixture.path)
				require.Len(t, listed.Data.Items, 1)
				assert.Equal(t, fixture.trade, listed.Data.Items[0].TradeNo)
			}
			var summary struct {
				Success bool
				Data    struct {
					Orders      int
					CreditedUSD decimal.Decimal `json:"credited_usd"`
					BonusUSD    decimal.Decimal `json:"bonus_usd"`
					Sites       []struct {
						SiteID   int    `json:"site_id"`
						SiteName string `json:"site_name"`
					} `json:"site_options"`
				}
			}
			modelManagementRequest(t, GetRechargeAuditSummary, http.MethodGet, "/summary?site_id=7&order_type=topup&status=success&keyword=original", nil, &summary)
			require.True(t, summary.Success)
			assert.Equal(t, 1, summary.Data.Orders)
			assert.Equal(t, "3", summary.Data.CreditedUSD.String())
			assert.Equal(t, "0.5", summary.Data.BonusUSD.String())
			require.Len(t, summary.Data.Sites, 4, "site choices remain available when the list is filtered")
			assert.Equal(t, 0, summary.Data.Sites[0].SiteID)
			assert.Equal(t, common.SystemName, summary.Data.Sites[0].SiteName)
			assert.Equal(t, 7, summary.Data.Sites[1].SiteID)
			assert.Equal(t, "North Studio", summary.Data.Sites[1].SiteName)
			assert.Equal(t, 99, summary.Data.Sites[3].SiteID)
			for _, filter := range []string{"site_id=-1", "site_id=invalid", "site_id=1.2", "site_id=999999999999999999999", "order_type=team"} {
				for _, handler := range []gin.HandlerFunc{GetRechargeAudit, GetRechargeAuditSummary} {
					var invalid struct{ Success bool }
					modelManagementRequest(t, handler, http.MethodGet, "/orders?"+filter, nil, &invalid)
					assert.False(t, invalid.Success, filter)
				}
			}
		})
	}
}
