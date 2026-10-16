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

	acceptUserPrefixes := []string{"accept-topup-"}
	acceptOrderPrefixes := []string{"ORPHAN-A5-", "LEGACY-FT-"}
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

	// (1) 非法金额
	_, err := model.CreateTopupOrder(1, 20, "alipay", "fp-x", "1.1.1.1")
	if err == nil || err != model.ErrInvalidTopupAmount {
		panic(fmt.Sprintf("(1) want ErrInvalidTopupAmount got %v", err))
	}
	_, err = model.CreateTopupOrder(1, 99, "alipay", "fp-x", "1.1.1.1")
	if err != model.ErrInvalidTopupAmount {
		panic(fmt.Sprintf("(1b) want ErrInvalidTopupAmount got %v", err))
	}
	fmt.Println("(1) illegal amounts 20/99 → ErrInvalidTopupAmount OK")

	// (2) 档位快照：$100 → tier 2.5%；首充已停 → FirstBonusRate=0、无 pending
	u1 := newUser("t2")
	o2, err := model.CreateTopupOrder(u1.Id, 100, "alipay", "fp-tier-"+common.GetRandomString(8), "10.2."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o2.TierBonusRate, 0.025) {
		panic(fmt.Sprintf("(2) tier rate want 0.025 got %v", o2.TierBonusRate))
	}
	if !almost(o2.FirstBonusRate, 0) {
		panic(fmt.Sprintf("(2) first rate want 0 got %v", o2.FirstBonusRate))
	}
	if !almost(o2.BonusAmountUSD, 2.5) || !almost(o2.PendingExtraUSD, 0) {
		panic(fmt.Sprintf("(2) want immediate 2.5 pending 0 got %v / %v", o2.BonusAmountUSD, o2.PendingExtraUSD))
	}
	if o2.ApplyFirstImmediately || o2.IsFirstCandidate {
		panic(fmt.Sprintf("(2) first path must be off, got %+v", o2))
	}
	fmt.Printf("(2) order %s snapshot tier-only $2.5 OK\n", o2.OrderNo)

	// (3) 入账：只档位赠送；不写 claim；ledger=topup_tier_bonus
	done, err := model.CompleteTopupOrder(o2.OrderNo)
	must(err)
	paid, _ := model.GetUserPaidQuota(u1.Id)
	bonus, _ := model.GetUserBonusBalance(u1.Id)
	wantPaid, _ := model.DollarsToQuota(100)
	wantBonus, _ := model.DollarsToQuota(2.5)
	if paid != wantPaid || int(bonus) != wantBonus {
		panic(fmt.Sprintf("(3) after complete paid=%d bonus=%d want %d/%d", paid, bonus, wantPaid, wantBonus))
	}
	claimed, _ := model.UserHasClaimedFirstTopup(u1.Id)
	if claimed {
		panic("(3) first_topup_claims must NOT be written")
	}
	var rrCount int64
	must(model.DB.Model(&model.RiskReview{}).
		Where("order_no = ? AND kind = ?", o2.OrderNo, model.RiskReviewKindFirstTopupExtra).
		Count(&rrCount).Error)
	if rrCount != 0 {
		panic(fmt.Sprintf("(3) risk_reviews first_topup_extra want 0 got %d", rrCount))
	}
	var ledPaid, ledBonus model.LedgerEntry
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?", u1.Id, "paid", "topup_paid", o2.OrderNo).First(&ledPaid).Error)
	must(model.DB.Where("user_id = ? AND account = ? AND kind = ? AND ref_id = ?", u1.Id, "bonus", "topup_tier_bonus", o2.OrderNo).First(&ledBonus).Error)
	if ledPaid.Delta != int64(wantPaid) || ledBonus.Delta != int64(wantBonus) {
		panic(fmt.Sprintf("(3) ledger deltas want paid=%d bonus=%d got %d/%d", wantPaid, wantBonus, ledPaid.Delta, ledBonus.Delta))
	}
	done2, err := model.CompleteTopupOrder(o2.OrderNo)
	must(err)
	if done2.Id != done.Id {
		panic("idempotent complete mismatch")
	}
	fmt.Println("(3) $100 → paid+tier bonus $2.5, no claim/risk, ledger topup_tier_bonus OK")

	// (4) 二次充值仍只档位
	o4, err := model.CreateTopupOrder(u1.Id, 100, "alipay", "fp-tier-second-"+common.GetRandomString(6), "10.4."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o4.BonusAmountUSD, 2.5) {
		panic(fmt.Sprintf("(4) second bonus want 2.5 got %v", o4.BonusAmountUSD))
	}
	_, err = model.CompleteTopupOrder(o4.OrderNo)
	must(err)
	bonus4, _ := model.GetUserBonusBalance(u1.Id)
	wantBonus4, _ := model.DollarsToQuota(2.5 + 2.5)
	if int(bonus4) != wantBonus4 {
		panic(fmt.Sprintf("(4) bonus after second want %d got %d", wantBonus4, bonus4))
	}
	fmt.Println("(4) second topup $100 → bonus $2.5 only OK")

	// (5) 缺指纹也不再进 pending / 不入队
	u5 := newUser("t5")
	o5, err := model.CreateTopupOrder(u5.Id, 100, "alipay", "", "10.5."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	if !almost(o5.BonusAmountUSD, 2.5) || !almost(o5.PendingExtraUSD, 0) {
		panic(fmt.Sprintf("(5) want 2.5+0 got %v / %v", o5.BonusAmountUSD, o5.PendingExtraUSD))
	}
	_, err = model.CompleteTopupOrder(o5.OrderNo)
	must(err)
	must(model.DB.Model(&model.RiskReview{}).
		Where("order_no = ? AND kind = ?", o5.OrderNo, model.RiskReviewKindFirstTopupExtra).
		Count(&rrCount).Error)
	if rrCount != 0 {
		panic(fmt.Sprintf("(5) risk_reviews want 0 got %d", rrCount))
	}
	fmt.Println("(5) missing device_fp → tier only, no risk enqueue OK")

	// (6) GetFirstTopupBonusRate 恒 0；改 option 内存不改变结算
	if model.GetFirstTopupBonusRate() != 0 {
		panic(fmt.Sprintf("(6) GetFirstTopupBonusRate want 0 got %v", model.GetFirstTopupBonusRate()))
	}
	u6 := newUser("t6")
	o6, err := model.CreateTopupOrder(u6.Id, 100, "alipay", "fp-snap-"+common.GetRandomString(8), "10.6."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	must(model.UpdateFirstTopupBonusRateString("0.20"))
	_, err = model.CompleteTopupOrder(o6.OrderNo)
	must(err)
	bonus6, _ := model.GetUserBonusBalance(u6.Id)
	if int(bonus6) != wantBonus {
		panic(fmt.Sprintf("(6) want tier bonus %d got %d", wantBonus, bonus6))
	}
	if model.GetFirstTopupBonusRate() != 0 {
		panic("(6) GetFirstTopupBonusRate must stay 0 after Update")
	}
	fmt.Println("(6) FirstTopupBonusRate update ignored; settlement tier-only OK")

	// (7) 设备占用不再阻断入账（不再写 claim）
	uA := newUser("fpA")
	fpShared := "fp-pending-" + common.GetRandomString(8)
	fpPtr := fpShared
	must(model.DB.Create(&model.FirstTopupClaim{
		UserId:   uA.Id,
		DeviceFp: &fpPtr,
		IP:       "10.7." + common.GetRandomString(3) + "." + common.GetRandomString(3),
		Status:   model.FirstTopupStatusPendingReview,
	}).Error)
	uB := newUser("fpB")
	oB, err := model.CreateTopupOrder(uB.Id, 100, "alipay", fpShared, "10.72."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	_, err = model.CompleteTopupOrder(oB.OrderNo)
	must(err)
	obCheck, err := model.GetTopupOrderByNo(oB.OrderNo)
	must(err)
	if obCheck.Status != model.TopupOrderStatusPaid {
		panic(fmt.Sprintf("(7) order want paid got %s", obCheck.Status))
	}
	fmt.Println("(7) occupied device_fp no longer blocks complete (no claim write) OK")

	// (8) 并发入账+扣费不死锁
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
	fmt.Println("(8) concurrent CompleteTopupOrder + DecreaseUserWallet no deadlock OK")

	// (9) 历史首充多送风控：手工插入仍可 Approve（新单不再入队）
	u9 := newUser("a5ok")
	o9, err := model.CreateTopupOrder(u9.Id, 100, "alipay", "", "10.9."+common.GetRandomString(3)+"."+common.GetRandomString(3))
	must(err)
	_, err = model.CompleteTopupOrder(o9.OrderNo)
	must(err)
	must(model.DB.Model(&model.RiskReview{}).
		Where("order_no = ?", o9.OrderNo).Count(&rrCount).Error)
	if rrCount != 0 {
		panic(fmt.Sprintf("(9) new order must not enqueue risk got %d", rrCount))
	}
	wbTier, _ := model.DollarsToQuota(2.5)
	wbExtra, _ := model.DollarsToQuota(7.5)
	legacyNo := "LEGACY-FT-" + common.GetRandomString(8)
	rr9 := model.RiskReview{
		Kind:      model.RiskReviewKindFirstTopupExtra,
		UserId:    u9.Id,
		OrderNo:   legacyNo,
		AmountUSD: 7.5,
		Status:    model.RiskReviewStatusPending,
		Reason:    "legacy_manual",
	}
	must(model.DB.Create(&rr9).Error)
	must(model.DB.Create(&model.FirstTopupClaim{
		UserId:  u9.Id,
		OrderNo: legacyNo,
		Status:  model.FirstTopupStatusPendingReview,
	}).Error)
	bonus9Before, _ := model.GetUserBonusBalance(u9.Id)
	if int(bonus9Before) != wbTier {
		panic(fmt.Sprintf("(9) before approve bonus want %d got %d", wbTier, bonus9Before))
	}
	approved, err := model.ApproveRiskReview(rr9.Id, 9001)
	must(err)
	if approved.Status != model.RiskReviewStatusApproved {
		panic(fmt.Sprintf("(9) status want approved got %s", approved.Status))
	}
	bonus9After, _ := model.GetUserBonusBalance(u9.Id)
	if int(bonus9After) != wbTier+wbExtra {
		panic(fmt.Sprintf("(9) after approve bonus want %d got %d", wbTier+wbExtra, bonus9After))
	}
	_, err = model.ApproveRiskReview(rr9.Id, 9001)
	must(err)
	bonus9Replay, _ := model.GetUserBonusBalance(u9.Id)
	if bonus9Replay != bonus9After {
		panic("(9) second approve credited twice")
	}
	fmt.Println("(9) new orders no enqueue; legacy first_topup_extra Approve still works OK")

	// (10) 历史 pending 驳回
	u10 := newUser("a5rj")
	legacyNo10 := "LEGACY-FT-" + common.GetRandomString(8)
	rr10 := model.RiskReview{
		Kind:      model.RiskReviewKindFirstTopupExtra,
		UserId:    u10.Id,
		OrderNo:   legacyNo10,
		AmountUSD: 7.5,
		Status:    model.RiskReviewStatusPending,
		Reason:    "legacy_manual",
	}
	must(model.DB.Create(&rr10).Error)
	must(model.DB.Create(&model.FirstTopupClaim{
		UserId:  u10.Id,
		OrderNo: legacyNo10,
		Status:  model.FirstTopupStatusPendingReview,
	}).Error)
	bonus10Before, _ := model.GetUserBonusBalance(u10.Id)
	rejected, err := model.RejectRiskReview(rr10.Id, 9002)
	must(err)
	if rejected.Status != model.RiskReviewStatusRejected {
		panic(fmt.Sprintf("(10) status want rejected got %s", rejected.Status))
	}
	bonus10After, _ := model.GetUserBonusBalance(u10.Id)
	if bonus10After != bonus10Before {
		panic("(10) reject must not change balances")
	}
	fmt.Println("(10) legacy reject → balances unchanged OK")

	// (11) 放行失败整单回滚
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

	fmt.Println("ALL ACCEPT TOPUP BONUS CASES PASSED (first-topup disabled)")
}
