package router

import (
	"bytes"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// adtySEOHomeLang 是首页的一个语言网址（任务 9.3，执行层决定 22）。
type adtySEOHomeLang struct {
	path        string // 网址路径，也是 canonical 的路径
	contentLang string // 约定 E 的内容语言码，配置和预渲染文件按它取值
	hreflang    string
	ogLocale    string
}

// adtySEOHomeLangs 的顺序就是 hreflang 标签的顺序；网址里的语言段是内容语言码转小写。
var adtySEOHomeLangs = []adtySEOHomeLang{
	{"/", "en", "en", "en_US"},
	{"/zh-cn", "zh-CN", "zh-Hans", "zh_CN"},
	{"/zh-tw", "zh-TW", "zh-Hant", "zh_TW"},
	{"/fr", "fr", "fr", "fr_FR"},
	{"/ja", "ja", "ja", "ja_JP"},
	{"/ru", "ru", "ru", "ru_RU"},
	{"/vi", "vi", "vi", "vi_VN"},
}

const (
	adtySEOHeadStart     = "<!--adty-head-->"
	adtySEOHeadEnd       = "<!--/adty-head-->"
	adtySEOPrerenderFile = "web/dist/adty-home.json"
	// 上游 system_setting.ServerAddress 的默认值，没改过就当没设
	adtyDefaultServerAddress = "http://localhost:3000"
)

// adtyPrerender 是构建时预渲染的首页正文（web/scripts/adty/prerender-home.tsx 写的 adty-home.json），
// 第一次用到时读一次。读不到只告警一次，之后只写头部。
type adtyPrerender struct {
	once  sync.Once
	pages map[string]string
}

func (p *adtyPrerender) load(buildFS fs.FS) map[string]string {
	p.once.Do(func() {
		data, err := fs.ReadFile(buildFS, adtySEOPrerenderFile)
		if err == nil {
			err = common.Unmarshal(data, &p.pages)
		}
		if err != nil {
			common.SysError(common.LogText("home page prerender %s unavailable, serving head tags only: %s", adtySEOPrerenderFile, err.Error()))
		}
	})
	return p.pages
}

// AdtySEOHomePages 处理首页 7 个语言网址的 GET 和 HEAD：把 index.html 里 <!--adty-head-->
// 占位区换成这一页的标题、描述、canonical、hreflang 和分享标签，<html lang> 换成这一页的语言，
// 并在 #root 前插入预渲染的正文（任务 9.3，执行层决定 23）。其余请求原样交给后面的处理。
// indexPage 用 main.go 注入过统计脚本的那份。
func AdtySEOHomePages(indexPage []byte, buildFS fs.FS) gin.HandlerFunc {
	prerender := &adtyPrerender{}
	var warnAddressOnce sync.Once
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			return
		}
		page, ok := adtySEOHomePage(c.Request.URL.Path)
		if !ok {
			return
		}

		// 分站（任务 11.3）：默认 noindex；开了 SeoIndex 才写正式头部，canonical 指向分站自己的地址。
		// 预渲染的正文是平台的落地页（顶栏写的是平台站名），分站不插正文，只写头部。
		if site := service.AdtySiteForRequest(c); site != nil {
			out := adtyRenderHomePage(indexPage, page, adtySiteSEO(site), "")
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", out)
			c.Abort()
			return
		}

		base := adtySEOBaseURL()
		if base == nil {
			warnAddressOnce.Do(func() {
				common.SysError(common.LogText("server address is not set; home pages are served without canonical and hreflang tags"))
			})
		}
		common.OptionMapRWMutex.RLock()
		customHome := strings.TrimSpace(common.OptionMap["HomePageContent"]) != ""
		common.OptionMapRWMutex.RUnlock()

		body := ""
		if !customHome {
			body = prerender.load(buildFS)[page.contentLang]
		}
		noindex := base != nil && !adtySameHost(c.Request.Host, base)
		out := adtyRenderHomePage(indexPage, page, adtyMainSEO(base, noindex), body)

		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", out)
		c.Abort()
	}
}

// adtySEOHomePage 按路径找首页语言，末尾的 / 不算。
func adtySEOHomePage(path string) (adtySEOHomeLang, bool) {
	trimmed := strings.TrimRight(path, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	for _, page := range adtySEOHomeLangs {
		if page.path == trimmed {
			return page, true
		}
	}
	return adtySEOHomeLang{}, false
}

// adtySEOBaseURL 是后台的服务器地址；没设或还是上游默认值时返回 nil。
func adtySEOBaseURL() *url.URL {
	raw := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if raw == "" || raw == adtyDefaultServerAddress {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed
}

// adtySameHost 比较请求的域名和服务器地址的域名，不看大小写和端口。
func adtySameHost(requestHost string, base *url.URL) bool {
	host := requestHost
	if h, _, err := net.SplitHostPort(requestHost); err == nil {
		host = h
	}
	return strings.EqualFold(host, base.Hostname())
}

func adtyRenderHomePage(indexPage []byte, page adtySEOHomeLang, head adtySEOHeadInput, body string) []byte {
	headTags := adtySEOHead(page, head)
	out := indexPage
	start := bytes.Index(out, []byte(adtySEOHeadStart))
	end := bytes.Index(out, []byte(adtySEOHeadEnd))
	if start >= 0 && end > start {
		replaced := make([]byte, 0, len(out)+len(headTags))
		replaced = append(replaced, out[:start]...)
		replaced = append(replaced, headTags...)
		replaced = append(replaced, out[end+len(adtySEOHeadEnd):]...)
		out = replaced
	}
	out = bytes.Replace(out, []byte(`<html lang="en">`),
		[]byte(`<html lang="`+page.hreflang+`" data-adty-seo="1">`), 1)
	if body != "" {
		prerendered := []byte(`<div id="adty-prerender">` + body + `</div>` + "\n    ")
		out = bytes.Replace(out, []byte(`<div id="root"`), append(prerendered, []byte(`<div id="root"`)...), 1)
	}
	return out
}

// adtySEOHeadInput 是一页首页头部的内容：主站来自 adty_seo_setting，分站来自站点品牌（任务 11.3）。
type adtySEOHeadInput struct {
	siteName    string
	title       func(lang string) string
	description func(lang string) string
	image       string
	base        *url.URL // nil 时不写 canonical、hreflang、og:url、结构化数据
	noindex     bool
	websiteData bool // 只有主站的 / 写 WebSite 结构化数据
}

// adtyMainSEO 是主站首页的头部。
func adtyMainSEO(base *url.URL, noindex bool) adtySEOHeadInput {
	seo := adty_setting.GetSEOSetting()
	return adtySEOHeadInput{
		siteName: common.AdtyBrandName,
		title: func(lang string) string {
			if title := adtyPickSEOText(seo.HomeTitle, lang); title != "" {
				return title
			}
			return common.AdtyBrandName
		},
		description: func(lang string) string { return adtyPickSEOText(seo.HomeDescription, lang) },
		image:       seo.OGImage,
		base:        base,
		noindex:     noindex,
		websiteData: true,
	}
}

// adtySiteSEO 是分站首页的头部：标题是站名（有标语时加上），描述是标语，分享图是 Logo。
// 没开 SeoIndex 时 noindex，不写 canonical；开了时 canonical 指向分站自己的地址，不指向主站。
func adtySiteSEO(site *model.AdtySite) adtySEOHeadInput {
	brand := service.AdtyBrandFor(site)
	title := brand.Name
	if tagline := strings.TrimSpace(brand.Tagline); tagline != "" {
		title = brand.Name + " - " + tagline
	}
	input := adtySEOHeadInput{
		siteName:    brand.Name,
		title:       func(string) string { return title },
		description: func(string) string { return strings.TrimSpace(brand.Tagline) },
		image:       brand.LogoURL,
		noindex:     !site.SeoIndex,
	}
	if site.SeoIndex {
		if parsed, err := url.Parse(brand.BaseURL); err == nil && parsed.Host != "" {
			input.base = parsed
		}
	}
	return input
}

// adtySEOHead 生成占位区换上的标签。所有值都经过 html.EscapeString。
func adtySEOHead(page adtySEOHomeLang, head adtySEOHeadInput) string {
	title := head.title(page.contentLang)
	description := head.description(page.contentLang)
	base := head.base

	var b strings.Builder
	esc := html.EscapeString
	writeMeta := func(attr, name, content string) {
		b.WriteString(`<meta ` + attr + `="` + name + `" content="` + esc(content) + `" />` + "\n    ")
	}
	b.WriteString("<title>" + esc(title) + "</title>\n    ")
	if description != "" {
		writeMeta("name", "description", description)
	}
	if head.noindex {
		writeMeta("name", "robots", "noindex")
	}
	if base != nil {
		origin := base.String()
		canonical := origin + page.path
		b.WriteString(`<link rel="canonical" href="` + esc(canonical) + `" />` + "\n    ")
		for _, alt := range adtySEOHomeLangs {
			b.WriteString(`<link rel="alternate" hreflang="` + alt.hreflang + `" href="` + esc(origin+alt.path) + `" />` + "\n    ")
		}
		b.WriteString(`<link rel="alternate" hreflang="x-default" href="` + esc(origin+"/") + `" />` + "\n    ")
		writeMeta("property", "og:url", canonical)
	}
	writeMeta("property", "og:type", "website")
	writeMeta("property", "og:site_name", head.siteName)
	writeMeta("property", "og:title", title)
	if description != "" {
		writeMeta("property", "og:description", description)
	}
	if image := adtySEOImageURL(head.image, base); image != "" {
		writeMeta("property", "og:image", image)
	}
	writeMeta("property", "og:locale", page.ogLocale)
	writeMeta("name", "twitter:card", "summary_large_image")
	if head.websiteData && base != nil && page.path == "/" {
		// Google 只认根域名首页的这段数据来决定搜索结果里的站名
		data, err := common.Marshal(map[string]any{
			"@context":      "https://schema.org",
			"@type":         "WebSite",
			"name":          common.AdtyBrandName,
			"alternateName": []string{common.AdtyBrandSlug, strings.TrimPrefix(common.AdtyBrandURL, "https://")},
			"url":           base.String() + "/",
		})
		if err == nil {
			b.WriteString(`<script type="application/ld+json">` + string(data) + "</script>\n    ")
		}
	}
	return strings.TrimRight(b.String(), " \n")
}

// adtyPickSEOText 取这种语言的文本，没填就用 en 的。
func adtyPickSEOText(texts map[string]string, lang string) string {
	if text := strings.TrimSpace(texts[lang]); text != "" {
		return text
	}
	return strings.TrimSpace(texts["en"])
}

// adtySEOImageURL 把以 / 开头的分享图拼成绝对地址；没有服务器地址时只能写相对地址。
func adtySEOImageURL(image string, base *url.URL) string {
	image = strings.TrimSpace(image)
	if image == "" || !strings.HasPrefix(image, "/") || strings.HasPrefix(image, "//") {
		return image
	}
	if base == nil {
		return image
	}
	return base.String() + image
}

// AdtyRobotsTxt 按站点返回 /robots.txt（任务 11.3）：没开 SEO 的分站整站 Disallow；主站和开了 SEO 的分站
// 只屏蔽接口。站点由 11.2 的 AdtySiteContext 放进上下文。
func AdtyRobotsTxt() gin.HandlerFunc {
	return func(c *gin.Context) {
		body := "User-agent: *\nDisallow: /api/\nDisallow: /v1/\nDisallow: /pay/\n"
		if site := service.AdtySiteForRequest(c); site != nil && !site.SeoIndex {
			body = "User-agent: *\nDisallow: /\n"
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
	}
}
