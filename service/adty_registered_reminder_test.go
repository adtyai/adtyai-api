package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisteredReminderUsesAccountSiteNotRequestSite(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := prepareSiteDB(t)
	_, accountSite := openActiveSite(t, db, "acct")
	_, requestSite := openActiveSite(t, db, "req")
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", accountSite.Id).Update("brand", `{"name":"Account Shop"}`).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", requestSite.Id).Update("brand", `{"name":"Request Shop"}`).Error)
	InvalidateAdtySiteCache()
	accountSite, err := GetAdtySite(accountSite.Id)
	require.NoError(t, err)
	requestSite, err = GetAdtySite(requestSite.Id)
	require.NoError(t, err)
	user := siteCustomer(t, db, accountSite, "remind-customer")
	ResetAdtyReminderThrottle()

	var gotFrom, gotContent string
	previous := adtySendFromAsync
	adtySendFromAsync = func(from, subject, to, content string) {
		gotFrom, gotContent = from, content
	}
	t.Cleanup(func() { adtySendFromAsync = previous })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/api/verification?email="+user.Email, nil)
	ctx.Set(AdtySiteContextKey, requestSite)
	require.True(t, AdtySendRegisteredReminder(ctx, user.Email))
	assert.Contains(t, gotContent, "Account Shop")
	assert.NotContains(t, gotContent, "Request Shop")
	assert.Contains(t, gotFrom, "Account Shop")
	assert.NotContains(t, gotFrom, "Request Shop")
}
