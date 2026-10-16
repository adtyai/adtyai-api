package model

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

// acceptOwnedUserTables 只归属用户的附属表（按 user_id 随 accept 用户一起删）。
// 口径与已删除的 v3 清理脚本（31ea8cc1 scripts/cleanup_accept_mismatch_users.sh）一致；表不存在时跳过。
var acceptOwnedUserTables = []string{
	"tokens", "user_sessions", "user_access_tokens", "checkins", "quota_data", "auth_flows",
	"two_fas", "two_fa_backup_codes", "passkey_credentials", "user_oauth_bindings",
	"external_identity_claims", "first_topup_claims", "aff_withdrawals", "user_bonus_wallets",
	// 以下两表 v3 脚本只校验不删（当时名单内为 0）；accept 用户会造这两类行，这里随用户删，删后仍校验为 0
	"risk_reviews", "balance_adjust_requests",
	// B-03 出资拆分表
	"adty_wallet_consume_hourly",
}

// CleanupAcceptUsersForTest 验收专用：在一个事务内清掉指定用户名前缀（必须以 accept- 开头）
// 造的用户及其流水/订单/退款/返佣/提现/日志等，保证 accept 脚本跑完不留对不上账的数据。
// orderPrefixes 额外按订单号前缀清 topup_orders / risk_reviews / first_topup_claims 及其退款/返佣（可空），只限名单内用户。
// 入口守卫：前缀校验 + RequireRecentAcceptBackup（近期可读备份）；删后事务内校验 37 列引用 + idempotency 悬空均为 0，否则回滚。
// 账本流水只增不改：事务内临时关 trg_ledger_entries_immutable，提交或回滚后触发器都恢复为开启。
// 注意：事务期间 ledger_entries 持排他锁（lock_timeout 5s），不能和联调写入或其他 accept 同时跑。
// 日志若配置了独立 LOG_DB，在主事务提交后单独删。
func CleanupAcceptUsersForTest(userPrefixes []string, orderPrefixes []string) (int, error) {
	if err := validateAcceptCleanupPrefixes(userPrefixes, orderPrefixes); err != nil {
		return 0, err
	}
	if err := RequireRecentAcceptBackup(); err != nil {
		return 0, err
	}
	likes := make([]string, 0, len(userPrefixes))
	for _, p := range userPrefixes {
		likes = append(likes, escapeLikeForTest(p)+"%")
	}
	orderLikes := make([]string, 0, len(orderPrefixes))
	for _, p := range orderPrefixes {
		orderLikes = append(orderLikes, escapeLikeForTest(p)+"%")
	}

	var uids []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec(`SET LOCAL lock_timeout = '5s'`).Error; err != nil {
				return err
			}
		}
		q := tx.Unscoped().Model(&User{})
		cond := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes))
		for _, l := range likes {
			cond = append(cond, "username LIKE ?")
			args = append(args, l)
		}
		if err := q.Where(strings.Join(cond, " OR "), args...).Pluck("id", &uids).Error; err != nil {
			return err
		}
		// 注意：这一步删的是所有「订单号匹配前缀、且 user_id 在 users 里已不存在」的行，
		// 不只限 accept 自造数据——真实用户被删后留下的同前缀订单/风控行也会被删；前缀须足够专属（见 validateAcceptCleanupPrefixes）。
		// 按订单号前缀删真孤儿行（user_id 在 users 里已不存在，如 topup_bonus 用伪用户 1999999991 造的 ORPHAN-A5- 风控）
		if err := deleteAcceptOrphanOrderRows(tx, orderLikes); err != nil {
			return err
		}
		if len(uids) == 0 {
			return checkAcceptOrphanOrderRows(tx, orderLikes)
		}
		// 按订单号前缀删，只删 user_id 在本次名单内的订单及其退款/返佣/首充/风控行
		for _, l := range orderLikes {
			sub := `SELECT order_no FROM topup_orders WHERE order_no LIKE ? AND user_id IN ?`
			for _, t := range []string{"topup_refunds", "aff_commissions"} {
				if err := execIfTable(tx, t, "DELETE FROM "+t+" WHERE order_no IN ("+sub+")", l, uids); err != nil {
					return err
				}
			}
			for _, t := range []string{"first_topup_claims", "risk_reviews", "topup_orders"} {
				if err := execIfTable(tx, t, "DELETE FROM "+t+" WHERE order_no LIKE ? AND user_id IN ?", l, uids); err != nil {
					return err
				}
			}
		}
		if err := tx.Exec(`DELETE FROM ledger_idempotency WHERE entry_id IN (SELECT id FROM ledger_entries WHERE user_id IN ?)`, uids).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&LedgerEntry{}).Where("user_id IN ?", uids).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			if err := DisableLedgerImmutableTrigger(tx); err != nil {
				return err
			}
			if err := tx.Where("user_id IN ?", uids).Delete(&LedgerEntry{}).Error; err != nil {
				return err
			}
			if err := EnableLedgerImmutableTrigger(tx); err != nil {
				return err
			}
		}
		if err := execIfTable(tx, "topup_refunds", `DELETE FROM topup_refunds WHERE order_no IN (SELECT order_no FROM topup_orders WHERE user_id IN ?)`, uids); err != nil {
			return err
		}
		if err := execIfTable(tx, "aff_commissions", `DELETE FROM aff_commissions WHERE inviter_id IN ? OR invitee_id IN ?`, uids, uids); err != nil {
			return err
		}
		if err := execIfTable(tx, "topup_orders", `DELETE FROM topup_orders WHERE user_id IN ?`, uids); err != nil {
			return err
		}
		for _, t := range acceptOwnedUserTables {
			if err := execIfTable(tx, t, "DELETE FROM "+t+" WHERE user_id IN ?", uids); err != nil {
				return err
			}
		}
		if err := execIfTable(tx, "redemptions", `UPDATE redemptions SET used_user_id = 0 WHERE used_user_id IN ?`, uids); err != nil {
			return err
		}
		if LOG_DB == nil || LOG_DB == DB {
			if err := execIfTable(tx, "logs", `DELETE FROM logs WHERE user_id IN ?`, uids); err != nil {
				return err
			}
		}
		if err := tx.Unscoped().Where("id IN ?", uids).Delete(&User{}).Error; err != nil {
			return err
		}
		if err := checkAcceptCleanupRefs(tx, uids); err != nil {
			return err
		}
		return checkAcceptOrphanOrderRows(tx, orderLikes)
	})
	if err != nil {
		return 0, err
	}
	if len(uids) > 0 && LOG_DB != nil && LOG_DB != DB {
		if err := LOG_DB.Where("user_id IN ?", uids).Delete(&Log{}).Error; err != nil {
			return len(uids), err
		}
	}
	return len(uids), nil
}

func execIfTable(tx *gorm.DB, table, sql string, args ...any) error {
	if !tx.Migrator().HasTable(table) {
		return nil
	}
	return tx.Exec(sql, args...).Error
}

func escapeLikeForTest(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// validateAcceptCleanupPrefixes 纯函数：拒绝过宽前缀，任何 DB 操作前调用。
// 用户名前缀必须形如 accept-<段>-，段非空且只含 [a-z0-9]（如 accept-rf-）；
// 订单号前缀必须长度 >= 4、以 - 结尾、不含 LIKE 通配符，且不能是 accept-/ACCEPT- 这类泛前缀。
func validateAcceptCleanupPrefixes(userPrefixes, orderPrefixes []string) error {
	if len(userPrefixes) == 0 {
		return fmt.Errorf("cleanup: no user prefix")
	}
	for _, p := range userPrefixes {
		if !acceptUserPrefixRe.MatchString(p) {
			return fmt.Errorf("cleanup: user prefix %q too broad, want accept-<segment>-", p)
		}
	}
	for _, p := range orderPrefixes {
		lp := strings.ToLower(p)
		if len(p) < 4 || !strings.HasSuffix(p, "-") || strings.ContainsAny(p, "%_ ") ||
			lp == "accept-" || lp == "order-" || lp == "test-" || strings.Trim(p, "-") == "" {
			return fmt.Errorf("cleanup: order prefix %q too broad", p)
		}
	}
	return nil
}

var acceptUserPrefixRe = regexp.MustCompile(`^accept-[a-z0-9]+-$`)

// acceptCleanupRefCols 与 v3 清理脚本（31ea8cc1，已删）的 REFCOLS 一致（37 列，加 idempotency 悬空共 38 项）。
var acceptCleanupRefCols = []string{
	"aff_commissions.invitee_id", "aff_commissions.inviter_id", "aff_commissions.voided_by",
	"aff_withdrawals.operator_id", "aff_withdrawals.user_id",
	"audit_logs.user_id", "auth_flows.user_id", "balance_adjust_requests.user_id", "checkins.user_id",
	"enterprise_price_rules.created_by", "enterprise_price_rules.user_id",
	"external_identity_claims.user_id", "first_topup_claims.user_id",
	"ledger_entries.operator_id", "ledger_entries.user_id", "logs.user_id", "midjourneys.user_id",
	"passkey_credentials.user_id", "quota_data.user_id", "redemptions.used_user_id", "redemptions.user_id",
	"risk_reviews.user_id", "subscription_orders.user_id", "subscription_pre_consume_records.user_id",
	"tasks.user_id", "tokens.user_id", "top_ups.user_id", "topup_orders.user_id", "topup_refunds.operator_id",
	"two_fa_backup_codes.user_id", "two_fas.user_id", "user_access_tokens.user_id", "user_bonus_wallets.user_id",
	"user_oauth_bindings.user_id", "user_sessions.user_id", "user_subscriptions.user_id", "users.inviter_id",
}

// checkAcceptCleanupRefs 删后事务内校验：引用已删用户的行 = 0、ledger_idempotency 悬空 = 0，否则报错回滚。
func checkAcceptCleanupRefs(tx *gorm.DB, uids []int) error {
	bad := make([]string, 0)
	for _, tc := range acceptCleanupRefCols {
		parts := strings.SplitN(tc, ".", 2)
		t, c := parts[0], parts[1]
		if !tx.Migrator().HasTable(t) || !tx.Migrator().HasColumn(t, c) {
			continue
		}
		var n int64
		if err := tx.Raw("SELECT count(*) FROM "+t+" WHERE "+c+" IN ?", uids).Scan(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			bad = append(bad, fmt.Sprintf("%s=%d", tc, n))
		}
	}
	var dangling int64
	if err := tx.Raw(`SELECT count(*) FROM ledger_idempotency i WHERE i.entry_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM ledger_entries e WHERE e.id = i.entry_id)`).Scan(&dangling).Error; err != nil {
		return err
	}
	if dangling > 0 {
		bad = append(bad, fmt.Sprintf("ledger_idempotency.dangling=%d", dangling))
	}
	if len(bad) > 0 {
		return fmt.Errorf("cleanup: refs remain after delete, rolled back: %s", strings.Join(bad, ", "))
	}
	return nil
}

var acceptOrderOwnedTables = []string{"first_topup_claims", "risk_reviews", "topup_orders"}

func deleteAcceptOrphanOrderRows(tx *gorm.DB, orderLikes []string) error {
	for _, l := range orderLikes {
		sub := `SELECT order_no FROM topup_orders WHERE order_no LIKE ? AND user_id NOT IN (SELECT id FROM users)`
		for _, t := range []string{"topup_refunds", "aff_commissions"} {
			if err := execIfTable(tx, t, "DELETE FROM "+t+" WHERE order_no IN ("+sub+")", l); err != nil {
				return err
			}
		}
		for _, t := range acceptOrderOwnedTables {
			if err := execIfTable(tx, t, "DELETE FROM "+t+" WHERE order_no LIKE ? AND user_id NOT IN (SELECT id FROM users)", l); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkAcceptOrphanOrderRows 删后断言：匹配订单前缀且 user_id 不存在的行为 0（ORPHAN-A5- 全是这种，即剩余为 0），否则回滚。
func checkAcceptOrphanOrderRows(tx *gorm.DB, orderLikes []string) error {
	bad := make([]string, 0)
	for _, l := range orderLikes {
		for _, t := range acceptOrderOwnedTables {
			if !tx.Migrator().HasTable(t) {
				continue
			}
			var n int64
			if err := tx.Raw("SELECT count(*) FROM "+t+" WHERE order_no LIKE ? AND user_id NOT IN (SELECT id FROM users)", l).Scan(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				bad = append(bad, fmt.Sprintf("%s[%s]=%d", t, l, n))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("cleanup: orphan order rows remain, rolled back: %s", strings.Join(bad, ", "))
	}
	return nil
}
