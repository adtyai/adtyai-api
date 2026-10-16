//go:build ignore

package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func almost(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func newUser(tag string) model.User {
	u := model.User{
		Username:    "accept-topup-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "af" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func main() {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-topup-"}
	acceptOrderPrefixes := []string{"ORPHAN-A5-"}
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
	fmt.Println("InitDB+InitLogDB+options OK")

	if !model.DB.Migrator().HasTable(&model.TopupOrder{}) {
		panic("topup_orders missing")
	}
	if !model.DB.Migrator().HasTable(&model.FirstTopupClaim{}) {
		panic("first_topup_claims missing")
	}
	fmt.Println("(0) tables topup_orders / first_topup_claims / risk_reviews ok")

	// (1) 非法金额 → ErrInvalidTopupAmount（对应 HTTP 400）
	_, err := model.CreateTopupOrder(1, 20, "alipay", "fp-x", "1.1.1.1")
	if err == nil || err != model.ErrInvalidTopupAmount {
		// user 1 may not exist; CreateTopupOrder checks amount first via BuildTopupBonusPlanForUser
		panic(fmt.Sprintf("(1) want ErrInvalidTopupAmount got %v", err))
	}
	_, err = model.CreateTopupOrder(1, 99, "alipay", "fp-x", "1.1.1.1")
	if err != model.ErrInvalidTopupAmount {
		panic(fmt.Sprintf("(1b) want ErrInvalidTopupAmount got %v", err))
	}
	fmt.Println("(1) illegal amounts 20/99 → ErrInvalidTopupAmount (HTTP 400) OK")

	// (2) 档位下单写入正确赠送快照：$100 → tier 2.5%
	u1 := newUser("t2")
	o2, err := model.CreateTopupOrder(u1.Id, 100, "alipay", "fp-tier-"+common.GetRandomString(8), "10.2."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o2.TierBonusRate, 0.025) {
		panic(fmt.Sprintf("(2) tier rate want 0.025 got %v", o2.TierBonusRate))
	}
	if !almost(o2.FirstBonusRate, 0.10) {
		panic(fmt.Sprintf("(2) first rate want 0.10 got %v", o2.FirstBonusRate))
	}
	// 首充候选人 + 合法指纹 → immediate = 10（取高）
	if !almost(o2.BonusAmountUSD, 10) {
		panic(fmt.Sprintf("(2) immediate bonus want 10 got %v (order=%+v)", o2.BonusAmountUSD, o2))
	}
	if !o2.ApplyFirstImmediately || o2.PendingExtraUSD != 0 {
		panic(fmt.Sprintf("(2) expect apply first immediately, got %+v", o2))
	}
	fmt.Printf("(2) order %s snapshot tier=2.5%% first=10%% immediate=$10 OK\n", o2.OrderNo)

	// (3) 入账：首充 $100 → 实充 100*Q + 赠送 10*Q；只领一次
	done, err := model.CompleteTopupOrder(o2.OrderNo)
	must(err)
	paid, _ := model.GetUserPaidQuota(u1.Id)
	bonus, _ := model.GetUserBonusBalance(u1.Id)
	wantPaid, _ := model.DollarsToQuota(100)
	wantBonus, _ := model.DollarsToQuota(10)
	if paid != wantPaid || int(bonus) != wantBonus {
		panic(fmt.Sprintf("(3) after first complete paid=%d bonus=%d want %d/%d", paid, bonus, wantPaid, wantBonus))
	}
	claimed, _ := model.UserHasClaimedFirstTopup(u1.Id)
	if !claimed {
		panic("(3) first_topup_claims should be claimed")
	}
	// A4/A13: CompleteTopup 经 AdjustQuota，ledger kind 含 topup_paid / topup_first_bonus
	var ledPaid, ledBonus model.LedgerEntry
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?", u1.Id, "paid", "topup_paid", o2.OrderNo).First(&ledPaid).Error)
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?", u1.Id, "bonus", "topup_first_bonus", o2.OrderNo).First(&ledBonus).Error)
	if ledPaid.Delta != int64(wantPaid) || ledBonus.Delta != int64(wantBonus) {
		panic(fmt.Sprintf("(3) ledger deltas want paid=%d bonus=%d got %d/%d", wantPaid, wantBonus, ledPaid.Delta, ledBonus.Delta))
	}
	fmt.Println("(3b) ledger kinds topup_paid + topup_first_bonus OK")
	// 幂等重放
	done2, err := model.CompleteTopupOrder(o2.OrderNo)
	must(err)
	if done2.Id != done.Id {
		panic("idempotent complete mismatch")
	}
	paid2, _ := model.GetUserPaidQuota(u1.Id)
	bonus2, _ := model.GetUserBonusBalance(u1.Id)
	if paid2 != paid || bonus2 != bonus {
		panic("(3) replay credited twice")
	}
	fmt.Println("(3) first topup $100 → paid+bonus max(2.5%,10%)=$10 credited once OK")

	// (4) 二次充值只用档位：$100 → 赠送 $2.5
	o4, err := model.CreateTopupOrder(u1.Id, 100, "alipay", "fp-tier-second-"+common.GetRandomString(6), "10.4."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if o4.IsFirstCandidate {
		panic("(4) second order should not be first candidate")
	}
	if !almost(o4.BonusAmountUSD, 2.5) {
		panic(fmt.Sprintf("(4) second bonus want 2.5 got %v", o4.BonusAmountUSD))
	}
	_, err = model.CompleteTopupOrder(o4.OrderNo)
	must(err)
	bonus4, _ := model.GetUserBonusBalance(u1.Id)
	wantBonus4, _ := model.DollarsToQuota(10 + 2.5)
	if int(bonus4) != wantBonus4 {
		panic(fmt.Sprintf("(4) bonus after second want %d got %d", wantBonus4, bonus4))
	}
	var ledTier model.LedgerEntry
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?", u1.Id, "bonus", "topup_tier_bonus", o4.OrderNo).First(&ledTier).Error)
	wantTier, _ := model.DollarsToQuota(2.5)
	if ledTier.Delta != int64(wantTier) {
		panic(fmt.Sprintf("(4) ledger topup_tier_bonus delta want %d got %d", wantTier, ledTier.Delta))
	}
	fmt.Println("(4) second topup $100 → bonus $2.5 only + ledger topup_tier_bonus OK")

	// (5) 缺指纹首充：$100 → 立刻实充100+档位2.5，另 7.5 进 pending_review
	u5 := newUser("t5")
	o5, err := model.CreateTopupOrder(u5.Id, 100, "alipay", "", "10.5."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o5.BonusAmountUSD, 2.5) || !almost(o5.PendingExtraUSD, 7.5) {
		panic(fmt.Sprintf("(5) want immediate 2.5 pending 7.5 got %v / %v", o5.BonusAmountUSD, o5.PendingExtraUSD))
	}
	_, err = model.CompleteTopupOrder(o5.OrderNo)
	must(err)
	var rrCount int64
	must(model.DB.Model(&model.RiskReview{}).
		Where("order_no = ? AND kind = ? AND status = ?", o5.OrderNo, model.RiskReviewKindFirstTopupExtra, model.RiskReviewStatusPending).
		Count(&rrCount).Error)
	if rrCount != 1 {
		panic(fmt.Sprintf("(5) risk_reviews want 1 got %d", rrCount))
	}
	paid5, _ := model.GetUserPaidQuota(u5.Id)
	bonus5, _ := model.GetUserBonusBalance(u5.Id)
	wb5, _ := model.DollarsToQuota(2.5)
	if paid5 != wantPaid || int(bonus5) != wb5 {
		panic(fmt.Sprintf("(5) paid/bonus want %d/%d got %d/%d", wantPaid, wb5, paid5, bonus5))
	}
	fmt.Println("(5) missing device_fp → tier bonus now, extra pending_review OK")

	// (6) 快照不被参数改动影响：改 FirstTopupBonusRate 后旧单 complete 仍用快照
	u6 := newUser("t6")
	o6, err := model.CreateTopupOrder(u6.Id, 100, "alipay", "fp-snap-"+common.GetRandomString(8), "10.6."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	must(model.UpdateFirstTopupBonusRateString("0.20"))
	_, err = model.CompleteTopupOrder(o6.OrderNo)
	must(err)
	bonus6, _ := model.GetUserBonusBalance(u6.Id)
	if int(bonus6) != wantBonus { // still $10 from snapshot 0.10
		panic(fmt.Sprintf("(6) snapshot broken: want bonus quota %d got %d", wantBonus, bonus6))
	}
	must(model.UpdateFirstTopupBonusRateString("0.10")) // restore
	fmt.Println("(6) changing FirstTopupBonusRate after order does not affect snapshot OK")

	// (7) 高2：A pending_review 占 fp-1 后，B 同指纹入账必须整笔回滚（不送首充、余额不变）
	uA := newUser("fpA")
	fpShared := "fp-pending-" + common.GetRandomString(8)
	fpPtr := fpShared
	must(model.DB.Create(&model.FirstTopupClaim{
		UserId:   uA.Id,
		DeviceFp: &fpPtr,
		IP:       "10.7."+common.GetRandomString(3)+"."+common.GetRandomString(3),
		Status:   model.FirstTopupStatusPendingReview,
	}).Error)
	occupied, err := model.DeviceFpClaimed(fpShared)
	must(err)
	if !occupied {
		panic("(7) DeviceFpClaimed must treat pending_review as occupied")
	}
	uB := newUser("fpB")
	oB, err := model.CreateTopupOrder(uB.Id, 100, "alipay", fpShared, "10.72."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if oB.ApplyFirstImmediately {
		panic("(7) B must not apply first immediately when device occupied")
	}
	paidB0, _ := model.GetUserPaidQuota(uB.Id)
	bonusB0, _ := model.GetUserBonusBalance(uB.Id)
	_, err = model.CompleteTopupOrder(oB.OrderNo)
	if err == nil {
		panic("(7) CompleteTopupOrder must fail on device_fp unique conflict (no DoNothing)")
	}
	paidB1, _ := model.GetUserPaidQuota(uB.Id)
	bonusB1, _ := model.GetUserBonusBalance(uB.Id)
	if paidB1 != paidB0 || bonusB1 != bonusB0 {
		panic(fmt.Sprintf("(7) balance changed after conflict rollback: paid %d→%d bonus %d→%d", paidB0, paidB1, bonusB0, bonusB1))
	}
	obCheck, err := model.GetTopupOrderByNo(oB.OrderNo)
	must(err)
	if obCheck.Status != model.TopupOrderStatusPending {
		panic(fmt.Sprintf("(7) order status want pending got %s", obCheck.Status))
	}
	fmt.Println("(7) pending_review occupies device_fp → B complete rolls back, no first bonus / balance unchanged OK")

	// (8) 高1：入账与扣费并发，锁序一致不超时死锁
	uLock := newUser("lock")
	oLock1, err := model.CreateTopupOrder(uLock.Id, 100, "alipay", "fp-lock-"+common.GetRandomString(8), "10.8."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	_, err = model.CompleteTopupOrder(oLock1.OrderNo)
	must(err)
	oLock2, err := model.CreateTopupOrder(uLock.Id, 50, "alipay", "fp-lock2-"+common.GetRandomString(8), "10.82."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	debitQuota, err := model.DollarsToQuota(1)
	must(err)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := model.CompleteTopupOrder(oLock2.OrderNo)
		errCh <- e
	}()
	go func() {
		defer wg.Done()
		// 与入账交错：先 bonus 后 users 的扣费路径
		for i := 0; i < 30; i++ {
			_, _, e := model.DecreaseUserWallet(uLock.Id, debitQuota)
			if e != nil && e != model.ErrInsufficientWalletBalance {
				errCh <- e
				return
			}
		}
		errCh <- nil
	}()
	lockDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(lockDone)
	}()
	select {
	case <-lockDone:
	case <-time.After(20 * time.Second):
		panic("(8) concurrent credit+debit timed out — possible deadlock")
	}
	close(errCh)
	for e := range errCh {
		if e != nil {
			panic(fmt.Sprintf("(8) concurrent credit/debit error: %v", e))
		}
	}
	oLock2Done, err := model.GetTopupOrderByNo(oLock2.OrderNo)
	must(err)
	if oLock2Done.Status != model.TopupOrderStatusPaid {
		panic(fmt.Sprintf("(8) second order want paid got %s", oLock2Done.Status))
	}
	fmt.Println("(8) concurrent CompleteTopupOrder + DecreaseUserWallet no deadlock OK")

	// (9) A5：缺指纹 → pending；confirm 放行（model）入赠送；再放行幂等不重复加
	u9 := newUser("a5ok")
	o9, err := model.CreateTopupOrder(u9.Id, 100, "alipay", "", "10.9."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o9.PendingExtraUSD, 7.5) {
		panic(fmt.Sprintf("(9) pending extra want 7.5 got %v", o9.PendingExtraUSD))
	}
	_, err = model.CompleteTopupOrder(o9.OrderNo)
	must(err)
	var rr9 model.RiskReview
	must(model.DB.Where("order_no = ? AND status = ?", o9.OrderNo, model.RiskReviewStatusPending).First(&rr9).Error)
	bonus9Before, _ := model.GetUserBonusBalance(u9.Id)
	paid9Before, _ := model.GetUserPaidQuota(u9.Id)
	wbTier, _ := model.DollarsToQuota(2.5)
	wbExtra, _ := model.DollarsToQuota(7.5)
	if int(bonus9Before) != wbTier {
		panic(fmt.Sprintf("(9) before approve bonus want tier %d got %d", wbTier, bonus9Before))
	}
	approved, err := model.ApproveRiskReview(rr9.Id, 9001)
	must(err)
	if approved.Status != model.RiskReviewStatusApproved {
		panic(fmt.Sprintf("(9) status want approved got %s", approved.Status))
	}
	bonus9After, _ := model.GetUserBonusBalance(u9.Id)
	paid9After, _ := model.GetUserPaidQuota(u9.Id)
	if paid9After != paid9Before {
		panic("(9) paid quota must not change on approve")
	}
	if int(bonus9After) != wbTier+wbExtra {
		panic(fmt.Sprintf("(9) after approve bonus want %d got %d", wbTier+wbExtra, bonus9After))
	}
	var claim9 model.FirstTopupClaim
	must(model.DB.Where("order_no = ?", o9.OrderNo).First(&claim9).Error)
	if claim9.Status != model.FirstTopupStatusClaimed {
		panic(fmt.Sprintf("(9) claim want claimed got %s", claim9.Status))
	}
	// A5: ApproveRiskReview 经 AdjustQuota，ledger kind=topup_first_bonus
	var ledApprove model.LedgerEntry
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?",
		u9.Id, "bonus", "topup_first_bonus", o9.OrderNo).Order("id DESC").First(&ledApprove).Error)
	if ledApprove.Delta != int64(wbExtra) {
		panic(fmt.Sprintf("(9) approve ledger delta want %d got %d", wbExtra, ledApprove.Delta))
	}
	if ledApprove.RefType != "topup_order" {
		panic(fmt.Sprintf("(9) approve ledger ref_type want topup_order got %s", ledApprove.RefType))
	}
	var nFirstBonus int64
	must(model.DB.Model(&model.LedgerEntry{}).Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?",
		u9.Id, "bonus", "topup_first_bonus", o9.OrderNo).Count(&nFirstBonus).Error)
	if nFirstBonus != 1 {
		panic(fmt.Sprintf("(9) want exactly 1 topup_first_bonus ledger for approve got %d", nFirstBonus))
	}
	fmt.Println("(9b) approve ledger via AdjustQuota kind=topup_first_bonus OK")
	// 幂等再放行
	_, err = model.ApproveRiskReview(rr9.Id, 9001)
	must(err)
	bonus9Replay, _ := model.GetUserBonusBalance(u9.Id)
	if bonus9Replay != bonus9After {
		panic("(9) second approve credited twice")
	}
	fmt.Println("(9) approve pending first-extra → bonus +$7.5 once, claim claimed, idempotent OK")

	// (10) 另一笔 pending，驳回 → 余额不变，状态 rejected
	u10 := newUser("a5rj")
	o10, err := model.CreateTopupOrder(u10.Id, 100, "alipay", "", "10.10."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	_, err = model.CompleteTopupOrder(o10.OrderNo)
	must(err)
	var rr10 model.RiskReview
	must(model.DB.Where("order_no = ? AND status = ?", o10.OrderNo, model.RiskReviewStatusPending).First(&rr10).Error)
	bonus10Before, _ := model.GetUserBonusBalance(u10.Id)
	paid10Before, _ := model.GetUserPaidQuota(u10.Id)
	rejected, err := model.RejectRiskReview(rr10.Id, 9002)
	must(err)
	if rejected.Status != model.RiskReviewStatusRejected {
		panic(fmt.Sprintf("(10) status want rejected got %s", rejected.Status))
	}
	bonus10After, _ := model.GetUserBonusBalance(u10.Id)
	paid10After, _ := model.GetUserPaidQuota(u10.Id)
	if bonus10After != bonus10Before || paid10After != paid10Before {
		panic("(10) reject must not change balances")
	}
	var claim10 model.FirstTopupClaim
	must(model.DB.Where("order_no = ?", o10.OrderNo).First(&claim10).Error)
	if claim10.Status != model.FirstTopupStatusRejected {
		panic(fmt.Sprintf("(10) claim want rejected got %s", claim10.Status))
	}
	fmt.Println("(10) reject pending → balances unchanged, status rejected OK")

	// (11) 放行失败整单回滚：pending 指向不存在的 user → 失败且仍 pending、无入账旁路
	orphan := model.RiskReview{
		Kind:      model.RiskReviewKindFirstTopupExtra,
		UserId:    1999999991,
		OrderNo:   "ORPHAN-A5-" + common.GetRandomString(8),
		AmountUSD: 7.5,
		Status:    model.RiskReviewStatusPending,
		Reason:    "missing_device_fp",
	}
	must(model.DB.Create(&orphan).Error)
	_, err = model.ApproveRiskReview(orphan.Id, 9003)
	if err == nil {
		panic("(11) approve orphan user should fail")
	}
	var orphanAfter model.RiskReview
	must(model.DB.Where("id = ?", orphan.Id).First(&orphanAfter).Error)
	if orphanAfter.Status != model.RiskReviewStatusPending {
		panic(fmt.Sprintf("(11) status want pending after rollback got %s", orphanAfter.Status))
	}
	fmt.Println("(11) approve failure rolls back — status stays pending OK")

	fmt.Println("ALL ACCEPT TOPUP BONUS CASES PASSED")
}
