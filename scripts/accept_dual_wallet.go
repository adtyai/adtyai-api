//go:build ignore

package main

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true // accept 脚本需要跑 migrateDB
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-dual-", "accept-settle-"}
	acceptOrderPrefixes := []string{}
	if n, err := model.CleanupAcceptUsersForTest(acceptUserPrefixes, acceptOrderPrefixes); err != nil {
		panic(fmt.Sprintf("pre-clean accept data: %v", err))
	} else {
		fmt.Printf("pre-clean accept data users=%d\n", n)
	}
	defer func() {
		if n, err := model.CleanupAcceptUsersForTest(acceptUserPrefixes, acceptOrderPrefixes); err != nil {
			fmt.Printf("post-clean accept data failed: %v\n", err)
		} else {
			fmt.Printf("post-clean accept data users=%d\n", n)
		}
	}()
	fmt.Println("InitDB+InitLogDB OK (migrated user_bonus_wallets / ledger)")

	// (a) table exists
	if !model.DB.Migrator().HasTable(&model.UserBonusWallet{}) {
		panic("user_bonus_wallets missing")
	}
	fmt.Println("(a) user_bonus_wallets exists")

	user := model.User{
		Username:    "accept-dual-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       10, // paid
		AffCode:     "af" + common.GetRandomString(6),
	}
	must(model.DB.Create(&user).Error)
	fmt.Printf("seed user id=%d paid=10\n", user.Id)

	must(model.IncreaseUserBonusQuota(user.Id, 5, "accept_seed", "accept bonus", "accept", "a1"))
	bonus, err := model.GetUserBonusBalance(user.Id)
	must(err)
	if bonus != 5 {
		panic(fmt.Sprintf("bonus want 5 got %d", bonus))
	}

	// (b) deduct bonus first: spend 7 → bonus 0, paid 8
	b, p, ok, err := model.TryReserveUserWallet(user.Id, 7)
	must(err)
	if !ok || b != 5 || p != 2 {
		panic(fmt.Sprintf("reserve7 ok=%v bonus=%d paid=%d", ok, b, p))
	}
	bonus, _ = model.GetUserBonusBalance(user.Id)
	paid, _ := model.GetUserPaidQuota(user.Id)
	if bonus != 0 || paid != 8 {
		panic(fmt.Sprintf("after spend7 bonus=%d paid=%d want 0/8", bonus, paid))
	}
	fmt.Println("(b) spend 7 with bonus5+paid10 → bonus0 paid8 OK")

	// restore for (c)
	must(model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error)
	must(model.DB.Model(&model.UserBonusWallet{}).Where("user_id = ?", user.Id).Update("balance", 100).Error)

	total, err := model.GetUserQuota(user.Id, true)
	must(err)
	if total != 100 {
		panic(fmt.Sprintf("total want 100 got %d", total))
	}
	ok, err = model.TryReserveUserQuota(user.Id, 40)
	must(err)
	if !ok {
		panic("(c) FAIL: bonus-only enough should reserve (no 403 path)")
	}
	bonus, _ = model.GetUserBonusBalance(user.Id)
	paid, _ = model.GetUserPaidQuota(user.Id)
	if bonus != 60 || paid != 0 {
		panic(fmt.Sprintf("after bonus-only spend bonus=%d paid=%d want 60/0", bonus, paid))
	}
	fmt.Println("(c) paid=0 bonus=100 spend 40 → OK (would not 403)")

	ok, err = model.TryReserveUserQuota(user.Id, 61)
	must(err)
	if ok {
		panic("insufficient should fail")
	}
	fmt.Println("(c2) insufficient → reserve false (maps to 403) OK")

	// (d) 结算补扣：实充1赠送0补扣5 → 失败且不为负
	user2 := model.User{
		Username:    "accept-settle-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       1, // paid only
		AffCode:     "af" + common.GetRandomString(6),
	}
	must(model.DB.Create(&user2).Error)
	must(model.EnsureUserBonusWallet(model.DB, user2.Id))
	b2, p2, err := model.DecreaseUserWallet(user2.Id, 5)
	if err == nil {
		panic(fmt.Sprintf("(d) FAIL: expected error, got bonus=%d paid=%d", b2, p2))
	}
	if !errors.Is(err, model.ErrInsufficientWalletBalance) {
		panic(fmt.Sprintf("(d) FAIL: want ErrInsufficientWalletBalance got %v", err))
	}
	paid2, _ := model.GetUserPaidQuota(user2.Id)
	bonus2, _ := model.GetUserBonusBalance(user2.Id)
	if paid2 != 1 || bonus2 != 0 {
		panic(fmt.Sprintf("(d) FAIL: after failed settle paid=%d bonus=%d want 1/0 (must not go negative)", paid2, bonus2))
	}
	fmt.Println("(d) paid=1 bonus=0 settle deduct 5 → fail, paid still 1 (not negative) OK")


	var ledgerCount int64
	must(model.DB.Model(&model.LedgerEntry{}).Where("user_id = ?", user.Id).Count(&ledgerCount).Error)
	fmt.Printf("ledger_entries for seed credits: %d\n", ledgerCount)
	fmt.Println("ACCEPT_DUAL_WALLET_PASS")
}
