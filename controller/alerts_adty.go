package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// AdminGetAlerts GET /api/admin/alerts
func AdminGetAlerts(c *gin.Context) {
	view := model.BuildAlertsConfigView()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    view,
	})
}

// AdminPutAlerts PUT /api/admin/alerts
// Empty/omitted bot_token does not change the stored token. Token never appears in audit plaintext.
func AdminPutAlerts(c *gin.Context) {
	var req model.AlertsPutInput
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	_, _, err := model.ApplyAlertsPut(req)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    model.BuildAlertsConfigView(),
	})
}

type alertsTestReq struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// AdminTestAlerts POST /api/admin/alerts/test
// Body: {level: "urgent"|"pending", text?: string}
func AdminTestAlerts(c *gin.Context) {
	var req alertsTestReq
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		adtyFail(c, http.StatusBadRequest, "bad_request", "invalid json", nil)
		return
	}
	level := strings.ToLower(strings.TrimSpace(req.Level))
	if level == "" {
		level = model.OpsAlertLevelPending
	}
	if level != model.OpsAlertLevelUrgent && level != model.OpsAlertLevelPending {
		adtyFail(c, http.StatusBadRequest, "invalid_level", "level must be urgent or pending", map[string]any{
			"level": req.Level,
		})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = fmt.Sprintf("[adtyai] alert test (%s)", level)
	}
	err := service.AdtySendTelegramAlert(level, "alert_test", text)
	if err != nil {
		msg := err.Error()
		code := "send_failed"
		status := http.StatusBadRequest
		if strings.Contains(msg, "not configured") {
			code = "not_configured"
			msg = "alert channel not configured"
		} else {
			msg = "failed to send test alert"
		}
		adtyFail(c, status, code, msg, nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "test alert sent",
		"data": gin.H{
			"level": level,
			"kind":  "alert_test",
		},
	})
}
