//go:build ignore

// 兑换码已下线：Redeem 必须拒绝，且不能改余额、状态或流水。
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
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-redeem-"}
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
	model.InitOptionMap()
	fmt.Println("InitDB+options OK")

	u := model.User{
		Username: "accept-redeem-" + common.GetRandomString(6), Password: "unused",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		AuthVersion: 1, Quota: 1234, AffCode: "rd" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	paid0, _ := model.GetUserPaidQuota(u.Id)
	bonus0, _ := model.GetUserBonusBalance(u.Id)

	const q = 5000
	r := model.Redemption{
		Name: "accept-redeem", Key: common.GetRandomString(32), Status: common.RedemptionCodeStatusEnabled,
		Quota: q, CreatedTime: common.GetTimestamp(),
	}
	must(r.Insert())
	defer model.DB.Delete(&model.Redemption{}, r.Id)

	got, err := model.Redeem(r.Key, u.Id)
	if !errors.Is(err, model.ErrRedemptionClosed) || got != 0 {
		panic(fmt.Sprintf("redeem want closed got %d %v", got, err))
	}
	paid1, _ := model.GetUserPaidQuota(u.Id)
	bonus1, _ := model.GetUserBonusBalance(u.Id)
	if paid1 != paid0 || bonus1 != bonus0 {
		panic(fmt.Sprintf("redeem changed paid %d→%d bonus %d→%d", paid0, paid1, bonus0, bonus1))
	}
	var rr model.Redemption
	must(model.DB.First(&rr, r.Id).Error)
	if rr.Status != common.RedemptionCodeStatusEnabled || rr.UsedUserId != 0 {
		panic(fmt.Sprintf("status=%d used_user=%d", rr.Status, rr.UsedUserId))
	}
	var n int64
	must(model.DB.Model(&model.LedgerEntry{}).Where("user_id = ? AND kind = ?", u.Id, "redemption_bonus").Count(&n).Error)
	if n != 0 {
		panic(fmt.Sprintf("redemption ledger rows=%d", n))
	}
	fmt.Println("ACCEPT redeem closed PASS")
}
