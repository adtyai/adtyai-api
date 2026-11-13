package controller

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func adtyInt64Param(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	return id, true
}

func PostAdtyTeamClose(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	if err := service.CloseAdtyTeam(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"closed": true})
}

func PostAdtyTeamProfile(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.UpdateAdtyTeamProfile(c.GetInt("id"), id, body.Name); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PostAdtyTeamLeave(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	if err := service.LeaveAdtyTeam(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"left": true})
}

func PostAdtyTeamDisableMember(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	memberID, err := strconv.Atoi(c.Param("userId"))
	if err != nil || memberID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.DisableAdtyTeamMember(c.GetInt("id"), id, memberID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"disabled": true})
}

func PostAdtyTeamBindKey(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		TokenId int `json:"token_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.TokenId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	_ = id
	if err := service.BindAdtyTeamKey(c.GetInt("id"), body.TokenId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"bound": true})
}

func PostAdtyTeamRevokeKey(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	tokenID, err := strconv.Atoi(c.Param("tokenId"))
	if err != nil || tokenID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.RevokeAdtyTeamKey(c.GetInt("id"), id, tokenID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"revoked": true})
}

func PostAdtyTeamTransfer(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		UserId int `json:"user_id"`
	}
	_ = common.DecodeJson(c.Request.Body, &body)
	if err := service.TransferAdtyTeamLeader(c.GetInt("id"), id, body.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"transferred": false})
}

func GetAdtyTelegramMe(c *gin.Context) {
	purpose := c.DefaultQuery("purpose", model.AdtyTelegramPurposeUser)
	if c.FullPath() != "" && strings.Contains(c.FullPath(), "/site/") {
		purpose = model.AdtyTelegramPurposeAdmin
	}
	view, err := service.GetAdtyTelegramBind(c.GetInt("id"), purpose)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtyTelegramBindStart(c *gin.Context) {
	purpose := model.AdtyTelegramPurposeUser
	if strings.Contains(c.FullPath(), "/site/") {
		if _, ok := adtyOwnerSiteOr403(c); !ok {
			return
		}
		purpose = model.AdtyTelegramPurposeAdmin
	}
	code, username, expires, err := service.StartAdtyTelegramBind(c.GetInt("id"), purpose)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"code": code, "bot_username": username, "expires_at": expires})
}

func PostAdtyTelegramBindConfirm(c *gin.Context) {
	var body struct {
		Purpose        string `json:"purpose"`
		Code           string `json:"code"`
		TelegramUserId int64  `json:"telegram_user_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if strings.Contains(c.FullPath(), "/site/") {
		if _, ok := adtyOwnerSiteOr403(c); !ok {
			return
		}
		body.Purpose = model.AdtyTelegramPurposeAdmin
	}
	if body.Purpose == "" {
		body.Purpose = model.AdtyTelegramPurposeUser
	}
	binding, err := service.ConfirmAdtyTelegramBind(c.GetInt("id"), body.Purpose, body.Code, body.TelegramUserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": binding.Status, "telegram_user_id": binding.TelegramUserId})
}

func PostAdtyTelegramWebhook(c *gin.Context) {
	var update service.AdtyTelegramUpdate
	if err := common.DecodeJson(c.Request.Body, &update); err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	reply, err := service.HandleAdtyTelegramUpdate(c.Param("purpose"), c.Param("secret"), update)
	if err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	chatID := int64(0)
	if update.Message != nil {
		chatID = update.Message.Chat.Id
	}
	c.JSON(http.StatusOK, gin.H{"method": "sendMessage", "chat_id": chatID, "text": reply})
}

func GetAdtyAdminTelegram(c *gin.Context) {
	bots, err := service.ListAdtyTelegramBots()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"bots": bots})
}

func PutAdtyAdminTelegram(c *gin.Context) {
	purpose := c.Param("purpose")
	var body struct {
		Token   string `json:"token"`
		Enabled bool   `json:"enabled"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if middleware.RequireSecurityProof(c, service.AdtyNoticeOperation(service.AdtyNoticeActionTelegramSave, purpose)) == nil {
		return
	}
	saved, err := service.SaveAdtyTelegramBot(purpose, body.Token, body.Enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.telegram.save", map[string]any{"purpose": purpose, "enabled": body.Enabled, "token_changed": body.Token != ""})
	common.ApiSuccess(c, saved)
}

func PostAdtyAdminTelegramTest(c *gin.Context) {
	purpose := c.Param("purpose")
	if middleware.RequireSecurityProof(c, service.AdtyNoticeOperation(service.AdtyNoticeActionTelegramTest, purpose)) == nil {
		return
	}
	if err := service.TestAdtyTelegramBot(purpose, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"tested": true})
}

func GetAdtyTickets(c *gin.Context) {
	rows, canNote, err := service.ListAdtyTickets(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"tickets": rows, "can_note": canNote})
}

func PostAdtyTicket(c *gin.Context) {
	var body service.AdtyTicketInput
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	body.Idempotency = c.GetHeader("Idempotency-Key")
	ticket, err := service.CreateAdtyTicket(c.GetInt("id"), body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ticket)
}

func GetAdtyTicket(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	detail, err := service.GetAdtyTicket(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func PostAdtyTicketReply(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	message, err := service.ReplyAdtyTicket(int64(c.GetInt("id")), id, body.Body, c.GetHeader("Idempotency-Key"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, message)
}

func PostAdtyTicketNote(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	message, err := service.NoteAdtyTicket(c.GetInt("id"), id, body.Body, c.GetHeader("Idempotency-Key"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, message)
}

func PostAdtyTicketStatus(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Status     string `json:"status"`
		Priority   string `json:"priority"`
		AssigneeId int    `json:"assignee_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SetAdtyTicketStatus(c.GetInt("id"), id, body.Status, body.Priority, body.AssigneeId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PostAdtyTicketImage(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		common.ApiErrorT(c, "Only images are accepted")
		return
	}
	image, err := service.AddAdtyTicketImage(c.GetInt("id"), id, raw)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": image.Id, "mime": image.Mime, "width": image.Width, "height": image.Height})
}

func GetAdtyTicketImage(c *gin.Context) {
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	imageID, ok := adtyInt64Param(c, "imageId")
	if !ok {
		return
	}
	path, mime, err := service.OpenAdtyTicketImage(c.GetInt("id"), id, imageID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", mime)
	c.File(path)
}

func GetAdtyTranslation(c *gin.Context) {
	view, err := service.GetAdtyTranslationConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtyTranslation(c *gin.Context) {
	var body service.AdtyTranslationInput
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if middleware.RequireSecurityProof(c, service.AdtyNoticeOperation(service.AdtyNoticeActionTranslation, body.Mode)) == nil {
		return
	}
	body.RequestHost = c.Request.Host
	view, err := service.SaveAdtyTranslationConfig(body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.translation.save", map[string]any{"mode": body.Mode, "model": body.ModelName})
	common.ApiSuccess(c, view)
}

func PostAdtyTranslationTest(c *gin.Context) {
	if middleware.RequireSecurityProof(c, service.AdtyNoticeOperation(service.AdtyNoticeActionTranslateTry, "translation")) == nil {
		return
	}
	view, err := service.TestAdtyTranslation(c.Request.Host)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtyTranslationRun(c *gin.Context) {
	var body struct {
		ObjectKey string `json:"object_key"`
		Source    string `json:"source"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	result, err := service.TranslateAdtyDynamic(body.ObjectKey, body.Source, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetAdtyTranslationUsage(c *gin.Context) {
	usage, err := service.AdtyTranslationUsageToday()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usage)
}

func GetAdtyGroupHealth(c *gin.Context) {
	usable := service.GetUserUsableGroups(c.GetString("group"))
	allowed := make([]string, 0, len(usable))
	for group := range usable {
		allowed = append(allowed, group)
	}
	rows, err := service.ListAdtyGroupHealthForUser(c.GetInt("id"), c.GetString("group"), c.Query("model"), allowed, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := "platform"
	if len(rows) > 0 {
		scope = rows[0].Scope
	}
	common.ApiSuccess(c, gin.H{"scope": scope, "groups": rows})
}

func PutAdtyGroupHealth(c *gin.Context) {
	var body struct {
		SuccessMin   string `json:"success_min"`
		StaleSeconds int    `json:"stale_seconds"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.PutAdtyGroupHealthConfig(body.SuccessMin, body.StaleSeconds); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.group_health.config", map[string]any{"success_min": body.SuccessMin})
	common.ApiSuccess(c, gin.H{"saved": true})
}

func PostAdtyGroupProbe(c *gin.Context) {
	var body struct {
		Group string `json:"group"`
		Model string `json:"model"`
		Ok    bool   `json:"ok"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.RecordAdtyGroupProbe(0, body.Group, body.Model, body.Ok, time.Now()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"recorded": true})
}
