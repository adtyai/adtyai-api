package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetAdtyOwnerStaff(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	page, offset, limit := adtyPage(c)
	query := model.DB.Model(&model.AdtySiteStaff{}).Where("site_id = ? AND owner_user_id = ? AND status <> ?", site.Id, c.GetInt("id"), "deleted")
	var total int64
	var staff []model.AdtySiteStaff
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&staff).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]service.AdtyStaffView, 0, len(staff))
	for _, member := range staff {
		items = append(items, service.AdtyStaffViewOf(member))
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}

func GetAdtyOwnerStaffAudits(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	page, offset, limit := adtyPage(c)
	query := model.DB.Model(&model.AdtySiteAudit{}).Where("site_id = ? AND action LIKE ?", site.Id, "staff.%")
	var total int64
	items := []model.AdtySiteAudit{}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	type staffAuditView struct {
		model.AdtySiteAudit
		StaffId    int `json:"staff_id"`
		CustomerId int `json:"customer_id"`
	}
	views := make([]staffAuditView, 0, len(items))
	for _, item := range items {
		var target struct {
			Id         int `json:"id"`
			StaffId    int `json:"staff_id"`
			CustomerId int `json:"customer_id"`
		}
		if common.UnmarshalJsonStr(item.NewValue, &target) != nil {
			common.ApiErrorStatus(c, http.StatusInternalServerError, service.ErrAdtyStaffInput)
			return
		}
		views = append(views, staffAuditView{AdtySiteAudit: item, StaffId: max(target.Id, target.StaffId), CustomerId: target.CustomerId})
	}
	page.SetTotal(int(total))
	page.SetItems(views)
	common.ApiSuccess(c, page)
}

func PostAdtyOwnerStaff(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var input service.AdtyStaffInput
	if common.DecodeJson(c.Request.Body, &input) != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, service.ErrAdtyStaffInput)
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionStaffCreate, site.Id) {
		return
	}
	staff, err := service.CreateAdtyStaff(c.GetInt("id"), site.Id, input)
	if err != nil {
		common.ApiErrorStatus(c, service.AdtyStaffErrorStatus(err), err)
		return
	}
	common.ApiSuccess(c, staff)
}

func PutAdtyOwnerStaff(c *gin.Context) {
	mutateAdtyOwnerStaff(c, service.AdtySiteOwnerActionStaffUpdate)
}
func PutAdtyOwnerStaffPassword(c *gin.Context) {
	mutateAdtyOwnerStaff(c, service.AdtySiteOwnerActionStaffPassword)
}
func DeleteAdtyOwnerStaff(c *gin.Context) {
	mutateAdtyOwnerStaff(c, service.AdtySiteOwnerActionStaffDelete)
}

func mutateAdtyOwnerStaff(c *gin.Context, action string) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	var input service.AdtyStaffInput
	if common.DecodeJson(c.Request.Body, &input) != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, service.ErrAdtyStaffInput)
		return
	}
	// Reject foreign IDs before consuming the proof.
	var count int64
	if err := model.DB.Model(&model.AdtySiteStaff{}).Where("id = ? AND site_id = ? AND owner_user_id = ? AND status <> ?", id, site.Id, c.GetInt("id"), "deleted").Count(&count).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if count != 1 {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Staff account not found"))
		return
	}
	if !adtyRequireOwnerProof(c, action, id) {
		return
	}
	staff, err := service.MutateAdtyStaff(c.GetInt("id"), site.Id, id, action, input)
	if err != nil {
		common.ApiErrorStatus(c, service.AdtyStaffErrorStatus(err), err)
		return
	}
	common.ApiSuccess(c, staff)
}

func adtyStaffRequest(c *gin.Context, permission string) (*model.AdtySiteStaff, *model.AdtySite, bool) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtyStaffDenied)
		return nil, nil, false
	}
	staff, site, err := service.AdtyStaffAccess(identity.UserID, identity.UserAuthVersion, permission)
	if err != nil {
		common.ApiErrorStatus(c, service.AdtyStaffErrorStatus(err), err)
		return nil, nil, false
	}
	if hostSite := service.AdtySiteForRequest(c); hostSite != nil && hostSite.Id != site.Id {
		common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtyStaffDenied)
		return nil, nil, false
	}
	return staff, service.AdtyStaffCustomerSite(site, staff), true
}

func GetAdtyStaffSelf(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "")
	if !ok {
		return
	}
	common.ApiSuccess(c, gin.H{"staff": service.AdtyStaffViewOf(*staff), "site_name": service.AdtySiteDisplayHost(site)})
}

func PostAdtyStaffCustomerPasswordReset(c *gin.Context) {
	if _, _, ok := adtyStaffRequest(c, ""); !ok {
		return
	}
	common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtyStaffCustomerPasswordReset)
}

func PostAdtyStaffCustomerDisable(c *gin.Context) {
	manageAdtyStaffCustomer(c, service.AdtyStaffActionCustomerDisable, "customers.disable")
}

func GetAdtyStaffBalanceRequests(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "")
	if !ok {
		return
	}
	rows, err := service.ListStaffBalanceRequests(site.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	own := make([]model.AdtyStaffBalanceRequest, 0, len(rows))
	for _, row := range rows {
		if row.StaffUserId == staff.UserId {
			own = append(own, row)
		}
	}
	common.ApiSuccess(c, gin.H{"items": own})
}

func GetAdtyOwnerBalanceRequests(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	rows, err := service.ListStaffBalanceRequests(site.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func GetAdtyAdminBalanceRequests(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	siteID, err := strconv.Atoi(c.Query("site_id"))
	if err != nil || siteID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	rows, err := service.ListStaffBalanceRequests(siteID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyStaffBalanceRequest(c *gin.Context) {
	if _, _, ok := adtyStaffRequest(c, ""); !ok {
		return
	}
	var body struct {
		CustomerID     int    `json:"customer_id"`
		Direction      string `json:"direction"`
		AmountUSD      string `json:"amount_usd"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(body.AmountUSD))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	key := strings.TrimSpace(body.IdempotencyKey)
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	row, err := service.SubmitAdtyStaffBalanceRequest(c.GetInt("id"), body.CustomerID, body.Direction, amount, key)
	if err != nil {
		if errors.Is(err, model.ErrAdtyIdempotencyConflict) {
			common.ApiErrorT(c, "This request was already used for another operation")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func PostAdtyOwnerBalanceReview(c *gin.Context) {
	reviewAdtyStaffBalance(c, "owner")
}

func PostAdtyAdminBalanceReview(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	reviewAdtyStaffBalance(c, "root")
}

func reviewAdtyStaffBalance(c *gin.Context, role string) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ReviewAdtyStaffBalanceRequest(c.GetInt("id"), role, id, body.Approve); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"reviewed": true})
}

func manageAdtyStaffCustomer(c *gin.Context, action, permission string) {
	_, site, ok := adtyStaffRequest(c, permission)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if _, err := service.GetAdtyOwnerCustomer(site, id); err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	// There are deliberately no mutable customer fields in this contract.
	var body map[string]json.RawMessage
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 1024), &body) != nil || len(body) != 0 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	if middleware.RequireSecurityProof(c, service.AdtyStaffCustomerOperation(action, id)) == nil {
		return
	}
	identity, _ := middleware.GetSessionAuthIdentity(c)
	if _, err := service.ManageAdtyStaffCustomer(identity.UserID, identity.UserAuthVersion, id, action); err != nil {
		common.ApiErrorStatus(c, service.AdtyStaffErrorStatus(err), err)
		return
	}
	common.ApiSuccess(c, nil)
}

// Record access before sending any customer data; no body, password or keys.
func adtyStaffReadAudit(c *gin.Context, staff *model.AdtySiteStaff, action string, target int) bool {
	after := gin.H{"staff_id": staff.Id, "customer_id": target}
	if err := model.DB.Create(&model.AdtySiteAudit{SiteId: staff.SiteId, OperatorId: staff.UserId, Action: action,
		NewValue: common.GetJsonString(after)}).Error; err != nil {
		common.ApiError(c, err)
		return false
	}
	if err := model.WriteOfficialAuditLog(c, model.OfficialAuditWrite{
		UserId: staff.UserId, Username: c.GetString("username"), ActorRole: c.GetInt("role"),
		SiteID: model.OfficialAuditSiteIDPointer(staff.SiteId), ObjectType: "site.staff",
		ObjectID: strconv.Itoa(staff.Id), Action: "adty." + action, Category: model.AuditCategoryOperation,
		Content: action, After: after, Result: model.AuditResultSuccess, Success: true,
	}); err != nil {
		common.SysLog(common.LogText("official staff audit write failed: %s", err.Error()))
	}
	return true
}

func GetAdtyStaffCustomers(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "customers.read")
	if !ok {
		return
	}
	page, offset, limit := adtyPage(c)
	items, total, err := service.ListAdtyOwnerCustomers(site, offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !adtyStaffReadAudit(c, staff, "staff.customers.read", 0) {
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func GetAdtyStaffCustomer(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "customers.read")
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	item, err := service.GetAdtyOwnerCustomer(site, id)
	if err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	if !adtyStaffReadAudit(c, staff, "staff.customer.read", id) {
		return
	}
	common.ApiSuccess(c, item)
}

func GetAdtyStaffCustomerTopUps(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "customers.topups")
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	page, offset, limit := adtyPage(c)
	items, total, err := service.ListAdtyOwnerCustomerTopUps(site, id, offset, limit)
	if err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	if !adtyStaffReadAudit(c, staff, "staff.topups.read", id) {
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

type adtyStaffUsage struct {
	Id               int    `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ModelName        string `json:"model_name"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

func GetAdtyStaffCustomerLogs(c *gin.Context) {
	staff, site, ok := adtyStaffRequest(c, "customers.usage")
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	if service.AdtySiteVisibility(site)["request_logs"] != "on" {
		common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtySiteHidden)
		return
	}
	if _, err := service.GetAdtyOwnerCustomer(site, id); err != nil {
		adtyOwnerHiddenOrError(c, err)
		return
	}
	var customer model.AdtySiteCustomer
	if err := model.DB.Where("user_id = ? AND site_id = ?", id, staff.SiteId).First(&customer).Error; err != nil {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Customer not found"))
		return
	}
	page, offset, limit := adtyPage(c)
	query := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND created_at >= ? AND type IN ?", id, customer.JoinedAt, []int{model.LogTypeConsume, model.LogTypeRefund})
	var total int64
	items := []adtyStaffUsage{}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Select("id", "created_at", "type", "model_name", "quota", "prompt_tokens", "completion_tokens").Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if !adtyStaffReadAudit(c, staff, "staff.usage.read", id) {
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}
