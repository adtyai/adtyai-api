package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// adminTopupOrderView 管理端订单响应：原 TopupOrder 字段原样平铺 + username（用户已删/不存在为 ""）。
// 不改 topup_orders 表结构。
type adminTopupOrderView struct {
	model.TopupOrder
	Username string `json:"username"`
}

// AdminListTopupOrders GET /api/admin/orders
// Query: status, user_id, payment_method, payment_provider, order_no,
//
//	create_time_from, create_time_to (unix), limit, offset
func AdminListTopupOrders(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	createFrom, _ := strconv.ParseInt(c.Query("create_time_from"), 10, 64)
	createTo, _ := strconv.ParseInt(c.Query("create_time_to"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	f := model.AdminTopupOrderListFilter{
		Status:          strings.TrimSpace(c.Query("status")),
		UserId:          userId,
		PaymentMethod:   strings.TrimSpace(c.Query("payment_method")),
		PaymentProvider: strings.TrimSpace(c.Query("payment_provider")),
		OrderNo:         strings.TrimSpace(c.Query("order_no")),
		CreateTimeFrom:  createFrom,
		CreateTimeTo:    createTo,
		Limit:           limit,
		Offset:          offset,
	}
	rows, total, err := model.ListAdminTopupOrders(f)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", err.Error(), nil)
		return
	}
	ids := make([]int, 0, len(rows))
	for _, o := range rows {
		ids = append(ids, o.UserId)
	}
	names, err := model.GetUsernamesByIds(ids)
	if err != nil {
		adtyFail(c, http.StatusOK, "internal_error", err.Error(), nil)
		return
	}
	out := make([]adminTopupOrderView, 0, len(rows))
	for _, o := range rows {
		out = append(out, adminTopupOrderView{TopupOrder: o, Username: names[o.UserId]})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    out,
		"total":   total,
	})
}

// AdminGetTopupOrder GET /api/admin/orders/:no — 管理端详情（与用户端隔离，仅 AdminAuth）。
func AdminGetTopupOrder(c *gin.Context) {
	orderNo := strings.TrimSpace(c.Param("no"))
	if orderNo == "" {
		adtyFail(c, http.StatusBadRequest, "order_no_required", "order_no required", nil)
		return
	}
	order, err := model.GetTopupOrderByNo(orderNo)
	if err != nil {
		status := http.StatusOK
		code := "bad_request"
		if errors.Is(err, model.ErrTopupOrderNotFound) {
			status = http.StatusNotFound
			code = "not_found"
		}
		adtyFail(c, status, code, err.Error(), nil)
		return
	}
	view := adminTopupOrderView{TopupOrder: *order}
	if names, err := model.GetUsernamesByIds([]int{order.UserId}); err == nil {
		view.Username = names[order.UserId]
	} else {
		common.SysError(common.LogText("admin topup order %d: username lookup failed, fallback to empty: %s", order.Id, err.Error()))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}
