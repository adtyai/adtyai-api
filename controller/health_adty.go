package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetHealth was GET /api/admin/health. Route unregistered per 总裁（服务状态删除）；handler/表保留.
// 只读汇总；同步轻量刷新 postgres/redis 以保证页新鲜。
func AdminGetHealth(c *gin.Context) {
	summary, err := model.BuildHealthSummary(c.Request.Context(), true)
	if err != nil {
		common.SysLog("admin health: " + err.Error())
		adtyFail(c, http.StatusInternalServerError, "health_probe_failed", "health probe failed", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// AdminRefreshHealth POST /api/admin/health/refresh
// 触发一轮内置探活写表后返回汇总。
func AdminRefreshHealth(c *gin.Context) {
	summary, err := model.BuildHealthSummary(c.Request.Context(), true)
	if err != nil {
		common.SysLog("admin health: " + err.Error())
		adtyFail(c, http.StatusInternalServerError, "health_probe_failed", "health probe failed", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}
