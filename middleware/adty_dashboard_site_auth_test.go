package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dashboardSiteMainHost  = "localhost"
	dashboardSiteAHost     = "shop-a.sites.example.com"
	dashboardSiteAAlias    = "alias.shop-a.example"
	dashboardSiteBHost     = "shop-b.sites.example.com"
	dashboardSiteProtected = "/protected"
	dashboardSiteSelfPath  = "/api/user/self"
	dashboardSiteStaffPath = "/api/adty/staff/self"
)

type dashboardSiteAuthFixture struct {
	siteA, siteB                                        *model.AdtySite
	mainUser, admin, root, ownerA, userA, userB, staffA *model.User
	mainSession, adminSession, rootSession              string
	ownerSession, sessionA, sessionB, staffSession      string
	patA, apiKey                                        string
}

func setupDashboardSiteAuthTest(t *testing.T) *dashboardSiteAuthFixture {
	t.Helper()
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})

	require.NoError(t, opened.DB.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.UserAccessToken{}, &model.Option{},
		&model.AuditLog{}, &model.Token{}, &model.AdtySite{}, &model.AdtySiteStaff{},
	))

	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	savedLegal := *adty_setting.GetLegalSetting()
	common.RedisEnabled = false
	common.SessionSecret = "dashboard-site-auth-test-secret"
	adty_setting.GetLegalSetting().SiteAgreementVersion = 3
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
		*adty_setting.GetLegalSetting() = savedLegal
	})
	require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))

	gin.SetMode(gin.TestMode)
	f := &dashboardSiteAuthFixture{}
	f.mainUser = createDashboardSiteUser(t, "main-user", common.RoleCommonUser, 0, 0)
	f.admin = createDashboardSiteUser(t, "main-admin", common.RoleAdminUser, 0, 0)
	f.root = createDashboardSiteUser(t, "main-root", common.RoleRootUser, 0, 0)
	f.ownerA = createDashboardSiteUser(t, "owner-a", common.RoleCommonUser, 0, 0)

	f.siteA = createDashboardSite(t, f.ownerA.Id, "shop-a")
	f.siteB = createDashboardSite(t, 0, "shop-b")
	f.userA = createDashboardSiteUser(t, "user-a", common.RoleCommonUser, f.siteA.Id, 0)
	f.userB = createDashboardSiteUser(t, "user-b", common.RoleCommonUser, f.siteB.Id, 0)
	f.staffA = createDashboardSiteUser(t, "staff-a", common.RoleCommonUser, f.siteA.Id, f.siteA.Id)
	require.NoError(t, model.DB.Create(&model.AdtySiteStaff{
		SiteId:      f.siteA.Id,
		OwnerUserId: f.ownerA.Id,
		UserId:      f.staffA.Id,
		Username:    f.staffA.Username,
		DisplayName: "Staff A",
		Role:        "support",
		Permissions: `["customers.read"]`,
		Status:      "enabled",
		Version:     1,
	}).Error)

	f.mainSession = issueDashboardSiteSession(t, f.mainUser.Id)
	f.adminSession = issueDashboardSiteSession(t, f.admin.Id)
	f.rootSession = issueDashboardSiteSession(t, f.root.Id)
	f.ownerSession = issueDashboardSiteSession(t, f.ownerA.Id)
	f.sessionA = issueDashboardSiteSession(t, f.userA.Id)
	f.sessionB = issueDashboardSiteSession(t, f.userB.Id)
	f.staffSession = issueDashboardSiteSession(t, f.staffA.Id)
	f.patA, _ = createMiddlewareScopedToken(t, f.userA.Id, 0, "profile:read")

	f.apiKey = "siteAuthRelayKey"
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         f.mainUser.Id,
		Key:            f.apiKey,
		Name:           "relay",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)
	return f
}

func createDashboardSiteUser(t *testing.T, username string, role, siteID, staffSiteID int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username, Password: "password-placeholder", Role: role,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		SiteId: siteID, StaffSiteId: staffSiteID,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func createDashboardSite(t *testing.T, ownerID int, subdomain string) *model.AdtySite {
	t.Helper()
	site := &model.AdtySite{
		OwnerUserId:  ownerID,
		Subdomain:    subdomain,
		RootDomain:   "sites.example.com",
		Status:       model.AdtySiteStatusActive,
		AgreementVer: service.AdtyCurrentSiteAgreementVersion(),
	}
	require.NoError(t, model.DB.Create(site).Error)
	return site
}

func issueDashboardSiteSession(t *testing.T, userID int) string {
	t.Helper()
	bundle, err := service.CreateLoginSession(userID, "password", "127.0.0.1", "dashboard-site-auth-test")
	require.NoError(t, err)
	require.NotEmpty(t, bundle.AccessToken)
	return bundle.AccessToken
}

func (f *dashboardSiteAuthFixture) attachRequestSite() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch service.AdtyNormalizeHost(c.Request.Host) {
		case dashboardSiteAHost, dashboardSiteAAlias:
			c.Set(service.AdtySiteContextKey, f.siteA)
		case dashboardSiteBHost:
			c.Set(service.AdtySiteContextKey, f.siteB)
		}
		c.Next()
	}
}

func (f *dashboardSiteAuthFixture) dashboardRouter(auth gin.HandlerFunc, path string) *gin.Engine {
	router := gin.New()
	router.Use(f.attachRequestSite())
	entered := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id"), "entered": true})
	}
	router.GET(path, auth, entered)
	return router
}

type dashboardSiteAuthBody struct {
	ID      int  `json:"id"`
	Entered bool `json:"entered"`
}

func dashboardSiteRequest(router *gin.Engine, path, host, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = host
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeDashboardSiteBody(t *testing.T, response *httptest.ResponseRecorder) dashboardSiteAuthBody {
	t.Helper()
	var body dashboardSiteAuthBody
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body
}

func TestDashboardSessionRejectedOnOtherSiteHosts(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(UserAuth(), dashboardSiteProtected)

	for _, host := range []string{dashboardSiteMainHost, dashboardSiteBHost} {
		response := dashboardSiteRequest(router, dashboardSiteProtected, host, f.sessionA)
		assert.Equal(t, http.StatusUnauthorized, response.Code, host)
		assert.Contains(t, response.Body.String(), "AUTH_UNAUTHORIZED", host)
		assert.NotContains(t, response.Body.String(), `"entered":true`, host)
	}
}

func TestDashboardSessionAcceptedOnOwnHostAndAlias(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(UserAuth(), dashboardSiteProtected)

	for _, host := range []string{dashboardSiteAHost, dashboardSiteAAlias} {
		response := dashboardSiteRequest(router, dashboardSiteProtected, host, f.sessionA)
		assert.Equal(t, http.StatusOK, response.Code, host)
		body := decodeDashboardSiteBody(t, response)
		assert.True(t, body.Entered, host)
		assert.Equal(t, f.userA.Id, body.ID, host)
	}
}

func TestMainSiteSessionsRejectedOnSubsiteHost(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	userRouter := f.dashboardRouter(UserAuth(), dashboardSiteProtected)
	adminRouter := f.dashboardRouter(AdminAuth(), dashboardSiteProtected)
	rootRouter := f.dashboardRouter(RootAuth(), dashboardSiteProtected)

	for _, tc := range []struct {
		name, token string
		router      *gin.Engine
		userID      int
	}{
		{name: "common main user", token: f.mainSession, router: userRouter, userID: f.mainUser.Id},
		{name: "admin", token: f.adminSession, router: adminRouter, userID: f.admin.Id},
		{name: "root", token: f.rootSession, router: rootRouter, userID: f.root.Id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok := dashboardSiteRequest(tc.router, dashboardSiteProtected, dashboardSiteMainHost, tc.token)
			assert.Equal(t, http.StatusOK, ok.Code)
			body := decodeDashboardSiteBody(t, ok)
			assert.True(t, body.Entered)
			assert.Equal(t, tc.userID, body.ID)

			denied := dashboardSiteRequest(tc.router, dashboardSiteProtected, dashboardSiteAHost, tc.token)
			assert.Equal(t, http.StatusUnauthorized, denied.Code)
			assert.Contains(t, denied.Body.String(), "AUTH_UNAUTHORIZED")
			assert.NotContains(t, denied.Body.String(), `"entered":true`)
		})
	}
}

func TestStaffSessionRejectedOnMainAndOtherSite(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(UserAuth(), dashboardSiteStaffPath)

	ok := dashboardSiteRequest(router, dashboardSiteStaffPath, dashboardSiteAHost, f.staffSession)
	assert.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	body := decodeDashboardSiteBody(t, ok)
	assert.True(t, body.Entered)
	assert.Equal(t, f.staffA.Id, body.ID)

	for _, host := range []string{dashboardSiteMainHost, dashboardSiteBHost} {
		response := dashboardSiteRequest(router, dashboardSiteStaffPath, host, f.staffSession)
		assert.Equal(t, http.StatusUnauthorized, response.Code, host)
		assert.Contains(t, response.Body.String(), "AUTH_UNAUTHORIZED", host)
		assert.NotContains(t, response.Body.String(), `"entered":true`, host)
	}

	// Same numeric comparison on the staff helper: main host is site_id 0,
	// not a skipped nil-host case.
	self := f.dashboardRouter(UserAuth(), dashboardSiteSelfPath)
	selfOK := dashboardSiteRequest(self, dashboardSiteSelfPath, dashboardSiteAHost, f.staffSession)
	assert.Equal(t, http.StatusOK, selfOK.Code, selfOK.Body.String())
	selfMain := dashboardSiteRequest(self, dashboardSiteSelfPath, dashboardSiteMainHost, f.staffSession)
	assert.Equal(t, http.StatusUnauthorized, selfMain.Code)
	assert.NotContains(t, selfMain.Body.String(), `"entered":true`)
}

func TestTryUserAuthDoesNotAttachCrossSiteSession(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(TryUserAuth(), dashboardSiteProtected)

	same := dashboardSiteRequest(router, dashboardSiteProtected, dashboardSiteAHost, f.sessionA)
	assert.Equal(t, http.StatusOK, same.Code)
	sameBody := decodeDashboardSiteBody(t, same)
	assert.True(t, sameBody.Entered)
	assert.Equal(t, f.userA.Id, sameBody.ID)

	for _, host := range []string{dashboardSiteMainHost, dashboardSiteBHost} {
		response := dashboardSiteRequest(router, dashboardSiteProtected, host, f.sessionA)
		assert.Equal(t, http.StatusOK, response.Code, host)
		body := decodeDashboardSiteBody(t, response)
		assert.True(t, body.Entered, host)
		assert.Zero(t, body.ID, "cross-site session must stay anonymous on %s: %s", host, response.Body.String())
	}
}

func TestDashboardPATIsSiteBoundAndRelayAPIKeyUsesTokenAuth(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	userRouter := f.dashboardRouter(UserAuth(), dashboardSiteSelfPath)

	ok := dashboardSiteRequest(userRouter, dashboardSiteSelfPath, dashboardSiteAHost, f.patA)
	assert.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	body := decodeDashboardSiteBody(t, ok)
	assert.True(t, body.Entered)
	assert.Equal(t, f.userA.Id, body.ID)

	for _, host := range []string{dashboardSiteMainHost, dashboardSiteBHost} {
		response := dashboardSiteRequest(userRouter, dashboardSiteSelfPath, host, f.patA)
		assert.Equal(t, http.StatusUnauthorized, response.Code, host)
		assert.Contains(t, response.Body.String(), "AUTH_UNAUTHORIZED", host)
		assert.NotContains(t, response.Body.String(), `"entered":true`, host)
	}

	// Relay / inference API keys go through TokenAuth, not authHelper.
	userDeniedKey := dashboardSiteRequest(userRouter, dashboardSiteSelfPath, dashboardSiteMainHost, f.apiKey)
	assert.Equal(t, http.StatusUnauthorized, userDeniedKey.Code)
	assert.NotContains(t, userDeniedKey.Body.String(), `"entered":true`)

	tokenRouter := f.dashboardRouter(TokenAuth(), "/v1/models")
	relayOK := dashboardSiteRequest(tokenRouter, "/v1/models", dashboardSiteMainHost, f.apiKey)
	assert.Equal(t, http.StatusOK, relayOK.Code, relayOK.Body.String())
	relayBody := decodeDashboardSiteBody(t, relayOK)
	assert.True(t, relayBody.Entered)
	assert.Equal(t, f.mainUser.Id, relayBody.ID)
}

func TestSiteOwnerSessionAllowedOnOwnSiteAndMain(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(UserAuth(), dashboardSiteProtected)

	for _, host := range []string{dashboardSiteMainHost, dashboardSiteAHost, dashboardSiteAAlias} {
		response := dashboardSiteRequest(router, dashboardSiteProtected, host, f.ownerSession)
		assert.Equal(t, http.StatusOK, response.Code, host)
		body := decodeDashboardSiteBody(t, response)
		assert.True(t, body.Entered, host)
		assert.Equal(t, f.ownerA.Id, body.ID, host)
	}

	denied := dashboardSiteRequest(router, dashboardSiteProtected, dashboardSiteBHost, f.ownerSession)
	assert.Equal(t, http.StatusUnauthorized, denied.Code)
	assert.Contains(t, denied.Body.String(), "AUTH_UNAUTHORIZED")
	assert.NotContains(t, denied.Body.String(), `"entered":true`)
}

func TestAdtyStaffDashboardAllowedRejectsNilHostAsMainSite(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	credCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	credCtx.Request = httptest.NewRequest(http.MethodGet, dashboardSiteStaffPath, nil)
	credCtx.Request.Header.Set("Authorization", "Bearer "+f.staffSession)
	user, identity, kind, err := classifyDashboardCredential(credCtx)
	require.NoError(t, err)
	require.Equal(t, dashboardCredentialInternal, kind)
	require.NotNil(t, user)

	denied := httptest.NewRecorder()
	deniedCtx, _ := gin.CreateTestContext(denied)
	deniedCtx.Request = httptest.NewRequest(http.MethodGet, dashboardSiteStaffPath, nil)
	deniedCtx.Request.Host = dashboardSiteMainHost
	setDashboardAuthContext(deniedCtx, user, identity, false)
	assert.False(t, adtyStaffDashboardAllowed(deniedCtx, user, identity), "nil host must be compared as site_id 0")
	assert.True(t, deniedCtx.IsAborted())
	assert.Equal(t, http.StatusForbidden, denied.Code)
	assert.Contains(t, denied.Body.String(), "Staff access is not permitted")
}

func TestTokenOrUserAuthRejectsCrossSiteDashboardSession(t *testing.T) {
	f := setupDashboardSiteAuthTest(t)
	router := f.dashboardRouter(TokenOrUserAuth(), dashboardSiteProtected)

	ok := dashboardSiteRequest(router, dashboardSiteProtected, dashboardSiteAHost, f.sessionA)
	assert.Equal(t, http.StatusOK, ok.Code)
	assert.Equal(t, f.userA.Id, decodeDashboardSiteBody(t, ok).ID)

	denied := dashboardSiteRequest(router, dashboardSiteProtected, dashboardSiteMainHost, f.sessionA)
	assert.Equal(t, http.StatusUnauthorized, denied.Code)
	assert.Contains(t, denied.Body.String(), "AUTH_UNAUTHORIZED")
	assert.NotContains(t, denied.Body.String(), `"entered":true`)

	relay := dashboardSiteRequest(router, dashboardSiteProtected, dashboardSiteMainHost, f.apiKey)
	assert.Equal(t, http.StatusOK, relay.Code, relay.Body.String())
	assert.Equal(t, f.mainUser.Id, decodeDashboardSiteBody(t, relay).ID)
}
