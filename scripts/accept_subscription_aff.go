//go:build ignore

package main

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newUser(tag, email, deviceFp, ip string) model.User {
	u := model.User{
		Username:         "accept-subaff-" + tag + "-" + common.GetRandomString(6),
		Password:         "unused",
		Role:             common.RoleCommonUser,
		Status:           common.UserStatusEnabled,
		Group:            "default",
		AuthVersion:      1,
		Quota:            0,
		AffCode:          "sa" + common.GetRandomString(6),
		Email:            email,
		RegisterIp:       ip,
		RegisterDeviceFp: deviceFp,
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func newPlan(price float64) *model.SubscriptionPlan {
	allow := true
	plan := &model.SubscriptionPlan{
		Title:           "Accept Sub Aff " + common.GetRandomString(4),
		PriceAmount:     price,
		Currency:        "USD",
		DurationUnit:    model.SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		TotalAmount:     100000,
		AllowBalancePay: &allow,
	}
	must(model.DB.Create(plan).Error)
	return plan
}

func pendingDirectOrder(userId, planId int, money float64, tradeNo string) *model.SubscriptionOrder {
	o := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          planId,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	must(o.Insert())
	return o
}

func countComm(tradeNo string) int64 {
	var n int64
	must(model.DB.Model(&model.AffCommission{}).Where("order_no = ?", tradeNo).Count(&n).Error)
	return n
}

func main() {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	must(model.DB.AutoMigrate(
		&model.AffCommission{},
		&model.SubscriptionPlan{},
		&model.SubscriptionOrder{},
		&model.UserSubscription{},
		&model.User{},
	))
	fmt.Println("InitDB OK")

	must(model.UpdateOption("AffSubscriptionCommissionRate", "0.10"))
	must(model.UpdateOption("AffTopupCommissionRate", "0.25")) // 故意设不同，证明订阅不计充值率
	must(model.UpdateOption("AffFreezeDays", "7"))
	rate := model.GetAffSubscriptionCommissionRate()
	if rate != 0.10 {
		panic(fmt.Sprintf("sub rate want 0.10 got %v", rate))
	}
	if model.GetAffTopupCommissionRate() != 0.25 {
		panic(fmt.Sprintf("topup rate want 0.25 got %v", model.GetAffTopupCommissionRate()))
	}

	paidUSD := 50.0
	plan := newPlan(paidUSD)

	// (1) 直接付款订阅实付 → 返佣（金额=实付×费率快照，与充值同规则）
	inv1 := newUser("inv1", "subaff-inv1@example.com", "fp-sub-inv1", "10.11.0.1")
	bee1 := newUser("bee1", "subaff-bee1@example.com", "fp-sub-bee1", "10.11.0.2")
	must(model.DB.Model(&bee1).Update("inviter_id", inv1.Id).Error)

	trade1 := "SUBAFF" + common.GetRandomString(12)
	pendingDirectOrder(bee1.Id, plan.Id, paidUSD, trade1)
	must(model.CompleteSubscriptionOrder(trade1, `{"provider":"epay"}`, model.PaymentProviderEpay, "alipay"))
	// sync (async may race)
	must(model.ProcessSubscriptionAffCommission(trade1))

	var c1 model.AffCommission
	must(model.DB.Where("order_no = ?", trade1).First(&c1).Error)
	if c1.Source != model.AffCommissionSourceSubscription {
		panic(fmt.Sprintf("(1) source want subscription got %s", c1.Source))
	}
	if c1.Status != model.AffCommissionStatusFrozen {
		panic(fmt.Sprintf("(1) status want frozen got %s", c1.Status))
	}
	wantQuota, _ := model.DollarsToQuota(paidUSD * 0.10)
	if c1.CommissionQuota != int64(wantQuota) {
		panic(fmt.Sprintf("(1) commission want %d got %d", wantQuota, c1.CommissionQuota))
	}
	if c1.PaidAmountUSD != paidUSD {
		panic(fmt.Sprintf("(1) paid want %v got %v", paidUSD, c1.PaidAmountUSD))
	}
	if c1.RateSnapshot != 0.10 {
		panic(fmt.Sprintf("(1) rate snapshot want 0.10 got %v", c1.RateSnapshot))
	}
	fmt.Println("(1) direct-pay subscription → commission OK")

	// (2) 余额买订阅 → 无返佣
	inv2 := newUser("inv2", "subaff-inv2@example.com", "fp-sub-inv2", "10.12.0.1")
	bee2 := newUser("bee2", "subaff-bee2@example.com", "fp-sub-bee2", "10.12.0.2")
	must(model.DB.Model(&bee2).Update("inviter_id", inv2.Id).Error)
	// give ample wallet quota (balance purchase uses Ceil(price*QuotaPerUnit))
	must(model.DB.Model(&bee2).Update("quota", 50_000_000).Error)

	planBal := newPlan(paidUSD)
	must(model.PurchaseSubscriptionWithBalance(bee2.Id, planBal.Id))

	var balOrder model.SubscriptionOrder
	must(model.DB.Where("user_id = ? AND payment_provider = ?", bee2.Id, model.PaymentProviderBalance).
		Order("id DESC").First(&balOrder).Error)
	must(model.ProcessSubscriptionAffCommission(balOrder.TradeNo))
	if n := countComm(balOrder.TradeNo); n != 0 {
		panic(fmt.Sprintf("(2) balance buy must have 0 commissions, got %d", n))
	}
	fmt.Println("(2) balance buy subscription → no commission OK")

	// (3) 同单重复回调/重放不计两次
	must(model.CompleteSubscriptionOrder(trade1, `{"replay":1}`, model.PaymentProviderEpay, "alipay"))
	must(model.ProcessSubscriptionAffCommission(trade1))
	must(model.ProcessSubscriptionAffCommission(trade1))
	if n := countComm(trade1); n != 1 {
		panic(fmt.Sprintf("(3) want 1 commission row got %d", n))
	}
	fmt.Println("(3) replay / double Process → single commission OK")

	// (4) 自邀规则复用：同 device_fp → blocked + risk_review
	inv3 := newUser("inv3", "subaff-inv3@example.com", "fp-sub-same", "10.13.0.1")
	bee3 := newUser("bee3", "subaff-bee3@example.com", "fp-sub-same", "10.13.0.2")
	must(model.DB.Model(&bee3).Update("inviter_id", inv3.Id).Error)
	trade4 := "SUBAFFSI" + common.GetRandomString(10)
	planSI := newPlan(30.0)
	pendingDirectOrder(bee3.Id, planSI.Id, 30.0, trade4)
	must(model.CompleteSubscriptionOrder(trade4, "", model.PaymentProviderEpay, "alipay"))
	must(model.ProcessSubscriptionAffCommission(trade4))
	var blocked model.AffCommission
	must(model.DB.Where("order_no = ?", trade4).First(&blocked).Error)
	if blocked.Status != model.AffCommissionStatusBlocked {
		panic(fmt.Sprintf("(4) want blocked got %s", blocked.Status))
	}
	if blocked.Source != model.AffCommissionSourceSubscription {
		panic(fmt.Sprintf("(4) source want subscription got %s", blocked.Source))
	}
	var rr model.RiskReview
	must(model.DB.Where("order_no = ? AND kind = ?", trade4, model.RiskReviewKindAffSelfInvite).First(&rr).Error)
	if rr.Status != model.RiskReviewStatusPending {
		panic("(4) risk review not pending")
	}
	fmt.Println("(4) self-invite (same device_fp) → blocked + risk_review OK")

	// (5) 改订阅佣金率生效；改充值佣金率不影响订阅计佣
	must(model.UpdateOption("AffSubscriptionCommissionRate", "0.20"))
	must(model.UpdateOption("AffTopupCommissionRate", "0.05"))
	if model.GetAffSubscriptionCommissionRate() != 0.20 {
		panic(fmt.Sprintf("(5) sub rate want 0.20 got %v", model.GetAffSubscriptionCommissionRate()))
	}
	if model.GetAffTopupCommissionRate() != 0.05 {
		panic(fmt.Sprintf("(5) topup rate want 0.05 got %v", model.GetAffTopupCommissionRate()))
	}
	inv5 := newUser("inv5", "subaff-inv5@example.com", "fp-sub-inv5", "10.15.0.1")
	bee5 := newUser("bee5", "subaff-bee5@example.com", "fp-sub-bee5", "10.15.0.2")
	must(model.DB.Model(&bee5).Update("inviter_id", inv5.Id).Error)
	trade5 := "SUBAFFR" + common.GetRandomString(10)
	plan5 := newPlan(40.0)
	pendingDirectOrder(bee5.Id, plan5.Id, 40.0, trade5)
	must(model.CompleteSubscriptionOrder(trade5, "", model.PaymentProviderEpay, "alipay"))
	must(model.ProcessSubscriptionAffCommission(trade5))
	var c5 model.AffCommission
	must(model.DB.Where("order_no = ?", trade5).First(&c5).Error)
	want5, _ := model.DollarsToQuota(40.0 * 0.20)
	if c5.CommissionQuota != int64(want5) {
		panic(fmt.Sprintf("(5) commission want %d (sub 0.20) got %d", want5, c5.CommissionQuota))
	}
	if c5.RateSnapshot != 0.20 {
		panic(fmt.Sprintf("(5) rate snapshot want 0.20 got %v", c5.RateSnapshot))
	}
	wrongTopup, _ := model.DollarsToQuota(40.0 * 0.05)
	if c5.CommissionQuota == int64(wrongTopup) {
		panic("(5) commission matched topup rate 0.05 — still reading AffTopupCommissionRate")
	}
	fmt.Println("(5) AffSubscriptionCommissionRate drives sub aff; AffTopupCommissionRate does not OK")

	// brief wait so async from (1)/(3)/(4)/(5) settles without polluting next runs
	time.Sleep(50 * time.Millisecond)

	fmt.Println("ALL SUBSCRIPTION AFF ACCEPT CHECKS PASSED")
}
