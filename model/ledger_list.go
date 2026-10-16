package model

import (
	"strings"
	"time"
)

// LedgerListFilter 流水列表筛选（用户自查与管理端共用）。
type LedgerListFilter struct {
	UserId         int // >0 时限定用户
	Accounts       []string // 空=不限；self 默认 paid+bonus
	Kind           string
	CreatedFrom    *time.Time // inclusive
	CreatedTo      *time.Time // inclusive
	Limit          int
	Offset         int
	OnlyHourly     bool // true 时仅 hour_bucket IS NOT NULL
}

// ListLedgerEntries 只读分页查询 ledger_entries；按 created_at DESC, id DESC。
// 无 TTL/过期过滤（赠送不过期，历史全可查）。
func ListLedgerEntries(f LedgerListFilter) ([]LedgerEntry, int64, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	q := DB.Model(&LedgerEntry{})
	if f.UserId > 0 {
		q = q.Where("user_id = ?", f.UserId)
	}
	if len(f.Accounts) > 0 {
		q = q.Where("account IN ?", f.Accounts)
	}
	if k := strings.TrimSpace(f.Kind); k != "" {
		q = q.Where("kind = ?", k)
	}
	if f.CreatedFrom != nil {
		q = q.Where("created_at >= ?", f.CreatedFrom.UTC())
	}
	if f.CreatedTo != nil {
		q = q.Where("created_at <= ?", f.CreatedTo.UTC())
	}
	if f.OnlyHourly {
		q = q.Where("hour_bucket IS NOT NULL")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []LedgerEntry
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if rows == nil {
		rows = []LedgerEntry{}
	}
	return rows, total, nil
}

// NormalizeLedgerAccountFilter 将 query account 规范为 paid|bonus；空串表示未指定。
func NormalizeLedgerAccountFilter(account string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(account)) {
	case "":
		return "", true
	case "paid", "quota":
		return "paid", true
	case "bonus":
		return "bonus", true
	default:
		return "", false
	}
}
