package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 站点品牌、邮件链接、支付跳回、分站首页的 SEO（任务 11.3）。用 11.2 的 siteFixture：
// 主站 main.example.com，分站 shop.sites.example.com（站名 Shop AI），关闭的站 gone 转给 shop。

type capturedMail struct{ from, subject, to, content string }

func TestNotificationInboxReadIsolation(t *testing.T) {
	f := newSiteFixture(t)
	setting := console_setting.GetConsoleSetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	setting.AnnouncementsEnabled = true
	setting.Announcements = `[{"id":11,"content":"Original announcement","type":"default","publishDate":"2026-01-01"}]`
	common.OptionMapRWMutex.Lock()
	common.OptionMap["Notice"] = " Account notice "
	common.OptionMapRWMutex.Unlock()
	a := adtySessionToken(t, f.db, "notification-a", common.RoleCommonUser)
	b := adtySessionToken(t, f.db, "notification-b", common.RoleCommonUser)
	var aUser model.User
	require.NoError(t, f.db.Where("username = ?", "notification-a").First(&aUser).Error)
	request := func(method, host, path, token, body string, want int) apiEnvelope {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		r := httptest.NewRecorder()
		f.engine.ServeHTTP(r, req)
		require.Equal(t, want, r.Code, r.Body.String())
		var result apiEnvelope
		if want == http.StatusOK {
			require.NoError(t, common.Unmarshal(r.Body.Bytes(), &result), r.Body.String())
		}
		return result
	}
	const main = "main.example.com"
	const inbox = "/api/adty/notifications"
	request(http.MethodGet, main, inbox, "", "", http.StatusUnauthorized)
	first := request(http.MethodGet, main, inbox, a, "", http.StatusOK)
	require.True(t, first.Success, first.Message)
	assert.Equal(t, "Account notice", first.Data["notice"])
	assert.Empty(t, first.Data["read_keys"])
	noticeKey := first.Data["notice_key"].(string)
	ann := first.Data["announcements"].([]any)[0].(map[string]any)
	annKey := ann["read_key"].(string)
	body := `{"keys":["` + noticeKey + `","` + annKey + `"]}`
	for range 2 {
		saved := request(http.MethodPost, main, inbox+"/read", a, body, http.StatusOK)
		require.True(t, saved.Success, saved.Message)
	}
	read := request(http.MethodGet, main, inbox, a, "", http.StatusOK)
	assert.Len(t, read.Data["read_keys"], 2)
	other := request(http.MethodGet, main, inbox+"?user_id="+fmt.Sprint(aUser.Id), b, "", http.StatusOK)
	assert.Empty(t, other.Data["read_keys"], "a query parameter cannot impersonate another account")
	wrong := createAdtyScopedToken(t, aUser.Id, "wallet:read")
	request(http.MethodGet, main, inbox, wrong, "", http.StatusForbidden)
	readOnly := createAdtyScopedToken(t, aUser.Id, "profile:read")
	assert.True(t, request(http.MethodGet, main, inbox, readOnly, "", http.StatusOK).Success)
	request(http.MethodPost, main, inbox+"/read", readOnly, body, http.StatusForbidden)
	write := createAdtyScopedToken(t, aUser.Id, "profile:write")
	assert.True(t, request(http.MethodPost, main, inbox+"/read", write, body, http.StatusOK).Success)
	for _, invalid := range []string{`{"keys":["forged"]}`, `{"keys":["` + strings.Repeat("0", 64) + `"]}`, `{"keys":[]}`} {
		assert.False(t, request(http.MethodPost, main, inbox+"/read", a, invalid, http.StatusOK).Success)
	}
	branch := request(http.MethodGet, "shop.sites.example.com", inbox, a, "", http.StatusOK)
	require.True(t, branch.Success, branch.Message)
	assert.Empty(t, branch.Data["notice"])
	assert.Empty(t, branch.Data["announcements"])
	assert.Empty(t, branch.Data["read_keys"])
	assert.False(t, request(http.MethodPost, "shop.sites.example.com", inbox+"/read", a, body, http.StatusOK).Success)
	setting.Announcements = `[{"id":11,"content":"Edited announcement","type":"default","publishDate":"2026-01-01"}]`
	edited := request(http.MethodGet, main, inbox, a, "", http.StatusOK)
	assert.Len(t, edited.Data["read_keys"], 1, "the old notice stays read but edited content is unread")
	newKey := edited.Data["announcements"].([]any)[0].(map[string]any)["read_key"].(string)
	assert.NotEqual(t, annKey, newKey)
	assert.False(t, request(http.MethodPost, main, inbox+"/read", a, body, http.StatusOK).Success, "a stale page must not acknowledge a new revision")
}

// captureSiteMail 把两条异步发信都换成同步记录；from 为空表示主站的全局发件人。
func captureSiteMail(t *testing.T) func() []capturedMail {
	t.Helper()
	var mu sync.Mutex
	var sent []capturedMail
	oldPlain, oldSite := service.AdtySendEmailAsync, service.AdtySendSiteEmailAsync
	service.AdtySendEmailAsync = func(subject, to, content string) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, capturedMail{"", subject, to, content})
	}
	service.AdtySendSiteEmailAsync = func(site *model.AdtySite, subject, to, content string) {
		mu.Lock()
		defer mu.Unlock()
		from := ""
		if site != nil {
			from = service.AdtySenderFor(site)
		}
		sent = append(sent, capturedMail{from, subject, to, content})
	}
	service.ResetAdtyReminderThrottle()
	t.Cleanup(func() {
		service.AdtySendEmailAsync, service.AdtySendSiteEmailAsync = oldPlain, oldSite
		service.ResetAdtyReminderThrottle()
	})
	return func() []capturedMail {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedMail(nil), sent...)
	}
}

func newBrandFixture(t *testing.T) *siteFixture {
	t.Helper()
	require.NoError(t, i18n.Init())
	f := newSiteFixture(t)
	SetAdtyWebRoutes(f.engine)
	common.OptionMap["ResendFrom"] = "ADTY AI <noreply@mail.adtyai.com>"
	common.OptionMap["EmailProvider"] = "resend"
	return f
}

func TestResetLinkEscapesPlusInEmail(t *testing.T) {
	f := newBrandFixture(t)
	sent := captureSiteMail(t)
	user := f.user(t, "plus-user", 0)
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", user.Id).Update("email", "a+b@example.com").Error)
	f.do(t, http.MethodGet, "main.example.com", "/api/reset_password?email="+url.QueryEscape("a+b@example.com"), "")
	mails := sent()
	require.Len(t, mails, 1)
	assert.Contains(t, mails[0].content, "/user/reset#email=a%2Bb%40example.com&token=")
	assert.NotContains(t, mails[0].content, "email=a+b@")
}

func TestResetLinkUsesUserSiteNotHost(t *testing.T) {
	f := newBrandFixture(t)
	sent := captureSiteMail(t)
	customer := f.user(t, "shop-customer", f.site.Id)
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", customer.Id).Update("email", "c@example.com").Error)
	mainUser := f.user(t, "main-user", 0)
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", mainUser.Id).Update("email", "m@example.com").Error)

	// 主站域名上找回：链接按账号所属站点，不按请求 Host。
	f.do(t, http.MethodGet, "main.example.com", "/api/reset_password?email="+url.QueryEscape("c@example.com"), "")
	denied := f.do(t, http.MethodGet, "evil.example.net", "/api/reset_password?email="+url.QueryEscape("c@example.com"), "")
	assert.Equal(t, http.StatusNotFound, denied.Code, "an unknown host cannot send a reset mail")
	f.do(t, http.MethodGet, "shop.sites.example.com", "/api/reset_password?email="+url.QueryEscape("m@example.com"), "")

	mails := sent()
	require.Len(t, mails, 2)
	assert.Contains(t, mails[0].content, "https://shop.sites.example.com/user/reset#email=c%40example.com&token=")
	assert.NotContains(t, mails[0].content, "evil.example.net")
	assert.Contains(t, mails[0].subject, "Shop AI")
	assert.Equal(t, "Shop AI <noreply@mail.adtyai.com>", mails[0].from)
	assert.Contains(t, mails[1].content, "https://main.example.com/user/reset#email=m%40example.com&token=", "主站用户在分站域名上找回，链接仍是主站")
	assert.NotContains(t, mails[1].subject, "Shop AI")
}

func TestVerificationMailUsesRequestSite(t *testing.T) {
	f := newBrandFixture(t)
	sent := captureSiteMail(t)

	f.do(t, http.MethodGet, "shop.sites.example.com", "/api/verification?email="+url.QueryEscape("new@example.com"), "")
	f.do(t, http.MethodGet, "main.example.com", "/api/verification?email="+url.QueryEscape("other@example.com"), "")

	mails := sent()
	require.Len(t, mails, 2)
	assert.Contains(t, mails[0].subject, "Shop AI")
	assert.Contains(t, mails[0].content, "Shop AI")
	assert.Equal(t, "Shop AI <noreply@mail.adtyai.com>", mails[0].from)
	assert.NotContains(t, mails[1].subject, "Shop AI")
	assert.Empty(t, mails[1].from, "主站用全局发件人")
}

func TestMailSubjectUsesSiteName(t *testing.T) {
	f := newBrandFixture(t)
	sent := captureSiteMail(t)
	// 站名里带 HTML：进邮件要转义
	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("brand", `{"name":"<b>Shop</b>"}`).Error)
	service.InvalidateAdtySiteCache()

	// 验证码接口按 IP 限频，换一个来源地址，不和上一个测试撞上
	req := httptest.NewRequest(http.MethodGet, "/api/verification?email="+url.QueryEscape("x@example.com"), nil)
	req.Host, req.RemoteAddr = "shop.sites.example.com", "198.51.100.7:4000"
	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	mails := sent()
	require.Len(t, mails, 1)
	assert.Contains(t, mails[0].subject, "&lt;b&gt;Shop&lt;/b&gt;")
	assert.NotContains(t, mails[0].content, "<b>Shop</b>")
}

func TestPayResultRedirectsToOwnerSite(t *testing.T) {
	f := newBrandFixture(t)
	customer := f.user(t, "pay-customer", f.site.Id)
	require.NoError(t, f.db.Create(&model.TopUp{UserId: customer.Id, TradeNo: "EPY-SITE-1", Status: common.TopUpStatusPending}).Error)
	mainUser := f.user(t, "pay-main", 0)
	require.NoError(t, f.db.Create(&model.TopUp{UserId: mainUser.Id, TradeNo: "EPY-MAIN-1", Status: common.TopUpStatusPending}).Error)

	siteURL := service.AdtyPayResultURL("EPY-SITE-1")
	assert.True(t, strings.HasPrefix(siteURL, "https://main.example.com/pay/result/"), "易支付的 return_url 一律指向主站")
	assert.NotContains(t, siteURL, "?", "return_url 不带 query，网关会自己往后加参数")

	rec := f.do(t, http.MethodGet, "main.example.com", strings.TrimPrefix(siteURL, "https://main.example.com")+"?out_trade_no=EPY-SITE-1&trade_status=TRADE_SUCCESS", "")
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://shop.sites.example.com/wallet?trade_no=EPY-SITE-1", rec.Header().Get("Location"))

	rec = f.do(t, http.MethodGet, "main.example.com", strings.TrimPrefix(service.AdtyPayResultURL("EPY-MAIN-1"), "https://main.example.com"), "")
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://main.example.com/wallet?trade_no=EPY-MAIN-1", rec.Header().Get("Location"))

	saved := service.AdtyLoadCustomDomains
	service.AdtyLoadCustomDomains = func() (map[string]int, error) {
		return map[string]int{"pay.customer-brand.com": f.site.Id}, nil
	}
	t.Cleanup(func() {
		service.AdtyLoadCustomDomains = saved
		service.InvalidateAdtySiteCache()
	})
	service.InvalidateAdtySiteCache()
	rec = f.do(t, http.MethodGet, "main.example.com", strings.TrimPrefix(siteURL, "https://main.example.com"), "")
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://shop.sites.example.com/wallet?trade_no=EPY-SITE-1", rec.Header().Get("Location"), "an active custom domain is not the pay result")
}

func TestPayResultIgnoresForeignOrder(t *testing.T) {
	f := newBrandFixture(t)
	customer := f.user(t, "foreign-customer", f.site.Id)
	require.NoError(t, f.db.Create(&model.TopUp{UserId: customer.Id, TradeNo: "EPY-FOREIGN", Status: common.TopUpStatusPending}).Error)
	good := strings.TrimPrefix(service.AdtyPayResultURL("EPY-FOREIGN"), "https://main.example.com")

	for name, path := range map[string]string{
		"没签名":     "/pay/result/EPY-FOREIGN",
		"签名对不上":   "/pay/result/EPY-FOREIGN.0000000000000000",
		"换了订单号":   strings.Replace(good, "EPY-FOREIGN", "EPY-OTHER", 1),
		"查不到的订单":  strings.TrimPrefix(service.AdtyPayResultURL("EPY-MISSING"), "https://main.example.com"),
		"带 query": "/pay/result?trade_no=EPY-FOREIGN",
	} {
		rec := f.do(t, http.MethodGet, "main.example.com", path, "")
		require.Equal(t, http.StatusFound, rec.Code, "%s: %s", name, rec.Body.String())
		assert.Equal(t, "https://main.example.com/wallet", rec.Header().Get("Location"), name)
	}
}

func TestEpayReturnURLPointsToPayResult(t *testing.T) {
	assert.Regexp(t, `^https?://[^?]+/pay/result/EPY-1\.[0-9a-f]{16}$`, service.AdtyPayResultURL("EPY-1"))
}

// 分站首页的 SEO：分站默认 noindex；开了 SeoIndex 才写正式头部，canonical 指向分站自己的地址；
// /robots.txt 按站点返回。
func seoEngine(t *testing.T) *gin.Engine {
	t.Helper()
	engine := gin.New()
	engine.Use(middleware.AdtySiteContext())
	engine.GET("/robots.txt", AdtyRobotsTxt())
	engine.GET("/sitemap.xml", AdtySitemap())
	engine.NoRoute(AdtySEOHomePages([]byte(seoIndexPage), fstest.MapFS{"web/dist/adty-home.json": {Data: []byte(seoPrerender)}}), func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(seoIndexPage))
	})
	return engine
}

func get(engine *gin.Engine, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestSiteHomeNoindexUnlessSeoIndex(t *testing.T) {
	f := newBrandFixture(t)
	engine := seoEngine(t)

	body := get(engine, "shop.sites.example.com", "/").Body.String()
	assert.Contains(t, body, `<meta name="robots" content="noindex" />`)
	assert.NotContains(t, body, `rel="canonical"`)
	assert.NotContains(t, body, "main.example.com", "分站页面不能指向主站")
	assert.Contains(t, body, "<title>Shop AI</title>")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("seo_index", true).Error)
	service.InvalidateAdtySiteCache()
	body = get(engine, "shop.sites.example.com", "/").Body.String()
	assert.NotContains(t, body, "noindex")
	assert.Contains(t, body, `<link rel="canonical" href="https://shop.sites.example.com/" />`)
	assert.Contains(t, body, `<link rel="alternate" hreflang="ja" href="https://shop.sites.example.com/ja" />`)
	assert.Contains(t, body, `<meta property="og:site_name" content="Shop AI" />`)
	assert.NotContains(t, body, "application/ld+json", "WebSite 结构化数据只给主站")
	assert.NotContains(t, body, "main.example.com")

	// 主站照旧
	mainBody := get(engine, "main.example.com", "/").Body.String()
	assert.Contains(t, mainBody, `<link rel="canonical" href="https://main.example.com/" />`)

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("brand", `{"name":"Shop AI","tagline":"Fast","title":"<Workspace> & AI"}`).Error)
	service.InvalidateAdtySiteCache()
	body = get(engine, "shop.sites.example.com", "/").Body.String()
	assert.Contains(t, body, "<title>&lt;Workspace&gt; &amp; AI</title>")
	assert.Contains(t, body, `<meta property="og:site_name" content="Shop AI" />`)
	assert.NotContains(t, body, "<Workspace>")
}

func TestSiteRobotsTxt(t *testing.T) {
	f := newBrandFixture(t)
	engine := seoEngine(t)

	rec := get(engine, "shop.sites.example.com", "/robots.txt")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "User-agent: *\nDisallow: /\n", rec.Body.String(), "没开 SEO 的分站整站不收录")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("seo_index", true).Error)
	service.InvalidateAdtySiteCache()
	body := get(engine, "shop.sites.example.com", "/robots.txt").Body.String()
	assert.Contains(t, body, "Disallow: /api/")
	assert.NotContains(t, body, "Disallow: /\n")
	assert.Contains(t, body, "Sitemap: https://shop.sites.example.com/sitemap.xml")

	mainBody := get(engine, "main.example.com", "/robots.txt").Body.String()
	assert.Contains(t, mainBody, "Disallow: /api/")
	assert.Contains(t, mainBody, "Disallow: /v1/")
	assert.Contains(t, mainBody, "Sitemap: https://main.example.com/sitemap.xml")
}

func TestSitemapListsEightHomeLanguages(t *testing.T) {
	f := newBrandFixture(t)
	engine := seoEngine(t)

	main := get(engine, "main.example.com", "/sitemap.xml")
	require.Equal(t, http.StatusOK, main.Code)
	assert.Contains(t, main.Header().Get("Content-Type"), "xml")
	body := main.Body.String()
	for _, path := range []string{"/", "/zh-cn", "/zh-tw", "/fr", "/ja", "/ko", "/ru", "/vi"} {
		want := "https://main.example.com" + path
		if path == "/" {
			want = "https://main.example.com/"
		}
		assert.Contains(t, body, "<loc>"+want+"</loc>")
	}
	assert.Contains(t, body, `hreflang="ko"`)
	assert.NotContains(t, body, "/wallet")
	assert.NotContains(t, body, "?ref=")

	closed := get(engine, "shop.sites.example.com", "/sitemap.xml")
	require.Equal(t, http.StatusOK, closed.Code)
	assert.NotContains(t, closed.Body.String(), "<loc>")

	require.NoError(t, f.db.Model(&model.AdtySite{}).Where("id = ?", f.site.Id).Update("seo_index", true).Error)
	service.InvalidateAdtySiteCache()
	indexed := get(engine, "shop.sites.example.com", "/sitemap.xml").Body.String()
	assert.Contains(t, indexed, "<loc>https://shop.sites.example.com/</loc>")
	assert.Contains(t, indexed, "<loc>https://shop.sites.example.com/ko</loc>")
	assert.NotContains(t, indexed, "main.example.com")
}

// /api/status 在分站上返回分站 widget 的 site key（任务 11.8 的遗留项）。
func TestStatusReturnsSiteTurnstileKey(t *testing.T) {
	f := newBrandFixture(t)
	cfg := withResellerSettingKeep(t)
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w1", SiteKey: "sk-sites"}}
	oldKey := common.TurnstileSiteKey
	common.TurnstileSiteKey = "sk-main"
	t.Cleanup(func() { common.TurnstileSiteKey = oldKey })
	status := func(host string) map[string]any {
		t.Helper()
		var envelope apiEnvelope
		require.NoError(t, common.Unmarshal(f.do(t, http.MethodGet, host, "/api/status", "").Body.Bytes(), &envelope))
		return envelope.Data
	}

	assert.Equal(t, "sk-sites", status("shop.sites.example.com")["turnstile_site_key"])
	assert.Equal(t, "sk-main", status("main.example.com")["turnstile_site_key"])
}

// withResellerSettingKeep 在当前分站设置上改，结束时还原（不像 withResellerSetting 那样重置成默认值）。
func withResellerSettingKeep(t *testing.T) *adty_setting.ResellerSetting {
	t.Helper()
	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	t.Cleanup(func() {
		*cfg = saved
		service.InvalidateAdtySiteCache()
	})
	return cfg
}
