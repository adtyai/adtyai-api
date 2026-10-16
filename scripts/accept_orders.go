//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
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
	c.Set("username", "accept-orders-admin")
	return c, w
}

func newUser(tag string) model.User {
	u := model.User{
		Username:    "accept-ord-" + tag + "-" + common.GetRandomString(6),
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Quota:       0,
		AffCode:     "ao" + common.GetRandomString(6),
	}
	must(model.DB.Create(&u).Error)
	must(model.EnsureUserBonusWallet(model.DB, u.Id))
	return u
}

func seedOrder(userId int, status, method, provider, orderNo string, createTime int64) *model.TopupOrder {
	if orderNo == "" {
		orderNo = "ACCEPT-ORD-" + common.GetRandomString(10)
	}
	if createTime == 0 {
		createTime = common.GetTimestamp()
	}
	o := &model.TopupOrder{
		OrderNo:         orderNo,
		UserId:          userId,
		AmountUSD:       10,
		PaidAmountUSD:   10,
		CreditAmountUSD: 10,
		BonusAmountUSD:  0.1,
		TierIndex:       0,
		TierBonusRate:   0.01,
		FirstBonusRate:  0.10,
		PaymentMethod:   method,
		PaymentProvider: provider,
		ProviderTradeNo: "ptn-" + common.GetRandomString(8),
		Status:          status,
		RefundStatus:    model.RefundStatusNone,
		CreateTime:      createTime,
	}
	must(model.DB.Create(o).Error)
	return o
}

func decodeList(body []byte) (success bool, total int64, rows []model.TopupOrder) {
	var resp struct {
		Success bool              `json:"success"`
		Total   int64             `json:"total"`
		Data    []model.TopupOrder `json:"data"`
		Message string            `json:"message"`
	}
	must(json.Unmarshal(body, &resp))
	return resp.Success, resp.Total, resp.Data
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
	must(model.DB.AutoMigrate(&model.TopupOrder{}, &model.User{}, &model.UserBonusWallet{}))
	must(model.EnsureLedgerSchema(model.DB))
	fmt.Println("InitDB + topup_orders OK")

	u1 := newUser("u1")
	u2 := newUser("u2")
	now := common.GetTimestamp()
	tOld := now - 86400*3

	pending := seedOrder(u1.Id, model.TopupOrderStatusPending, "alipay", "epay", "", now)
	paid := seedOrder(u1.Id, model.TopupOrderStatusPaid, "usdt", "bepusdt", "", now-10)
	failed := seedOrder(u2.Id, model.TopupOrderStatusFailed, "wxpay", "epay", "", tOld)
	needManual := seedOrder(u2.Id, model.TopupOrderStatusNeedManual, "usdt", "bepusdt", "", now-20)
	_ = failed
	_ = needManual

	// (1) list all for u1 — at least pending+paid
	c, w := adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/orders?user_id=%d&limit=50&offset=0", u1.Id))
	controller.AdminListTopupOrders(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) list status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	ok, total, rows := decodeList(w.Body.Bytes())
	if !ok {
		panic(fmt.Sprintf("(1) success=false body=%s", w.Body.String()))
	}
	if total < 2 || len(rows) < 2 {
		panic(fmt.Sprintf("(1) u1 want >=2 got total=%d len=%d", total, len(rows)))
	}
	fmt.Printf("(1) list user_id=%d total=%d OK\n", u1.Id, total)

	// (2) filter status=pending
	c, w = adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/orders?user_id=%d&status=pending", u1.Id))
	controller.AdminListTopupOrders(c)
	ok, total, rows = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(2) status=pending fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.Status != model.TopupOrderStatusPending {
			panic(fmt.Sprintf("(2) non-pending row %s status=%s", r.OrderNo, r.Status))
		}
	}
	fmt.Println("(2) filter status=pending OK")

	// (3) exact order_no
	c, w = adminCtx(http.MethodGet, "/api/admin/orders?order_no="+paid.OrderNo)
	controller.AdminListTopupOrders(c)
	ok, total, rows = decodeList(w.Body.Bytes())
	if !ok || total != 1 || len(rows) != 1 || rows[0].OrderNo != paid.OrderNo {
		panic(fmt.Sprintf("(3) order_no exact fail body=%s", w.Body.String()))
	}
	if rows[0].ProviderTradeNo == "" || rows[0].RefundStatus == "" {
		panic("(3) missing provider_trade_no or refund_status in payload")
	}
	fmt.Printf("(3) order_no exact + fields OK no=%s\n", paid.OrderNo)

	// (4) payment_method + payment_provider
	c, w = adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/orders?user_id=%d&payment_method=usdt&payment_provider=bepusdt", u1.Id))
	controller.AdminListTopupOrders(c)
	ok, total, rows = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(4) payment filter fail body=%s", w.Body.String()))
	}
	for _, r := range rows {
		if r.PaymentMethod != "usdt" || r.PaymentProvider != "bepusdt" {
			panic(fmt.Sprintf("(4) bad row method=%s provider=%s", r.PaymentMethod, r.PaymentProvider))
		}
	}
	fmt.Println("(4) payment_method/provider filter OK")

	// (5) time range: only old failed for u2
	c, w = adminCtx(http.MethodGet, fmt.Sprintf(
		"/api/admin/orders?user_id=%d&create_time_from=%d&create_time_to=%d",
		u2.Id, tOld-60, tOld+60))
	controller.AdminListTopupOrders(c)
	ok, total, rows = decodeList(w.Body.Bytes())
	if !ok || total < 1 {
		panic(fmt.Sprintf("(5) time range fail body=%s", w.Body.String()))
	}
	foundFailed := false
	for _, r := range rows {
		if r.OrderNo == failed.OrderNo {
			foundFailed = true
		}
		if r.CreateTime < tOld-60 || r.CreateTime > tOld+60 {
			panic(fmt.Sprintf("(5) out of range create_time=%d", r.CreateTime))
		}
	}
	if !foundFailed {
		panic("(5) expected failed order in time window")
	}
	fmt.Println("(5) create_time range OK")

	// (6) pagination: limit=1 offset=0 then offset=1 for u1
	c, w = adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/orders?user_id=%d&limit=1&offset=0", u1.Id))
	controller.AdminListTopupOrders(c)
	ok, total0, page0 := decodeList(w.Body.Bytes())
	if !ok || len(page0) != 1 {
		panic(fmt.Sprintf("(6a) page0 fail body=%s", w.Body.String()))
	}
	c, w = adminCtx(http.MethodGet, fmt.Sprintf("/api/admin/orders?user_id=%d&limit=1&offset=1", u1.Id))
	controller.AdminListTopupOrders(c)
	ok, total1, page1 := decodeList(w.Body.Bytes())
	if !ok || len(page1) != 1 {
		panic(fmt.Sprintf("(6b) page1 fail body=%s", w.Body.String()))
	}
	if total0 != total1 || total0 < 2 {
		panic(fmt.Sprintf("(6) total mismatch %d/%d", total0, total1))
	}
	if page0[0].OrderNo == page1[0].OrderNo {
		panic("(6) pagination returned same row")
	}
	fmt.Printf("(6) pagination OK total=%d page0=%s page1=%s\n", total0, page0[0].OrderNo, page1[0].OrderNo)

	// (7) GET detail
	c, w = adminCtx(http.MethodGet, "/api/admin/orders/"+pending.OrderNo)
	c.Params = gin.Params{{Key: "no", Value: pending.OrderNo}}
	controller.AdminGetTopupOrder(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(7) detail status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	var detailResp struct {
		Success bool             `json:"success"`
		Data    model.TopupOrder `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &detailResp))
	if !detailResp.Success || detailResp.Data.OrderNo != pending.OrderNo {
		panic(fmt.Sprintf("(7) detail bad body=%s", w.Body.String()))
	}
	fmt.Println("(7) GET /api/admin/orders/:no OK")

	// (8) AdminAuth: unauthenticated → 401
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	g := engine.Group("/api/admin/orders")
	g.Use(middleware.AdminAuth())
	g.GET("", controller.AdminListTopupOrders)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/orders", nil)
	ww := httptest.NewRecorder()
	engine.ServeHTTP(ww, req)
	if ww.Code != http.StatusUnauthorized && ww.Code != http.StatusForbidden {
		panic(fmt.Sprintf("(8) no-auth want 401/403 got %d body=%s", ww.Code, ww.Body.String()))
	}
	routerSrc, err := os.ReadFile("router/api-router.go")
	must(err)
	src := string(routerSrc)
	if !strings.Contains(src, `adminOrders.Use(middleware.AdminAuth())`) {
		panic("(8) router missing AdminAuth on adminOrders")
	}
	if !strings.Contains(src, `adminOrders.GET("", controller.AdminListTopupOrders)`) {
		panic("(8) router missing GET list")
	}
	fmt.Printf("(8) AdminAuth rejects anonymous (%d) + router wired OK\n", ww.Code)

	// (9) complete smoke (unchanged path)
	completeNo := pending.OrderNo
	body, _ := json.Marshal(map[string]any{"confirm": true})
	req = httptest.NewRequest(http.MethodPost, "/api/admin/orders/"+completeNo+"/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ww = httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(ww)
	gc.Request = req
	gc.Params = gin.Params{{Key: "no", Value: completeNo}}
	gc.Set("id", 1)
	gc.Set("role", common.RoleRootUser)
	gc.Set("username", "accept-orders-admin")
	controller.AdminCompleteAdtyTopup(gc)
	if ww.Code != 200 {
		panic(fmt.Sprintf("(9) complete status want 200 got %d body=%s", ww.Code, ww.Body.String()))
	}
	var completeResp struct {
		Success bool             `json:"success"`
		Data    model.TopupOrder `json:"data"`
	}
	must(json.Unmarshal(ww.Body.Bytes(), &completeResp))
	if !completeResp.Success || completeResp.Data.Status != model.TopupOrderStatusPaid {
		panic(fmt.Sprintf("(9) complete bad body=%s", ww.Body.String()))
	}
	fmt.Printf("(9) complete smoke OK no=%s → paid\n", completeNo)

	// (10) not found detail
	c, w = adminCtx(http.MethodGet, "/api/admin/orders/NO-SUCH-ORDER")
	c.Params = gin.Params{{Key: "no", Value: "NO-SUCH-ORDER"}}
	controller.AdminGetTopupOrder(c)
	if w.Code != http.StatusNotFound {
		panic(fmt.Sprintf("(10) want 404 got %d body=%s", w.Code, w.Body.String()))
	}
	fmt.Println("(10) detail 404 OK")

	fmt.Println("ALL ACCEPT ORDERS PASSED")
}
