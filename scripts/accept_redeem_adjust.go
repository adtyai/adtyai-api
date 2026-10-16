//go:build ignore

// A27/A4 验收：兑换码入赠送钱包，经 AdjustQuota + ledger_entries（kind=redemption_bonus），不动实充。
package main

import (
	"fmt"
	"strconv"

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

	// (1) 首次兑换成功 → bonus +q，paid 不变
	got, err := model.Redeem(r.Key, u.Id)
	must(err)
	if got != q {
		panic(fmt.Sprintf("(1) redeem returned %d want %d", got, q))
	}
	paid1, _ := model.GetUserPaidQuota(u.Id)
	bonus1, _ := model.GetUserBonusBalance(u.Id)
	if paid1 != paid0 || int64(bonus1)-int64(bonus0) != q {
		panic(fmt.Sprintf("(1) paid %d→%d bonus %d→%d", paid0, paid1, bonus0, bonus1))
	}
	fmt.Println("(1) redeem credits bonus, paid unchanged OK")

	// (2) ledger kind redemption_bonus
	var led []model.LedgerEntry
	must(model.DB.Where("user_id = ? AND kind = ?", u.Id, "redemption_bonus").Find(&led).Error)
	if len(led) != 1 || led[0].Account != "bonus" || led[0].Delta != q || led[0].RefId != strconv.Itoa(r.Id) {
		panic(fmt.Sprintf("(2) ledger unexpected: %+v", led))
	}
	fmt.Println("(2) ledger account=bonus kind=redemption_bonus ref_id=id OK")

	// (3) status used
	var rr model.Redemption
	must(model.DB.First(&rr, r.Id).Error)
	if rr.Status != common.RedemptionCodeStatusUsed || rr.UsedUserId != u.Id {
		panic(fmt.Sprintf("(3) status=%d used_user=%d", rr.Status, rr.UsedUserId))
	}
	fmt.Println("(3) redemption status=used OK")

	// (4) 同码再兑 → ErrRedeemFailed，余额不变
	if _, err := model.Redeem(r.Key, u.Id); err != model.ErrRedeemFailed {
		panic(fmt.Sprintf("(4) second redeem want ErrRedeemFailed got %v", err))
	}
	paid2, _ := model.GetUserPaidQuota(u.Id)
	bonus2, _ := model.GetUserBonusBalance(u.Id)
	var n int64
	model.DB.Model(&model.LedgerEntry{}).Where("user_id = ? AND kind = ?", u.Id, "redemption_bonus").Count(&n)
	if paid2 != paid1 || bonus2 != bonus1 || n != 1 {
		panic("(4) second redeem changed balance/ledger")
	}
	fmt.Println("(4) second redeem fails, balances unchanged OK")
	fmt.Println("ACCEPT redeem_adjust PASS")
}
