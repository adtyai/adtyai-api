package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// parseLedgerTimeQuery 解析 created_from / created_to：优先 unix 秒，其次 RFC3339。
func parseLedgerTimeQuery(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if sec, err := strconv.ParseInt(raw, 10, 64); err == nil {
		t := time.Unix(sec, 0).UTC()
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}

// GetWalletLedgerSelf GET /api/wallet/ledger/self
// Query: account(paid|bonus|quota)、kind、created_from、created_to、limit、offset
// 仅当前登录用户。默认返回 paid+bonus。邀请退款作废/扣回记在 aff 账户，
// 选这两种类型时改查 aff，避免筛选项永远为空。
func GetWalletLedgerSelf(c *gin.Context) {
	uid := c.GetInt("id")
	if uid <= 0 {
		adtyFail(c, http.StatusUnauthorized, "unauthorized", "unauthorized", nil)
		return
	}

	accountRaw := c.Query("account")
	acct, ok := model.NormalizeLedgerAccountFilter(accountRaw)
	if !ok {
		adtyFail(c, http.StatusBadRequest, "invalid_account", "account must be paid or bonus", map[string]any{
			"account": accountRaw,
		})
		return
	}
	kind := strings.TrimSpace(c.Query("kind"))
	accounts := ledgerSelfAccounts(acct, kind)

	from, err := parseLedgerTimeQuery(c.Query("created_from"))
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_created_from", "invalid created_from", nil)
		return
	}
	to, err := parseLedgerTimeQuery(c.Query("created_to"))
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_created_to", "invalid created_to", nil)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	rows, total, err := model.ListLedgerEntries(model.LedgerListFilter{
		UserId:      uid,
		Accounts:    accounts,
		Kind:        kind,
		CreatedFrom: from,
		CreatedTo:   to,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"total":   total,
	})
}

// ledgerSelfAccounts keeps the default wallet view on paid+bonus.
// Invite-refund void and claw are written on the affiliate account, so those
// filters must look there or the named option always returns nothing.
// aff_refund_claw_paid is a paid-account row and stays in the default view.
func ledgerSelfAccounts(accountFilter, kind string) []string {
	switch kind {
	case "aff_refund_void", "aff_refund_claw":
		return []string{"aff"}
	default:
		if accountFilter != "" {
			return []string{accountFilter}
		}
		return []string{"paid", "bonus"}
	}
}

// AdminListBonusLedger GET /api/admin/wallet/bonus-ledger
// Query: user_id、kind、created_from、created_to、limit、offset
// 只查 account=bonus；无过期过滤。
func AdminListBonusLedger(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	from, err := parseLedgerTimeQuery(c.Query("created_from"))
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_created_from", "invalid created_from", nil)
		return
	}
	to, err := parseLedgerTimeQuery(c.Query("created_to"))
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_created_to", "invalid created_to", nil)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	rows, total, err := model.ListLedgerEntries(model.LedgerListFilter{
		UserId:      userId,
		Accounts:    []string{"bonus"},
		Kind:        strings.TrimSpace(c.Query("kind")),
		CreatedFrom: from,
		CreatedTo:   to,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"total":   total,
	})
}
