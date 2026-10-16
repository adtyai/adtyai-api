package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetOverview GET /api/admin/overview
// A01 运营总览：今日/昨日充值与调用消费、待处理提现；无毛利、无 tickets。
func AdminGetOverview(c *gin.Context) {
	data, err := model.BuildAdminOverview()
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
