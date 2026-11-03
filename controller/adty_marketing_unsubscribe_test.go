package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 旧退订链接只签用户 id。联系人表还留着改绑前的邮箱时，不能把后来占用这个邮箱的人一起退订。
func TestUnsubscribeTokenDoesNotMarkLaterEmailOccupant(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, model.MigrateAdtyTables())
	oldSecret := common.SessionSecret
	if strings.TrimSpace(common.CryptoSecret) == "" && strings.TrimSpace(oldSecret) == "" {
		common.SessionSecret = "unsubscribe-token-secret"
	}
	t.Cleanup(func() { common.SessionSecret = oldSecret })

	owner := &model.User{Username: "old-owner", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "moved@example.com", Quota: 1}
	later := &model.User{Username: "later-mail", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Email: "reused@example.com", Quota: 1}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(later).Error)
	_, err := model.SaveAdtyMarketingConsent(model.AdtyMarketingConsentInput{UserId: owner.Id, Email: "reused@example.com", Consent: true, Source: "register"})
	require.NoError(t, err)
	_, err = model.SaveAdtyMarketingConsent(model.AdtyMarketingConsentInput{UserId: later.Id, Email: "reused@example.com", Consent: true, Source: "register"})
	require.NoError(t, err)

	token := service.AdtyMarketingUnsubscribeToken(owner.Id)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/adty/marketing/unsubscribe?token="+url.QueryEscape(token), strings.NewReader("List-Unsubscribe=One-Click"))
	PostAdtyMarketingUnsubscribe(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	ownerContact, err := model.GetAdtyMarketingContact(owner.Id)
	require.NoError(t, err)
	require.NotNil(t, ownerContact)
	require.True(t, ownerContact.Unsubscribed)
	laterContact, err := model.GetAdtyMarketingContact(later.Id)
	require.NoError(t, err)
	require.NotNil(t, laterContact)
	require.False(t, laterContact.Unsubscribed)
	require.True(t, laterContact.Consent)
}
