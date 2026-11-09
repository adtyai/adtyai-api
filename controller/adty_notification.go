package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtyNotifications(c *gin.Context) {
	inbox, err := service.GetAdtyNotificationInbox(c.GetInt("id"), service.AdtySiteForRequest(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, inbox)
}

func ReadAdtyNotifications(c *gin.Context) {
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8192), &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.MarkAdtyNotificationInboxRead(c.GetInt("id"), service.AdtySiteForRequest(c), body.Keys); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
