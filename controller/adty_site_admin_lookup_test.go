package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminSiteSearchKeepsUsernameCase(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteEarningDaily{}, &model.AdtyTopupOrder{}, &model.TopUp{}))
	owner := &model.User{Username: "OwnerMix", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(&model.AdtySite{OwnerUserId: owner.Id, Subdomain: "mixsite", RootDomain: "example.com", Status: model.AdtySiteStatusActive}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/adty/admin/sites?keyword=OwnerMix", nil)
	GetAdtyAdminSites(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), "mixsite")
	require.Contains(t, recorder.Body.String(), "OwnerMix")
}

func TestRejectSiteIgnoresInvalidCostFactor(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteAudit{}))
	owner := &model.User{Username: "reject-owner", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(owner).Error)
	site := &model.AdtySite{OwnerUserId: owner.Id, Subdomain: "rejectme", RootDomain: "example.com", Status: model.AdtySiteStatusPending}
	require.NoError(t, db.Create(site).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/adty/admin/sites/"+strconv.Itoa(site.Id)+"/review", strings.NewReader(`{"approve":false,"cost_factor":"2","note":"no"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(site.Id)}}
	ctx.Set("id", owner.Id)
	ReviewAdtyAdminSite(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	require.Equal(t, model.AdtySiteStatusClosed, stored.Status)
}
