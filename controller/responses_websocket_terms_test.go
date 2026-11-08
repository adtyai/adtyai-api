package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesWSRechecksSiteTermsOnEveryCreate(t *testing.T) {
	user, token := setupResponsesWSRequestTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AdtySite{}, &model.AdtySiteCustomer{}))
	site := model.AdtySite{OwnerUserId: user.Id + 100, Subdomain: "ws-terms", RootDomain: "example.test", Status: model.AdtySiteStatusActive}
	require.NoError(t, model.DB.Create(&site).Error)
	require.NoError(t, model.DB.Model(user).Update("site_id", site.Id).Error)
	customer := model.AdtySiteCustomer{UserId: user.Id, SiteId: site.Id, TermsVersion: "7"}
	require.NoError(t, model.DB.Create(&customer).Error)
	legal := adty_setting.GetLegalSetting()
	previous := legal.SiteCustomerTermsVersion
	legal.SiteCustomerTermsVersion = 7
	t.Cleanup(func() { legal.SiteCustomerTermsVersion = previous })
	runner, _ := newResponsesWSTestRunner(t, token)
	called := false
	handle := func(*gin.Context) *types.NewAPIError { called = true; return nil }
	request := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
	}
	require.Nil(t, runner(request(), "accepted", handle))
	require.True(t, called)
	legal.SiteCustomerTermsVersion = 8
	called = false
	err := runner(request(), "stale", handle)
	require.NotNil(t, err)
	require.Equal(t, http.StatusForbidden, err.StatusCode)
	require.False(t, called, "stale terms must stop the event before relay and billing")
	require.NoError(t, model.DB.Model(&customer).Update("terms_version", "8").Error)
	require.Nil(t, runner(request(), "accepted-again", handle))
	require.True(t, called)
}
