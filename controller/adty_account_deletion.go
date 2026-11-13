package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var sendAccountDeletionEmail = service.AdtySendSiteEmail

func accountDeletionCodeKey(userID int, email string) string {
	return fmt.Sprintf("%d:%s", userID, model.NormalizeEmail(email))
}

// SendAccountDeletionCode emails a single-use code bound to this account.
// A send failure leaves no usable code and does not delete the account.
func SendAccountDeletionCode(c *gin.Context) {
	setAuthNoStore(c)
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	email := model.NormalizeEmail(user.Email)
	if email == "" || !strings.Contains(email, "@") {
		common.ApiErrorT(c, "Add an email address before deleting this account")
		return
	}
	if err := common.VerificationStoreReady(); err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Please try again later"))
		return
	}
	code := common.GenerateVerificationCode(6)
	key := accountDeletionCodeKey(user.Id, email)
	if err := common.RegisterVerificationCodeWithKey(key, code, common.AccountDeletionPurpose); err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Please try again later"))
		return
	}
	site := service.AdtySiteForRequest(c)
	systemName := map[string]any{"SystemName": service.AdtyBrandFor(site).HTMLName()}
	subject := i18n.T(c, i18n.MsgEmailAccountDeletionSubject, systemName)
	content := fmt.Sprintf("<p>%s</p><p>%s</p><p>%s</p>",
		i18n.T(c, i18n.MsgEmailAccountDeletionGreeting, systemName),
		i18n.T(c, i18n.MsgEmailAccountDeletionCode, map[string]any{"Code": code}),
		i18n.T(c, i18n.MsgEmailAccountDeletionValidity, map[string]any{"Minutes": common.VerificationValidMinutes}))
	if err := sendAccountDeletionEmail(site, subject, email, content); err != nil {
		_, _ = common.ConsumeVerificationCodeWithKey(key, code, common.AccountDeletionPurpose)
		common.ApiErrorT(c, "Failed to send email")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{}})
}

func consumeAccountDeletionCode(c *gin.Context, userID int, email string) (string, time.Time, bool) {
	var body struct {
		Code string `json:"verification_code"`
	}
	if c.Request.Body != nil {
		_ = common.DecodeJson(c.Request.Body, &body)
	}
	code := strings.TrimSpace(body.Code)
	if code == "" || strings.TrimSpace(email) == "" {
		common.ApiErrorT(c, "Email confirmation code is invalid")
		return "", time.Time{}, false
	}
	issued, ok := common.ConsumeVerificationCodeWithKey(accountDeletionCodeKey(userID, email), code, common.AccountDeletionPurpose)
	if !ok {
		common.ApiErrorT(c, "Email confirmation code is invalid")
		return "", time.Time{}, false
	}
	return code, issued, true
}
