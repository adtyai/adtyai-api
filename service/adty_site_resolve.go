package service

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// 站点表的进程内缓存（执行层决定 28）。变更时清缓存；多实例下按 SYNC_FREQUENCY 过期重读（第八节）。
// 11.8 的自定义域名表建好后，在 AdtyLoadCustomDomains 里挂上「已 active 的自定义域名 → 站点 id」。

type adtySiteSnapshot struct {
	byId     map[int]*model.AdtySite
	byHost   map[string]*model.AdtySite // <子域名>.<根域名>
	byCustom map[string]*model.AdtySite // 已 active 的自定义域名
	loadedAt time.Time
}

var (
	adtySiteCacheMu sync.Mutex
	adtySiteCache   *adtySiteSnapshot
)

// AdtyLoadCustomDomains 由 11.8 注册：返回已经 active 的自定义域名（小写主机名 → 站点 id）。
var AdtyLoadCustomDomains func() (map[string]int, error)

// AdtySiteNames returns display names without exposing the sites' financial or private settings.
// A missing historical site keeps its ID and an empty name; site 0 is always the main site.
func AdtySiteNames(siteIDs []int) (map[int]string, error) {
	names := map[int]string{0: common.SystemName}
	ids := make([]int, 0, len(siteIDs))
	seen := map[int]bool{}
	for _, id := range siteIDs {
		if id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		return names, nil
	}
	var sites []model.AdtySite
	if err := model.DB.Select("id", "subdomain", "root_domain", "brand").Where("id IN ?", ids).Find(&sites).Error; err != nil {
		return nil, err
	}
	for i := range sites {
		names[sites[i].Id] = AdtyBrandFor(&sites[i]).Name
	}
	return names, nil
}

// InvalidateAdtySiteCache 在站点或自定义域名变更后调用，下次读取时重建。定价缓存一起清：
// 关站转移客户时会同时写定价。
func InvalidateAdtySiteCache() {
	adtySiteCacheMu.Lock()
	adtySiteCache = nil
	adtySiteCacheMu.Unlock()
	InvalidateAdtySitePriceCache()
}

func adtySiteCacheTTL() time.Duration {
	seconds := common.SyncFrequency
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func loadAdtySiteSnapshot() *adtySiteSnapshot {
	adtySiteCacheMu.Lock()
	defer adtySiteCacheMu.Unlock()
	if adtySiteCache != nil && time.Since(adtySiteCache.loadedAt) < adtySiteCacheTTL() {
		return adtySiteCache
	}
	snapshot := &adtySiteSnapshot{
		byId: map[int]*model.AdtySite{}, byHost: map[string]*model.AdtySite{},
		byCustom: map[string]*model.AdtySite{}, loadedAt: time.Now(),
	}
	var sites []model.AdtySite
	if err := model.DB.Find(&sites).Error; err != nil {
		// 读不到站点表时不缓存，下次再试；这次当成没有分站。
		common.SysError(common.LogText("load adty sites failed: %s", err.Error()))
		return snapshot
	}
	for i := range sites {
		site := &sites[i]
		snapshot.byId[site.Id] = site
		snapshot.byHost[strings.ToLower(site.Subdomain+"."+site.RootDomain)] = site
	}
	if AdtyLoadCustomDomains != nil {
		domains, err := AdtyLoadCustomDomains()
		if err != nil {
			common.SysError(common.LogText("load adty custom domains failed: %s", err.Error()))
		}
		for host, siteId := range domains {
			if site, ok := snapshot.byId[siteId]; ok {
				snapshot.byCustom[strings.ToLower(host)] = site
			}
		}
	}
	adtySiteCache = snapshot
	return snapshot
}

// AdtySiteById 从缓存取站点（只读，调用方不要改返回值）；没有返回 nil。
func AdtySiteById(siteId int) *model.AdtySite {
	if siteId <= 0 {
		return nil
	}
	return loadAdtySiteSnapshot().byId[siteId]
}

// AdtySiteForUser 是用户所属的站点；主站用户、查不到用户或站点时返回 nil（11.3 的邮件链接用它）。
func AdtySiteForUser(userId int) *model.AdtySite {
	if userId <= 0 {
		return nil
	}
	user, err := model.GetUserCache(userId)
	if err != nil {
		return nil
	}
	return AdtySiteById(user.SiteId)
}

// AdtySiteContextKey 是 gin 上下文里当前站点的键（11.2 的中间件写入）。
const AdtySiteContextKey = "adty_site"

// AdtyNormalizeHost 去掉端口、末尾的点，转小写。
func AdtyNormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func hostOfURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return AdtyNormalizeHost(parsed.Host)
}

// AdtyIsMainHost 说明这是不是主站自己的主机名：ServerAddress 和接入地址 api_base_url。
func AdtyIsMainHost(host string) bool {
	host = AdtyNormalizeHost(host)
	if host == "" {
		return false
	}
	return host == hostOfURL(system_setting.ServerAddress) || host == hostOfURL(adty_setting.GetSiteSetting().APIBaseURL)
}

// AdtyAPIKeyHostStatus checks the key owner's site against the trusted request
// Host. It resolves the host itself so every token-authenticated route is covered,
// including routes mounted without the site-context middleware.
func AdtyAPIKeyHostStatus(siteID int, host string) int {
	if AdtyIsMainHost(host) {
		if siteID == 0 {
			return 0
		}
		return http.StatusForbidden
	}
	if !resellerSetting().Enabled {
		return http.StatusNotFound
	}
	site := AdtySiteForHost(host)
	if site == nil {
		return http.StatusNotFound
	}
	if !AdtySiteServesCustomers(site) || site.Id != siteID {
		return http.StatusForbidden
	}
	return 0
}

// AdtySiteForHost 按主机名查站点：先查已 active 的自定义域名，再按「子域名.根域名」查。
// 没有对应站点返回 nil。站点状态由调用方判断（pending 的站点还没开，调用方当作查不到）。
// AdtyIsCustomDomainHost reports an active custom hostname, not a platform subdomain.
func AdtyIsCustomDomainHost(host string) bool {
	host = AdtyNormalizeHost(host)
	if host == "" {
		return false
	}
	_, ok := loadAdtySiteSnapshot().byCustom[host]
	return ok
}

func AdtySiteForHost(host string) *model.AdtySite {
	host = AdtyNormalizeHost(host)
	if host == "" {
		return nil
	}
	snapshot := loadAdtySiteSnapshot()
	if site, ok := snapshot.byCustom[host]; ok {
		return site
	}
	return snapshot.byHost[host]
}

// AdtyRejectSessionAPIStatus 判断这个 Host 能不能调用登录、注册、验证码和找回密码。
// 主站和还在服务的平台子域名返回 0。自定义域名和已关闭的站返回 403，其余 Host 返回 404。
func AdtyRejectSessionAPIStatus(host string) int {
	host = AdtyNormalizeHost(host)
	if host == "" || AdtyIsMainHost(host) || !resellerSetting().Enabled {
		return 0
	}
	snapshot := loadAdtySiteSnapshot()
	if _, ok := snapshot.byCustom[host]; ok {
		return http.StatusForbidden
	}
	if site := snapshot.byHost[host]; site != nil {
		if site.Status == model.AdtySiteStatusClosed {
			return http.StatusForbidden
		}
		return 0
	}
	return http.StatusNotFound
}

// AdtySiteForRequest 是当前请求的站点（11.2 的中间件放进上下文）；主站返回 nil。
// 只认站点表，不读 Host 头之外的东西（防 Host header injection，执行层决定 28）。
func AdtySiteForRequest(c *gin.Context) *model.AdtySite {
	if c == nil {
		return nil
	}
	value, ok := c.Get(AdtySiteContextKey)
	if !ok {
		return nil
	}
	site, _ := value.(*model.AdtySite)
	return site
}

// AdtySiteServesCustomers 说明站点现在对客户开放：active、paused、closing 都照常识别（11.2）。
func AdtySiteServesCustomers(site *model.AdtySite) bool {
	if site == nil {
		return false
	}
	switch site.Status {
	case model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing:
		return true
	}
	return false
}

// AdtySiteBaseURL 是站点的平台子域名地址，不用自定义域名；nil 返回 ServerAddress。
// 协议跟主站 ServerAddress 一样（生产是 https）。邮件链接和支付跳回都用它（11.2、11.3）。
func AdtySiteBaseURL(site *model.AdtySite) string {
	main := strings.TrimRight(system_setting.ServerAddress, "/")
	if site == nil {
		return main
	}
	scheme := "https"
	if parsed, err := url.Parse(main); err == nil && parsed.Scheme != "" {
		scheme = parsed.Scheme
	}
	return scheme + "://" + AdtySiteDisplayHost(site)
}

// AdtyOverrideStatus 在 /api/status 末尾调用（controller/misc.go）：当前请求是分站时，换成分站的站名、Logo、
// 地址，关掉第三方登录和 Passkey，并清掉主站页脚、公告和 Passkey 域名。
// Turnstile 继续用全局 site key，不在这里查 widget。
// GetStatus 持有 OptionMap 的读锁，这里只读请求上下文和进程内站点，不查库，也不再拿这把锁。
func AdtyOverrideStatus(c *gin.Context, data map[string]any) {
	// Check-in rewards are not a product. Hide the calendar even if an old
	// option row still says the feature is on.
	data["checkin_enabled"] = false
	site := AdtySiteForRequest(c)
	if site == nil {
		return
	}
	brand := AdtyBrandFor(site)
	data["system_name"] = brand.Name
	data["logo"] = brand.LogoURL
	data["site_title"] = brand.Title
	data["favicon"] = brand.FaviconURL
	data["theme_color"] = brand.ThemeColor
	data["server_address"] = AdtySiteBaseURL(site)
	data["adty_site"] = map[string]any{"id": site.Id, "status": site.Status, "subdomain": site.Subdomain}
	if version := adty_setting.GetLegalSetting().SiteCustomerTermsVersion; version > 0 {
		data["site_customer_terms_version"] = version
	}
	for _, key := range []string{"github_oauth", "discord_oauth", "linuxdo_oauth", "telegram_oauth", "wechat_login", "oidc_enabled", "passkey_login"} {
		data[key] = false
	}
	delete(data, "custom_oauth_providers")
	for _, key := range []string{"footer_html", "wechat_qrcode", "docs_link", "passkey_rp_id"} {
		data[key] = ""
	}
	for _, key := range []string{"chats", "api_info", "announcements", "faq", "passkey_rp_ids"} {
		data[key] = []any{}
	}
}

// AdtySiteLoginAllowed 是 site_login_only 的检查（11.2）：开着时分站客户只能在自己的分站登录，
// 主站用户只能在主站登录。只在建会话的入口调用（密码、二次验证、Passkey、第三方登录），API 令牌不受影响。
// 分站功能关着时中间件不识别站点，所有请求都算主站，这里不查库直接放行（上游测试数登录的查询次数）。
func AdtySiteLoginAllowed(c *gin.Context, userId int) bool {
	if !adtySessionHostAllowed(c) {
		return false
	}
	cfg := resellerSetting()
	if !cfg.Enabled || !cfg.SiteLoginOnly {
		return true
	}
	var users []model.User
	if err := model.DB.Select("id", "site_id").Where("id = ?", userId).Limit(1).Find(&users).Error; err != nil || len(users) == 0 {
		return false
	}
	current := 0
	if site := AdtySiteForRequest(c); site != nil {
		current = site.Id
	}
	return users[0].SiteId == current
}

// adtySessionHostAllowed 拒绝在自定义域名和已关闭分站的平台子域名上建立会话。
// 这些主机可以提供页面和 API，登录和二次验证必须回到平台子域名，已关闭的站直接拒绝。
func adtySessionHostAllowed(c *gin.Context) bool {
	if c == nil || c.Request == nil || !resellerSetting().Enabled {
		return true
	}
	host := AdtyNormalizeHost(c.Request.Host)
	if host == "" || AdtyIsMainHost(host) {
		return true
	}
	snapshot := loadAdtySiteSnapshot()
	if _, ok := snapshot.byCustom[host]; ok {
		return false
	}
	if site := snapshot.byHost[host]; site != nil && site.Status == model.AdtySiteStatusClosed {
		return false
	}
	return true
}

// AdtyRegisterSiteId 决定新注册用户归哪个站（11.2）：当前站点 active 或 paused 时归这个站；
// 主站返回 0。closing 的站已经通知客户要关了，不再接受注册（ok 为 false）。
func AdtyRegisterSiteId(c *gin.Context) (siteId int, ok bool) {
	site := AdtySiteForRequest(c)
	if site == nil {
		return 0, true
	}
	switch site.Status {
	case model.AdtySiteStatusActive, model.AdtySiteStatusPaused:
		return site.Id, true
	}
	return 0, false
}

// AdtyMainLegalConsentRequired is true when the main site has published an
// agreement or privacy policy that registration must accept and record.
func AdtyMainLegalConsentRequired() bool {
	legal := system_setting.GetLegalSettings()
	return strings.TrimSpace(legal.UserAgreement) != "" || strings.TrimSpace(legal.PrivacyPolicy) != ""
}

// AdtyRegisterTermsAccepted 是分站注册的客户条款检查（11.7）。主站，或条款还没发布
// （版本号为 0）时直接通过。分站上必须带查询参数 site_terms_version，而且等于当前版本。
func AdtyRegisterTermsAccepted(c *gin.Context, siteId int) bool {
	if siteId <= 0 || adty_setting.GetLegalSetting().SiteCustomerTermsVersion <= 0 {
		return true
	}
	if c == nil || c.Request == nil {
		return false
	}
	return adtyRequestSiteTermsVersion(c) == AdtyCurrentSiteCustomerTermsVersion()
}

// AdtyOAuthSiteTermsContextKey is set only by the OAuth callback after loading
// its immutable flow. A present empty value must override callback query input.
const AdtyOAuthSiteTermsContextKey = "adty_oauth_site_terms_version"

func adtyRequestSiteTermsVersion(c *gin.Context) string {
	if value, present := c.Get(AdtyOAuthSiteTermsContextKey); present {
		version, _ := value.(string)
		return strings.TrimSpace(version)
	}
	return strings.TrimSpace(c.Query("site_terms_version"))
}

// AdtyAcceptedSiteCustomerTerms 返回这次注册可以写入客户行的条款版本。版本没发布，
// 或查询参数不是当前版本时返回空，调用方不写 TermsVersion。
func AdtyAcceptedSiteCustomerTerms(c *gin.Context) string {
	if c == nil || c.Request == nil || adty_setting.GetLegalSetting().SiteCustomerTermsVersion <= 0 {
		return ""
	}
	current := AdtyCurrentSiteCustomerTermsVersion()
	if adtyRequestSiteTermsVersion(c) != current {
		return ""
	}
	return current
}

// AdtyCheckSiteCustomerRisk 由 11.5 注册：注册时按指纹判断疑似自用、疑似主站迁移，返回风控标记和原因。
var AdtyCheckSiteCustomerRisk func(siteId, userId int, ipHash, deviceHash string) (flag, reason string)

// AdtyRecordSiteCustomer 在分站注册成功后写客户行（和 users.site_id 同步）。出错只记日志，不影响注册。
func AdtyRecordSiteCustomer(userId, siteId int, ipHash, deviceHash, termsVersion string) {
	now := time.Now().Unix()
	customer := model.AdtySiteCustomer{UserId: userId, SiteId: siteId, JoinedAt: now, TermsVersion: termsVersion}
	if termsVersion != "" {
		customer.TermsAt = now
	}
	if AdtyCheckSiteCustomerRisk != nil {
		customer.RiskFlag, customer.RiskReason = AdtyCheckSiteCustomerRisk(siteId, userId, ipHash, deviceHash)
	}
	if err := model.DB.Create(&customer).Error; err != nil {
		common.SysError(common.LogText("record site customer failed user=%d site=%d: %s", userId, siteId, err.Error()))
	}
}
