package controller

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PutAdtyMainTeamAgreementPrice(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtyMainTeamAgreementPrice(id, body.Discount); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PostAdtyTeamTopup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		AmountUSD string `json:"amount_usd"`
		Method    string `json:"method"`
		ChannelId int    `json:"channel_id"` // 易支付：负责人选的通道，0 表示自动挑
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
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if method := strings.TrimSpace(body.Method); method != "" {
		order, err := service.StartAdtyTeamTopupPayment(c.GetInt("id"), id, amount, method, body.ChannelId, key, c.ClientIP())
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, order)
		return
	}
	order, err := service.CreateAdtyTeamTopupOrder(c.GetInt("id"), id, amount, key)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, order)
}

func GetAdtyTeamTopups(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	rows, err := service.ListAdtyTeamTopups(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyTeam(c *gin.Context) {
	common.ApiErrorStatus(c, http.StatusForbidden, service.ErrAdtyTeamForbidden)
}

func PostAdtyMainTeam(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		Name     string `json:"name"`
		LeaderId int    `json:"leader_id"`
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	created, err := service.CreateAdtyMainTeamByRoot(c.GetInt("id"), body.LeaderId, body.Name, body.Discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}

func PostAdtyOwnerSiteTeam(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		Name     string `json:"name"`
		LeaderId int    `json:"leader_id"`
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	created, err := service.CreateAdtyOwnerSiteTeam(c.GetInt("id"), site.Id, body.LeaderId, body.Name, body.Discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}

func GetAdtyTeamMine(c *gin.Context) {
	mine, err := service.GetAdtyTeamMine(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"team": mine})
}

func parseAdtyTeamInviteTTL(c *gin.Context) (time.Duration, bool) {
	ttl := service.AdtyTeamInviteDefaultTTL
	if c.Request.Body == nil {
		return ttl, true
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 4096))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ttl, true
	}
	var body struct {
		TTLSeconds *int64 `json:"ttl_seconds"`
	}
	if err := common.Unmarshal(raw, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	if body.TTLSeconds == nil {
		return ttl, true
	}
	if *body.TTLSeconds <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	return time.Duration(*body.TTLSeconds) * time.Second, true
}

func PostAdtyTeamInvite(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	ttl, ok := parseAdtyTeamInviteTTL(c)
	if !ok {
		return
	}
	issued, err := service.CreateAdtyTeamInvite(c.GetInt("id"), int64(id), ttl)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, issued)
}

func GetAdtyTeamInvites(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	rows, err := service.ListAdtyTeamInvites(c.GetInt("id"), int64(id))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyTeamRevokeInvite(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	inviteId, err := strconv.ParseInt(c.Param("inviteId"), 10, 64)
	if err != nil || inviteId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	view, err := service.RevokeAdtyTeamInvite(c.GetInt("id"), int64(id), inviteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtyTeamAccept(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	member, err := service.AcceptAdtyTeamInvite(c.GetInt("id"), body.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}

func PostAdtyMainTeamRegistration(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	ttl, ok := parseAdtyTeamInviteTTL(c)
	if !ok {
		return
	}
	issued, err := service.CreateAdtyMainTeamRegistration(c.GetInt("id"), ttl)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, issued)
}

func GetAdtyMainTeamRegistrations(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	rows, err := service.ListAdtyMainTeamRegistrations(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyMainTeamRegistrationRevoke(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	view, err := service.RevokeAdtyMainTeamRegistration(c.GetInt("id"), int64(id))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtyOwnerSiteTeamRegistration(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	ttl, ok := parseAdtyTeamInviteTTL(c)
	if !ok {
		return
	}
	issued, err := service.CreateAdtyOwnerSiteTeamRegistration(c.GetInt("id"), site.Id, ttl)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, issued)
}

func GetAdtyOwnerSiteTeamRegistrations(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	rows, err := service.ListAdtyOwnerSiteTeamRegistrations(c.GetInt("id"), site.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyOwnerSiteTeamRegistrationRevoke(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	view, err := service.RevokeAdtyOwnerSiteTeamRegistration(c.GetInt("id"), site.Id, int64(id))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtyTeamRegistrationComplete(c *gin.Context) {
	var body struct {
		Code     string `json:"code"`
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
		SiteId   *int   `json:"site_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	created, err := service.CompleteAdtyTeamRegistration(body.Code, body.Username, body.Password, body.Name, body.SiteId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}
