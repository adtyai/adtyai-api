package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
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

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	return &ownerFixture{db: db, engine: engine, site: site, owner: owner, token: token, other: other}
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
	require.Len(t, wallets, 3)
	balances := map[string]string{}
	for _, wallet := range wallets {
		assert.Equal(t, f.site.Id, wallet.SiteId)
		balances[wallet.Kind] = wallet.Balance.String()
	}
	assert.Equal(t, map[string]string{"operating": "10", "earnings": "5", "margin": "0"}, balances)

	path := fmt.Sprintf("/api/adty/site/wallet-ledgers?kind=operating&site_id=%d&page_size=1", f.other.Id)
	for page, want := range []string{"mine-operating-debit", "mine-operating-credit"} {
		response := f.call(t, http.MethodGet, fmt.Sprintf("%s&p=%d", path, page+1), "", "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		body := decodeAdtyData[struct {
			Total int64                        `json:"total"`
			Items []model.AdtySiteWalletLedger `json:"items"`
		}](t, response.Body)
		assert.EqualValues(t, 2, body.Total)
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
		// 优惠码（12.4 站长出资）：新建 target 为 0，修改和删除 target 为码 id。
		{"promo create", http.MethodPost, "/api/adty/site/promo-codes", `{"code":"verify-new","kind":"percent","value":"1","enabled":true}`, service.AdtySiteOwnerActionPromo, 0},
		{"promo update", http.MethodPut, fmt.Sprintf("/api/adty/site/promo-codes/%d", promo.Id), `{"kind":"percent","value":"1.5","enabled":false}`, service.AdtySiteOwnerActionPromo, promo.Id},
		{"promo delete", http.MethodDelete, fmt.Sprintf("/api/adty/site/promo-codes/%d", doomed.Id), "", service.AdtySiteOwnerActionPromo, doomed.Id},
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
			if tc.method == http.MethodDelete && strings.HasPrefix(tc.path, "/api/adty/site/promo-codes/") {
				// 码已经删掉，找不到码在验证之前就返回。
				assert.Contains(t, replay.Body.String(), "Promo code not found")
				return
			}
			assert.Contains(t, replay.Body.String(), "SECURITY_PROOF_CONSUMED", "a proof works once")
		})
	}
	var stored model.User
	require.NoError(t, f.db.First(&stored, customer.Id).Error)
	assert.EqualValues(t, 1_000_000+500_000, stored.Quota, "the trial was granted exactly once")
	var prices int64
	require.NoError(t, f.db.Model(&model.AdtySitePrice{}).Where("site_id = ?", f.site.Id).Count(&prices).Error)
	assert.EqualValues(t, 2, prices)
	var created, dropped int64
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("code = ?", "VERIFY-NEW").Count(&created).Error)
	require.NoError(t, f.db.Model(&model.AdtyPromoCode{}).Where("id = ?", doomed.Id).Count(&dropped).Error)
	assert.EqualValues(t, 1, created, "the promo code was created exactly once")
	assert.Zero(t, dropped)
	require.NoError(t, f.db.First(&promo, promo.Id).Error)
	assert.False(t, promo.Enabled)
	var sender model.AdtySiteSenderDomain
	require.NoError(t, f.db.First(&sender, "site_id = ?", f.site.Id).Error)
	assert.Equal(t, model.AdtySenderDomainDisabled, sender.Status)
	foreign := f.call(t, http.MethodDelete, "/api/adty/site/promo-codes/999999", "", sessionProof(t, f.owner, service.AdtySiteOwnerActionPromo, 999999))
	assert.Contains(t, foreign.Body.String(), "Promo code not found", "unknown codes fail before the proof is spent")

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
