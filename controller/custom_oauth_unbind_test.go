package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUnbindReportsSuccessWhenUserRereadFails(t *testing.T) {
	user, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(user).Update("email", "unbind-reread@example.com").Error)
	require.NoError(t, model.DB.Create(&model.UserOAuthBinding{
		UserId: user.Id, ProviderId: 31, ProviderUserId: "linked-subject",
	}).Error)

	db := model.DB
	armed := false
	const deleteHook = "f095_arm_user_reread"
	const queryHook = "f095_fail_user_reread"
	require.NoError(t, db.Callback().Delete().After("gorm:delete").Register(deleteHook, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "user_oauth_bindings" {
			armed = true
		}
	}))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(queryHook, func(tx *gorm.DB) {
		if !armed || tx.Statement.Dest == nil {
			return
		}
		if err := tx.Statement.Parse(tx.Statement.Dest); err != nil || tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Table == "users" {
			armed = false
			_ = tx.AddError(errors.New("user read failed after unbind"))
		}
	}))
	t.Cleanup(func() {
		db.Callback().Delete().Remove(deleteHook)
		db.Callback().Query().Remove(queryHook)
	})

	payload, err := common.Marshal(service.AccountUnbindingContext{ProviderID: 31})
	require.NoError(t, err)
	operation := service.VerificationOperation{Scope: service.VerificationScopeAccountUnbind, Context: payload}
	proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
	handler := func(c *gin.Context) {
		c.Params = gin.Params{{Key: "provider_id", Value: "31"}}
		UnbindCustomOAuth(c)
	}
	response := securityEnrollmentRequest(http.MethodDelete, "/api/user/oauth/bindings/31", "", proof, identity, handler)
	require.Contains(t, response.Body.String(), `"success":true`)
	require.Contains(t, response.Body.String(), `"notification_warning":true`)
	require.NotContains(t, response.Body.String(), `"success":false`)

	var bindings int64
	require.NoError(t, model.DB.Model(&model.UserOAuthBinding{}).Where("user_id = ? AND provider_id = ?", user.Id, 31).Count(&bindings).Error)
	require.Zero(t, bindings)
}
