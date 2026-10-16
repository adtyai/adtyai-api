//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
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
		Username:         "accept-rf-" + tag + "-" + common.GetRandomString(6),
		Password:         "unused",
		Role:             common.RoleCommonUser,
		Status:           common.UserStatusEnabled,
		Group:            "default",
		AuthVersion:      1,
		Quota:            0,
		AffCode:          "rf" + common.GetRandomString(6),
		Email:            email,
		RegisterIp:       ip,
		RegisterDeviceFp: deviceFp,
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func makePaidOrder(userId int, amountUSD int64, bonusUSD float64, deviceFp, ip string) *model.TopupOrder {
	orderNo := "RF" + common.GetRandomString(12)
	o := &model.TopupOrder{
		OrderNo:         orderNo,
		UserId:          userId,
		AmountUSD:       amountUSD,
		PaidAmountUSD:   float64(amountUSD),
		CreditAmountUSD: float64(amountUSD),
		BonusAmountUSD:  bonusUSD,
		TierIndex:       0,
		TierBonusRate:   0.01,
		FirstBonusRate:  0.10,
		DeviceFp:        deviceFp,
		ClientIP:        ip,
		PaymentMethod:   "alipay",
		Status:          model.TopupOrderStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	must(model.DB.Create(o).Error)
	return o
}

func callRefund(adminId int, orderNo string, body any) (int, map[string]any) {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/orders/"+orderNo+"/refund", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	ww := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(ww)
	gc.Request = req
	gc.Params = gin.Params{{Key: "no", Value: orderNo}}
	gc.Set("id", adminId)
	gc.Set("role", common.RoleRootUser)
	controller.AdminRefundTopupOrder(gc)
	var resp map[string]any
	_ = json.Unmarshal(ww.Body.Bytes(), &resp)
	return ww.Code, resp
}

func main() {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-rf-"}
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
	must(model.DB.AutoMigrate(
		&model.TopupOrder{}, &model.TopupRefund{}, &model.AffCommission{},
		&model.AffWithdrawal{}, &model.User{}, &model.UserBonusWallet{},
		&model.LedgerEntry{}, &model.LedgerIdempotency{},
	))
	fmt.Println("InitDB+InitLogDB OK")

	admin := newUser("admin", "admin-rf@example.com", "fp-admin-rf", "10.9.0.1")
	must(model.DB.Model(&admin).Update("role", common.RoleRootUser).Error)

	must(model.UpdateOption("AffTopupCommissionRate", "0.10"))
	must(model.UpdateOption("AffFreezeDays", "7"))

	// ---------- 42: 同 request_id → 409，仅一条 topup_refunds ----------
	inv42 := newUser("inv42", "inv42@example.com", "fp-inv42", "10.42.0.1")
	bee42 := newUser("bee42", "bee42@example.com", "fp-bee42", "10.42.0.2")
	must(model.DB.Model(&bee42).Update("inviter_id", inv42.Id).Error)
	o42 := makePaidOrder(bee42.Id, 100, 8.0, "fp-bee42", "10.42.0.2")
	_, err := model.CompleteTopupOrder(o42.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o42.OrderNo))

	rid42 := "req-42-" + common.GetRandomString(8)
	code, resp := callRefund(admin.Id, o42.OrderNo, map[string]any{
		"amount": 10, "reason": "test-42", "request_id": rid42, "confirm": true,
	})
	if code != http.StatusOK || resp["success"] != true {
		panic(fmt.Sprintf("(42) first refund want 200 got %d %v", code, resp))
	}
	code2, resp2 := callRefund(admin.Id, o42.OrderNo, map[string]any{
		"amount": 10, "reason": "test-42-dup", "request_id": rid42, "confirm": true,
	})
	if code2 != http.StatusConflict {
		panic(fmt.Sprintf("(42) dup request_id want 409 got %d %v", code2, resp2))
	}
	var cnt42 int64
	must(model.DB.Model(&model.TopupRefund{}).Where("request_id = ?", rid42).Count(&cnt42).Error)
	if cnt42 != 1 {
		panic(fmt.Sprintf("(42) want 1 refund row got %d", cnt42))
	}
	fmt.Println("(42) same request_id → 409, one row OK")

	// ---------- 53: 缺 request_id → 400 ----------
	code53, resp53 := callRefund(admin.Id, o42.OrderNo, map[string]any{
		"amount": 1, "reason": "no-rid", "confirm": true,
	})
	if code53 != http.StatusBadRequest {
		panic(fmt.Sprintf("(53) missing request_id want 400 got %d %v", code53, resp53))
	}
	fmt.Println("(53) missing request_id → 400 OK")

	// confirm missing → 400
	codeC, _ := callRefund(admin.Id, o42.OrderNo, map[string]any{
		"amount": 1, "reason": "x", "request_id": "no-confirm-" + common.GetRandomString(4),
	})
	if codeC != http.StatusBadRequest {
		panic(fmt.Sprintf("missing confirm want 400 got %d", codeC))
	}
	fmt.Println("(confirm) missing confirm → 400 OK")

	// ---------- 43: 实充余额不足可退 → 400 ----------
	inv43 := newUser("inv43", "inv43@example.com", "fp-inv43", "10.43.0.1")
	bee43 := newUser("bee43", "bee43@example.com", "fp-bee43", "10.43.0.2")
	must(model.DB.Model(&bee43).Update("inviter_id", inv43.Id).Error)
	o43 := makePaidOrder(bee43.Id, 100, 0, "fp-bee43", "10.43.0.2")
	_, err = model.CompleteTopupOrder(o43.OrderNo)
	must(err)
	// 花掉大部分实充，只留 $5
	keep, _ := model.DollarsToQuota(5.0)
	must(model.DB.Model(&model.User{}).Where("id = ?", bee43.Id).Update("quota", keep).Error)
	var before43 int
	must(model.DB.Model(&model.User{}).Where("id = ?", bee43.Id).Select("quota").Scan(&before43).Error)
	code43, resp43 := callRefund(admin.Id, o43.OrderNo, map[string]any{
		"amount": 50, "reason": "too-much", "request_id": "req-43-" + common.GetRandomString(6), "confirm": true,
	})
	if code43 != http.StatusBadRequest {
		panic(fmt.Sprintf("(43) want 400 got %d %v", code43, resp43))
	}
	var after43 int
	must(model.DB.Model(&model.User{}).Where("id = ?", bee43.Id).Select("quota").Scan(&after43).Error)
	if after43 != before43 {
		panic(fmt.Sprintf("(43) balance changed %d -> %d", before43, after43))
	}
	if after43 < 0 {
		panic("(43) balance negative")
	}
	fmt.Println("(43) refund > paid balance → 400, unchanged OK")

	// ---------- 44: 冻结返佣按比例作废 ----------
	must(model.UpdateOption("AffFreezeDays", "7"))
	inv44 := newUser("inv44", "inv44@example.com", "fp-inv44", "10.44.0.1")
	bee44 := newUser("bee44", "bee44@example.com", "fp-bee44", "10.44.0.2")
	must(model.DB.Model(&bee44).Update("inviter_id", inv44.Id).Error)
	o44 := makePaidOrder(bee44.Id, 100, 0, "fp-bee44", "10.44.0.2")
	_, err = model.CompleteTopupOrder(o44.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o44.OrderNo))
	var c44 model.AffCommission
	must(model.DB.Where("order_no = ?", o44.OrderNo).First(&c44).Error)
	if c44.Status != model.AffCommissionStatusFrozen {
		panic(fmt.Sprintf("(44) want frozen got %s", c44.Status))
	}
	origComm := c44.CommissionQuota
	wantClaw, _ := model.DollarsToQuota(3.0) // 30/100 * 10
	code44, resp44 := callRefund(admin.Id, o44.OrderNo, map[string]any{
		"amount": 30, "reason": "void-frozen", "request_id": "req-44-" + common.GetRandomString(6), "confirm": true,
	})
	if code44 != http.StatusOK {
		panic(fmt.Sprintf("(44) refund want 200 got %d %v", code44, resp44))
	}
	must(model.DB.Where("order_no = ?", o44.OrderNo).First(&c44).Error)
	if c44.CommissionQuota != origComm-int64(wantClaw) {
		panic(fmt.Sprintf("(44) commission want %d got %d (orig %d claw %d)", origComm-int64(wantClaw), c44.CommissionQuota, origComm, wantClaw))
	}
	var invAff44 int
	must(model.DB.Model(&model.User{}).Where("id = ?", inv44.Id).Select("aff_quota").Scan(&invAff44).Error)
	if invAff44 != 0 {
		panic(fmt.Sprintf("(44) frozen void must not touch aff_quota, got %d", invAff44))
	}
	var rf44 model.TopupRefund
	must(model.DB.Where("order_no = ?", o44.OrderNo).Order("id desc").First(&rf44).Error)
	if rf44.AffClawed != int64(wantClaw) {
		panic(fmt.Sprintf("(44) aff_clawed want %d got %d", wantClaw, rf44.AffClawed))
	}
	fmt.Println("(44) frozen commission proportional void OK")

	// ---------- 45: 已解冻扣回可为负 ----------
	must(model.UpdateOption("AffFreezeDays", "0"))
	inv45 := newUser("inv45", "inv45@example.com", "fp-inv45", "10.45.0.1")
	bee45 := newUser("bee45", "bee45@example.com", "fp-bee45", "10.45.0.2")
	must(model.DB.Model(&bee45).Update("inviter_id", inv45.Id).Error)
	o45 := makePaidOrder(bee45.Id, 100, 0, "fp-bee45", "10.45.0.2")
	_, err = model.CompleteTopupOrder(o45.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o45.OrderNo))
	var inv45u model.User
	must(model.DB.Select("aff_quota").Where("id = ?", inv45.Id).First(&inv45u).Error)
	avail10, _ := model.DollarsToQuota(10.0)
	if inv45u.AffQuota != avail10 {
		panic(fmt.Sprintf("(45) setup aff want %d got %d", avail10, inv45u.AffQuota))
	}
	// 花掉邀请人大部分 aff，只留 $1，再退 $100 → 扣 $10，结果为负
	keepAff, _ := model.DollarsToQuota(1.0)
	must(model.DB.Model(&model.User{}).Where("id = ?", inv45.Id).Update("aff_quota", keepAff).Error)
	code45, resp45 := callRefund(admin.Id, o45.OrderNo, map[string]any{
		"amount": 100, "reason": "claw-neg", "request_id": "req-45-" + common.GetRandomString(6), "confirm": true,
	})
	if code45 != http.StatusOK {
		panic(fmt.Sprintf("(45) want 200 got %d %v", code45, resp45))
	}
	must(model.DB.Select("aff_quota").Where("id = ?", inv45.Id).First(&inv45u).Error)
	wantNeg := keepAff - avail10 // 1 - 10 = -9 dollars in quota
	if inv45u.AffQuota != wantNeg {
		panic(fmt.Sprintf("(45) aff want %d (neg) got %d", wantNeg, inv45u.AffQuota))
	}
	fmt.Println("(45) available commission claw can go negative OK")

	// ---------- 51: 两笔部分退 + shortfall ----------
	must(model.UpdateOption("AffFreezeDays", "0"))
	inv51 := newUser("inv51", "inv51@example.com", "fp-inv51", "10.51.0.1")
	bee51 := newUser("bee51", "bee51@example.com", "fp-bee51", "10.51.0.2")
	must(model.DB.Model(&bee51).Update("inviter_id", inv51.Id).Error)
	o51 := makePaidOrder(bee51.Id, 100, 8.0, "fp-bee51", "10.51.0.2")
	_, err = model.CompleteTopupOrder(o51.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o51.OrderNo))

	code51a, resp51a := callRefund(admin.Id, o51.OrderNo, map[string]any{
		"amount": 30, "reason": "partial1", "request_id": "req-51a-" + common.GetRandomString(6), "confirm": true,
	})
	if code51a != http.StatusOK {
		panic(fmt.Sprintf("(51a) want 200 got %d %v", code51a, resp51a))
	}
	var ord51 model.TopupOrder
	must(model.DB.Where("order_no = ?", o51.OrderNo).First(&ord51).Error)
	if ord51.RefundStatus != model.RefundStatusPartial || ord51.RefundedAmount != 30 {
		panic(fmt.Sprintf("(51a) status/amount want partial/30 got %s/%v", ord51.RefundStatus, ord51.RefundedAmount))
	}
	var rf51a model.TopupRefund
	must(model.DB.Where("order_no = ?", o51.OrderNo).Order("id asc").First(&rf51a).Error)
	wantBonus24, _ := model.DollarsToQuota(2.4)
	wantAff3, _ := model.DollarsToQuota(3.0)
	wantPaid30, _ := model.DollarsToQuota(30.0)
	if rf51a.BonusClawed != int64(wantBonus24) {
		panic(fmt.Sprintf("(51a) bonus_clawed want %d got %d", wantBonus24, rf51a.BonusClawed))
	}
	if rf51a.AffClawed != int64(wantAff3) {
		panic(fmt.Sprintf("(51a) aff_clawed want %d got %d", wantAff3, rf51a.AffClawed))
	}
	data51a := resp51a["data"].(map[string]any)
	if int64(data51a["paid_clawed"].(float64)) != int64(wantPaid30) {
		panic(fmt.Sprintf("(51a) paid_clawed want %d got %v", wantPaid30, data51a["paid_clawed"]))
	}

	// 用户赠送只留 $1
	keepBonus, _ := model.DollarsToQuota(1.0)
	must(model.DB.Model(&model.UserBonusWallet{}).Where("user_id = ?", bee51.Id).Update("balance", keepBonus).Error)

	code51b, resp51b := callRefund(admin.Id, o51.OrderNo, map[string]any{
		"amount": 70, "reason": "partial2", "request_id": "req-51b-" + common.GetRandomString(6), "confirm": true,
	})
	if code51b != http.StatusOK {
		panic(fmt.Sprintf("(51b) want 200 got %d %v", code51b, resp51b))
	}
	must(model.DB.Where("order_no = ?", o51.OrderNo).First(&ord51).Error)
	if ord51.RefundStatus != model.RefundStatusFull || ord51.RefundedAmount != 100 {
		panic(fmt.Sprintf("(51b) want full/100 got %s/%v", ord51.RefundStatus, ord51.RefundedAmount))
	}
	var rf51b model.TopupRefund
	must(model.DB.Where("order_no = ? AND reason = ?", o51.OrderNo, "partial2").First(&rf51b).Error)
	wantShort46, _ := model.DollarsToQuota(4.6) // 5.6 want - 1 clawed
	wantAff7, _ := model.DollarsToQuota(7.0)
	if rf51b.BonusClawed != int64(keepBonus) {
		panic(fmt.Sprintf("(51b) bonus_clawed want %d got %d", keepBonus, rf51b.BonusClawed))
	}
	if rf51b.BonusShortfall != int64(wantShort46) {
		panic(fmt.Sprintf("(51b) shortfall want %d got %d", wantShort46, rf51b.BonusShortfall))
	}
	if rf51b.AffClawed != int64(wantAff7) {
		panic(fmt.Sprintf("(51b) aff_clawed want %d got %d", wantAff7, rf51b.AffClawed))
	}
	fmt.Println("(51) two partial refunds + shortfall OK")

	// ---------- 52: 可退被余额卡住 → 400 ----------
	inv52 := newUser("inv52", "inv52@example.com", "fp-inv52", "10.52.0.1")
	bee52 := newUser("bee52", "bee52@example.com", "fp-bee52", "10.52.0.2")
	must(model.DB.Model(&bee52).Update("inviter_id", inv52.Id).Error)
	o52 := makePaidOrder(bee52.Id, 100, 8.0, "fp-bee52", "10.52.0.2")
	_, err = model.CompleteTopupOrder(o52.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o52.OrderNo))
	_, _ = callRefund(admin.Id, o52.OrderNo, map[string]any{
		"amount": 30, "reason": "52-first", "request_id": "req-52a-" + common.GetRandomString(6), "confirm": true,
	})
	// 实充只留 $20
	keep20, _ := model.DollarsToQuota(20.0)
	must(model.DB.Model(&model.User{}).Where("id = ?", bee52.Id).Update("quota", keep20).Error)
	var before52q int
	must(model.DB.Model(&model.User{}).Where("id = ?", bee52.Id).Select("quota").Scan(&before52q).Error)
	var before52b int64
	must(model.DB.Model(&model.UserBonusWallet{}).Where("user_id = ?", bee52.Id).Select("balance").Scan(&before52b).Error)
	code52, resp52 := callRefund(admin.Id, o52.OrderNo, map[string]any{
		"amount": 70, "reason": "52-blocked", "request_id": "req-52b-" + common.GetRandomString(6), "confirm": true,
	})
	if code52 != http.StatusBadRequest {
		panic(fmt.Sprintf("(52) want 400 got %d %v", code52, resp52))
	}
	var after52q int
	must(model.DB.Model(&model.User{}).Where("id = ?", bee52.Id).Select("quota").Scan(&after52q).Error)
	var after52b int64
	must(model.DB.Model(&model.UserBonusWallet{}).Where("user_id = ?", bee52.Id).Select("balance").Scan(&after52b).Error)
	if after52q != before52q || after52b != before52b {
		panic(fmt.Sprintf("(52) balances changed q %d->%d b %d->%d", before52q, after52q, before52b, after52b))
	}
	fmt.Println("(52) refund capped by balance → 400 OK")

	// ---------- 55: 轻量并发（退款 × N）无死锁 ----------
	inv55 := newUser("inv55", "inv55@example.com", "fp-inv55", "10.55.0.1")
	bee55 := newUser("bee55", "bee55@example.com", "fp-bee55", "10.55.0.2")
	must(model.DB.Model(&bee55).Update("inviter_id", inv55.Id).Error)
	o55 := makePaidOrder(bee55.Id, 100, 5.0, "fp-bee55", "10.55.0.2")
	_, err = model.CompleteTopupOrder(o55.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o55.OrderNo))

	const N = 10
	type r55 struct {
		code int
		err  string
	}
	ch := make(chan r55, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, resp := callRefund(admin.Id, o55.OrderNo, map[string]any{
				"amount": 5, "reason": fmt.Sprintf("conc-%d", i),
				"request_id": fmt.Sprintf("req-55-%d-%s", i, common.GetRandomString(4)),
				"confirm":    true,
			})
			msg, _ := resp["message"].(string)
			ch <- r55{code: c, err: msg}
		}(i)
	}
	wg.Wait()
	close(ch)
	okN, failN := 0, 0
	for r := range ch {
		if r.code == http.StatusOK {
			okN++
		} else {
			failN++
			// 超额 400 可接受；死锁/超时不可
			if r.code == http.StatusInternalServerError || containsFold(r.err, "deadlock") || containsFold(r.err, "timeout") {
				panic(fmt.Sprintf("(55) deadlock/timeout: %d %s", r.code, r.err))
			}
		}
	}
	var q55 int
	must(model.DB.Model(&model.User{}).Where("id = ?", bee55.Id).Select("quota").Scan(&q55).Error)
	var b55 int64
	must(model.DB.Model(&model.UserBonusWallet{}).Where("user_id = ?", bee55.Id).Select("balance").Scan(&b55).Error)
	if q55 < 0 || b55 < 0 {
		panic(fmt.Sprintf("(55) negative balance paid=%d bonus=%d", q55, b55))
	}
	fmt.Printf("(55) concurrent refunds ok=%d fail=%d (no deadlock, non-neg paid/bonus) OK\n", okN, failN)

	
	// A20: paid/bonus 退款流水经 AdjustQuota，kind 保持 topup_refund / topup_refund_bonus_claw
	{
		var nPaid, nBonus int64
		must(model.DB.Model(&model.LedgerEntry{}).Where("kind = ?", "topup_refund").Count(&nPaid).Error)
		must(model.DB.Model(&model.LedgerEntry{}).Where("kind = ?", "topup_refund_bonus_claw").Count(&nBonus).Error)
		if nPaid < 1 {
			panic(fmt.Sprintf("(A20) want >=1 topup_refund ledger got %d", nPaid))
		}
		if nBonus < 1 {
			panic(fmt.Sprintf("(A20) want >=1 topup_refund_bonus_claw ledger got %d", nBonus))
		}
		fmt.Printf("(A20) ledger kinds topup_refund=%d topup_refund_bonus_claw=%d OK\n", nPaid, nBonus)
	}

	fmt.Println("ALL REFUND ACCEPT CHECKS PASSED")
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				match := true
				for j := 0; j < len(sub); j++ {
					a, b := s[i+j], sub[j]
					if a >= 'A' && a <= 'Z' {
						a += 32
					}
					if b >= 'A' && b <= 'Z' {
						b += 32
					}
					if a != b {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
			return false
		})()))
}
