package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetReconcile GET /api/admin/reconcile?date=YYYY-MM-DD&force=true
// date 缺省=昨天（Asia/Shanghai）。force 缺省=false：当日已 ok 则直接返回（skipped）；
// force=true 重跑对账（含缺 bucket 补跑）。
func AdminGetReconcile(c *gin.Context) {
	date := c.Query("date")
	force, ok := parseReconcileForce(c.Query("force"))
	if !ok {
		adtyFail(c, http.StatusBadRequest, "invalid_parameters", "invalid force (want true/false)", nil)
		return
	}
	result, err := model.RunDailyReconcile(c.Request.Context(), date, force)
	if err != nil {
		code := "bad_request"
		if errors.Is(err, model.ErrInvalidBusinessDate) {
			code = "invalid_parameters"
		}
		adtyFail(c, http.StatusBadRequest, code, err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// parseReconcileForce 空=false；其余按 strconv.ParseBool（1/0/true/false 等）。
func parseReconcileForce(raw string) (bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return v, true
}
