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
// 仅当前登录用户；默认返回 paid+bonus（含小时汇总行），每行带 account 供前端画两列。
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
	accounts := []string{"paid", "bonus"}
	if acct != "" {
		accounts = []string{acct}
	}

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
