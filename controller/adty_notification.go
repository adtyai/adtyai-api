package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetAdtyNotifications(c *gin.Context) {
	before := 0
	if raw := c.Query("before_id"); raw != "" {
		var err error
		before, err = strconv.Atoi(raw)
		if err != nil || before <= 0 {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	inbox, err := service.GetAdtyNotificationInbox(c.GetInt("id"), service.AdtySiteForRequest(c), before)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, inbox)
}

func GetAdtyAnnouncement(c *gin.Context) {
	inbox, err := service.GetAdtyNotificationInbox(c.GetInt("id"), service.AdtySiteForRequest(c), 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	item, ok := service.AnnouncementDetail(inbox.Announcements, c.Param("key"))
	if !ok {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Announcement not found"))
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, item)
}

func PostAdtyNotificationHide(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.HideAdtyNotificationForUser(c.GetInt("id"), body.Key); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"hidden": true})
}

func PostAdtyNotificationTodoComplete(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.CompleteAdtyTodo(c.GetInt("id"), body.Key); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"completed": true})
}

// Root and owner routes select the publishing domain on the server. Request
// fields cannot impersonate a publisher or choose another owner's domain.
func adtyNotificationIssuer(c *gin.Context) (*model.AdtySite, bool) {
	if c.GetBool("notification_root") {
		return nil, true
	}
	return adtyOwnerSiteOr403(c)
}

func GetAdtyManagedNotifications(c *gin.Context) {
	issuer, ok := adtyNotificationIssuer(c)
	if !ok {
		return
	}
	issuerID := 0
	if issuer != nil {
		issuerID = issuer.Id
	}
	page, start, limit := adtyPage(c)
	items, total, err := service.ListAdtyManagedNotifications(issuerID, start, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}

func WriteAdtyManagedNotification(c *gin.Context) {
	issuer, ok := adtyNotificationIssuer(c)
	if !ok {
		return
	}
	id := 0
	if c.Request.Method != http.MethodPost {
		id, ok = adtyPathId(c)
		if !ok {
			return
		}
	}
	var input service.AdtyNotificationInput
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 65536), &input); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	row, err := service.SaveAdtyNotificationPublication(c.GetInt("id"), issuer, id, input, c.Request.Method == http.MethodDelete)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Not found"))
		return
	}
	if errors.Is(err, model.ErrAdtyNotificationConflict) {
		common.ApiErrorStatus(c, http.StatusConflict, err)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
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
