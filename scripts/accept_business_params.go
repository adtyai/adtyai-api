//go:build ignore

package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func almost(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func main() {
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	fmt.Println("InitDB+options OK")

	// (1) GET catalog lists all keys with current values
	rows := model.ListBusinessParamsWithValues()
	if want := len(model.BusinessParamCatalog()); len(rows) != want {
		panic(fmt.Sprintf("(1) want %d rows (catalog) got %d", want, len(rows)))
	}
	keys := map[string]bool{}
	for _, r := range rows {
		keys[r.Key] = true
		if r.Value == "" && r.Default == "" && !r.Nullable {
			panic(fmt.Sprintf("(1) empty value for %s", r.Key))
		}
	}
	for _, need := range []string{"FirstTopupBonusRate", "TopupBonusTiers", "Price", "TopupOrderRateLimitNum", "DiskAlertCritPct"} {
		if !keys[need] {
			panic("(1) missing " + need)
		}
	}
	if keys["EpayInstanceDailyLimit"] {
		panic("(1) EpayInstanceDailyLimit must not appear")
	}
	fmt.Printf("(1) GET catalog %d keys + values OK\n", len(rows))

	// (2) no-confirm path is controller-level; here assert value unchanged without UpdateOption
	before := model.BusinessParamCurrentValue("FirstTopupBonusRate")
	// simulate: validation alone must not mutate
	_ = model.ValidateBusinessParam("FirstTopupBonusRate", "0.25", nil)
	after := model.BusinessParamCurrentValue("FirstTopupBonusRate")
	if before != after {
		panic("(2) validate mutated value")
	}
	fmt.Printf("(2) without UpdateOption FirstTopupBonusRate stays %s OK (confirm gate is API)\n", before)

	// (3) confirm-equivalent: UpdateOption with valid value
	must(model.ValidateBusinessParam("FirstTopupBonusRate", "0.18", nil))
	must(model.UpdateOption("FirstTopupBonusRate", "0.18"))
	if model.BusinessParamCurrentValue("FirstTopupBonusRate") != "0.18" {
		panic("(3) value not updated")
	}
	if !almost(model.GetFirstTopupBonusRate(), 0.18) {
		panic(fmt.Sprintf("(3) runtime rate want 0.18 got %v", model.GetFirstTopupBonusRate()))
	}
	fmt.Println("(3) confirm UpdateOption FirstTopupBonusRate=0.18 OK")

	// (4) out of range
	if err := model.ValidateBusinessParam("FirstTopupBonusRate", "1.5", nil); err == nil {
		panic("(4) rate=1.5 should fail")
	}
	if err := model.ValidateBusinessParam("DiskAlertCritPct", "80", map[string]string{"DiskAlertWarnPct": "80"}); err == nil {
		panic("(4) crit<=warn should fail")
	}
	if err := model.ValidateBusinessParam("Price", "21", nil); err == nil {
		panic("(4) Price=21 should fail")
	}
	fmt.Println("(4) out-of-range validations → error OK")

	// (5) old order snapshot unaffected by FirstTopupBonusRate change
	u := model.User{
		Username:    "accept-bp-" + common.GetRandomString(6),
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
	// set rate to 0.10, create order, then change to 0.18 — snapshot stays 0.10
	must(model.UpdateOption("FirstTopupBonusRate", "0.10"))
	ord, err := model.CreateTopupOrder(u.Id, 100, "alipay", "fp-bp-"+common.GetRandomString(8), "10.9.9.9")
	must(err)
	snapFirst := ord.FirstBonusRate
	must(model.UpdateOption("FirstTopupBonusRate", "0.18"))
	reloaded, err := model.GetTopupOrderByNo(ord.OrderNo)
	must(err)
	if !almost(reloaded.FirstBonusRate, snapFirst) || !almost(snapFirst, 0.10) {
		panic(fmt.Sprintf("(5) snapshot mutated: snap=%v reloaded=%v live=%v", snapFirst, reloaded.FirstBonusRate, model.GetFirstTopupBonusRate()))
	}
	fmt.Printf("(5) order %s FirstBonusRate snapshot=%.2f intact after live→0.18 OK\n", ord.OrderNo, snapFirst)

	// (6) TopupOrderRateLimit wired
	must(model.UpdateOption("TopupOrderRateLimitNum", "12"))
	must(model.UpdateOption("TopupOrderRateLimitDuration", "90"))
	n, d := model.GetTopupOrderRateLimit()
	if n != 12 || d != 90 {
		panic(fmt.Sprintf("(6) GetTopupOrderRateLimit want 12/90 got %d/%d", n, d))
	}
	// router source assertion (file presence)
	routerSrc, err := os.ReadFile("router/api-router.go")
	must(err)
	if !strings.Contains(string(routerSrc), "TopupOrderRateLimit()") {
		panic("(6) router must use middleware.TopupOrderRateLimit()")
	}
	if strings.Contains(string(routerSrc), "POST(\"/order\", middleware.CriticalRateLimit()") {
		panic("(6) topup order still on CriticalRateLimit")
	}
	fmt.Println("(6) TopupOrderRateLimit params + router wiring OK")

	// (7) batch with one illegal item → none take effect
	beforeRate := model.BusinessParamCurrentValue("FirstTopupBonusRate")
	beforeNum := model.BusinessParamCurrentValue("TopupOrderRateLimitNum")
	err7 := model.ApplyBusinessParamUpdates(map[string]string{
		"FirstTopupBonusRate":    "0.22",
		"TopupOrderRateLimitNum": "0", // min=1 → illegal
	})
	if err7 == nil {
		panic("(7) expected validation error for illegal batch item")
	}
	if model.BusinessParamCurrentValue("FirstTopupBonusRate") != beforeRate {
		panic(fmt.Sprintf("(7) FirstTopupBonusRate changed despite illegal sibling: %s→%s", beforeRate, model.BusinessParamCurrentValue("FirstTopupBonusRate")))
	}
	if model.BusinessParamCurrentValue("TopupOrderRateLimitNum") != beforeNum {
		panic(fmt.Sprintf("(7) TopupOrderRateLimitNum changed: %s→%s", beforeNum, model.BusinessParamCurrentValue("TopupOrderRateLimitNum")))
	}
	fmt.Printf("(7) batch illegal sibling → none applied (err=%v) OK\n", err7)

	// (8) batch two legal items → both take effect
	must(model.ApplyBusinessParamUpdates(map[string]string{
		"FirstTopupBonusRate":    "0.16",
		"TopupOrderRateLimitNum": "15",
	}))
	if model.BusinessParamCurrentValue("FirstTopupBonusRate") != "0.16" {
		panic("(8) FirstTopupBonusRate want 0.16 got " + model.BusinessParamCurrentValue("FirstTopupBonusRate"))
	}
	if model.BusinessParamCurrentValue("TopupOrderRateLimitNum") != "15" {
		panic("(8) TopupOrderRateLimitNum want 15 got " + model.BusinessParamCurrentValue("TopupOrderRateLimitNum"))
	}
	if !almost(model.GetFirstTopupBonusRate(), 0.16) {
		panic(fmt.Sprintf("(8) runtime rate want 0.16 got %v", model.GetFirstTopupBonusRate()))
	}
	n8, _ := model.GetTopupOrderRateLimit()
	if n8 != 15 {
		panic(fmt.Sprintf("(8) runtime rate-limit num want 15 got %d", n8))
	}
	fmt.Println("(8) batch two legal → both applied OK")

	// restore sensible defaults for shared accept DB
	must(model.UpdateOption("FirstTopupBonusRate", "0.10"))
	must(model.UpdateOption("TopupOrderRateLimitNum", "10"))
	must(model.UpdateOption("TopupOrderRateLimitDuration", "60"))
	_ = operation_setting.Price

	fmt.Println("ALL business-params accept checks PASSED")
}
