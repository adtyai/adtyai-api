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

func userCtx(uid int, method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", uid)
	c.Set("role", common.RoleCommonUser)
	c.Set("username", fmt.Sprintf("accept-ledger-u%d", uid))
	return c, w
}

func adminCtx(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-ledger-admin")
	return c, w
}

func newUser(tag string, role int) model.User {
	u := model.User{
		Username:    "accept-wled-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "wl" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func decodeList(body []byte) (success bool, total int64, rows []model.LedgerEntry, message string) {
	var resp struct {
		Success bool               `json:"success"`
		Total   int64              `json:"total"`
		Data    []model.LedgerEntry `json:"data"`
		Message string             `json:"message"`
	}
	must(json.Unmarshal(body, &resp))
	return resp.Success, resp.Total, resp.Data, resp.Message
}

func seedHourly(userId int, account string, delta, bal int64, hb time.Time) {
	hb = hb.UTC().Truncate(time.Hour)
	entry := model.LedgerEntry{
		UserId:       userId,
		Account:      account,
		Kind:         model.LedgerKindAPIConsumeHourly,
		Delta:        delta,
		BalanceAfter: bal,
		Reason:       "accept hourly",
		RefType:      "hourly_rollup",
		RefId:        hb.Format(time.RFC3339),
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
	acceptUserPrefixes := []string{"accept-wled-"}
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
	must(model.DB.AutoMigrate(&model.User{}, &model.UserBonusWallet{}, &model.UserSession{}))
	must(model.EnsureLedgerSchema(model.DB))
	fmt.Println("InitDB + ledger schema OK")

	u1 := newUser("u1", common.RoleCommonUser)
	u2 := newUser("u2", common.RoleCommonUser)

	// seed: u1 paid + bonus + hourly; u2 paid + bonus
	_, err := model.AdjustQuota(nil, u1.Id, "paid", 1000, "seed paid", "accept_seed", "p1", "", 0)
	must(err)
	_, err = model.AdjustQuota(nil, u1.Id, "bonus", 500, "seed bonus", "accept_seed", "b1", "", 0)
	must(err)
	_, err = model.AdjustQuota(nil, u2.Id, "paid", 2000, "seed paid u2", "accept_seed", "p2", "", 0)
	must(err)
	_, err = model.AdjustQuota(nil, u2.Id, "bonus", 800, "seed bonus u2", "accept_seed", "b2", "", 0)
	must(err)
	hb := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Hour)
	seedHourly(u1.Id, "paid", -100, 900, hb)
	seedHourly(u1.Id, "bonus", -50, 450, hb)
	fmt.Printf("seeded u1=%d u2=%d + hourly @ %s\n", u1.Id, u2.Id, hb.Format(time.RFC3339))

	// (1) self: u1 sees own paid+bonus including hourly; not u2
	c, w := userCtx(u1.Id, http.MethodGet, "/api/wallet/ledger/self?limit=50&offset=0")
	controller.GetWalletLedgerSelf(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	ok, total, rows, _ := decodeList(w.Body.Bytes())
	if !ok || total < 4 {
		panic(fmt.Sprintf("(1) want success total>=4 got ok=%v total=%d body=%s", ok, total, w.Body.String()))
	}
	sawPaid, sawBonus, sawHourly := false, false, false
	for _, r := range rows {
		if r.UserId != u1.Id {
			panic(fmt.Sprintf("(1) leaked other user_id=%d", r.UserId))
		}
		if r.Account != "paid" && r.Account != "bonus" {
			panic(fmt.Sprintf("(1) unexpected account=%s", r.Account))
		}
		if r.Account == "paid" {
			sawPaid = true
		}
		if r.Account == "bonus" {
			sawBonus = true
		}
		if r.Kind == model.LedgerKindAPIConsumeHourly && r.HourBucket != nil {
			sawHourly = true
		}
	}
	if !sawPaid || !sawBonus || !sawHourly {
		panic(fmt.Sprintf("(1) missing columns/hourly paid=%v bonus=%v hourly=%v", sawPaid, sawBonus, sawHourly))
	}
	fmt.Printf("(1) self list OK total=%d (paid+bonus+hourly)\n", total)

	// (2) self account=bonus filter
	c, w = userCtx(u1.Id, http.MethodGet, "/api/wallet/ledger/self?account=bonus")
	controller.GetWalletLedgerSelf(c)
	ok, total, rows, _ = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(2) bonus filter fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.Account != "bonus" || r.UserId != u1.Id {
			panic(fmt.Sprintf("(2) bad row account=%s user=%d", r.Account, r.UserId))
		}
	}
	fmt.Println("(2) self account=bonus OK")

	// (3) self kind=api_consume_hourly
	c, w = userCtx(u1.Id, http.MethodGet, "/api/wallet/ledger/self?kind="+model.LedgerKindAPIConsumeHourly)
	controller.GetWalletLedgerSelf(c)
	ok, total, rows, _ = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(3) kind filter fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.Kind != model.LedgerKindAPIConsumeHourly {
			panic("(3) non-hourly row")
		}
	}
	fmt.Println("(3) self kind=api_consume_hourly OK")

	// (4) pagination
	c, w = userCtx(u1.Id, http.MethodGet, "/api/wallet/ledger/self?limit=1&offset=0")
	controller.GetWalletLedgerSelf(c)
	ok, total0, page0, _ := decodeList(w.Body.Bytes())
	if !ok || len(page0) != 1 {
		panic(fmt.Sprintf("(4a) page0 fail body=%s", w.Body.String()))
	}
	c, w = userCtx(u1.Id, http.MethodGet, "/api/wallet/ledger/self?limit=1&offset=1")
	controller.GetWalletLedgerSelf(c)
	ok, total1, page1, _ := decodeList(w.Body.Bytes())
	if !ok || len(page1) != 1 {
		panic(fmt.Sprintf("(4b) page1 fail body=%s", w.Body.String()))
	}
	if total0 != total1 || total0 < 2 {
		panic(fmt.Sprintf("(4) total mismatch %d/%d", total0, total1))
	}
	if page0[0].Id == page1[0].Id && page0[0].CreatedAt.Equal(page1[0].CreatedAt) {
		panic("(4) pagination same row")
	}
	fmt.Printf("(4) self pagination OK total=%d\n", total0)

	// (5) u2 self isolation — no u1 rows
	c, w = userCtx(u2.Id, http.MethodGet, "/api/wallet/ledger/self")
	controller.GetWalletLedgerSelf(c)
	ok, _, rows, _ = decodeList(w.Body.Bytes())
	if !ok {
		panic(fmt.Sprintf("(5) u2 self fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.UserId != u2.Id {
			panic(fmt.Sprintf("(5) u2 saw user_id=%d", r.UserId))
		}
	}
	fmt.Println("(5) self isolation OK")

	// (6) admin bonus-ledger: only bonus; can see both users
	c, w = adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/wallet/bonus-ledger?user_id=%d", u1.Id))
	controller.AdminListBonusLedger(c)
	ok, total, rows, _ = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(6) admin bonus fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.Account != "bonus" {
			panic(fmt.Sprintf("(6) non-bonus account=%s", r.Account))
		}
		if r.UserId != u1.Id {
			panic(fmt.Sprintf("(6) wrong user %d", r.UserId))
		}
	}
	// no user_id → may include u1+u2 bonus
	c, w = adminCtx(http.MethodGet, "/api/admin/wallet/bonus-ledger?limit=100")
	controller.AdminListBonusLedger(c)
	ok, _, rows, _ = decodeList(w.Body.Bytes())
	if !ok {
		panic(fmt.Sprintf("(6b) admin all fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.Account != "bonus" {
			panic(fmt.Sprintf("(6b) non-bonus %s", r.Account))
		}
	}
	fmt.Println("(6) admin bonus-ledger only bonus OK")

	// (7) admin time range around hourly (bonus hourly for u1)
	from := hb.Add(-time.Minute).Unix()
	to := hb.Add(time.Hour).Unix()
	c, w = adminCtx(http.MethodGet, fmt.Sprintf(
		"/api/admin/wallet/bonus-ledger?user_id=%d&created_from=%d&created_to=%d",
		u1.Id, from, to))
	controller.AdminListBonusLedger(c)
	ok, total, rows, _ = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(7) time range fail body=%s", w.Body.String()))
	}
	foundHourlyBonus := false
	for _, r := range rows {
		if r.Kind == model.LedgerKindAPIConsumeHourly {
			foundHourlyBonus = true
		}
	}
	if !foundHourlyBonus {
		panic("(7) expected hourly bonus in window")
	}
	fmt.Println("(7) admin time range OK")

	// (8) UserAuth anonymous reject
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	wg := engine.Group("/api/wallet")
	wg.Use(middleware.UserAuth())
	wg.GET("/ledger/self", controller.GetWalletLedgerSelf)
	req := httptest.NewRequest(http.MethodGet, "/api/wallet/ledger/self", nil)
	ww := httptest.NewRecorder()
	engine.ServeHTTP(ww, req)
	if ww.Code != http.StatusUnauthorized && ww.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(8) self anon want 401/403 got %d body=%s", ww.Code, ww.Body.String()))
	}
	fmt.Printf("(8) UserAuth rejects anonymous (%d) OK\n", ww.Code)

	// (9) AdminAuth anonymous + non-admin reject
	engine2 := gin.New()
	ag := engine2.Group("/api/admin/wallet")
	ag.Use(middleware.AdminAuth())
	ag.GET("/bonus-ledger", controller.AdminListBonusLedger)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/wallet/bonus-ledger", nil)
	ww = httptest.NewRecorder()
	engine2.ServeHTTP(ww, req)
	if ww.Code != http.StatusUnauthorized && ww.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(9a) admin anon want 401/403 got %d body=%s", ww.Code, ww.Body.String()))
	}
	anonCode := ww.Code

	commonUser := newUser("common", common.RoleCommonUser)
	bundle, err := service.CreateLoginSession(commonUser.Id, "password", "127.0.0.1", "accept-wallet-ledger")
	must(err)
	req = httptest.NewRequest(http.MethodGet, "/api/admin/wallet/bonus-ledger", nil)
	req.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	ww = httptest.NewRecorder()
	engine2.ServeHTTP(ww, req)
	if ww.Code != http.StatusForbidden && ww.Code != http.StatusUnauthorized {
		panic(fmt.Sprintf("(9b) non-admin want 403/401 got %d body=%s", ww.Code, ww.Body.String()))
	}
	fmt.Printf("(9) AdminAuth rejects anonymous(%d) + non-admin(%d) OK\n", anonCode, ww.Code)

	// (10) router wiring
	routerSrc, err := os.ReadFile("router/api-router.go")
	must(err)
	src := string(routerSrc)
	checks := []string{
		`walletRoute.Use(middleware.UserAuth())`,
		`walletRoute.GET("/ledger/self", controller.GetWalletLedgerSelf)`,
		`adminWallet.Use(middleware.AdminAuth())`,
		`adminWallet.GET("/bonus-ledger", controller.AdminListBonusLedger)`,
	}
	for _, s := range checks {
		if !strings.Contains(src, s) {
			panic("(10) router missing: " + s)
		}
	}
	fmt.Println("(10) router wired OK")

	fmt.Println("ALL ACCEPT WALLET LEDGER PASSED")
}
