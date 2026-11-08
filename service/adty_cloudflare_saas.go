package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
)

// 自定义域名（任务 11.8），走 Cloudflare for SaaS 的自定义主机名。按官方 API 文档的示例写，没有实测
// （11.0 推迟到上线前），摘要在 adtyai-design/research/verify-p11/cf_api_digest.md。
// 测试替换 AdtyCloudflare 的 BaseURL 和 HTTP，指向 httptest 假服务器。API Token 和响应原文都不打日志：
// Turnstile widget 的 PUT 响应带 secret。

const adtyCloudflareDefaultBaseURL = "https://api.cloudflare.com/client/v4"

// AdtyCloudflareClient 调 Cloudflare 的 REST API。字段都可以在测试里换掉。
type AdtyCloudflareClient struct {
	BaseURL string
	HTTP    *http.Client
	Token   func() string
}

// AdtyCloudflare 是业务代码用的客户端，Token 读分站设置里的 cloudflare_apiToken。
var AdtyCloudflare = &AdtyCloudflareClient{
	BaseURL: adtyCloudflareDefaultBaseURL,
	HTTP:    &http.Client{Timeout: 20 * time.Second},
	Token:   func() string { return adty_setting.GetResellerSetting().CloudflareApiToken },
}

var (
	// adtyCustomHostnameRecordDelay：创建后隔多久再 GET 取验证记录（POST 的响应可能不带）。测试换成 0。
	adtyCustomHostnameRecordDelay = 3 * time.Second
	// adtyCloudflareSleep 用来等待 Retry-After 和上面的延时，测试换掉。
	adtyCloudflareSleep = time.Sleep
	// adtyCustomDomainTimeout：这么久还没通就标 failed。
	adtyCustomDomainTimeout = 72 * time.Hour
	// adtyLookupOwnershipTXT 查归属 TXT。测试换成假的，避免真去解析。
	adtyLookupOwnershipTXT = func(ctx context.Context, name string) ([]string, error) {
		return net.DefaultResolver.LookupTXT(ctx, name)
	}
)

const (
	adtyCloudflareMaxRetries      = 3
	adtyCloudflareMaxRetryAfter   = 60 * time.Second
	adtyCloudflareDuplicateCode   = 1406
	adtyTurnstileDomainsPerWidget = 10
)

var (
	ErrAdtyCustomDomainOff     = common.NewMessage("Custom domains are not available")
	ErrAdtyCustomDomainInvalid = common.NewMessage("Enter a subdomain such as api.example.com; root domains and our own domains cannot be used")
	ErrAdtyCustomDomainTaken   = common.NewMessage("This domain is already used by another site")
	ErrAdtyCustomDomainLimit   = common.NewMessage("This site already has the maximum number of custom domains")
	ErrAdtyCustomDomainRate    = common.NewMessage("This site has added too many custom domains this hour")
	ErrAdtyCloudflareRejected  = common.NewMessage("Cloudflare rejected the request; check the API token permissions")
	ErrAdtyCloudflareFailed    = common.NewMessage("Cloudflare could not complete the request; try again later")
)

// AdtyCloudflareError 是 Cloudflare 返回的错误（外壳 success=false 或非 2xx）。
type AdtyCloudflareError struct {
	StatusCode int
	Code       int
	Message    string
}

func (e *AdtyCloudflareError) Error() string {
	return fmt.Sprintf("cloudflare api error: status=%d code=%d message=%s", e.StatusCode, e.Code, e.Message)
}

type adtyCloudflareEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result common.RawMessage `json:"result"`
}

// do 发一个请求，429 时按 Retry-After 退避后重试。out 为 nil 时丢弃 result。
func (c *AdtyCloudflareClient) do(ctx context.Context, method, path string, body any, out any) error {
	token := strings.TrimSpace(c.Token())
	if token == "" {
		return ErrAdtyCustomDomainOff
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = common.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < adtyCloudflareMaxRetries {
			wait := time.Second
			if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
				wait = time.Duration(seconds) * time.Second
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			adtyCloudflareSleep(min(wait, adtyCloudflareMaxRetryAfter))
			continue
		}
		var envelope adtyCloudflareEnvelope
		decodeErr := common.DecodeJson(io.LimitReader(resp.Body, 4<<20), &envelope)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success || decodeErr != nil {
			apiErr := &AdtyCloudflareError{StatusCode: resp.StatusCode}
			if len(envelope.Errors) > 0 {
				apiErr.Code, apiErr.Message = envelope.Errors[0].Code, envelope.Errors[0].Message
			}
			return apiErr
		}
		if out == nil || len(envelope.Result) == 0 {
			return nil
		}
		return common.Unmarshal(envelope.Result, out)
	}
}

// adtyCustomHostname 是自定义主机名接口的 result，只取用得到的字段。
type adtyCustomHostname struct {
	Id                    string            `json:"id"`
	Hostname              string            `json:"hostname"`
	Status                string            `json:"status"`
	Metadata              map[string]string `json:"custom_metadata"`
	OwnershipVerification struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
	SSL struct {
		Status            string `json:"status"`
		ValidationRecords []struct {
			TxtName  string `json:"txt_name"`
			TxtValue string `json:"txt_value"`
		} `json:"validation_records"`
	} `json:"ssl"`
	VerificationErrors []string `json:"verification_errors"`
}

func adtyCustomHostnameGeneration(domain *model.AdtySiteDomain) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s", domain.Id, domain.SiteId, domain.Hostname, domain.OwnershipToken))))
}

func (c *AdtyCloudflareClient) createCustomHostname(ctx context.Context, zone string, domain *model.AdtySiteDomain) (*adtyCustomHostname, error) {
	var out adtyCustomHostname
	body := map[string]any{
		"hostname":        domain.Hostname,
		"ssl":             map[string]string{"method": "http", "type": "dv"},
		"custom_metadata": map[string]string{"site_id": strconv.Itoa(domain.SiteId), "adty_domain_id": strconv.Itoa(domain.Id), "adty_generation": adtyCustomHostnameGeneration(domain)},
	}
	err := c.do(ctx, http.MethodPost, "/zones/"+url.PathEscape(zone)+"/custom_hostnames", body, &out)
	return &out, err
}

func (c *AdtyCloudflareClient) findCustomHostname(ctx context.Context, zone, hostname string) (*adtyCustomHostname, error) {
	var out []adtyCustomHostname
	path := "/zones/" + url.PathEscape(zone) + "/custom_hostnames?hostname=" + url.QueryEscape(hostname)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if strings.EqualFold(out[i].Hostname, hostname) {
			return &out[i], nil
		}
	}
	return nil, nil
}

func (c *AdtyCloudflareClient) getCustomHostname(ctx context.Context, zone, id string) (*adtyCustomHostname, error) {
	var out adtyCustomHostname
	err := c.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zone)+"/custom_hostnames/"+url.PathEscape(id), nil, &out)
	return &out, err
}

func (c *AdtyCloudflareClient) deleteCustomHostname(ctx context.Context, zone, id string) error {
	err := c.do(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zone)+"/custom_hostnames/"+url.PathEscape(id), nil, nil)
	var apiErr *AdtyCloudflareError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil // 已经不在了
	}
	return err
}

// adtyVerifyRecord 是给站长看的一条 DNS 记录。
type adtyVerifyRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// verifyRecordsFor 生成站长要加的记录：CNAME 指向 cname_target，再加 Cloudflare 要的 TXT（如果有）。
func verifyRecordsFor(hostname, cnameTarget string, h *adtyCustomHostname) string {
	records := []adtyVerifyRecord{{Type: "CNAME", Name: hostname, Value: cnameTarget}}
	if h != nil {
		if h.OwnershipVerification.Name != "" && h.OwnershipVerification.Value != "" {
			records = append(records, adtyVerifyRecord{Type: "TXT", Name: h.OwnershipVerification.Name, Value: h.OwnershipVerification.Value})
		}
		for _, record := range h.SSL.ValidationRecords {
			if record.TxtName != "" && record.TxtValue != "" {
				records = append(records, adtyVerifyRecord{Type: "TXT", Name: record.TxtName, Value: record.TxtValue})
			}
		}
	}
	data, _ := common.Marshal(records)
	return string(data)
}

// adtyMainHost 是主站的域名（来自后台的服务器地址），小写，没设就是空。
func adtyMainHost() string {
	parsed, err := url.Parse(strings.TrimSpace(system_setting.ServerAddress))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// adtyRegistrableDomain 粗略取「最后两段」作注册域名，用来判断主站域名下的子域名。
func adtyRegistrableDomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func adtyHostUnder(host, domain string) bool {
	return domain != "" && (host == domain || strings.HasSuffix(host, "."+domain))
}

// normalizeCustomDomain 校验站长填的域名：小写、去掉末尾的点；必须是子域名（注册域本身拒绝，
// example.co.uk 这种多段后缀的根域名也拒绝），不能是 root_domains、cname_target、主站域名下的。
func normalizeCustomDomain(raw string) (string, error) {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if host == "" || len(host) > 253 || adtyCustomDomainIsApex(host) {
		return "", ErrAdtyCustomDomainInvalid
	}
	for _, label := range strings.Split(host, ".") {
		if !adty_setting.ResellerLabelPattern.MatchString(label) {
			return "", ErrAdtyCustomDomainInvalid
		}
	}
	cfg := resellerSetting()
	blocked := append([]string{}, cfg.RootDomains...)
	if target := strings.ToLower(strings.TrimSpace(cfg.CnameTarget)); target != "" {
		blocked = append(blocked, adtyRegistrableDomain(target))
	}
	if fallback := strings.ToLower(strings.TrimSpace(cfg.FallbackOrigin)); fallback != "" {
		blocked = append(blocked, adtyRegistrableDomain(fallback))
	}
	if main := adtyMainHost(); main != "" && main != "localhost" {
		blocked = append(blocked, adtyRegistrableDomain(main))
	}
	blocked = append(blocked, strings.TrimPrefix(common.AdtyBrandURL, "https://"))
	for _, domain := range blocked {
		if adtyHostUnder(host, strings.ToLower(strings.TrimSpace(domain))) {
			return "", ErrAdtyCustomDomainInvalid
		}
	}
	return host, nil
}

// adtyCustomDomainIsApex 用公共后缀表判断主机名是不是注册域本身。认不出的也当作根域名拒绝。
func adtyCustomDomainIsApex(host string) bool {
	registrable, err := publicsuffix.EffectiveTLDPlusOne(host)
	return err != nil || registrable == host
}

// adtyCustomDomainAllowCreate 检查这个站还能不能再新建一个自定义域名。
// reusingCountedRow 为真时，这一行已经是本站的 pending、active 或 failed，不占新名额；每小时新建数两种都算。
func adtyCustomDomainAllowCreate(tx *gorm.DB, siteId int, reusingCountedRow bool, now int64) error {
	cfg := resellerSetting()
	if !reusingCountedRow {
		var counted int64
		err := tx.Model(&model.AdtySiteDomain{}).
			Where("site_id = ? AND status IN ?", siteId, []string{
				model.AdtySiteDomainPending, model.AdtySiteDomainActive, model.AdtySiteDomainFailed,
			}).Count(&counted).Error
		if err != nil {
			return err
		}
		if int(counted) >= cfg.MaxCustomDomainsPerSite {
			return ErrAdtyCustomDomainLimit
		}
	}
	var recent int64
	err := tx.Model(&model.AdtySiteDomain{}).
		Where("site_id = ? AND created_at >= ?", siteId, now-3600).Count(&recent).Error
	if err != nil {
		return err
	}
	if int(recent) >= cfg.CustomDomainCreatesPerHour {
		return ErrAdtyCustomDomainRate
	}
	return nil
}

// adtyOwnerSite 是这个用户名下能绑定域名的站：active 或 paused。
func adtyOwnerSite(ownerUserId int) (*model.AdtySite, error) {
	var site model.AdtySite
	err := model.DB.Where("owner_user_id = ? AND status IN ?", ownerUserId,
		[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused}).First(&site).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdtySiteNotFound
	}
	return &site, err
}

// adtyCloudflareUserError 把 Cloudflare 的错误换成给人看的提示；原始错误只记日志（不含响应原文和 Token）。
func adtyCloudflareUserError(err error) error {
	var apiErr *AdtyCloudflareError
	if errors.As(err, &apiErr) {
		common.SysError(common.LogText("cloudflare request failed: status=%d code=%d", apiErr.StatusCode, apiErr.Code))
		if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
			return ErrAdtyCloudflareRejected
		}
		return ErrAdtyCloudflareFailed
	}
	common.SysError(common.LogText("cloudflare request failed: %s", err.Error()))
	return ErrAdtyCloudflareFailed
}

// AddAdtySiteDomain 给站长的站绑定一个自定义域名。先发归属 TXT，查到令牌才在 Cloudflare 建主机名。
// 别的站 pending、active、failed 的主机名算占用。removed 可以重加，但必须换新令牌并重新验证。
func AddAdtySiteDomain(ctx context.Context, ownerUserId int, rawHost string) (*model.AdtySiteDomain, error) {
	cfg := resellerSetting()
	if !cfg.CustomDomainEnabled || !cfg.CloudflareConfigured() {
		return nil, ErrAdtyCustomDomainOff
	}
	site, err := adtyOwnerSite(ownerUserId)
	if err != nil {
		return nil, err
	}
	host, err := normalizeCustomDomain(rawHost)
	if err != nil {
		return nil, err
	}
	var domain model.AdtySiteDomain
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.AdtySiteDomain
		result := tx.Where("hostname = ?", host).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		found := result.RowsAffected > 0
		if found {
			// Re-read after taking the same lock used by checker and removal.
			if err := adtyLockDomainRow(tx, existing.Id, &existing); err != nil {
				return err
			}
		}
		if found && existing.SiteId != site.Id && existing.Status != model.AdtySiteDomainRemoved {
			return ErrAdtyCustomDomainTaken
		}
		if found && existing.SiteId == site.Id && (existing.Status == model.AdtySiteDomainPending || existing.Status == model.AdtySiteDomainActive) {
			domain = existing
			return nil
		}
		now := time.Now().Unix()
		reusingCounted := found && existing.SiteId == site.Id && existing.Status != model.AdtySiteDomainRemoved
		if err := adtyCustomDomainAllowCreate(tx, site.Id, reusingCounted, now); err != nil {
			return err
		}
		token, err := adtyNewOwnershipToken()
		if err != nil {
			return err
		}
		domain = model.AdtySiteDomain{
			SiteId: site.Id, Hostname: host, OwnershipToken: token, Status: model.AdtySiteDomainPending,
			VerifyRecords: adtyOwnershipRecords(host, token, strings.TrimSpace(cfg.CnameTarget)),
			LastCheckedAt: now, CreatedAt: now,
		}
		if !found {
			return tx.Create(&domain).Error
		}
		domain.Id = existing.Id
		return tx.Model(&model.AdtySiteDomain{}).Where("id = ?", existing.Id).Updates(map[string]any{
			"site_id": site.Id, "ownership_token": token, "cf_hostname_id": "", "status": model.AdtySiteDomainPending,
			"ssl_status": "", "verify_records": domain.VerifyRecords, "turnstile_widget": "",
			"last_error": "", "last_checked_at": now, "created_at": now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if err := adtyAttachCustomHostname(ctx, &domain); err != nil {
		return nil, err
	}
	return &domain, nil
}

func adtyNewOwnershipToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func adtyOwnershipTXTName(host string) string {
	return "_cf-custom-hostname." + host
}

func adtyOwnershipRecords(host, token, cnameTarget string) string {
	records := []adtyVerifyRecord{
		{Type: "TXT", Name: adtyOwnershipTXTName(host), Value: token},
		{Type: "CNAME", Name: host, Value: cnameTarget},
	}
	data, _ := common.Marshal(records)
	return string(data)
}

func adtyOwnershipTXTMatches(ctx context.Context, host, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	values, err := adtyLookupOwnershipTXT(ctx, adtyOwnershipTXTName(host))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, err
	}
	for _, value := range values {
		if strings.TrimSpace(value) == token {
			return true, nil
		}
	}
	return false, nil
}

// adtyAttachCustomHostname 在归属 TXT 对上以后才向 Cloudflare 建主机名。
func adtyDomainSnapshotQuery(tx *gorm.DB, domain *model.AdtySiteDomain) *gorm.DB {
	return tx.Model(&model.AdtySiteDomain{}).Where(
		"id = ? AND site_id = ? AND hostname = ? AND status = ? AND ownership_token = ? AND cf_hostname_id = ?",
		domain.Id, domain.SiteId, domain.Hostname, domain.Status, domain.OwnershipToken, domain.CFHostnameId)
}

// The write acquires a database row lock on PostgreSQL/MySQL and a writer lock
// on SQLite before reading. All lifecycle entry points hold this transaction
// across their external requests, so another node cannot remove/reassign an
// in-flight attachment, widget update or timeout cleanup.
func adtyLockDomainRow(tx *gorm.DB, id int, current *model.AdtySiteDomain) error {
	if err := tx.Model(&model.AdtySiteDomain{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		return err
	}
	return model.AdtyLockForUpdate(tx).First(current, id).Error
}

func adtyWithDomainLifecycle(ctx context.Context, snapshot *model.AdtySiteDomain, action func(*gorm.DB, *model.AdtySiteDomain) error) error {
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.AdtySiteDomain
		if err := adtyLockDomainRow(tx, snapshot.Id, &current); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if current.SiteId != snapshot.SiteId || current.Hostname != snapshot.Hostname || current.OwnershipToken != snapshot.OwnershipToken {
			return nil
		}
		if err := action(tx, &current); err != nil {
			return err
		}
		*snapshot = current
		return nil
	})
}

func adtyAttachCustomHostname(ctx context.Context, domain *model.AdtySiteDomain) error {
	return adtyWithDomainLifecycle(ctx, domain, func(tx *gorm.DB, current *model.AdtySiteDomain) error {
		return adtyAttachCustomHostnameLocked(ctx, tx, current)
	})
}

func adtyAttachCustomHostnameLocked(ctx context.Context, tx *gorm.DB, domain *model.AdtySiteDomain) error {
	if domain.Status != model.AdtySiteDomainPending || domain.CFHostnameId != "" || domain.OwnershipToken == "" {
		return nil
	}
	matched, err := adtyOwnershipTXTMatches(ctx, domain.Hostname, domain.OwnershipToken)
	if err != nil {
		_ = adtyDomainSnapshotQuery(tx, domain).Update("last_error", "ownership lookup failed").Error
		return nil
	}
	if !matched {
		return nil
	}
	cfg := resellerSetting()
	zone := strings.TrimSpace(cfg.CloudflareZoneId)
	created, err := AdtyCloudflare.createCustomHostname(ctx, zone, domain)
	var apiErr *AdtyCloudflareError
	if errors.As(err, &apiErr) && apiErr.Code == adtyCloudflareDuplicateCode {
		created, err = AdtyCloudflare.findCustomHostname(ctx, zone, domain.Hostname)
		if err == nil && created == nil {
			err = apiErr
		}
	}
	if err != nil {
		return adtyCloudflareUserError(err)
	}
	if adtyCustomHostnameRecordDelay > 0 {
		adtyCloudflareSleep(adtyCustomHostnameRecordDelay)
	}
	detail, err := AdtyCloudflare.getCustomHostname(ctx, zone, created.Id)
	if err != nil {
		common.SysError(common.LogText("cloudflare custom hostname detail failed for site %d", domain.SiteId))
		detail = created
	}
	now := time.Now().Unix()
	records := verifyRecordsFor(domain.Hostname, strings.TrimSpace(cfg.CnameTarget), detail)
	result := adtyDomainSnapshotQuery(tx, domain).Updates(map[string]any{
		"cf_hostname_id": created.Id, "ssl_status": detail.SSL.Status,
		"verify_records": records, "last_error": "", "last_checked_at": now,
	})
	if result.Error == nil && result.RowsAffected > 0 {
		domain.CFHostnameId, domain.SSLStatus = created.Id, detail.SSL.Status
		domain.VerifyRecords, domain.LastCheckedAt = records, now
	}
	return result.Error
}

// RemoveAdtySiteDomain 是站长删除自己站上的域名。
func RemoveAdtySiteDomain(ctx context.Context, ownerUserId, domainId int) error {
	site, err := adtyOwnerSite(ownerUserId)
	if err != nil {
		return err
	}
	var domain model.AdtySiteDomain
	if model.DB.Where("id = ? AND site_id = ?", domainId, site.Id).Limit(1).Find(&domain).RowsAffected == 0 {
		return ErrAdtySiteNotFound
	}
	return removeAdtySiteDomain(ctx, &domain)
}

// RemoveAdtySiteDomainByAdmin 是主站管理员停用一个域名（11.9）。
func RemoveAdtySiteDomainByAdmin(ctx context.Context, domainId int) error {
	var domain model.AdtySiteDomain
	if model.DB.Where("id = ?", domainId).Limit(1).Find(&domain).RowsAffected == 0 {
		return ErrAdtySiteNotFound
	}
	return removeAdtySiteDomain(ctx, &domain)
}

// removeAdtySiteDomain 从 Turnstile widget 和 Cloudflare 删掉，行改成 removed。
func removeAdtySiteDomain(ctx context.Context, domain *model.AdtySiteDomain) error {
	err := adtyWithDomainLifecycle(ctx, domain, func(tx *gorm.DB, current *model.AdtySiteDomain) error {
		return removeAdtySiteDomainLocked(ctx, tx, current)
	})
	if err == nil {
		InvalidateAdtySiteCache()
	}
	return err
}

func removeAdtySiteDomainLocked(ctx context.Context, tx *gorm.DB, domain *model.AdtySiteDomain) error {
	if domain.Status == model.AdtySiteDomainRemoved {
		return nil
	}
	zone := strings.TrimSpace(resellerSetting().CloudflareZoneId)
	hostnameID := domain.CFHostnameId
	if hostnameID == "" {
		// A remote create may have succeeded before the database transaction
		// rolled back. Only our exact generation marker proves it is ours;
		// an unrecorded duplicate/find result is never deleted speculatively.
		candidate, err := AdtyCloudflare.findCustomHostname(ctx, zone, domain.Hostname)
		if err != nil {
			return adtyCloudflareUserError(err)
		}
		if candidate != nil && candidate.Metadata["site_id"] == strconv.Itoa(domain.SiteId) &&
			candidate.Metadata["adty_domain_id"] == strconv.Itoa(domain.Id) && candidate.Metadata["adty_generation"] == adtyCustomHostnameGeneration(domain) {
			hostnameID = candidate.Id
		}
	}
	// Scan configured widgets too: a successful remote PUT may not have been
	// recorded before rollback. The domain generation remains locked here.
	if hostnameID != "" || domain.TurnstileWidget != "" {
		if err := adtyTurnstileRemoveDomain(ctx, tx, domain.TurnstileWidget, domain.Hostname); err != nil {
			return adtyCloudflareUserError(err)
		}
	}
	if hostnameID != "" {
		if err := AdtyCloudflare.deleteCustomHostname(ctx, zone, hostnameID); err != nil {
			return adtyCloudflareUserError(err)
		}
	}
	if err := adtyDomainSnapshotQuery(tx, domain).Updates(map[string]any{
		"status": model.AdtySiteDomainRemoved, "turnstile_widget": "", "last_checked_at": time.Now().Unix(),
	}).Error; err != nil {
		return err
	}
	domain.Status, domain.TurnstileWidget = model.AdtySiteDomainRemoved, ""
	return nil
}

// adtyRemoveDomainsOfClosedSite 在关站后删掉这个站的所有域名（AdtyOnSiteClosed）。失败只记日志，
// 行保持原状，管理员可以在分站管理里再删。
func adtyRemoveDomainsOfClosedSite(site *model.AdtySite) {
	var domains []model.AdtySiteDomain
	if err := model.DB.Where("site_id = ? AND status <> ?", site.Id, model.AdtySiteDomainRemoved).Find(&domains).Error; err != nil {
		common.SysError(common.LogText("load domains of closed site %d failed: %s", site.Id, err.Error()))
		return
	}
	for i := range domains {
		if err := removeAdtySiteDomain(context.Background(), &domains[i]); err != nil {
			common.SysError(common.LogText("remove domain %d of closed site %d failed", domains[i].Id, site.Id))
		}
	}
}

// adtyMarkCustomDomainFailed 把超时的域名标成 failed。Cloudflare 上还有主机名时先删掉；删不掉就保持 pending，下次再试。
func adtyMarkCustomDomainFailed(ctx context.Context, tx *gorm.DB, zone string, domain *model.AdtySiteDomain, updates map[string]any) error {
	if domain.CFHostnameId != "" {
		if err := AdtyCloudflare.deleteCustomHostname(ctx, zone, domain.CFHostnameId); err != nil {
			common.SysError(common.LogText("delete timed out custom hostname for domain %d failed", domain.Id))
			updates["last_error"] = "cloudflare delete failed"
			return adtyDomainSnapshotQuery(tx, domain).Updates(updates).Error
		}
		updates["cf_hostname_id"] = ""
	}
	updates["status"] = model.AdtySiteDomainFailed
	return adtyDomainSnapshotQuery(tx, domain).Updates(updates).Error
}

func adtyAssignCheckedDomainTurnstile(ctx context.Context, tx *gorm.DB, domain *model.AdtySiteDomain) error {
	if domain.Status != model.AdtySiteDomainActive || domain.TurnstileWidget != "" {
		return nil
	}
	widget, err := adtyTurnstileAssign(ctx, tx, domain.Hostname)
	if err != nil {
		common.SysError(common.LogText("turnstile assign for domain %d failed", domain.Id))
		return nil
	}
	if widget == "" {
		return nil
	}
	if err := adtyDomainSnapshotQuery(tx, domain).Where("turnstile_widget = ?", "").Update("turnstile_widget", widget).Error; err != nil {
		return err
	}
	domain.TurnstileWidget = widget
	return nil
}

// RunAdtySiteDomainCheckOnce 查一遍 pending 的域名：status 和 ssl.status 都 active 才改成 active 并登记
// Turnstile；超过 72 小时还没通就标 failed，并删掉 Cloudflare 上的主机名。active 但还没登记进 widget 的也重试登记。
func RunAdtySiteDomainCheckOnce(ctx context.Context, now time.Time) error {
	cfg := resellerSetting()
	if !cfg.CloudflareConfigured() {
		return nil
	}
	var domains []model.AdtySiteDomain
	if err := model.DB.Where("status = ? OR (status = ? AND turnstile_widget = ?)",
		model.AdtySiteDomainPending, model.AdtySiteDomainActive, "").Find(&domains).Error; err != nil {
		return err
	}
	changed := false
	for i := range domains {
		activated, err := adtyCheckDomainSnapshot(ctx, &domains[i], now)
		if err != nil {
			return err
		}
		changed = changed || activated
	}
	if changed {
		InvalidateAdtySiteCache()
	}
	return nil
}

func adtyCheckDomainSnapshot(ctx context.Context, snapshot *model.AdtySiteDomain, now time.Time) (changed bool, err error) {
	err = adtyWithDomainLifecycle(ctx, snapshot, func(tx *gorm.DB, domain *model.AdtySiteDomain) error {
		cfg := resellerSetting()
		zone := strings.TrimSpace(cfg.CloudflareZoneId)
		if domain.Status == model.AdtySiteDomainActive {
			return adtyAssignCheckedDomainTurnstile(ctx, tx, domain)
		}
		if domain.Status != model.AdtySiteDomainPending {
			return nil
		}
		if domain.CFHostnameId == "" {
			if err := adtyAttachCustomHostnameLocked(ctx, tx, domain); err != nil {
				return err
			}
			if domain.CFHostnameId == "" {
				updates := map[string]any{"last_checked_at": now.Unix()}
				if now.Sub(time.Unix(domain.CreatedAt, 0)) > adtyCustomDomainTimeout {
					return adtyMarkCustomDomainFailed(ctx, tx, zone, domain, updates)
				}
				return adtyDomainSnapshotQuery(tx, domain).Updates(updates).Error
			}
		}
		updates := map[string]any{"last_checked_at": now.Unix()}
		detail, err := AdtyCloudflare.getCustomHostname(ctx, zone, domain.CFHostnameId)
		switch {
		case err != nil:
			// 网络或 Cloudflare 出错：保持 pending，下次再查。
			var apiErr *AdtyCloudflareError
			if errors.As(err, &apiErr) {
				updates["last_error"] = fmt.Sprintf("cloudflare %d", apiErr.Code)
			}
		case detail.Status == "active" && detail.SSL.Status == "active":
			updates["status"], updates["ssl_status"], updates["last_error"] = model.AdtySiteDomainActive, detail.SSL.Status, ""
		default:
			updates["ssl_status"] = detail.SSL.Status
			updates["verify_records"] = verifyRecordsFor(domain.Hostname, strings.TrimSpace(cfg.CnameTarget), detail)
			if len(detail.VerificationErrors) > 0 {
				updates["last_error"] = strings.Join(detail.VerificationErrors, "; ")
			}
		}
		if updates["status"] == nil && now.Sub(time.Unix(domain.CreatedAt, 0)) > adtyCustomDomainTimeout {
			return adtyMarkCustomDomainFailed(ctx, tx, zone, domain, updates)
		}
		result := adtyDomainSnapshotQuery(tx, domain).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 && updates["status"] == model.AdtySiteDomainActive {
			changed = true
			domain.Status = model.AdtySiteDomainActive
			if err := adtyAssignCheckedDomainTurnstile(ctx, tx, domain); err != nil {
				return err
			}
		}
		return nil
	})
	return changed, err
}

// adtyLoadActiveCustomDomains 是站点识别用的「active 的自定义域名 → 站点 id」（AdtyLoadCustomDomains）。
func adtyLoadActiveCustomDomains() (map[string]int, error) {
	var domains []model.AdtySiteDomain
	if err := model.DB.Select("hostname", "site_id").Where("status = ?", model.AdtySiteDomainActive).Find(&domains).Error; err != nil {
		return nil, err
	}
	hosts := make(map[string]int, len(domains))
	for _, domain := range domains {
		hosts[strings.ToLower(domain.Hostname)] = domain.SiteId
	}
	return hosts, nil
}

func startAdtySiteDomainTask() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			if err := RunAdtySiteDomainCheckOnce(ctx, time.Now()); err != nil {
				common.SysError(common.LogText("custom domain check task: %s", err.Error()))
			}
			cancel()
		}
	}()
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, startAdtySiteDomainTask)
	AdtyLoadCustomDomains = adtyLoadActiveCustomDomains
	previous := AdtyOnSiteClosed
	AdtyOnSiteClosed = func(site *model.AdtySite) {
		if previous != nil {
			previous(site)
		}
		adtyRemoveDomainsOfClosedSite(site)
	}
}

// ListAdtySiteDomains 列出站长自己站上没删除的自定义域名。closing 仍可查看（11.6）。
func ListAdtySiteDomains(ownerUserId int) ([]model.AdtySiteDomain, error) {
	site, err := AdtyOwnerSite(ownerUserId)
	if errors.Is(err, ErrAdtySiteNotOwner) {
		return nil, ErrAdtySiteNotFound
	}
	if err != nil {
		return nil, err
	}
	domains := []model.AdtySiteDomain{}
	err = model.DB.Where("site_id = ? AND status <> ?", site.Id, model.AdtySiteDomainRemoved).Order("id").Find(&domains).Error
	return domains, err
}
