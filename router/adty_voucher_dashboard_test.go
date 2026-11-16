package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dashboardPage struct {
	Total         int64            `json:"total"`
	Items         []map[string]any `json:"items"`
	MarginAllowed bool             `json:"margin_allowed"`
}

func decodeDashboardPage(t *testing.T, body []byte) dashboardPage {
	t.Helper()
	var response struct {
		Success bool          `json:"success"`
		Data    dashboardPage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(body, &response), string(body))
	require.True(t, response.Success, string(body))
	return response.Data
}

func TestRootManagesVouchersFromTheDashboard(t *testing.T) {
	f := newOwnerFixture(t)
	root := adtySessionToken(t, f.db, "voucher-root", common.RoleRootUser)

	limit := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers/limit", "", root)
	require.Contains(t, limit.Body.String(), `"limit_usd":"0"`, limit.Body.String())
	require.Contains(t, adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/vouchers/limit", `{"limit_usd":"100"}`, root).Body.String(), `"success":true`)
	issued := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/vouchers", `{"purpose":"main_user","amount_usd":"12.5","reason":"promo"}`, root)
	require.Contains(t, issued.Body.String(), `"success":true`, issued.Body.String())
	limit = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers/limit", "", root)
	assert.Contains(t, limit.Body.String(), `"limit_usd":"100"`)
	assert.Contains(t, limit.Body.String(), `"frozen_usd":"12.5"`)

	listed := adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers?status=pending&purpose=main_user", "", root)
	assert.NotContains(t, listed.Body.String(), "code_hash", "the code hash must never leave the server")
	page := decodeDashboardPage(t, listed.Body.Bytes())
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, model.AdtyVoucherPurposeMainUser, page.Items[0]["purpose"])
	assert.NotEmpty(t, page.Items[0]["code_tail"])
	id := int(page.Items[0]["id"].(float64))

	cancelled := adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/adty/admin/vouchers/%d/cancel", id), "", root)
	require.Contains(t, cancelled.Body.String(), `"success":true`, cancelled.Body.String())
	limit = adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers/limit", "", root)
	assert.Contains(t, limit.Body.String(), `"frozen_usd":"0"`, "cancelling releases the issuance cap")
	page = decodeDashboardPage(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers?status=cancelled", "", root).Body.Bytes())
	assert.EqualValues(t, 1, page.Total)
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/vouchers?status=lost", "", root).Body.String(), `"success":true`)

	marginPath := fmt.Sprintf("/api/adty/admin/vouchers/sites/%d/margin", f.site.Id)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, marginPath, "", root).Body.String(), `"allowed":false`)
	require.Contains(t, adtyJSON(t, f.engine, http.MethodPut, marginPath, `{"allowed":true}`, root).Body.String(), `"success":true`)
	assert.Contains(t, adtyJSON(t, f.engine, http.MethodGet, marginPath, "", root).Body.String(), `"allowed":true`)

	for _, path := range []string{"/api/adty/admin/vouchers", "/api/adty/admin/vouchers/limit", marginPath} {
		assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, path, "", f.token).Body.String(), `"success":true`, path)
	}
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodPost, fmt.Sprintf("/api/adty/admin/vouchers/%d/cancel", id), "", f.token).Body.String(), `"success":true`)
}

func TestOwnerVoucherListShowsOnlyCodesTheOwnerIssued(t *testing.T) {
	f := newOwnerFixture(t)
	root := adtySessionToken(t, f.db, "voucher-list-root", common.RoleRootUser)
	require.NoError(t, service.EnsureAdtySiteWallets(f.site.Id))
	_, err := service.ApplyAdtySiteWalletChange(f.site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(30), service.AdtySiteWalletChange{
		Type: "seed", Reason: "seed earnings", IdempotencyKey: "owner-voucher-list-seed",
	})
	require.NoError(t, err)
	issued := f.call(t, http.MethodPost, "/api/adty/site/vouchers", `{"purpose":"site_customer","amount_usd":"4","reason":"customer"}`,
		sessionProof(t, f.owner, service.AdtySiteOwnerActionVoucherIssue, f.site.Id))
	require.Contains(t, issued.Body.String(), `"success":true`, issued.Body.String())
	// A margin code Root issues to this site is funded by the platform, not by the owner.
	require.Contains(t, adtyJSON(t, f.engine, http.MethodPut, "/api/adty/admin/vouchers/limit", `{"limit_usd":"50"}`, root).Body.String(), `"success":true`)
	margin := adtyJSON(t, f.engine, http.MethodPost, "/api/adty/admin/vouchers", fmt.Sprintf(`{"purpose":"site_margin","site_id":%d,"amount_usd":"5","reason":"margin"}`, f.site.Id), root)
	require.Contains(t, margin.Body.String(), `"success":true`, margin.Body.String())

	page := decodeDashboardPage(t, f.call(t, http.MethodGet, "/api/adty/site/vouchers", "", "").Body.Bytes())
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, model.AdtyVoucherPurposeSiteCustomer, page.Items[0]["purpose"])
	assert.False(t, page.MarginAllowed)

	require.Contains(t, adtyJSON(t, f.engine, http.MethodPut, fmt.Sprintf("/api/adty/admin/vouchers/sites/%d/margin", f.site.Id), `{"allowed":true}`, root).Body.String(), `"success":true`)
	page = decodeDashboardPage(t, f.call(t, http.MethodGet, "/api/adty/site/vouchers?status=pending", "", "").Body.Bytes())
	assert.True(t, page.MarginAllowed)
	assert.EqualValues(t, 1, page.Total)
}

func TestRootListsSiteWithdrawalsAcrossSites(t *testing.T) {
	f := newOwnerFixture(t)
	root := adtySessionToken(t, f.db, "withdraw-list-root", common.RoleRootUser)
	rows := []model.AdtySiteWithdrawal{
		{SiteId: f.site.Id, OwnerUserId: f.owner.Id, AmountUSD: decimal.NewFromInt(20), FeeUSD: decimal.NewFromInt(1), NetUSD: decimal.NewFromInt(19),
			Method: "usdt", Network: "TRC20", AccountNo: "T-address", Rate: "1", PayoutAmount: "19", PayoutCurrency: "USDT",
			Status: model.AdtySiteWithdrawPending, IdempotencyKey: "withdraw-list-1"},
		{SiteId: f.other.Id, OwnerUserId: f.other.OwnerUserId, AmountUSD: decimal.NewFromInt(8), FeeUSD: decimal.Zero, NetUSD: decimal.NewFromInt(8),
			Method: "alipay", AccountNo: "a@example.com", AccountName: "Rival", Rate: "7", PayoutAmount: "56", PayoutCurrency: "CNY",
			Status: model.AdtySiteWithdrawPaid, IdempotencyKey: "withdraw-list-2"},
	}
	require.NoError(t, f.db.Create(&rows).Error)

	page := decodeDashboardPage(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/site-withdrawals", "", root).Body.Bytes())
	require.EqualValues(t, 2, page.Total)
	assert.Equal(t, "rival", page.Items[0]["subdomain"], "newest first, with the site subdomain")
	assert.Equal(t, "center", page.Items[1]["subdomain"])

	page = decodeDashboardPage(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/site-withdrawals?status=pending", "", root).Body.Bytes())
	require.EqualValues(t, 1, page.Total)
	assert.Equal(t, "T-address", page.Items[0]["account_no"])
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/site-withdrawals?status=unknown", "", root).Body.String(), `"success":true`)
	assert.NotContains(t, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/admin/site-withdrawals", "", f.token).Body.String(), `"success":true`)
}
