package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeCloudflare 是假的 Cloudflare API：自定义主机名（zone 级）、Turnstile widget（账号级）、siteverify。
// 返回的 JSON 照 adtyai-design/research/verify-p11/cf_api_digest.md 的示例。
type fakeCloudflare struct {
	mu sync.Mutex
	t  *testing.T
	// 自定义主机名：id → 主机名、status、ssl.status
	hostnames map[string]*fakeHostname
	nextId    int
	// createDuplicate 为真时 POST 返回 1406（主机名已经在 Cloudflare 上）
	createDuplicate bool
	// rateLimitOnce 为真时下一个请求返回 429 和 Retry-After: 2
	rateLimitOnce bool
	// widgets：sitekey → domains；PUT 的响应带 secret
	widgets  map[string][]string
	puts     []string // 每次 PUT 的 sitekey
	deletes  []string
	requests []string
}

type fakeHostname struct {
	hostname, status, sslStatus string
}

func newFakeCloudflare(t *testing.T) (*fakeCloudflare, *httptest.Server) {
	f := &fakeCloudflare{t: t, hostnames: map[string]*fakeHostname{}, widgets: map[string][]string{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fakeCloudflare) writeResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"success":true,"errors":[],"messages":[],"result":`+result+`}`)
}

func (f *fakeCloudflare) hostnameJSON(id string, h *fakeHostname) string {
	return `{"id":"` + id + `","hostname":"` + h.hostname + `","status":"` + h.status + `",` +
		`"ownership_verification":{"name":"_cf-custom-hostname.` + h.hostname + `","type":"txt","value":"own-` + id + `"},` +
		`"ssl":{"id":"ssl-` + id + `","method":"http","type":"dv","status":"` + h.sslStatus + `",` +
		`"validation_records":[{"txt_name":"_acme-challenge.` + h.hostname + `","txt_value":"acme-` + id + `","status":"pending"}]},` +
		`"created_at":"2026-10-31T00:00:00Z"}`
}

func (f *fakeCloudflare) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	require.Equal(f.t, "Bearer cf-token", r.Header.Get("Authorization"))
	if f.rateLimitOnce {
		f.rateLimitOnce = false
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10000,"message":"rate limited"}]}`)
		return
	}
	body, _ := io.ReadAll(r.Body)
	const zone = "/zones/zone-1/custom_hostnames"
	const account = "/accounts/acct-1/challenges/widgets/"
	switch {
	case r.Method == http.MethodPost && r.URL.Path == zone:
		var req struct {
			Hostname string `json:"hostname"`
			SSL      struct {
				Method string `json:"method"`
				Type   string `json:"type"`
			} `json:"ssl"`
		}
		require.NoError(f.t, common.Unmarshal(body, &req))
		assert.Equal(f.t, "http", req.SSL.Method)
		assert.Equal(f.t, "dv", req.SSL.Type)
		if f.createDuplicate {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":1406,"message":"Duplicate custom hostname found."}],"result":null}`)
			return
		}
		f.nextId++
		id := "cf-" + string(rune('0'+f.nextId))
		f.hostnames[id] = &fakeHostname{hostname: req.Hostname, status: "pending", sslStatus: "initializing"}
		// POST 的响应不一定带验证记录（官方文档），这里故意不带，逼着代码隔一会儿再 GET。
		f.writeResult(w, `{"id":"`+id+`","hostname":"`+req.Hostname+`","status":"pending","ssl":{"status":"initializing"}}`)
	case r.Method == http.MethodGet && r.URL.Path == zone:
		name := r.URL.Query().Get("hostname")
		items := []string{}
		for id, h := range f.hostnames {
			if h.hostname == name {
				items = append(items, f.hostnameJSON(id, h))
			}
		}
		f.writeResult(w, "["+strings.Join(items, ",")+"]")
	case strings.HasPrefix(r.URL.Path, zone+"/"):
		id := strings.TrimPrefix(r.URL.Path, zone+"/")
		h, ok := f.hostnames[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":1436,"message":"not found"}]}`)
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.writeResult(w, f.hostnameJSON(id, h))
		case http.MethodDelete:
			f.deletes = append(f.deletes, id)
			delete(f.hostnames, id)
			f.writeResult(w, `{"id":"`+id+`"}`)
		default:
			f.writeResult(w, f.hostnameJSON(id, h))
		}
	case strings.HasPrefix(r.URL.Path, account):
		key := strings.TrimPrefix(r.URL.Path, account)
		domains, ok := f.widgets[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10404,"message":"widget not found"}]}`)
			return
		}
		if r.Method == http.MethodPut {
			var req struct {
				Domains []string `json:"domains"`
				Mode    string   `json:"mode"`
				Name    string   `json:"name"`
				Region  string   `json:"region"`
			}
			require.NoError(f.t, common.Unmarshal(body, &req))
			require.NotEmpty(f.t, req.Mode, "PUT 是整体替换，mode 要原样带回")
			require.NotEmpty(f.t, req.Name, "PUT 是整体替换，name 要原样带回")
			require.Equal(f.t, "world", req.Region, "region 创建后不能改，要原样带回")
			f.widgets[key] = req.Domains
			f.puts = append(f.puts, key)
			domains = req.Domains
			// PUT 的响应带 secret（官方文档），不能进日志。
			f.writeResult(w, `{"sitekey":"`+key+`","name":"`+req.Name+`","mode":"`+req.Mode+`","region":"world","domains":`+jsonStrings(domains)+`,"secret":"LEAKED-WIDGET-SECRET"}`)
			return
		}
		f.writeResult(w, `{"sitekey":"`+key+`","name":"platform","mode":"managed","region":"world","domains":`+jsonStrings(domains)+`}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// fullPlatformWidget 是登记满 10 个域名的第一个 widget。
func fullPlatformWidget() []string {
	return []string{"adtyai.com", "sites.example.com", "a.org", "b.org", "c.org", "d.org", "e.org", "f.org", "g.org", "h.org"}
}

func jsonStrings(values []string) string {
	data, _ := common.Marshal(values)
	return string(data)
}

func (f *fakeCloudflare) setStatus(id, status, sslStatus string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hostnames[id].status, f.hostnames[id].sslStatus = status, sslStatus
}

// prepareDomainDB 在分站库上设好 Cloudflare：token、zone、账号、CNAME 目标，两个 widget（各有 secret），
// 打开自定义域名；Cloudflare 客户端指向假服务器，创建后等待的时间换成 0。
func prepareDomainDB(t *testing.T) (*gorm.DB, *fakeCloudflare) {
	t.Helper()
	db := prepareSiteDB(t)
	fake, server := newFakeCloudflare(t)
	cfg := adty_setting.GetResellerSetting()
	cfg.CustomDomainEnabled = true
	cfg.CloudflareApiToken, cfg.CloudflareZoneId, cfg.CloudflareAccountId = "cf-token", "zone-1", "acct-1"
	cfg.CnameTarget = "customers.sites.example.com"
	cfg.TurnstileWidgets = []adty_setting.ResellerTurnstileWidget{{Id: "w-platform", SiteKey: "sk-platform"}, {Id: "w-two", SiteKey: "sk-two"}}
	cfg.TurnstileWidgetSecret = map[string]string{"w-platform": "secret-platform", "w-two": "secret-two"}
	fake.widgets["w-platform"] = []string{"adtyai.com", "sites.example.com"}
	fake.widgets["w-two"] = []string{}

	oldClient, oldWait, oldSleep, oldLookup := AdtyCloudflare, adtyCustomHostnameRecordDelay, adtyCloudflareSleep, adtyLookupOwnershipTXT
	AdtyCloudflare = &AdtyCloudflareClient{BaseURL: server.URL, HTTP: server.Client(), Token: func() string { return "cf-token" }}
	adtyCustomHostnameRecordDelay = 0
	adtyCloudflareSleep = func(time.Duration) {}
	// 默认测试里 TXT 已经配好，这样原来的建主机名用例仍然一次完成。
	adtyLookupOwnershipTXT = func(_ context.Context, name string) ([]string, error) {
		host := strings.TrimPrefix(name, "_cf-custom-hostname.")
		var row model.AdtySiteDomain
		if model.DB.Where("hostname = ?", host).Limit(1).Find(&row).RowsAffected == 0 || row.OwnershipToken == "" {
			return nil, nil
		}
		return []string{row.OwnershipToken}, nil
	}
	t.Cleanup(func() {
		AdtyCloudflare, adtyCustomHostnameRecordDelay, adtyCloudflareSleep, adtyLookupOwnershipTXT = oldClient, oldWait, oldSleep, oldLookup
	})
	require.NoError(t, db.AutoMigrate(&model.AdtySiteDomain{}))
	return db, fake
}

func TestCustomDomainRejectsApexAndOwnDomains(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "apex")
	otherOwner, _ := openActiveSite(t, db, "other")
	_, err := AddAdtySiteDomain(context.Background(), otherOwner.Id, "taken.example.org")
	require.NoError(t, err)

	cases := []struct {
		name, host string
	}{
		{"根域名", "example.org"},
		{"主站域名", "www.adtyai.com"},
		{"平台根域名下的子域名", "evil.sites.example.com"},
		{"平台根域名本身", "sites.example.com"},
		{"CNAME 目标", "customers.sites.example.com"},
		{"别的站已占用", "taken.example.org"},
		{"非法字符", "bad_host.example.org"},
		{"带协议", "https://x.example.org"},
		{"多段后缀的根域名", "example.co.uk"},
	}
	for _, tc := range cases {
		_, err := AddAdtySiteDomain(context.Background(), owner.Id, tc.host)
		assert.Error(t, err, tc.name)
	}
	assert.Equal(t, 1, len(fake.hostnames), "被拒的域名不能调 Cloudflare")

	multi, err := AddAdtySiteDomain(context.Background(), owner.Id, "www.example.co.uk")
	require.NoError(t, err)
	assert.Equal(t, "www.example.co.uk", multi.Hostname)

	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "API.Example.ORG")
	require.NoError(t, err)
	assert.Equal(t, "api.example.org", domain.Hostname)
}

func TestCustomDomainRequiresOwnershipTxt(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "txt")
	other, _ := openActiveSite(t, db, "txt-other")
	adtyLookupOwnershipTXT = func(context.Context, string) ([]string, error) {
		return []string{"wrong-token"}, nil
	}

	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.owned.example")
	require.NoError(t, err)
	assert.Empty(t, domain.CFHostnameId)
	assert.Empty(t, fake.hostnames, "没有正确的 TXT 时不在 Cloudflare 建主机名")
	assert.NotEmpty(t, domain.OwnershipToken)
	assert.Contains(t, domain.VerifyRecords, domain.OwnershipToken)

	require.NoError(t, RemoveAdtySiteDomain(context.Background(), owner.Id, domain.Id))
	stolen, err := AddAdtySiteDomain(context.Background(), other.Id, "api.owned.example")
	require.NoError(t, err)
	assert.Equal(t, domain.Id, stolen.Id)
	assert.NotEqual(t, domain.OwnershipToken, stolen.OwnershipToken, "重加换新令牌")
	assert.Empty(t, stolen.CFHostnameId)
	assert.Empty(t, fake.hostnames)

	token := stolen.OwnershipToken
	adtyLookupOwnershipTXT = func(_ context.Context, name string) ([]string, error) {
		if name == "_cf-custom-hostname.api.owned.example" {
			return []string{token}, nil
		}
		return nil, nil
	}
	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))
	var stored model.AdtySiteDomain
	require.NoError(t, db.First(&stored, stolen.Id).Error)
	assert.NotEmpty(t, stored.CFHostnameId)
	assert.Equal(t, 1, len(fake.hostnames))
}

func TestCustomDomainLimitsPerSite(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, site := openActiveSite(t, db, "limit")
	cfg := adty_setting.GetResellerSetting()
	cfg.MaxCustomDomainsPerSite = 2
	cfg.CustomDomainCreatesPerHour = 3

	_, err := AddAdtySiteDomain(context.Background(), owner.Id, "a.limit.example")
	require.NoError(t, err)
	_, err = AddAdtySiteDomain(context.Background(), owner.Id, "b.limit.example")
	require.NoError(t, err)
	_, err = AddAdtySiteDomain(context.Background(), owner.Id, "c.limit.example")
	assert.ErrorIs(t, err, ErrAdtyCustomDomainLimit)
	assert.Equal(t, 2, len(fake.hostnames))

	require.NoError(t, db.Model(&model.AdtySiteDomain{}).Where("hostname = ?", "b.limit.example").Update("status", model.AdtySiteDomainFailed).Error)
	retried, err := AddAdtySiteDomain(context.Background(), owner.Id, "b.limit.example")
	require.NoError(t, err, "重试已有的 failed 域名不占新名额")
	assert.Equal(t, "b.limit.example", retried.Hostname)

	cfg.MaxCustomDomainsPerSite = 10
	cfg.CustomDomainCreatesPerHour = 1
	require.NoError(t, db.Model(&model.AdtySiteDomain{}).Where("site_id = ?", site.Id).Update("created_at", time.Now().Unix()-7200).Error)
	_, err = AddAdtySiteDomain(context.Background(), owner.Id, "d.limit.example")
	require.NoError(t, err)
	_, err = AddAdtySiteDomain(context.Background(), owner.Id, "e.limit.example")
	assert.ErrorIs(t, err, ErrAdtyCustomDomainRate)
}

func TestCustomDomainRequiresSwitchAndCloudflare(t *testing.T) {
	db, _ := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "switch")

	adty_setting.GetResellerSetting().CustomDomainEnabled = false
	_, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.switch.org")
	assert.ErrorIs(t, err, ErrAdtyCustomDomainOff)

	adty_setting.GetResellerSetting().CustomDomainEnabled = true
	adty_setting.GetResellerSetting().CloudflareApiToken = ""
	_, err = AddAdtySiteDomain(context.Background(), owner.Id, "api.switch.org")
	assert.ErrorIs(t, err, ErrAdtyCustomDomainOff)
}

func TestCustomDomainStoresVerifyRecordsFromLaterGet(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "records")

	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.records.org")
	require.NoError(t, err)

	assert.Equal(t, model.AdtySiteDomainPending, domain.Status)
	assert.Equal(t, "cf-1", domain.CFHostnameId)
	assert.Contains(t, domain.VerifyRecords, "customers.sites.example.com", "CNAME 记录指向 cname_target")
	assert.Contains(t, domain.VerifyRecords, "_cf-custom-hostname.api.records.org", "POST 没带，要从后来的 GET 里取")
	assert.Contains(t, fake.requests, "GET /zones/zone-1/custom_hostnames/cf-1")
}

func TestCustomDomainReusesDuplicateHostname(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "dup")
	fake.hostnames["cf-existing"] = &fakeHostname{hostname: "api.dup.org", status: "pending", sslStatus: "pending_validation"}
	fake.createDuplicate = true

	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.dup.org")
	require.NoError(t, err)
	assert.Equal(t, "cf-existing", domain.CFHostnameId, "1406 时按主机名查列表，沿用已有的 id")
}

func TestCloudflareRateLimitBacksOffByRetryAfter(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "ratelimit")
	var slept []time.Duration
	adtyCloudflareSleep = func(d time.Duration) { slept = append(slept, d) }
	fake.rateLimitOnce = true

	_, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.ratelimit.org")
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{2 * time.Second}, slept)
}

func TestCustomDomainActivatesOnlyWhenBothActive(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, site := openActiveSite(t, db, "activate")
	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.activate.org")
	require.NoError(t, err)
	now := time.Now()

	for _, step := range []struct {
		status, ssl string
		want        string
	}{
		{"active", "pending_validation", model.AdtySiteDomainPending},
		{"pending", "active", model.AdtySiteDomainPending},
		{"active", "active", model.AdtySiteDomainActive},
	} {
		fake.setStatus("cf-1", step.status, step.ssl)
		require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), now))
		var stored model.AdtySiteDomain
		require.NoError(t, db.First(&stored, domain.Id).Error)
		assert.Equal(t, step.want, stored.Status, "%s / %s", step.status, step.ssl)
	}

	hosts, err := adtyLoadActiveCustomDomains()
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"api.activate.org": site.Id}, hosts, "只有 active 的进站点识别")
}

func TestCustomDomainFailedAfterTimeout(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "timeout")
	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.timeout.org")
	require.NoError(t, err)
	require.NotEmpty(t, domain.CFHostnameId)

	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Unix(domain.CreatedAt, 0).Add(71*time.Hour)))
	var stored model.AdtySiteDomain
	require.NoError(t, db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainPending, stored.Status)
	assert.NotEmpty(t, stored.CFHostnameId)
	assert.Empty(t, fake.deletes)

	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Unix(domain.CreatedAt, 0).Add(73*time.Hour)))
	require.NoError(t, db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainFailed, stored.Status)
	assert.Empty(t, stored.CFHostnameId)
	assert.Equal(t, []string{domain.CFHostnameId}, fake.deletes)
	_, stillThere := fake.hostnames[domain.CFHostnameId]
	assert.False(t, stillThere)

	// 站长可以重试：同一个主机名再加一次，回到 pending
	retried, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.timeout.org")
	require.NoError(t, err)
	assert.Equal(t, domain.Id, retried.Id)
	assert.Equal(t, model.AdtySiteDomainPending, retried.Status)
}

func TestTurnstileAssignsWidgetWithSpace(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "widget")
	// 第一个 widget 已经满 10 个
	fake.widgets["w-platform"] = fullPlatformWidget()
	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.widget.org")
	require.NoError(t, err)
	fake.setStatus("cf-1", "active", "active")

	logs := captureSysLogs(t)
	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))

	var stored model.AdtySiteDomain
	require.NoError(t, db.First(&stored, domain.Id).Error)
	assert.Equal(t, "w-two", stored.TurnstileWidget)
	assert.Equal(t, []string{"api.widget.org"}, fake.widgets["w-two"])
	assert.Len(t, fake.widgets["w-platform"], 10, "满了的 widget 不动")
	assert.NotContains(t, logs(), "LEAKED-WIDGET-SECRET", "PUT 的响应带 secret，不能进日志")
}

func TestTurnstileNoSpaceLeavesDomainActiveWithoutWidget(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "full")
	ten := []string{"a.org", "b.org", "c.org", "d.org", "e.org", "f.org", "g.org", "h.org", "i.org", "j.org"}
	fake.widgets["w-platform"], fake.widgets["w-two"] = ten, ten
	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.full.org")
	require.NoError(t, err)
	fake.setStatus("cf-1", "active", "active")

	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))

	var stored model.AdtySiteDomain
	require.NoError(t, db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainActive, stored.Status, "名额用完时域名照常能用")
	assert.Empty(t, stored.TurnstileWidget)
}

func TestTurnstileUsesSiteSecret(t *testing.T) {
	db, fake := prepareDomainDB(t)
	// 第一个 widget 满了，自定义域名进第二个
	fake.widgets["w-platform"] = fullPlatformWidget()
	owner, site := openActiveSite(t, db, "secret")
	_, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.secret.org")
	require.NoError(t, err)
	fake.setStatus("cf-1", "active", "active")
	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))
	InvalidateAdtySiteCache()

	// 自定义域名上的请求：用它登记进的 widget 的 secret，hostname 要对上
	choice := AdtyTurnstileFor(AdtySiteById(site.Id), "api.secret.org")
	assert.Equal(t, "secret-two", choice.Secret)
	assert.Equal(t, "sk-two", choice.SiteKey)

	// 平台子域名和主站：第一个 widget
	assert.Equal(t, "secret-platform", AdtyTurnstileFor(AdtySiteById(site.Id), "secret.sites.example.com").Secret)
	main := AdtyTurnstileFor(nil, "adtyai.com")
	assert.Empty(t, main.Secret, "主站用上游的 TurnstileSecretKey")
}

func TestTurnstileVerifyChecksHostname(t *testing.T) {
	_, server := newSiteverifyServer(t, `{"success":true,"hostname":"api.site.org"}`)
	oldVerify := adtyTurnstileVerifyURL
	adtyTurnstileVerifyURL = server.URL
	t.Cleanup(func() { adtyTurnstileVerifyURL = oldVerify })

	ok, err := AdtyTurnstileVerify(context.Background(), "secret", "token", "1.2.3.4", "api.site.org")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = AdtyTurnstileVerify(context.Background(), "secret", "token", "1.2.3.4", "other.site.org")
	require.NoError(t, err)
	assert.False(t, ok, "token 在别的域名上拿到的，不算")

	ok, err = AdtyTurnstileVerify(context.Background(), "secret", strings.Repeat("x", 2049), "", "api.site.org")
	require.NoError(t, err)
	assert.False(t, ok, "超过 2048 字符直接拒")
}

func newSiteverifyServer(t *testing.T, response string) (*[]url.Values, *httptest.Server) {
	var got []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		got = append(got, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return &got, server
}

func TestDeleteDomainRemovesFromWidget(t *testing.T) {
	db, fake := prepareDomainDB(t)
	// 第一个 widget 满了，自定义域名进第二个
	fake.widgets["w-platform"] = fullPlatformWidget()
	owner, _ := openActiveSite(t, db, "delete")
	domain, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.delete.org")
	require.NoError(t, err)
	fake.setStatus("cf-1", "active", "active")
	require.NoError(t, RunAdtySiteDomainCheckOnce(context.Background(), time.Now()))
	require.Equal(t, []string{"api.delete.org"}, fake.widgets["w-two"])

	require.NoError(t, RemoveAdtySiteDomain(context.Background(), owner.Id, domain.Id))

	assert.Equal(t, []string{"cf-1"}, fake.deletes)
	assert.Empty(t, fake.widgets["w-two"])
	var stored model.AdtySiteDomain
	require.NoError(t, db.First(&stored, domain.Id).Error)
	assert.Equal(t, model.AdtySiteDomainRemoved, stored.Status)
	hosts, err := adtyLoadActiveCustomDomains()
	require.NoError(t, err)
	assert.Empty(t, hosts)
}

func TestOwnerCannotRemoveOtherSiteDomain(t *testing.T) {
	db, _ := prepareDomainDB(t)
	owner, _ := openActiveSite(t, db, "mine")
	other, _ := openActiveSite(t, db, "theirs")
	domain, err := AddAdtySiteDomain(context.Background(), other.Id, "api.theirs.org")
	require.NoError(t, err)

	assert.ErrorIs(t, RemoveAdtySiteDomain(context.Background(), owner.Id, domain.Id), ErrAdtySiteNotFound)
}

func TestClosedSiteRemovesItsDomains(t *testing.T) {
	db, fake := prepareDomainDB(t)
	owner, site := openActiveSite(t, db, "closed")
	_, err := AddAdtySiteDomain(context.Background(), owner.Id, "api.closed.org")
	require.NoError(t, err)

	adtyRemoveDomainsOfClosedSite(site)

	assert.Equal(t, []string{"cf-1"}, fake.deletes)
}

// captureSysLogs 把 common.SysLog、SysError 写的日志收进缓冲区，返回读取函数。
func captureSysLogs(t *testing.T) func() string {
	t.Helper()
	var buffer strings.Builder
	var mu sync.Mutex
	writer := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buffer.Write(p)
	})
	common.LogWriterMu.Lock()
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = writer, writer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr
		common.LogWriterMu.Unlock()
	})
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return buffer.String()
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
