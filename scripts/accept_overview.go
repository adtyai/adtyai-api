//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func adminCtx(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-overview-admin")
	return c, w
}

func newUser(tag string, role int) model.User {
	u := model.User{
		Username:    "accept-ov-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "ov" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func getOverview() (model.AdminOverviewResponse, int, string) {
	c, w := adminCtx(http.MethodGet, "/api/admin/overview")
	controller.AdminGetOverview(c)
	body := w.Body.String()
	var resp struct {
		Success bool                       `json:"success"`
		Data    model.AdminOverviewResponse `json:"data"`
		Message string                     `json:"message"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &resp))
	if w.Code == 200 && !resp.Success {
		panic("success=false: " + body)
	}
	return resp.Data, w.Code, body
}

func assertNoForbiddenKeys(raw string) {
	lower := strings.ToLower(raw)
	banned := []string{
		"profit", "margin", "gross", "cost", "ticket",
		"毛利", "成本", "亏本",
	}
	for _, b := range banned {
		if strings.Contains(lower, b) {
			panic(fmt.Sprintf("forbidden key/word %q in response: %s", b, raw))
		}
	}
}

func seedHourly(userId int, account string, delta, bal int64, hb time.Time) {
	hb = hb.UTC().Truncate(time.Hour)
	entry := model.LedgerEntry{
		UserId:       userId,
		Account:      account,
		Kind:         model.LedgerKindAPIConsumeHourly,
		Delta:        delta,
		BalanceAfter: bal,
		Reason:       "accept overview hourly",
		RefType:      "hourly_rollup",
		RefId:        "accept-ov-" + hb.Format(time.RFC3339) + "-" + account + "-" + common.GetRandomString(4),
		HourBucket:   &hb,
		OperatorId:   0,
		CreatedAt:    hb,
	}
	must(model.DB.Create(&entry).Error)
}

func main() {
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())

	// 不绕账本留脏数据：本脚本会直接改余额/插流水来造场景，跑前跑后都清掉自己造的 accept 用户及其流水/订单等，
	// 保证对账（collectUserBalanceDiffs）不留不平用户。清理事务内临时关账本防篡改触发器，不能和联调写入同时跑。
	acceptUserPrefixes := []string{"accept-ov-"}
	acceptOrderPrefixes := []string{"ACCEPT-OV-"}
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
	must(model.DB.AutoMigrate(
		&model.User{},
		&model.UserBonusWallet{},
		&model.UserSession{},
		&model.TopupOrder{},
		&model.AffWithdrawal{},
	))
	must(model.EnsureLedgerSchema(model.DB))
	fmt.Println("InitDB + schema OK")

	loc := model.ShanghaiLocation()
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	// (1) Admin GET 200 + shape/shape + no forbidden fields
	before, code, body := getOverview()
	if code != 200 {
		panic(fmt.Sprintf("(1) want 200 got %d body=%s", code, body))
	}
	assertNoForbiddenKeys(body)
	if before.Timezone != model.ReconcileTimezone {
		panic(fmt.Sprintf("(1) timezone want %s got %s", model.ReconcileTimezone, before.Timezone))
	}
	if before.Today.Date != todayStart.Format("2006-01-02") {
		panic(fmt.Sprintf("(1) today.date want %s got %s", todayStart.Format("2006-01-02"), before.Today.Date))
	}
	if before.Yesterday.Date != yesterdayStart.Format("2006-01-02") {
		panic(fmt.Sprintf("(1) yesterday.date want %s got %s", yesterdayStart.Format("2006-01-02"), before.Yesterday.Date))
	}
	if before.AsOf == "" {
		panic("(1) as_of empty")
	}
	fmt.Printf("(1) Admin GET 200 OK today=%s yesterday=%s pending=%d\n",
		before.Today.Date, before.Yesterday.Date, before.PendingWithdrawals)

	u := newUser("seed", common.RoleCommonUser)

	// (2) seed today paid topup + yesterday hourly consume + pending withdrawals
	const wantPaidUSD = 42.5
	orderNo := "ACCEPT-OV-" + common.GetRandomString(10)
	completeAt := now.Unix()
	ord := model.TopupOrder{
		OrderNo:         orderNo,
		UserId:          u.Id,
		AmountUSD:       42,
		PaidAmountUSD:   wantPaidUSD,
		CreditAmountUSD: wantPaidUSD,
		BonusAmountUSD:  0,
		TierIndex:       0,
		Status:          model.TopupOrderStatusPaid,
		RefundStatus:    model.RefundStatusNone,
		CreateTime:      completeAt - 60,
		CompleteTime:    completeAt,
	}
	must(model.DB.Create(&ord).Error)

	// yesterday Shanghai → pick a mid-day UTC hour_bucket from BusinessDayHourBuckets
	ybuckets := model.BusinessDayHourBuckets(yesterdayStart)
	if len(ybuckets) < 2 {
		panic("(2) yesterday buckets empty")
	}
	hb := ybuckets[len(ybuckets)/2]
	const wantConsumePaid int64 = 777
	const wantConsumeBonus int64 = 111
	seedHourly(u.Id, "paid", -wantConsumePaid, 0, hb)
	seedHourly(u.Id, "bonus", -wantConsumeBonus, 0, hb)

	// pending / approved / pay_unknown / pay_failed count; paid/rejected/final do not
	seedWithdraw := func(status string) {
		w := model.AffWithdrawal{
			UserId:      u.Id,
			AmountQuota: 100,
			FeeQuota:    0,
			NetQuota:    100,
			IdemKey:     "accept-ov-w-" + status + "-" + common.GetRandomString(8),
			Status:      status,
		}
		must(model.DB.Create(&w).Error)
	}
	seedWithdraw(model.AffWithdrawStatusPending)
	seedWithdraw(model.AffWithdrawStatusApproved)
	seedWithdraw(model.AffWithdrawStatusPayUnknown)
	seedWithdraw(model.AffWithdrawStatusPayFailed)
	seedWithdraw(model.AffWithdrawStatusPaid)           // not pending
	seedWithdraw(model.AffWithdrawStatusRejected)       // not pending
	seedWithdraw(model.AffWithdrawStatusPayFailedFinal) // not pending
	const wantPendingDelta int64 = 4

	after, code2, body2 := getOverview()
	if code2 != 200 {
		panic(fmt.Sprintf("(2) want 200 got %d body=%s", code2, body2))
	}
	assertNoForbiddenKeys(body2)

	paidDelta := after.Today.TopupPaidUSD - before.Today.TopupPaidUSD
	ordersDelta := after.Today.TopupPaidOrders - before.Today.TopupPaidOrders
	if paidDelta < wantPaidUSD-0.001 || paidDelta > wantPaidUSD+0.001 {
		panic(fmt.Sprintf("(2) today topup_paid_usd delta want ~%.2f got %.6f (before=%.6f after=%.6f)",
			wantPaidUSD, paidDelta, before.Today.TopupPaidUSD, after.Today.TopupPaidUSD))
	}
	if ordersDelta != 1 {
		panic(fmt.Sprintf("(2) today topup_paid_orders delta want 1 got %d", ordersDelta))
	}

	consumePaidDelta := after.Yesterday.ConsumeQuotaPaid - before.Yesterday.ConsumeQuotaPaid
	consumeBonusDelta := after.Yesterday.ConsumeQuotaBonus - before.Yesterday.ConsumeQuotaBonus
	consumeTotalDelta := after.Yesterday.ConsumeQuota - before.Yesterday.ConsumeQuota
	if consumePaidDelta != wantConsumePaid {
		panic(fmt.Sprintf("(2) yesterday consume_quota_paid delta want %d got %d", wantConsumePaid, consumePaidDelta))
	}
	if consumeBonusDelta != wantConsumeBonus {
		panic(fmt.Sprintf("(2) yesterday consume_quota_bonus delta want %d got %d", wantConsumeBonus, consumeBonusDelta))
	}
	if consumeTotalDelta != wantConsumePaid+wantConsumeBonus {
		panic(fmt.Sprintf("(2) yesterday consume_quota delta want %d got %d", wantConsumePaid+wantConsumeBonus, consumeTotalDelta))
	}

	pendingDelta := after.PendingWithdrawals - before.PendingWithdrawals
	if pendingDelta != wantPendingDelta {
		panic(fmt.Sprintf("(2) pending_withdrawals delta want %d got %d (before=%d after=%d)",
			wantPendingDelta, pendingDelta, before.PendingWithdrawals, after.PendingWithdrawals))
	}
	fmt.Printf("(2) seeded deltas OK topup+%.2f/%d consume_paid+%d consume_bonus+%d pending+%d\n",
		paidDelta, ordersDelta, consumePaidDelta, consumeBonusDelta, pendingDelta)

	// (3) AdminAuth: anonymous + non-admin
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	ag := engine.Group("/api/admin/overview")
	ag.Use(middleware.AdminAuth())
	ag.GET("", controller.AdminGetOverview)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	ww := httptest.NewRecorder()
	engine.ServeHTTP(ww, req)
	if ww.Code != http.StatusUnauthorized && ww.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(3a) anon want 401/403 got %d body=%s", ww.Code, ww.Body.String()))
	}
	anonCode := ww.Code

	commonUser := newUser("common", common.RoleCommonUser)
	bundle, err := service.CreateLoginSession(commonUser.Id, "password", "127.0.0.1", "accept-overview")
	must(err)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	req.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	ww = httptest.NewRecorder()
	engine.ServeHTTP(ww, req)
	if ww.Code != http.StatusForbidden && ww.Code != http.StatusUnauthorized {
		panic(fmt.Sprintf("(3b) non-admin want 403/401 got %d body=%s", ww.Code, ww.Body.String()))
	}
	fmt.Printf("(3) AdminAuth rejects anonymous(%d) + non-admin(%d) OK\n", anonCode, ww.Code)

	// (4) router wiring
	routerSrc, err := os.ReadFile("router/api-router.go")
	must(err)
	src := string(routerSrc)
	checks := []string{
		`adminOverview := apiRouter.Group("/admin/overview")`,
		`adminOverview.Use(middleware.AdminAuth())`,
		`adminOverview.GET("", controller.AdminGetOverview)`,
	}
	for _, s := range checks {
		if !strings.Contains(src, s) {
			panic("(4) router missing: " + s)
		}
	}
	fmt.Println("(4) router wired OK")

	fmt.Println("ALL accept_overview PASSED")
}
