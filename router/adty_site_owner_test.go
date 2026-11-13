package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 站长中心（任务 11.6）和站长能看到什么（任务 11.7）。

type ownerFixture struct {
	db     *gorm.DB
	engine *gin.Engine
	site   *model.AdtySite
	owner  model.User
	token  string
	other  *model.AdtySite
}

func TestOwnerStaffLifecycleAndScope(t *testing.T) {
	f := newOwnerFixture(t)
	mine := f.customer(t, f.site, "staff-customer", "private@example.com")
	theirs := f.customer(t, f.other, "staff-other", "other@example.com")
	create := `{"username":"employee-1","display_name":"Support One","password":"employee-password-42","role":"support"}`
	require.Equal(t, http.StatusForbidden, f.call(t, http.MethodPost, "/api/adty/site/staff", create, "").Code)
	proof := sessionProof(t, f.owner, "staff.create", f.site.Id)
	response := f.call(t, http.MethodPost, "/api/adty/site/staff", create, proof)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	staff := decodeAdtyData[struct {
		Id          int
		Permissions []string
		Version     int64
	}](t, response.Body)
	require.Positive(t, staff.Id)
	var employee model.User
	require.NoError(t, f.db.Where("username = ?", "employee-1").First(&employee).Error)
	assert.True(t, common.ValidatePasswordAndHash("employee-password-42", employee.Password))
	assert.NotContains(t, response.Body.String(), "employee-password-42")
	assert.NotContains(t, response.Body.String(), employee.Password)
	assert.ElementsMatch(t, []string{"customers.read"}, staff.Permissions)
	for _, token := range []string{createAdtyScopedToken(t, f.owner.Id, "wallet:read"), createAdtyScopedToken(t, employee.Id, "wallet:read")} {
		assert.Equal(t, http.StatusForbidden, f.siteJSON(t, http.MethodGet, "/api/adty/site/staff", "", token).Code)
		assert.Equal(t, http.StatusForbidden, f.siteJSON(t, http.MethodGet, "/api/adty/staff/self", "", token).Code)
	}
	assert.Contains(t, f.call(t, http.MethodPost, "/api/adty/site/staff", create, proof).Body.String(), "SECURITY_PROOF_CONSUMED")
	login, err := service.CreateLoginSession(employee.Id, "password", "127.0.0.1", "staff-test")
	require.NoError(t, err)
	self := f.siteJSON(t, http.MethodGet, "/api/user/self", "", login.AccessToken)
	require.Equal(t, http.StatusOK, self.Code, self.Body.String())
	assert.Contains(t, self.Body.String(), fmt.Sprintf(`"staff_site_id":%d`, f.site.Id))
	assert.Contains(t, self.Body.String(), `"adty_site_owner":false`)
	path := fmt.Sprintf("/api/adty/staff/customers/%d", mine.Id)
	list := f.siteJSON(t, http.MethodGet, "/api/adty/staff/customers?site_id="+fmt.Sprint(f.other.Id), "", login.AccessToken)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	assert.Contains(t, list.Body.String(), "staff-customer")
	for _, hidden := range []string{"staff-other", "employee-1", "private@example.com", `"balance"`, `"used_quota"`, `"tokens"`} {
		assert.NotContains(t, list.Body.String(), hidden)
	}
	assert.Equal(t, http.StatusNotFound, f.siteJSON(t, http.MethodGet, fmt.Sprintf("/api/adty/staff/customers/%d", theirs.Id), "", login.AccessToken).Code)
	for _, suffix := range []string{"/topups", "/logs"} {
		assert.Equal(t, http.StatusForbidden, f.siteJSON(t, http.MethodGet, path+suffix, "", login.AccessToken).Code)
	}
	for _, denied := range []string{"/api/adty/site/self", "/api/adty/site/staff", "/api/adty/site/earnings", "/api/adty/site/wallets", "/api/token/", "/api/user/topup/info", "/api/user/referral/profile"} {
		assert.Equal(t, http.StatusForbidden, f.siteJSON(t, http.MethodGet, denied, "", login.AccessToken).Code, denied)
	}
	staffPath := fmt.Sprintf("/api/adty/site/staff/%d", staff.Id)
	update := `{"display_name":"Finance One","role":"finance","permissions":["customers.read","customers.balance","customers.topups","customers.usage"],"status":"enabled","expected_version":1}`
	for _, wrongProof := range []string{sessionProof(t, f.owner, "staff.password", staff.Id), sessionProof(t, f.owner, "staff.update", staff.Id+1)} {
		assert.Equal(t, http.StatusForbidden, f.call(t, http.MethodPut, staffPath, update, wrongProof).Code)
	}
	response = f.call(t, http.MethodPut, staffPath, update, sessionProof(t, f.owner, "staff.update", staff.Id))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, http.StatusConflict, f.call(t, http.MethodPut, staffPath, update, sessionProof(t, f.owner, "staff.update", staff.Id)).Code)
	assert.Equal(t, http.StatusUnauthorized, f.siteJSON(t, http.MethodGet, path, "", login.AccessToken).Code)
	_, _, err = service.RefreshLoginSession(login.RefreshToken, login.Session.SID, "127.0.0.1", "staff-test")
	assert.Error(t, err, "old refresh cannot restore revoked permissions")
	login, err = service.CreateLoginSession(employee.Id, "password", "127.0.0.1", "staff-test")
	require.NoError(t, err)
	response = f.siteJSON(t, http.MethodGet, path, "", login.AccessToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"balance":1000000`)
	assert.NotContains(t, response.Body.String(), `"tokens"`)
	require.NoError(t, f.db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", mine.Id).Update("joined_at", 1000).Error)
	for _, createdAt := range []int64{999, 1001} {
		require.NoError(t, f.db.Create(&model.Log{UserId: mine.Id, CreatedAt: createdAt, Type: model.LogTypeConsume, ModelName: fmt.Sprintf("model-%d", createdAt), Quota: 123, TokenName: "secret-key-name", Content: "secret-request", Ip: "192.0.2.23", Other: `{"key":"sk-secret"}`}).Error)
		require.NoError(t, f.db.Create(&model.AdtyTopupOrder{UserId: mine.Id, SiteId: f.site.Id, TradeNo: fmt.Sprintf("staff-topup-%d", createdAt), CreatedAt: createdAt, CreditQuota: 456, PaidTxHash: "secret-paid-tx"}).Error)
	}
	for _, suffix := range []string{"/topups", "/logs"} {
		records := f.siteJSON(t, http.MethodGet, path+suffix, "", login.AccessToken)
		require.Equal(t, http.StatusOK, records.Code, records.Body.String())
		assert.Contains(t, records.Body.String(), `"total":1`)
		assert.Contains(t, records.Body.String(), "1001")
		for _, secret := range []string{"model-999", "staff-topup-999", "secret-paid-tx", "sk-secret", "secret-request", "192.0.2.23", "secret-key-name", `"content"`, `"other"`, `"token_name"`, `"ip"`} {
			assert.NotContains(t, records.Body.String(), secret)
		}
	}
	// Disabling and enabling invalidates every old JWT and refresh generation.
	for i, status := range []string{"disabled", "enabled"} {
		body := fmt.Sprintf(`{"display_name":"Finance One","role":"finance","status":%q,"expected_version":%d}`, status, i+2)
		response = f.call(t, http.MethodPut, staffPath, body, sessionProof(t, f.owner, "staff.update", staff.Id))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, http.StatusUnauthorized, f.siteJSON(t, http.MethodGet, path, "", login.AccessToken).Code)
	}
	login, err = service.CreateLoginSession(employee.Id, "password", "127.0.0.1", "staff-test")
	require.NoError(t, err)
	response = f.call(t, http.MethodPut, staffPath+"/password", `{"password":"replacement-password-42","expected_version":4}`, sessionProof(t, f.owner, "staff.password", staff.Id))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, http.StatusUnauthorized, f.siteJSON(t, http.MethodGet, path, "", login.AccessToken).Code)
	require.NoError(t, f.db.First(&employee, employee.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("replacement-password-42", employee.Password))
	assert.False(t, common.ValidatePasswordAndHash("employee-password-42", employee.Password))
	response = f.call(t, http.MethodDelete, staffPath, `{"expected_version":5}`, sessionProof(t, f.owner, "staff.delete", staff.Id))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = service.CreateLoginSession(employee.Id, "password", "127.0.0.1", "staff-test")
	assert.Error(t, err)
	audits := f.call(t, http.MethodGet, "/api/adty/site/staff/audits", "", "")
	require.Equal(t, http.StatusOK, audits.Code, audits.Body.String())
	assert.Contains(t, audits.Body.String(), "staff.create")
	assert.Contains(t, audits.Body.String(), "staff.customers.read")
	for _, secret := range []string{"employee-password-42", "replacement-password-42", "private@example.com", employee.Password} {
		assert.NotContains(t, audits.Body.String(), secret)
	}
}

func TestOwnerStaffRejectsForbiddenGrantsAndForeignOwner(t *testing.T) {
	f := newOwnerFixture(t)
	for _, grant := range []string{"customers.keys", "wallets.write", "customers.credit", "site.manage", "*"} {
		body := fmt.Sprintf(`{"username":"bad-grant","display_name":"Bad","password":"employee-password-42","role":"manager","permissions":[%q]}`, grant)
		response := f.call(t, http.MethodPost, "/api/adty/site/staff", body, sessionProof(t, f.owner, "staff.create", f.site.Id))
		assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	}
	var count int64
	require.NoError(t, f.db.Model(&model.User{}).Where("username = ?", "bad-grant").Count(&count).Error)
	assert.Zero(t, count)
	plain := adtySessionToken(t, f.db, "not-staff-owner", common.RoleCommonUser)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/staff", "", plain).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/staff/self", "", plain).Code)
	var otherOwner model.User
	require.NoError(t, f.db.First(&otherOwner, f.other.OwnerUserId).Error)
	foreign, err := service.CreateAdtyStaff(otherOwner.Id, f.other.Id, service.AdtyStaffInput{Username: "foreign-staff", DisplayName: "Other", Password: "employee-password-42", Role: "support"})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, f.call(t, http.MethodPut, fmt.Sprintf("/api/adty/site/staff/%d", foreign.Id), `{"expected_version":1}`, "").Code)
}

func TestOwnerStaffCustomerSecurityActions(t *testing.T) {
	f := newOwnerFixture(t)
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://main.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })
	sent := captureSiteMail(t)
	mine := f.customer(t, f.site, "security-customer", "customer+reset@example.com")
	other := f.customer(t, f.other, "security-other", "other@example.com")
	staff, err := service.CreateAdtyStaff(f.owner.Id, f.site.Id, service.AdtyStaffInput{Username: "security-staff", DisplayName: "Support", Password: "employee-password-42", Role: "support"})
	require.NoError(t, err)
	login, err := service.CreateLoginSession(staff.UserId, "password", "127.0.0.1", "staff-security")
	require.NoError(t, err)
	call := func(path, proof string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Host = f.siteHost()
		req.Header.Set("Authorization", "Bearer "+login.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Security-Proof", proof)
		response := httptest.NewRecorder()
		f.engine.ServeHTTP(response, req)
		return response
	}
	path := fmt.Sprintf("/api/adty/staff/customers/%d", mine.Id)
	for _, action := range []string{"password-reset", "disable"} {
		assert.Equal(t, http.StatusForbidden, call(path+"/"+action, "").Code, "write access defaults to denied")
	}
	input := fmt.Sprintf(`{"display_name":"Support","role":"support","permissions":["customers.read","customers.password","customers.disable"],"status":"enabled","expected_version":%d}`, staff.Version)
	response := f.call(t, http.MethodPut, fmt.Sprintf("/api/adty/site/staff/%d", staff.Id), input, sessionProof(t, f.owner, "staff.update", staff.Id))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	login, err = service.CreateLoginSession(staff.UserId, "password", "127.0.0.1", "staff-security")
	require.NoError(t, err)
	issue := func(action string, target int) string {
		binding, err := service.BindVerificationOperation(service.VerificationOperation{Scope: "site.staff.customer.manage", Context: []byte(fmt.Sprintf(`{"action":%q,"target":%d}`, action, target))})
		require.NoError(t, err)
		proof, _, err := service.IssueSecurityProof(service.AuthIdentity{UserID: staff.UserId, SessionID: login.Session.SID, UserAuthVersion: 2, SessionVersion: 1}, service.VerificationMethodPassword, binding)
		require.NoError(t, err)
		return proof
	}
	assert.Equal(t, http.StatusForbidden, call(path+"/password-reset", "").Code)
	for _, proof := range []string{issue("customer.disable", mine.Id), issue("customer.password.reset", mine.Id+1000)} {
		assert.Equal(t, http.StatusForbidden, call(path+"/password-reset", proof).Code)
	}
	assert.Equal(t, http.StatusNotFound, call(fmt.Sprintf("/api/adty/staff/customers/%d/password-reset", other.Id), "").Code)
	assert.Equal(t, http.StatusNotFound, call(fmt.Sprintf("/api/adty/staff/customers/%d/disable", staff.UserId), "").Code)
	for _, action := range []string{"password-reset", "disable"} {
		assert.Equal(t, http.StatusForbidden, f.siteJSON(t, http.MethodPost, path+"/"+action, `{}`, createAdtyScopedToken(t, staff.UserId, "wallet:write")).Code)
	}
	customerLogin, err := service.CreateLoginSession(mine.Id, "password", "127.0.0.1", "customer-security")
	require.NoError(t, err)
	proof := issue("customer.password.reset", mine.Id)
	response = call(path+"/password-reset", proof)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, sent(), 1)
	mail := sent()[0]
	assert.Equal(t, mine.Email, mail.to)
	assert.Contains(t, mail.content, "https://center.sites.example.com/user/reset#email=")
	assert.NotContains(t, response.Body.String(), mine.Email)
	assert.NotContains(t, response.Body.String(), "token=")
	assert.Equal(t, http.StatusForbidden, call(path+"/password-reset", proof).Code)
	var unchanged model.User
	require.NoError(t, f.db.First(&unchanged, mine.Id).Error)
	assert.Equal(t, mine.Password, unchanged.Password, "only the customer chooses a replacement password")
	assert.Equal(t, mine.AuthVersion, unchanged.AuthVersion, "requesting instructions does not log the customer out")
	code := regexp.MustCompile(`token=([a-zA-Z0-9_-]+)`).FindStringSubmatch(mail.content)
	require.Len(t, code, 2)
	resetBody := fmt.Sprintf(`{"email":%q,"token":%q,"password":"customer-selected-password-42"}`, mine.Email, code[1])
	reset := f.siteJSON(t, http.MethodPost, "/api/user/reset", resetBody, "")
	require.Equal(t, http.StatusOK, reset.Code, reset.Body.String())
	assert.Contains(t, reset.Body.String(), `"success":true`)
	assert.Equal(t, http.StatusUnauthorized, f.siteJSON(t, http.MethodGet, "/api/user/self", "", customerLogin.AccessToken).Code)
	require.NoError(t, f.db.First(&unchanged, mine.Id).Error)
	assert.True(t, common.ValidatePasswordAndHash("customer-selected-password-42", unchanged.Password))
	customerLogin, err = service.CreateLoginSession(mine.Id, "password", "127.0.0.1", "customer-security")
	require.NoError(t, err)
	response = call(path+"/disable", issue("customer.disable", mine.Id))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, http.StatusUnauthorized, f.siteJSON(t, http.MethodGet, "/api/user/self", "", customerLogin.AccessToken).Code)
	_, _, err = service.RefreshLoginSession(customerLogin.RefreshToken, customerLogin.Session.SID, "127.0.0.1", "customer-security")
	assert.Error(t, err)
	require.NoError(t, f.db.First(&unchanged, mine.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, unchanged.Status)
	assert.Equal(t, mine.Quota, unchanged.Quota)
	audits := f.call(t, http.MethodGet, "/api/adty/site/staff/audits", "", "")
	assert.Contains(t, audits.Body.String(), "staff.customer.password.request")
	assert.Contains(t, audits.Body.String(), "staff.customer.disable")
	for _, secret := range []string{code[1], mine.Email, unchanged.Password, "customer-selected-password-42"} {
		assert.NotContains(t, audits.Body.String(), secret)
	}
}

// sessionFor 给 adtySessionToken 建的用户签一个 proof（和 /api/verify 签的一样，方法是密码）。
func sessionProof(t *testing.T, user model.User, action string, target int) string {
	t.Helper()
	binding, err := service.BindVerificationOperation(service.AdtySiteOwnerOperation(action, target))
	require.NoError(t, err)
	identity := service.AuthIdentity{UserID: user.Id, SessionID: "sid-" + user.Username, UserAuthVersion: 1, SessionVersion: 1}
	proof, _, err := service.IssueSecurityProof(identity, service.VerificationMethodPassword, binding)
	require.NoError(t, err)
	return proof
}

func newOwnerFixture(t *testing.T) *ownerFixture {
	t.Helper()
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.AuthFlow{}, &model.TwoFA{}, &model.PasskeyCredential{},
		&model.AuditLog{}, &model.Log{}, &model.Option{}, &model.TopUp{}, &model.UserAccessToken{}, &model.AdtyUserFingerprint{}))
	require.NoError(t, model.MigrateAdtyTables())
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(2.0), new(5.0), new(20.0)
	oldPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldPerUnit })

	token := adtySessionToken(t, db, "center-owner", common.RoleCommonUser)
	var owner model.User
	require.NoError(t, db.Where("username = ?", "center-owner").First(&owner).Error)
	hash, err := common.HashAccountPassword("owner-password-1")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", owner.Id).Update("password", hash).Error)
	site, err := service.OpenAdtySite(owner.Id, "center", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	otherOwner := model.User{Username: "other-owner", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&otherOwner).Error)
	other, err := service.OpenAdtySite(otherOwner.Id, "rival", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	current := service.AdtyCurrentSiteAgreementVersion()
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id IN ?", []int{site.Id, other.Id}).Update("agreement_ver", current).Error)
	site.AgreementVer, other.AgreementVer = current, current

	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if requestSite := service.AdtySiteForHost(c.Request.Host); requestSite != nil {
			c.Set(service.AdtySiteContextKey, requestSite)
		}
		c.Next()
	})
	SetApiRouter(engine)
	return &ownerFixture{db: db, engine: engine, site: site, owner: owner, token: token, other: other}
}

func (f *ownerFixture) siteHost() string {
	return service.AdtySiteDisplayHost(f.site)
}

func (f *ownerFixture) siteJSON(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	return adtyJSONOnHost(t, f.engine, f.siteHost(), method, path, body, token)
}

func (f *ownerFixture) customer(t *testing.T, site *model.AdtySite, name, email string) model.User {
	t.Helper()
	user := model.User{Username: name, Email: email, Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		AuthVersion: 1, SiteId: site.Id, Quota: 1_000_000, UsedQuota: 250_000}
	require.NoError(t, f.db.Create(&user).Error)
	require.NoError(t, f.db.Create(&model.AdtySiteCustomer{UserId: user.Id, SiteId: site.Id, JoinedAt: time.Now().Unix()}).Error)
	return user
}

func (f *ownerFixture) call(t *testing.T, method, path, body, proof string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	if proof != "" {
		req.Header.Set("X-Security-Proof", proof)
	}
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, req)
	return recorder
}

func TestOwnerEndpointsRequireActiveSite(t *testing.T) {
	f := newOwnerFixture(t)
	assert.Equal(t, http.StatusOK, f.call(t, http.MethodGet, "/api/adty/site/self", "", "").Code)

	plain := adtySessionToken(t, f.db, "not-an-owner", common.RoleCommonUser)
	for _, path := range []string{"/api/adty/site/self", "/api/adty/site/stats", "/api/adty/site/earnings", "/api/adty/site/customers", "/api/adty/site/export?kind=earnings"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		recorder := httptest.NewRecorder()
		f.engine.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusForbidden, recorder.Code, path)
	}

	_, err := service.ScheduleAdtySiteClose(f.site.Id, 1, service.AdtySiteClosePlan{Immediate: true})
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, f.call(t, http.MethodGet, "/api/adty/site/self", "", "").Code, "a closed site has no owner center")
}

func TestOwnerCannotSeeOtherSiteCustomer(t *testing.T) {
	f := newOwnerFixture(t)
	mine := f.customer(t, f.site, "mine-1", "mine@example.com")
	theirs := f.customer(t, f.other, "theirs-1", "theirs@example.com")
	mainUser := model.User{Username: "main-1", Password: "x", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: "main@example.com"}
	require.NoError(t, f.db.Create(&mainUser).Error)

	assert.Equal(t, http.StatusOK, f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d", mine.Id), "", "").Code)
	for _, id := range []int{theirs.Id, mainUser.Id} {
		for _, suffix := range []string{"", "/logs", "/topups"} {
			resp := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d%s", id, suffix), "", "")
			assert.Equal(t, http.StatusNotFound, resp.Code, "%d%s", id, suffix)
		}
	}
	list := f.call(t, http.MethodGet, "/api/adty/site/customers", "", "")
	assert.Contains(t, list.Body.String(), "mine-1")
	assert.NotContains(t, list.Body.String(), "theirs-1")
	assert.NotContains(t, list.Body.String(), "main-1")
	lookup := f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=theirs@example.com", "", "")
	assert.Contains(t, lookup.Body.String(), `"success":false`)
	report := f.call(t, http.MethodPost, "/api/adty/site/report", fmt.Sprintf(`{"user_id":%d,"reason":"abuse"}`, theirs.Id), "")
	assert.Contains(t, report.Body.String(), `"success":false`, "cannot report another site's customer")
}

func TestOwnerOrdersUseFrozenSiteScope(t *testing.T) {
	f := newOwnerFixture(t)
	mine := f.customer(t, f.site, "order-mine", "order-mine@example.com")
	theirs := f.customer(t, f.other, "order-theirs", "order-theirs@example.com")
	for _, fixture := range []struct {
		trade         string
		siteID, buyer int
		face          int64
	}{
		{"order-before-move", f.site.Id, mine.Id, 3}, {"order-other-site", f.other.Id, theirs.Id, 91},
		{"order-main-site", 0, mine.Id, 13}, {"order-unknown-site", -1, mine.Id, 17},
	} {
		require.NoError(t, f.db.Create(&model.TopUp{TradeNo: fixture.trade, UserId: fixture.buyer, Status: common.TopUpStatusSuccess,
			PaymentProvider: model.PaymentProviderStripe, Amount: fixture.face}).Error)
		if fixture.siteID >= 0 {
			require.NoError(t, f.db.Create(&model.AdtyTopupOrder{TradeNo: fixture.trade, UserId: fixture.buyer,
				SiteId: fixture.siteID, Provider: model.PaymentProviderStripe, FaceUSD: decimal.NewFromInt(fixture.face)}).Error)
		}
	}
	require.NoError(t, f.db.Model(&mine).Update("site_id", f.other.Id).Error)
	require.NoError(t, f.db.Model(&theirs).Update("site_id", f.site.Id).Error)
	response := f.call(t, http.MethodGet, "/api/adty/site/orders", "", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	listed := decodeAdtyData[struct {
		Total int
		Items []struct {
			TradeNo string `json:"trade_no"`
			SiteID  int    `json:"site_id"`
			Actions struct{ Complete, Refund, Query bool }
		}
	}](t, response.Body)
	require.Equal(t, 1, listed.Total)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, "order-before-move", listed.Items[0].TradeNo)
	assert.Equal(t, f.site.Id, listed.Items[0].SiteID)
	assert.False(t, listed.Items[0].Actions.Complete)
	assert.False(t, listed.Items[0].Actions.Refund)
	assert.False(t, listed.Items[0].Actions.Query)
	for _, suffix := range []string{""} {
		path := "/api/adty/site/orders" + suffix
		read := createAdtyScopedToken(t, f.owner.Id, "wallet:read")
		write := createAdtyScopedToken(t, f.owner.Id, "wallet:write")
		allowed := adtyJSON(t, f.engine, http.MethodGet, path, "", read)
		require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
		assert.Contains(t, allowed.Body.String(), `"success":true`)
		assert.NotContains(t, allowed.Body.String(), "order-other-site")
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", write).Code)
		assert.Equal(t, http.StatusUnauthorized, adtyJSON(t, f.engine, http.MethodGet, path, "", "").Code)
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", adtySessionToken(t, f.db, "order-plain"+suffix, common.RoleCommonUser)).Code)
		spoofed := f.call(t, http.MethodGet, fmt.Sprintf("%s?site_id=%d", path, f.other.Id), "", "")
		require.Equal(t, http.StatusOK, spoofed.Code, spoofed.Body.String())
		assert.Contains(t, spoofed.Body.String(), `"success":true`)
		assert.NotContains(t, spoofed.Body.String(), "order-other-site")
		assert.NotContains(t, spoofed.Body.String(), `"91"`)
	}
	summaryResponse := f.call(t, http.MethodGet, "/api/adty/site/orders/summary", "", "")
	assert.Equal(t, http.StatusForbidden, summaryResponse.Code)
	assert.NotContains(t, summaryResponse.Body.String(), "credited_usd")
	assert.NotContains(t, summaryResponse.Body.String(), "bonus_usd")
	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/orders", "", "").Body.String(), "order-before-move")
	for _, status := range []string{model.AdtySiteStatusPaused, model.AdtySiteStatusClosing} {
		require.NoError(t, f.db.Model(f.site).Update("status", status).Error)
		assert.Equal(t, http.StatusOK, f.call(t, http.MethodGet, "/api/adty/site/orders", "", "").Code)
	}
	require.NoError(t, f.db.Model(f.site).Update("agreement_ver", "old").Error)
	assert.Equal(t, http.StatusForbidden, f.call(t, http.MethodGet, "/api/adty/site/orders", "", "").Code)
}

func TestOrderDetailsRespectFrozenSiteAndRedactPrivateEvidence(t *testing.T) {
	f := newOwnerFixture(t)
	buyer := f.customer(t, f.site, "trace-buyer", "trace@example.com")
	for _, order := range []struct {
		trade  string
		siteID int
	}{{"trace-mine", f.site.Id}, {"trace-other", f.other.Id}, {"trace-main", 0}} {
		require.NoError(t, f.db.Create(&model.TopUp{TradeNo: order.trade, UserId: buyer.Id, PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusSuccess}).Error)
		require.NoError(t, f.db.Create(&model.AdtyTopupOrder{TradeNo: order.trade, UserId: buyer.Id, SiteId: order.siteID, Provider: model.PaymentProviderStripe,
			PaidSource: model.AdtyPaymentSourceManual, PaidTxHash: "private-manual-proof", ProviderPayload: "private-provider-payload", RefundReason: "private-refund-reason"}).Error)
	}
	require.NoError(t, f.db.Model(&buyer).Update("site_id", f.other.Id).Error)
	require.NoError(t, f.db.Create(&model.AdtyBalanceLedger{UserId: buyer.Id, Type: "refund", RefType: "topup", RefId: "trace-mine", DeltaQuota: -123, Reason: "private-admin-reason", OperatorId: 41}).Error)
	require.NoError(t, f.db.Create(&model.AdtyBalanceLedger{UserId: buyer.Id, Type: "manual_adjust", RefType: "adjustment", RefId: "unrelated-adjustment", Reason: "unrelated-private-funds"}).Error)
	path := "/api/adty/site/orders/trace-mine?section=wallet"
	response := f.call(t, http.MethodGet, path, "", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"success":true`)
	assert.Contains(t, response.Body.String(), `"delta_quota":-123`)
	for _, secret := range []string{"private-manual-proof", "private-provider-payload", "private-refund-reason", "private-admin-reason", "unrelated-private-funds"} {
		assert.NotContains(t, response.Body.String(), secret)
	}
	assert.NotContains(t, response.Body.String(), "balance_before")
	assert.NotContains(t, response.Body.String(), "balance_after")
	assert.NotContains(t, f.call(t, http.MethodGet, "/api/adty/site/orders", "", "").Body.String(), "private-manual-proof")
	for _, section := range []string{"commissions", "commission_movements"} {
		assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/orders/trace-mine?section="+section, "", "").Body.String(), `"success":false`)
	}
	for _, trade := range []string{"trace-other", "trace-main", "missing-order"} {
		assert.Equal(t, http.StatusNotFound, f.call(t, http.MethodGet, "/api/adty/site/orders/"+trade, "", "").Code)
	}
	read := createAdtyScopedToken(t, f.owner.Id, "wallet:read")
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, path, "", read).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", createAdtyScopedToken(t, f.owner.Id, "wallet:write")).Code)
	assert.Equal(t, http.StatusUnauthorized, adtyJSON(t, f.engine, http.MethodGet, path, "", "").Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", adtySessionToken(t, f.db, "trace-plain", common.RoleCommonUser)).Code)
	require.NoError(t, f.db.Model(f.site).Update("agreement_ver", "old").Error)
	assert.Equal(t, http.StatusForbidden, f.call(t, http.MethodGet, path, "", "").Code)
}

func TestOwnerWalletReadAPIsStayWithinSite(t *testing.T) {
	f := newOwnerFixture(t)
	for _, credit := range []struct {
		siteID int
		kind   string
		amount int64
		key    string
	}{
		{f.site.Id, model.AdtySiteWalletOperating, 12, "mine-operating-credit"},
		{f.site.Id, model.AdtySiteWalletEarnings, 5, "mine-earning-credit"},
		{f.site.Id, model.AdtySiteWalletOperating, -2, "mine-operating-debit"},
		{f.other.Id, model.AdtySiteWalletOperating, 91, "other-private-credit"},
	} {
		_, err := service.ApplyAdtySiteWalletChange(credit.siteID, credit.kind, decimal.NewFromInt(credit.amount), service.AdtySiteWalletChange{
			Type: "test", Reason: credit.key, IdempotencyKey: "private-idem-" + credit.key,
		})
		require.NoError(t, err)
	}
	response := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/wallets?site_id=%d", f.other.Id), "", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	wallets := decodeAdtyData[[]model.AdtySiteWallet](t, response.Body)
	require.Len(t, wallets, 2)
	balances := map[string]string{}
	for _, wallet := range wallets {
		assert.Equal(t, f.site.Id, wallet.SiteId)
		balances[wallet.Kind] = wallet.Balance.String()
	}
	assert.Equal(t, map[string]string{"earnings": "5", "margin": "0"}, balances)
	assert.NotContains(t, response.Body.String(), `"operating"`)
	var storedOperating int64
	require.NoError(t, f.db.Model(&model.AdtySiteWallet{}).Where("site_id = ? AND kind = ?", f.site.Id, model.AdtySiteWalletOperating).Count(&storedOperating).Error)
	assert.EqualValues(t, 1, storedOperating, "hiding the operating wallet must not delete its ledger account")

	deniedOperating := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/wallet-ledgers?kind=operating&site_id=%d", f.other.Id), "", "")
	assert.Equal(t, http.StatusForbidden, deniedOperating.Code)
	assert.NotContains(t, deniedOperating.Body.String(), "mine-operating-debit")

	path := fmt.Sprintf("/api/adty/site/wallet-ledgers?kind=earnings&site_id=%d&page_size=1", f.other.Id)
	for page, want := range []string{"mine-earning-credit"} {
		response := f.call(t, http.MethodGet, fmt.Sprintf("%s&p=%d", path, page+1), "", "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		body := decodeAdtyData[struct {
			Total int64                        `json:"total"`
			Items []model.AdtySiteWalletLedger `json:"items"`
		}](t, response.Body)
		assert.EqualValues(t, 1, body.Total)
		require.Len(t, body.Items, 1)
		assert.Equal(t, f.site.Id, body.Items[0].SiteId)
		assert.Equal(t, want, body.Items[0].Reason)
		assert.NotContains(t, response.Body.String(), "private-idem-")
		assert.NotContains(t, response.Body.String(), "other-private-credit")
	}
	invalid := f.call(t, http.MethodGet, "/api/adty/site/wallet-ledgers?kind=unknown", "", "")
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	plain := adtySessionToken(t, f.db, "wallet-not-owner", common.RoleCommonUser)
	for _, path := range []string{"/api/adty/site/wallets", "/api/adty/site/wallet-ledgers"} {
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", plain).Code)
	}
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("agreement_ver", "outdated").Error)
	assert.Equal(t, http.StatusForbidden, f.call(t, http.MethodGet, "/api/adty/site/wallets", "", "").Code)
}

func TestRootWalletReadAPIsRequireRootAndReadScope(t *testing.T) {
	f := newAdtyGuardFixture(t)
	site := model.AdtySite{OwnerUserId: f.admin.Id, Subdomain: "root-wallet", Status: model.AdtySiteStatusClosed}
	require.NoError(t, f.db.Create(&site).Error)
	var root model.User
	require.NoError(t, f.db.Where("username = ?", "guard-root").First(&root).Error)
	rootRead := createAdtyScopedToken(t, root.Id, "option:read")
	rootWrite := createAdtyScopedToken(t, root.Id, "option:write")
	for _, suffix := range []string{"wallets", "wallet-ledgers"} {
		path := fmt.Sprintf("/api/adty/admin/sites/%d/%s", site.Id, suffix)
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", f.token).Code)
		for _, token := range []string{f.root, rootRead} {
			response := adtyJSON(t, f.engine, http.MethodGet, path, "", token)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), `"success":true`)
		}
		assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, path, "", rootWrite).Code)
		missing := adtyJSON(t, f.engine, http.MethodGet, fmt.Sprintf("/api/adty/admin/sites/999999/%s", suffix), "", f.root)
		assert.Equal(t, http.StatusNotFound, missing.Code)
	}
	var count int64
	require.NoError(t, f.db.Model(&model.AdtySiteWallet{}).Where("site_id = ?", site.Id).Count(&count).Error)
	assert.Zero(t, count, "reading a legacy site must not create balances or history")
	ownerRead := createAdtyScopedToken(t, f.admin.Id, "wallet:read")
	ownerWrite := createAdtyScopedToken(t, f.admin.Id, "wallet:write")
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
		"status": model.AdtySiteStatusActive, "agreement_ver": service.AdtyCurrentSiteAgreementVersion(),
	}).Error)
	assert.Equal(t, http.StatusOK, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/wallets", "", ownerRead).Code)
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodGet, "/api/adty/site/wallets", "", ownerWrite).Code)
}

func TestRootTierConfigurationIsDynamicAndValidated(t *testing.T) {
	f := newAdtyGuardFixture(t)
	const path = "/api/adty/admin/reseller/tiers"
	const valid = `{"tiers":[{"code":"configurable","name":"Configurable","cost_factor":"0.7","deposit_usd":"20","enabled":true}]}`
	assert.Equal(t, http.StatusForbidden, adtyJSON(t, f.engine, http.MethodPut, path, valid, f.token).Code)
	response := adtyJSON(t, f.engine, http.MethodPut, path, valid, f.root)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	tiers := decodeAdtyData[[]model.AdtySiteTier](t, response.Body)
	require.Len(t, tiers, 1)
	assert.Equal(t, "configurable", tiers[0].Code)
	require.NotNil(t, tiers[0].CostFactor)
	assert.Equal(t, "0.7", tiers[0].CostFactor.String())
	for _, invalid := range []string{
		`{}`,
		`{"tiers":[{"code":"configurable","name":"Configurable","cost_factor":"0.0000001"}]}`,
		`{"tiers":[{"code":"configurable","name":"Configurable","deposit_usd":"-1"}]}`,
	} {
		response := adtyJSON(t, f.engine, http.MethodPut, path, invalid, f.root)
		assert.Contains(t, response.Body.String(), `"success":false`)
	}
	response = adtyJSON(t, f.engine, http.MethodGet, path, "", f.root)
	tiers = decodeAdtyData[[]model.AdtySiteTier](t, response.Body)
	require.Len(t, tiers, 1)
	assert.True(t, tiers[0].Enabled)
	assert.Equal(t, "20", tiers[0].DepositUSD.String())
	assert.Equal(t, "0.7", tiers[0].CostFactor.String(), "invalid updates must not partially change a tier")
}

func TestSensitiveOwnerActionsRequireVerify(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "verify-customer", "v@example.com")
	require.NoError(t, f.db.Model(&model.ReferralCommissionAccount{}).Where("user_id = ?", f.owner.Id).Update("available_amount", 100).Error)
	domain := model.AdtySiteDomain{SiteId: f.site.Id, Hostname: "api.verify-brand.com", Status: "pending", CreatedAt: time.Now().Unix()}
	require.NoError(t, f.db.Create(&domain).Error)
	promo := model.AdtyPromoCode{Code: "VERIFY-EDIT", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: f.site.Id,
		Funder: model.AdtyPromoFunderSiteOwner, Enabled: true, CreatedBy: f.owner.Id}
	require.NoError(t, f.db.Create(&promo).Error)
	doomed := model.AdtyPromoCode{Code: "VERIFY-DROP", Kind: model.AdtyPromoKindPercent, Value: decimal.NewFromInt(1), SiteId: f.site.Id,
		Funder: model.AdtyPromoFunderSiteOwner, Enabled: true, CreatedBy: f.owner.Id}
	require.NoError(t, f.db.Create(&doomed).Error)
	// 没有 Resend 域名 id 的发件域名，删除时不调 Resend（测试不联网）。
	require.NoError(t, f.db.Create(&model.AdtySiteSenderDomain{SiteId: f.site.Id, Domain: "mail.verify-brand.com", Status: model.AdtySenderDomainPending, CreatedAt: time.Now().Unix()}).Error)
	cases := []struct {
		name, method, path, body, action string
		target                           int
	}{
		{"site pricing", http.MethodPut, "/api/adty/site/pricing", `{"default_markup":"1.2","expected_version":0}`, service.AdtySiteOwnerActionPricing, 0},
		{"customer pricing", http.MethodPut, fmt.Sprintf("/api/adty/site/customers/%d/pricing", customer.Id), `{"default_markup":"1.1","expected_version":0}`, service.AdtySiteOwnerActionPricing, customer.Id},
		{"bonus", http.MethodPut, "/api/adty/site/self", `{"bonus_rates":{"100":2}}`, service.AdtySiteOwnerActionBonus, f.site.Id},
		{"trial", http.MethodPost, fmt.Sprintf("/api/adty/site/customers/%d/trial", customer.Id), `{"amount_usd":"1","idempotency_key":"k1"}`, service.AdtySiteOwnerActionTrial, customer.Id},
		{"domain delete", http.MethodDelete, fmt.Sprintf("/api/adty/site/domains/%d", domain.Id), "", service.AdtySiteOwnerActionDomain, domain.Id},
		// 发件域名（11.3）：一个站最多一个，target 为站点 id。
		{"sender domain delete", http.MethodDelete, "/api/adty/site/sender-domain", "", service.AdtySiteOwnerActionSender, f.site.Id},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing := f.call(t, tc.method, tc.path, tc.body, "")
			assert.Equal(t, http.StatusForbidden, missing.Code)
			assert.Contains(t, missing.Body.String(), "SECURITY_PROOF_REQUIRED")

			wrongTarget := f.call(t, tc.method, tc.path, tc.body, sessionProof(t, f.owner, tc.action, tc.target+1000))
			assert.Contains(t, wrongTarget.Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH", "a proof for another target is rejected")
			wrongAction := service.AdtySiteOwnerActionTrial
			if tc.action == wrongAction {
				wrongAction = service.AdtySiteOwnerActionPricing
			}
			mismatch := f.call(t, tc.method, tc.path, tc.body, sessionProof(t, f.owner, wrongAction, max(tc.target, 1)))
			assert.Contains(t, mismatch.Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH", "a proof for another action is rejected")

			proof := sessionProof(t, f.owner, tc.action, tc.target)
			resp := f.call(t, tc.method, tc.path, tc.body, proof)
			assert.NotContains(t, resp.Body.String(), "SECURITY_PROOF", resp.Body.String())
			replay := f.call(t, tc.method, tc.path, tc.body, proof)
			assert.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED", "a proof works once")
		})
	}
	var stored model.User
	require.NoError(t, f.db.First(&stored, customer.Id).Error)
	assert.EqualValues(t, 1_000_000+500_000, stored.Quota, "the trial was granted exactly once")
	var prices int64
	require.NoError(t, f.db.Model(&model.AdtySitePrice{}).Where("site_id = ?", f.site.Id).Count(&prices).Error)
	assert.EqualValues(t, 2, prices)
	retired := f.call(t, http.MethodPost, "/api/adty/site/promo-codes", `{"code":"verify-new","kind":"percent","value":"1","enabled":true}`, sessionProof(t, f.owner, service.AdtySiteOwnerActionPromo, 0))
	assert.Contains(t, retired.Body.String(), "Promo codes are not available")
	var created, dropped int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("code = ?", "VERIFY-NEW").Count(&created).Error)
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("id = ?", doomed.Id).Count(&dropped).Error)
	assert.Zero(t, created, "a retired promo write must not create a code")
	assert.EqualValues(t, 1, dropped, "a retired delete must not remove a historical code")
	require.NoError(t, f.db.First(&promo, promo.Id).Error)
	assert.True(t, promo.Enabled)
	var sender model.AdtySiteSenderDomain
	require.NoError(t, f.db.First(&sender, "site_id = ?", f.site.Id).Error)
	assert.Equal(t, model.AdtySenderDomainDisabled, sender.Status)
	foreign := f.call(t, http.MethodDelete, "/api/adty/site/promo-codes/999999", "", sessionProof(t, f.owner, service.AdtySiteOwnerActionPromo, 999999))
	assert.Contains(t, foreign.Body.String(), "Promo codes are not available")

	// 品牌和 SEO 开关不需要二次验证；品牌字段限制字数。
	assert.Contains(t, f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"name":"Center AI"},"seo_index":true}`, "").Body.String(), `"success":true`)
	tooLong := f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"name":"`+strings.Repeat("长", 41)+`"}}`, "")
	assert.Contains(t, tooLong.Body.String(), `"success":false`)
	external := f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"logo_url":"https://evil.example/x.svg"}}`, "")
	assert.Contains(t, external.Body.String(), `"success":false`)
}

func TestClosingSiteRejectsBrandSeoAndReport(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "closing-customer", "close@example.com")
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("status", model.AdtySiteStatusClosing).Error)
	require.NoError(t, f.db.Create(&model.AdtySiteDomain{SiteId: f.site.Id, Hostname: "api.closing-profile.example", Status: model.AdtySiteDomainPending, CreatedAt: time.Now().Unix()}).Error)
	service.InvalidateAdtySiteCache()

	brand := f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"name":"Closing AI"},"seo_index":true}`, "")
	assert.Contains(t, brand.Body.String(), "This site is closing and can only be viewed")
	report := f.call(t, http.MethodPost, "/api/adty/site/report", fmt.Sprintf(`{"user_id":%d,"reason":"abuse"}`, customer.Id), "")
	assert.Contains(t, report.Body.String(), "This site is closing and can only be viewed")

	var stored model.AdtySite
	require.NoError(t, f.db.First(&stored, f.site.Id).Error)
	assert.NotContains(t, stored.Brand, "Closing AI")
	assert.False(t, stored.SeoIndex)
	var reports int64
	require.NoError(t, f.db.Model(&model.AdtySiteReport{}).Where("site_id = ?", f.site.Id).Count(&reports).Error)
	assert.Zero(t, reports)
	profile := f.call(t, http.MethodGet, "/api/adty/site/self", "", "").Body.String()
	assert.Contains(t, profile, `"success":true`)
	assert.Contains(t, profile, `"status":"closing"`)
	assert.Contains(t, profile, "api.closing-profile.example")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	service.InvalidateAdtySiteCache()
	assert.Contains(t, f.call(t, http.MethodPut, "/api/adty/site/self", `{"brand":{"name":"Paused AI"},"seo_index":true}`, "").Body.String(), `"success":true`)
	assert.Contains(t, f.call(t, http.MethodPost, "/api/adty/site/report", fmt.Sprintf(`{"user_id":%d,"reason":"abuse"}`, customer.Id), "").Body.String(), `"success":true`)
	require.NoError(t, f.db.First(&stored, f.site.Id).Error)
	assert.Contains(t, stored.Brand, "Paused AI")
	assert.True(t, stored.SeoIndex)
	require.NoError(t, f.db.Model(&model.AdtySiteReport{}).Where("site_id = ?", f.site.Id).Count(&reports).Error)
	assert.EqualValues(t, 1, reports)
}

func TestOwnerExportRejectsWhenOverRowLimit(t *testing.T) {
	f := newOwnerFixture(t)
	old := controller.AdtyOwnerExportMaxRows
	controller.AdtyOwnerExportMaxRows = 2
	t.Cleanup(func() { controller.AdtyOwnerExportMaxRows = old })
	today := time.Now().Format("2006-01-02")
	require.NoError(t, f.db.Create(&[]model.AdtySiteEarningDaily{
		{SiteId: f.site.Id, UserId: 1, Day: today + "+0", Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(1), Earning: decimal.NewFromInt(1), Status: model.AdtySiteEarningPending},
		{SiteId: f.site.Id, UserId: 1, Day: today + "+1", Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(9), Earning: decimal.NewFromInt(9), Status: model.AdtySiteEarningPending},
	}).Error)
	within := f.call(t, http.MethodGet, "/api/adty/site/export?kind=earnings", "", "")
	require.Equal(t, http.StatusOK, within.Code)
	assert.Contains(t, within.Body.String(), today+"+1")
	assert.Contains(t, within.Header().Get("Content-Type"), "text/csv")

	require.NoError(t, f.db.Create(&model.AdtySiteEarningDaily{
		SiteId: f.site.Id, UserId: 1, Day: today + "+2", Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(3), Earning: decimal.NewFromInt(3), Status: model.AdtySiteEarningPending,
	}).Error)
	over := f.call(t, http.MethodGet, "/api/adty/site/export?kind=earnings", "", "")
	assert.Contains(t, over.Body.String(), "This export has more than")
	assert.Contains(t, over.Body.String(), `"success":false`)
	assert.NotContains(t, over.Body.String(), "revenue_usd")
	assert.NotContains(t, over.Header().Get("Content-Type"), "text/csv")

	f.customer(t, f.site, "export-a", "a@example.com")
	f.customer(t, f.site, "export-b", "b@example.com")
	f.customer(t, f.site, "export-c", "c@example.com")
	customers := f.call(t, http.MethodGet, "/api/adty/site/export?kind=customers", "", "")
	assert.Contains(t, customers.Body.String(), "This export has more than")
	assert.NotContains(t, customers.Body.String(), "export-c")
}

func TestExportMasksEmail(t *testing.T) {
	f := newOwnerFixture(t)
	f.customer(t, f.site, "export-customer", "alice@gmail.com")
	f.customer(t, f.site, "=cmd|calc", "bob@example.com")
	f.site.Visibility = `{"customer_email":"full"}`
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("visibility", f.site.Visibility).Error)
	service.InvalidateAdtySiteCache()

	resp := f.call(t, http.MethodGet, "/api/adty/site/export?kind=customers", "", "")
	require.Equal(t, http.StatusOK, resp.Code)
	body := resp.Body.String()
	assert.Contains(t, body, "alice@gmail.com", "full visibility includes the email in export")
	assert.NotContains(t, body, `"a***@gmail.com"`)
	assert.Contains(t, body, `"'=cmd|calc"`, "cells starting with = are neutralised")
	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/export?kind=earnings", "", "").Body.String(), "day,user_id,kind")
	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/export?kind=logs", "", "").Body.String(), `"success":false`, "only three exports")
}

func TestOwnerViewMasksEmailWhenVisibilityIsFull(t *testing.T) {
	f := newOwnerFixture(t)
	f.customer(t, f.site, "full-mail-customer", "dave@gmail.com")
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("visibility", `{"customer_email":"full"}`).Error)
	service.InvalidateAdtySiteCache()

	list := f.call(t, http.MethodGet, "/api/adty/site/customers", "", "").Body.String()
	assert.Contains(t, list, "dave@gmail.com")
	assert.NotContains(t, list, `"d***@gmail.com"`)
	lookup := f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=dave@gmail.com", "", "").Body.String()
	assert.Contains(t, lookup, "dave@gmail.com")
}

func TestOwnerViewMasksEmailByDefault(t *testing.T) {
	f := newOwnerFixture(t)
	f.customer(t, f.site, "masked-customer", "carol@outlook.com")
	body := f.call(t, http.MethodGet, "/api/adty/site/customers", "", "").Body.String()
	assert.Contains(t, body, `"c***@outlook.com"`)
	assert.NotContains(t, body, "carol@outlook.com")
	assert.Contains(t, body, `"balance":1000000`)
}

func TestOwnerViewRespectsSiteOverride(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "override-customer", "dave@example.com")
	adty_setting.GetResellerSetting().Visibility["customer_balance"] = "off"
	body := f.call(t, http.MethodGet, "/api/adty/site/customers", "", "").Body.String()
	assert.NotContains(t, body, `"balance"`, "the global default hides balances")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).
		Update("visibility", `{"customer_email":"full","customer_balance":"on","request_logs":"off"}`).Error)
	service.InvalidateAdtySiteCache()
	body = f.call(t, http.MethodGet, "/api/adty/site/customers", "", "").Body.String()
	assert.Contains(t, body, "dave@example.com")
	assert.NotContains(t, body, `"d***@example.com"`)
	assert.Contains(t, body, `"balance":1000000`)
	logs := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d/logs", customer.Id), "", "")
	assert.Equal(t, http.StatusForbidden, logs.Code, "request logs switched off for this site")
}

func TestOwnerLogsUseUserProjection(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "logs-customer", "erin@example.com")
	other := model.NewLogOther()
	other.SetPublic("model_ratio", 2.5)
	other.SetPublic("upstream_model_name", "secret-upstream")
	other.SetPublic("cache_tokens", 12)
	other.SetAdmin("site_markup", 1.4)
	require.NoError(t, f.db.Create(&model.Log{UserId: customer.Id, Type: model.LogTypeConsume, ModelName: "gpt-x", Quota: 1400, ChannelId: 7,
		Ip: "203.0.113.9", UpstreamRequestId: "up-1", CreatedAt: time.Now().Unix(), Other: other.JSONString()}).Error)
	failed := model.NewLogOther()
	failed.SetPublic("error_type", "timeout")
	failed.SetPublic("status_code", 502)
	failed.SetPublic("upstream_model_name", "hidden-upstream")
	require.NoError(t, f.db.Create(&model.Log{UserId: customer.Id, Type: model.LogTypeError, ModelName: "gpt-x", Content: "provider timed out",
		ChannelId: 9, Ip: "198.51.100.8", CreatedAt: time.Now().Unix(), Other: failed.JSONString()}).Error)
	require.NoError(t, f.db.Create(&model.Log{UserId: customer.Id, Type: model.LogTypeTopup, Content: "topup-should-hide", CreatedAt: time.Now().Unix()}).Error)

	body := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d/logs", customer.Id), "", "").Body.String()
	assert.Contains(t, body, `"model_name":"gpt-x"`)
	assert.Contains(t, body, `"quota":1400`)
	assert.Contains(t, body, "cache_tokens")
	assert.Contains(t, body, `"type":5`)
	assert.Contains(t, body, "provider timed out")
	assert.Contains(t, body, "timeout")
	assert.Contains(t, body, `\"status_code\":502`)
	assert.NotContains(t, body, "topup-should-hide")
	for _, hidden := range []string{"channel", "203.0.113.9", "198.51.100.8", "up-1", "upstream", "model_ratio", "admin_info", "site_markup", "hidden-upstream"} {
		assert.NotContains(t, body, hidden)
	}
}

func TestLookupExactOnlyAndRateLimited(t *testing.T) {
	f := newOwnerFixture(t)
	f.customer(t, f.site, "frank", "frank@example.com")
	adty_setting.GetResellerSetting().CustomerLookupPerHour = 3

	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=frank", "", "").Body.String(), `"username":"frank"`)
	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=FRANK@example.com", "", "").Body.String(), `"username":"frank"`, "emails compare normalised")
	assert.Contains(t, f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=fra", "", "").Body.String(), `"success":false`, "no partial matches")
	limited := f.call(t, http.MethodGet, "/api/adty/site/customers/lookup?q=frank", "", "")
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)

	var audits []model.AdtySiteLookup
	require.NoError(t, f.db.Where("site_id = ?", f.site.Id).Order("id").Find(&audits).Error)
	require.Len(t, audits, 3, "every lookup is audited, found or not")
	assert.Equal(t, []bool{true, true, false}, []bool{audits[0].Found, audits[1].Found, audits[2].Found})
}

func TestOwnerCannotSeeTokenKey(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "token-customer", "g@example.com")
	require.NoError(t, f.db.Create(&model.Token{UserId: customer.Id, Key: "supersecretkeyvalue", Name: "prod", Status: common.TokenStatusEnabled, AccessedTime: 123}).Error)
	body := f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d", customer.Id), "", "").Body.String()
	assert.Contains(t, body, `"name":"prod"`)
	assert.Contains(t, body, `"accessed_time":123`)
	assert.NotContains(t, body, "supersecretkeyvalue")
	assert.NotContains(t, body, `"key"`)

	adty_setting.GetResellerSetting().Visibility["customer_tokens"] = "off"
	assert.NotContains(t, f.call(t, http.MethodGet, fmt.Sprintf("/api/adty/site/customers/%d", customer.Id), "", "").Body.String(), `"tokens"`)
}

func TestOwnerStatsAndEarnings(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "stats-customer", "h@example.com")
	today := time.Now().Format("2006-01-02")
	require.NoError(t, f.db.Create(&[]model.AdtySiteEarningDaily{
		{SiteId: f.site.Id, UserId: customer.Id, Day: today, Kind: model.AdtySiteEarningUsage, Quota: 62_500_000,
			Revenue: decimal.NewFromInt(125), Platform: decimal.NewFromInt(80), Earning: decimal.NewFromInt(45), Status: model.AdtySiteEarningPending},
		{SiteId: f.site.Id, UserId: customer.Id, Day: today, Kind: model.AdtySiteEarningBonus, Earning: decimal.NewFromInt(-5), Status: model.AdtySiteEarningPending},
		{SiteId: f.other.Id, UserId: customer.Id, Day: today, Kind: model.AdtySiteEarningUsage, Revenue: decimal.NewFromInt(999), Earning: decimal.NewFromInt(999), Status: model.AdtySiteEarningPending},
	}).Error)

	var stats struct {
		Data service.AdtySiteStats `json:"data"`
	}
	require.NoError(t, common.Unmarshal(f.call(t, http.MethodGet, "/api/adty/site/stats", "", "").Body.Bytes(), &stats))
	assert.True(t, stats.Data.Total.RevenueUSD.Equal(decimal.NewFromInt(125)))
	assert.True(t, stats.Data.Total.EarningUSD.Equal(decimal.NewFromInt(40)), "usage $45 minus bonus $5")
	assert.True(t, stats.Data.Total.PlatformUSD.Add(stats.Data.Total.EarningUSD).Sub(stats.Data.Total.BonusUSD).Equal(stats.Data.Total.RevenueUSD))
	assert.EqualValues(t, 1, stats.Data.Total.NewCustomers)

	earnings := f.call(t, http.MethodGet, "/api/adty/site/earnings", "", "").Body.String()
	assert.Contains(t, earnings, `"pending_usd":"40"`)
	assert.NotContains(t, earnings, "999", "other sites' rows never show up")
}

func TestOwnerCenterRequiresCurrentAgreement(t *testing.T) {
	legal := adty_setting.GetLegalSetting()
	oldVersion := legal.SiteAgreementVersion
	legal.SiteAgreementVersion = oldVersion + 3
	t.Cleanup(func() { legal.SiteAgreementVersion = oldVersion })
	f := newOwnerFixture(t)
	require.NoError(t, f.db.Model(f.site).Update("agreement_ver", "0").Error)
	blocked := f.call(t, http.MethodGet, "/api/adty/site/self", "", "")
	assert.Equal(t, http.StatusForbidden, blocked.Code)
	assert.Contains(t, blocked.Body.String(), "Accept the current site agreement first")

	wrong := f.call(t, http.MethodPost, "/api/adty/site/terms/accept", `{"version":"0"}`, "")
	assert.Equal(t, http.StatusForbidden, wrong.Code)
	current := service.AdtyCurrentSiteAgreementVersion()
	accepted := f.call(t, http.MethodPost, "/api/adty/site/terms/accept", `{"version":"`+current+`"}`, "")
	require.Contains(t, accepted.Body.String(), `"success":true`, accepted.Body.String())
	assert.Equal(t, http.StatusOK, f.call(t, http.MethodGet, "/api/adty/site/self", "", "").Code)
}

func sessionTokenFor(t *testing.T, user model.User) string {
	t.Helper()
	now := time.Now().Unix()
	sid := "sid-" + user.Username
	require.NoError(t, model.CreateUserSession(&model.UserSession{
		SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + user.Username,
		LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600,
	}))
	token, _, err := service.IssueAccessToken(service.AuthIdentity{
		UserID: user.Id, SessionID: sid, UserAuthVersion: user.AuthVersion, SessionVersion: 1,
	})
	require.NoError(t, err)
	return token
}

func callAs(t *testing.T, engine *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func TestCustomerAcceptsSiteTerms(t *testing.T) {
	f := newOwnerFixture(t)
	customer := f.customer(t, f.site, "terms-customer", "terms@example.com")
	legal := adty_setting.GetLegalSetting()
	old := legal.SiteCustomerTermsVersion
	legal.SiteCustomerTermsVersion = old + 2
	t.Cleanup(func() { legal.SiteCustomerTermsVersion = old })
	token := sessionTokenFor(t, customer)
	current := service.AdtyCurrentSiteCustomerTermsVersion()

	self := callAs(t, f.engine, token, http.MethodGet, "/api/user/self", "")
	require.Equal(t, http.StatusOK, self.Code, self.Body.String())
	assert.Contains(t, self.Body.String(), `"site_terms_required":true`)
	assert.Contains(t, self.Body.String(), `"site_terms_version":"`+current+`"`)

	blockedKey := callAs(t, f.engine, token, http.MethodPost, "/api/token/", `{"name":"before-terms","expired_time":-1,"unlimited_quota":true}`)
	assert.Equal(t, http.StatusForbidden, blockedKey.Code, blockedKey.Body.String())
	assert.Contains(t, blockedKey.Body.String(), "Accept the current site customer terms first")
	blockedQuote := callAs(t, f.engine, token, http.MethodPost, "/api/adty/topup/quote", `{"amount":10,"method":"usdt.trc20"}`)
	assert.Equal(t, http.StatusForbidden, blockedQuote.Code, blockedQuote.Body.String())
	assert.Contains(t, blockedQuote.Body.String(), "Accept the current site customer terms first")

	wrong := callAs(t, f.engine, token, http.MethodPost, "/api/adty/site/terms/accept", `{"version":"0"}`)
	assert.Contains(t, wrong.Body.String(), `"success":false`)
	var row model.AdtySiteCustomer
	require.NoError(t, f.db.Where("user_id = ?", customer.Id).First(&row).Error)
	assert.Empty(t, row.TermsVersion)

	accepted := callAs(t, f.engine, token, http.MethodPost, "/api/adty/site/terms/accept", `{"version":"`+current+`"}`)
	require.Contains(t, accepted.Body.String(), `"success":true`, accepted.Body.String())
	assert.Contains(t, accepted.Body.String(), `"site_terms_required":false`)
	require.NoError(t, f.db.Where("user_id = ?", customer.Id).First(&row).Error)
	assert.Equal(t, current, row.TermsVersion)
	assert.NotZero(t, row.TermsAt)

	again := callAs(t, f.engine, token, http.MethodGet, "/api/user/self", "")
	assert.Contains(t, again.Body.String(), `"site_terms_required":false`)
}

func TestDeleteCustomDomainRequiresCurrentAgreement(t *testing.T) {
	cloudflareCalls := 0
	cloudflare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cloudflareCalls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "api.stale-agreement.example", r.URL.Query().Get("hostname"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
	}))
	defer cloudflare.Close()
	oldCloudflare := service.AdtyCloudflare
	service.AdtyCloudflare = &service.AdtyCloudflareClient{BaseURL: cloudflare.URL, HTTP: cloudflare.Client(), Token: func() string { return "synthetic-token" }}
	t.Cleanup(func() { service.AdtyCloudflare = oldCloudflare })
	legal := adty_setting.GetLegalSetting()
	oldVersion := legal.SiteAgreementVersion
	legal.SiteAgreementVersion = oldVersion + 3
	t.Cleanup(func() { legal.SiteAgreementVersion = oldVersion })
	f := newOwnerFixture(t)
	domain := model.AdtySiteDomain{SiteId: f.site.Id, Hostname: "api.stale-agreement.example", Status: model.AdtySiteDomainPending, CreatedAt: time.Now().Unix()}
	require.NoError(t, f.db.Create(&domain).Error)
	require.NoError(t, f.db.Model(f.site).Update("agreement_ver", "0").Error)

	path := fmt.Sprintf("/api/adty/site/domains/%d", domain.Id)
	proof := sessionProof(t, f.owner, service.AdtySiteOwnerActionDomain, domain.Id)
	blocked := f.call(t, http.MethodDelete, path, "", proof)
	assert.Equal(t, http.StatusForbidden, blocked.Code)
	assert.Contains(t, blocked.Body.String(), "Accept the current site agreement first")
	var stored model.AdtySiteDomain
	require.NoError(t, f.db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainPending, stored.Status)
	require.Zero(t, cloudflareCalls, "an unaccepted agreement must prevent external domain operations")

	current := service.AdtyCurrentSiteAgreementVersion()
	accepted := f.call(t, http.MethodPost, "/api/adty/site/terms/accept", `{"version":"`+current+`"}`, "")
	require.Contains(t, accepted.Body.String(), `"success":true`, accepted.Body.String())
	deleted := f.call(t, http.MethodDelete, path, "", sessionProof(t, f.owner, service.AdtySiteOwnerActionDomain, domain.Id))
	require.Contains(t, deleted.Body.String(), `"success":true`, deleted.Body.String())
	require.NoError(t, f.db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainRemoved, stored.Status)
	require.Equal(t, 1, cloudflareCalls, "removal checks for an owned orphan before marking the domain removed")
}
