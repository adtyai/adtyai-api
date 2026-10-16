package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type adminOrderResp struct {
	Success bool             `json:"success"`
	Data    []map[string]any `json:"data"`
	Total   int64            `json:"total"`
}

func seedAdminOrders(t *testing.T) (alice, bob int) {
	t.Helper()
	database := modelManagementDB(t, "sqlite", "")
	require.NoError(t, database.AutoMigrate(&model.TopupOrder{}))
	ua := model.User{Username: "alice", Password: "unused-password", DisplayName: "a", AffCode: "b09a"}
	ub := model.User{Username: "bob", Password: "unused-password", DisplayName: "b", AffCode: "b09b"}
	require.NoError(t, database.Create(&ua).Error)
	require.NoError(t, database.Create(&ub).Error)
	orders := []model.TopupOrder{
		{OrderNo: "B09-A1", UserId: ua.Id, AmountUSD: 10, Status: model.TopupOrderStatusPaid, RefundStatus: "none", CreateTime: 1},
		{OrderNo: "B09-B1", UserId: ub.Id, AmountUSD: 30, Status: model.TopupOrderStatusNeedManual, RefundStatus: "none", CreateTime: 2},
		{OrderNo: "B09-A2", UserId: ua.Id, AmountUSD: 50, Status: model.TopupOrderStatusNeedManual, RefundStatus: "none", CreateTime: 3},
		{OrderNo: "B09-X1", UserId: 999999, AmountUSD: 100, Status: model.TopupOrderStatusPending, RefundStatus: "none", CreateTime: 4},
	}
	for i := range orders {
		require.NoError(t, database.Create(&orders[i]).Error)
	}
	return ua.Id, ub.Id
}

func adminOrderGet(t *testing.T, handler gin.HandlerFunc, path string, params gin.Params) (int, string) {
	t.Helper()
	rec := runGinHandler(handler, http.MethodGet, path, params)
	return rec.Code, rec.Body.String()
}

func TestAdminListTopupOrders_Username(t *testing.T) {
	_, _ = seedAdminOrders(t)

	// 统计本次请求里查 users 表的次数：批量一次，不随订单数增长
	var userQueries int32
	cbName := "b09:count_users"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(cbName, func(db *gorm.DB) {
		if db.Statement.Table == "users" {
			atomic.AddInt32(&userQueries, 1)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(cbName) })

	var resp adminOrderResp
	_, body := adminOrderGet(t, AdminListTopupOrders, "/api/admin/orders", nil)
	require.NoError(t, unmarshalString(body, &resp), body)
	require.True(t, resp.Success, body)
	require.EqualValues(t, 4, resp.Total)
	require.Len(t, resp.Data, 4)
	require.EqualValues(t, 1, atomic.LoadInt32(&userQueries), "username lookup must be one batched query")

	want := map[string]string{"B09-A1": "alice", "B09-A2": "alice", "B09-B1": "bob", "B09-X1": ""}
	for _, row := range resp.Data {
		no := row["order_no"].(string)
		name, ok := row["username"]
		require.True(t, ok, "username key present for %s", no)
		require.Equal(t, want[no], name, no)
		// 原有字段保持不变
		for _, k := range []string{"id", "order_no", "user_id", "amount_usd", "status", "refund_status", "create_time", "payment_provider"} {
			_, has := row[k]
			require.True(t, has, "field %s kept on %s", k, no)
		}
	}

	// status=need_manual 过滤
	var nm adminOrderResp
	_, body = adminOrderGet(t, AdminListTopupOrders, "/api/admin/orders?status=need_manual", nil)
	require.NoError(t, unmarshalString(body, &nm), body)
	require.EqualValues(t, 2, nm.Total)
	got := map[string]string{}
	for _, row := range nm.Data {
		require.Equal(t, "need_manual", row["status"])
		got[row["order_no"].(string)] = row["username"].(string)
	}
	require.Equal(t, map[string]string{"B09-A2": "alice", "B09-B1": "bob"}, got)
}

func TestAdminGetTopupOrder_Username(t *testing.T) {
	_, _ = seedAdminOrders(t)
	for no, wantName := range map[string]string{"B09-B1": "bob", "B09-X1": ""} {
		var resp struct {
			Success bool           `json:"success"`
			Data    map[string]any `json:"data"`
		}
		_, body := adminOrderGet(t, AdminGetTopupOrder, "/api/admin/orders/"+no, gin.Params{{Key: "no", Value: no}})
		require.NoError(t, unmarshalString(body, &resp), body)
		require.True(t, resp.Success, body)
		require.Equal(t, no, resp.Data["order_no"])
		require.Equal(t, wantName, resp.Data["username"])
		_, has := resp.Data["user_id"]
		require.True(t, has)
	}
	code, _ := adminOrderGet(t, AdminGetTopupOrder, "/api/admin/orders/NOPE", gin.Params{{Key: "no", Value: "NOPE"}})
	require.Equal(t, http.StatusNotFound, code)
}

func unmarshalString(s string, out any) error {
	return json.NewDecoder(strings.NewReader(s)).Decode(out)
}

func runGinHandler(handler gin.HandlerFunc, method, path string, params gin.Params) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Params = params
	c.Set("role", common.RoleRootUser)
	handler(c)
	return rec
}
