//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func acceptCreateWithdraw(userId int, amount int64, idem, account, chain string) (*model.AffWithdrawal, error) {
	return model.CreateAffWithdrawal(userId, model.AffWithdrawCreateInput{
		AmountQuota: amount,
		IdemKey:     idem,
		PayMethod:   model.AffPayMethodUSDT,
		PayAccount:  account,
		UsdtChain:   chain,
	})
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newUser(tag, email, deviceFp, ip string) model.User {
	u := model.User{
		Username:         "accept-aff-" + tag + "-" + common.GetRandomString(6),
		Password:         "unused",
		Role:             common.RoleCommonUser,
		Status:           common.UserStatusEnabled,
		Group:            "default",
		AuthVersion:      1,
		Quota:            0,
		AffCode:          "af" + common.GetRandomString(6),
		Email:            email,
		RegisterIp:       ip,
		RegisterDeviceFp: deviceFp,
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func makePaidOrder(userId int, amountUSD int64, bonusUSD float64, deviceFp, ip string) *model.TopupOrder {
	orderNo := "AFF" + common.GetRandomString(12)
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

func main() {
	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	must(model.DB.AutoMigrate(&model.AffCommission{}, &model.AffWithdrawal{}, &model.User{}))
	fmt.Println("InitDB+InitLogDB OK")

	admin := newUser("admin", "admin-aff@example.com", "fp-admin", "10.0.0.1")
	must(model.DB.Model(&admin).Update("role", common.RoleRootUser).Error)

	// force known rates for accept
	must(model.UpdateOption("AffTopupCommissionRate", "0.10"))
	must(model.UpdateOption("AffFreezeDays", "7"))
	must(model.UpdateOption("AffMinWithdrawAmount", "1"))
	must(model.UpdateOption("AffWithdrawFeeRate", "0"))

	rate := model.GetAffTopupCommissionRate()
	if rate != 0.10 {
		panic(fmt.Sprintf("rate want 0.10 got %v", rate))
	}

	// (1) 充值实付后产生冻结返佣；赠送不计
	inviter := newUser("inv1", "inviter1@example.com", "fp-inv1", "10.1.0.1")
	invitee := newUser("bee1", "invitee1@example.com", "fp-bee1", "10.1.0.2")
	must(model.DB.Model(&invitee).Update("inviter_id", inviter.Id).Error)
	invitee.InviterId = inviter.Id

	o1 := makePaidOrder(invitee.Id, 100, 8.0, "fp-bee1", "10.1.0.2") // bonus 8 should not affect commission
	_, err := model.CompleteTopupOrder(o1.OrderNo)
	must(err)
	// sync (async may race)
	must(model.ProcessTopupAffCommission(o1.OrderNo))

	var c1 model.AffCommission
	must(model.DB.Where("order_no = ?", o1.OrderNo).First(&c1).Error)
	if c1.Status != model.AffCommissionStatusFrozen {
		panic(fmt.Sprintf("(1) status want frozen got %s", c1.Status))
	}
	wantQuota, _ := model.DollarsToQuota(100.0 * 0.10)
	if c1.CommissionQuota != int64(wantQuota) {
		panic(fmt.Sprintf("(1) commission want %d got %d (bonus must not inflate)", wantQuota, c1.CommissionQuota))
	}
	if c1.FreezeDaysSnap != 7 {
		panic(fmt.Sprintf("(1) freeze snapshot want 7 got %d", c1.FreezeDaysSnap))
	}
	var inv model.User
	must(model.DB.Select("aff_quota").Where("id = ?", inviter.Id).First(&inv).Error)
	if inv.AffQuota != 0 {
		panic(fmt.Sprintf("(1) frozen must not credit aff_quota yet, got %d", inv.AffQuota))
	}
	fmt.Println("(1) paid topup → frozen commission (bonus ignored) OK")

	// (2) 同 device_fp 互邀不计佣并进待审
	inv2 := newUser("inv2", "inviter2@example.com", "fp-same-xx", "10.2.0.1")
	bee2 := newUser("bee2", "invitee2@example.com", "fp-same-xx", "10.2.0.2")
	must(model.DB.Model(&bee2).Update("inviter_id", inv2.Id).Error)
	o2 := makePaidOrder(bee2.Id, 50, 0, "fp-same-xx", "10.2.0.2")
	_, err = model.CompleteTopupOrder(o2.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o2.OrderNo))
	var blockedComm model.AffCommission
	must(model.DB.Where("order_no = ?", o2.OrderNo).First(&blockedComm).Error)
	if blockedComm.Status != model.AffCommissionStatusBlocked {
		panic(fmt.Sprintf("(2) want blocked got %s", blockedComm.Status))
	}
	var rr model.RiskReview
	must(model.DB.Where("order_no = ? AND kind = ?", o2.OrderNo, model.RiskReviewKindAffSelfInvite).First(&rr).Error)
	if rr.Status != model.RiskReviewStatusPending {
		panic("(2) risk review not pending")
	}
	fmt.Println("(2) same device_fp → blocked + risk_review OK")

	// prepare available balance for withdraw tests: freeze_days=0
	must(model.UpdateOption("AffFreezeDays", "0"))
	inv3 := newUser("inv3", "inviter3@example.com", "fp-inv3", "10.3.0.1")
	bee3 := newUser("bee3", "invitee3@example.com", "fp-bee3", "10.3.0.3")
	must(model.DB.Model(&bee3).Update("inviter_id", inv3.Id).Error)
	o3 := makePaidOrder(bee3.Id, 100, 0, "fp-bee3", "10.3.0.3")
	_, err = model.CompleteTopupOrder(o3.OrderNo)
	must(err)
	must(model.ProcessTopupAffCommission(o3.OrderNo))
	must(model.DB.Select("aff_quota").Where("id = ?", inv3.Id).First(&inv).Error)
	availQuota, _ := model.DollarsToQuota(10.0) // 10% of 100
	if inv.AffQuota != availQuota {
		// re-read after commission
		must(model.DB.Select("aff_quota").Where("id = ?", inv3.Id).First(&inv).Error)
	}
	if inv.AffQuota != availQuota {
		panic(fmt.Sprintf("setup aff_quota want %d got %d", availQuota, inv.AffQuota))
	}

	// (3) 同 idem_key 提现只一单
	idem := "idem-" + common.GetRandomString(8)
	w1, err := acceptCreateWithdraw(inv3.Id, int64(availQuota)/2, idem, "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", "trc20")
	must(err)
	w1b, err := acceptCreateWithdraw(inv3.Id, int64(availQuota)/2, idem, "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", "trc20")
	must(err)
	if w1.Id != w1b.Id {
		panic(fmt.Sprintf("(3) idem want same id %d vs %d", w1.Id, w1b.Id))
	}
	var cnt int64
	must(model.DB.Model(&model.AffWithdrawal{}).Where("idem_key = ?", idem).Count(&cnt).Error)
	if cnt != 1 {
		panic(fmt.Sprintf("(3) want 1 row got %d", cnt))
	}
	fmt.Println("(3) same idem_key → one withdrawal OK")

	// (4) rejected 解冻恢复
	before := 0
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&before).Error)
	frozenAmt := w1.AmountQuota
	_, err = model.RejectAffWithdrawal(w1.Id, admin.Id, "test reject")
	must(err)
	after := 0
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&after).Error)
	if after != before+int(frozenAmt) {
		panic(fmt.Sprintf("(4) reject unfreeze want %d got %d (before %d amt %d)", before+int(frozenAmt), after, before, frozenAmt))
	}
	fmt.Println("(4) rejected → unfreeze OK")

	// (5) pay_failed 仍冻结；重试回 approved 后再 paid；pay_unknown 拒绝重试；
	//     pay_unknown→mark-failed→pay_failed_final 不可 retry（堵住绕开）
	idem2 := "idem2-" + common.GetRandomString(8)
	w2, err := acceptCreateWithdraw(inv3.Id, int64(availQuota)/2, idem2, "0x1111111111111111111111111111111111111111", "bep20")
	must(err)
	mid := 0
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&mid).Error)
	_, err = model.ApproveAffWithdrawal(w2.Id, admin.Id)
	must(err)
	_, err = model.MarkAffWithdrawalPayFailed(w2.Id, admin.Id)
	must(err)
	still := 0
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&still).Error)
	if still != mid {
		panic(fmt.Sprintf("(5) pay_failed must keep frozen, quota changed %d -> %d", mid, still))
	}
	retried, err := model.RetryAffWithdrawal(w2.Id, admin.Id)
	must(err)
	if retried.Status != model.AffWithdrawStatusApproved {
		panic("(5) retry want approved")
	}
	paid, err := model.MarkAffWithdrawalPaid(w2.Id, admin.Id, "")
	must(err)
	if paid.Status != model.AffWithdrawStatusPaid {
		panic("(5) want paid")
	}

	// pay_unknown reject retry (direct)
	idem3 := "idem3-" + common.GetRandomString(8)
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&still).Error)
	if still < int(availQuota)/4 {
		must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Update("aff_quota", still+int(availQuota)).Error)
		still = still + int(availQuota)
	}
	w3, err := acceptCreateWithdraw(inv3.Id, int64(availQuota)/4, idem3, "0x2222222222222222222222222222222222222222", "polygon")
	must(err)
	_, err = model.ApproveAffWithdrawal(w3.Id, admin.Id)
	must(err)
	_, err = model.MarkAffWithdrawalPayUnknown(w3.Id, admin.Id)
	must(err)
	_, err = model.RetryAffWithdrawal(w3.Id, admin.Id)
	if err == nil || err != model.ErrAffWithdrawRetryForbidden {
		panic(fmt.Sprintf("(5) pay_unknown retry want forbidden got %v", err))
	}

	// pay_unknown → mark-failed → pay_failed_final → retry 仍禁
	failedFromUnknown, err := model.MarkAffWithdrawalPayFailed(w3.Id, admin.Id)
	must(err)
	if failedFromUnknown.Status != model.AffWithdrawStatusPayFailedFinal {
		panic(fmt.Sprintf("(5b) from unknown mark-failed want pay_failed_final got %s", failedFromUnknown.Status))
	}
	_, err = model.RetryAffWithdrawal(w3.Id, admin.Id)
	if err == nil || err != model.ErrAffWithdrawRetryForbidden {
		panic(fmt.Sprintf("(5b) pay_failed_final retry want forbidden got %v", err))
	}
	fmt.Println("(5) pay_failed freeze+retry+paid; pay_unknown/failed_final reject retry OK")

	// (6) missing confirm → 400（approve / mark-paid / mark-failed / mark-unknown / retry）
	idem4 := "idem4-" + common.GetRandomString(8)
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Update("aff_quota", int(availQuota)).Error)
	w4, err := acceptCreateWithdraw(inv3.Id, int64(availQuota)/5, idem4, "0x3333333333333333333333333333333333333333", "xlayer")
	must(err)

	hitConfirm := func(path string) {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		ww := httptest.NewRecorder()
		gc, _ := gin.CreateTestContext(ww)
		gc.Request = req
		gc.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", w4.Id)}}
		gc.Set("id", admin.Id)
		gc.Set("role", common.RoleRootUser)
		switch {
		case strings.HasSuffix(path, "/approve"):
			controller.AdminApproveAffWithdrawal(gc)
		case strings.HasSuffix(path, "/mark-paid"):
			controller.AdminMarkAffWithdrawalPaid(gc)
		case strings.HasSuffix(path, "/mark-failed"):
			controller.AdminMarkAffWithdrawalFailed(gc)
		case strings.HasSuffix(path, "/mark-unknown"):
			controller.AdminMarkAffWithdrawalUnknown(gc)
		case strings.HasSuffix(path, "/retry"):
			controller.AdminRetryAffWithdrawal(gc)
		default:
			panic("unknown path " + path)
		}
		if ww.Code != http.StatusBadRequest {
			panic(fmt.Sprintf("(6) %s without confirm want 400 got %d body=%s", path, ww.Code, ww.Body.String()))
		}
		var resp map[string]any
		_ = json.Unmarshal(ww.Body.Bytes(), &resp)
		if resp["code"] != "confirm_required" {
			panic(fmt.Sprintf("(6) %s want confirm_required got %v", path, resp))
		}
	}
	base := fmt.Sprintf("/api/admin/aff/withdrawals/%d", w4.Id)
	hitConfirm(base + "/approve")
	hitConfirm(base + "/mark-paid")
	hitConfirm(base + "/mark-failed")
	hitConfirm(base + "/mark-unknown")
	hitConfirm(base + "/retry")
	fmt.Println("(6) missing confirm on approve/mark-paid/mark-failed/mark-unknown/retry → 400 OK")

	// (7) 同 idem_key 并发两请求：aff_quota 只扣一次，仅一单
	idemConc := "idem-conc-" + common.GetRandomString(8)
	concAmt := int64(availQuota) / 5
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Update("aff_quota", int(availQuota)).Error)
	var beforeConc int
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&beforeConc).Error)

	type concRes struct {
		w   *model.AffWithdrawal
		err error
	}
	ch := make(chan concRes, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, e := acceptCreateWithdraw(inv3.Id, concAmt, idemConc, "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", "trc20")
			ch <- concRes{w: w, err: e}
		}()
	}
	wg.Wait()
	close(ch)
	var got []*model.AffWithdrawal
	for r := range ch {
		must(r.err)
		if r.w == nil {
			panic("(7) nil withdrawal")
		}
		got = append(got, r.w)
	}
	if got[0].Id != got[1].Id {
		panic(fmt.Sprintf("(7) concurrent idem want same id %d vs %d", got[0].Id, got[1].Id))
	}
	var rowCnt int64
	must(model.DB.Model(&model.AffWithdrawal{}).Where("idem_key = ?", idemConc).Count(&rowCnt).Error)
	if rowCnt != 1 {
		panic(fmt.Sprintf("(7) want 1 withdrawal row got %d", rowCnt))
	}
	var afterConc int
	must(model.DB.Model(&model.User{}).Where("id = ?", inv3.Id).Select("aff_quota").Scan(&afterConc).Error)
	if afterConc != beforeConc-int(concAmt) {
		panic(fmt.Sprintf("(7) aff_quota double-debit: before=%d after=%d want deduct %d once", beforeConc, afterConc, concAmt))
	}
	fmt.Println("(7) concurrent same idem_key → single debit OK")

	fmt.Println("ALL AFF ACCEPT CHECKS PASSED")
}
