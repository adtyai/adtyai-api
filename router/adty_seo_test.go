package router

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seoIndexPage 是 web/index.html 构建后的样子：占位区里有默认标题、描述和 noindex。
const seoIndexPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <!--adty-head-->
    <title>ADTY AI</title>
    <meta name="description" content="default" />
    <meta name="robots" content="noindex" />
    <!--/adty-head-->
    <!--umami-->
  </head>
  <body>
    <div id="root" translate="no" class="notranslate"></div>
  </body>
</html>
`

const seoPrerender = `{"en":"<h1>EN hero</h1>","zh-CN":"<h1>中文首屏</h1>","zh-TW":"<h1>繁體首屏</h1>","fr":"<h1>FR</h1>","ja":"<h1>JA</h1>","ko":"<h1>한국어 첫 화면</h1>","ru":"<h1>RU</h1>","vi":"<h1>VI</h1>"}`

type seoFixture struct {
	serverAddress string
	title         map[string]string
	description   map[string]string
	homeContent   string
	files         fstest.MapFS
}

func newSEOFixture() seoFixture {
	return seoFixture{
		serverAddress: "https://adtyai.com",
		title:         map[string]string{"en": "ADTY AI - one key", "zh-CN": "ADTY AI 一个密钥"},
		description:   map[string]string{"en": "English description"},
		files: fstest.MapFS{
			"web/dist/adty-home.json": {Data: []byte(seoPrerender)},
		},
	}
}

// serve 按 fixture 设好配置，建一个只挂 AdtySEOHomePages 和一个返回 index.html 的兜底处理的引擎，发一个 GET。
func (f seoFixture) serve(t *testing.T, host, target string) *httptest.ResponseRecorder {
	t.Helper()
	oldAddress := system_setting.ServerAddress
	oldSEO := *adty_setting.GetSEOSetting()
	common.OptionMapRWMutex.Lock()
	oldHome, hadHome := common.OptionMap["HomePageContent"]
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap["HomePageContent"] = f.homeContent
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		system_setting.ServerAddress = oldAddress
		*adty_setting.GetSEOSetting() = oldSEO
		common.OptionMapRWMutex.Lock()
		if hadHome {
			common.OptionMap["HomePageContent"] = oldHome
		} else {
			delete(common.OptionMap, "HomePageContent")
		}
		common.OptionMapRWMutex.Unlock()
	})
	system_setting.ServerAddress = f.serverAddress
	seo := adty_setting.GetSEOSetting()
	seo.HomeTitle = f.title
	seo.HomeDescription = f.description
	seo.OGImage = "/adty-og.png"

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(AdtySEOHomePages([]byte(seoIndexPage), f.files), func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(seoIndexPage))
	})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestHomePagesSEOHead(t *testing.T) {
	cases := []struct {
		path, lang, title, locale string
	}{
		{"/", "en", "ADTY AI - one key", "en_US"},
		{"/zh-cn", "zh-Hans", "ADTY AI 一个密钥", "zh_CN"},
		{"/zh-tw", "zh-Hant", "ADTY AI - one key", "zh_TW"},
		{"/fr", "fr", "ADTY AI - one key", "fr_FR"},
		{"/ja", "ja", "ADTY AI - one key", "ja_JP"},
		{"/ko", "ko", "ADTY AI - one key", "ko_KR"},
		{"/ru", "ru", "ADTY AI - one key", "ru_RU"},
		{"/vi", "vi", "ADTY AI - one key", "vi_VN"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := newSEOFixture().serve(t, "adtyai.com", tc.path)
			body := rec.Body.String()

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
			assert.Contains(t, body, `<html lang="`+tc.lang+`" data-adty-seo="1">`)
			assert.Equal(t, 1, strings.Count(body, "<title>"))
			assert.Contains(t, body, "<title>"+tc.title+"</title>")
			// 只有 en 填了描述，其余语言回退 en
			assert.Contains(t, body, `<meta name="description" content="English description" />`)
			canonical := "https://adtyai.com" + tc.path
			assert.Contains(t, body, `<link rel="canonical" href="`+canonical+`" />`)
			assert.Contains(t, body, `<meta property="og:url" content="`+canonical+`" />`)
			assert.Contains(t, body, `<meta property="og:locale" content="`+tc.locale+`" />`)
			assert.Contains(t, body, `<meta property="og:image" content="https://adtyai.com/adty-og.png" />`)
			assert.Contains(t, body, `<meta name="twitter:card" content="summary_large_image" />`)
			assert.Equal(t, 9, strings.Count(body, `<link rel="alternate" hreflang=`))
			assert.Contains(t, body, `<link rel="alternate" hreflang="x-default" href="https://adtyai.com/" />`)
			assert.Contains(t, body, `<link rel="alternate" hreflang="zh-Hant" href="https://adtyai.com/zh-tw" />`)
			assert.NotContains(t, body, "noindex")
			assert.NotContains(t, body, "<!--adty-head-->")
			prerender := strings.Index(body, `<div id="adty-prerender">`)
			root := strings.Index(body, `<div id="root"`)
			require.GreaterOrEqual(t, prerender, 0, "正文要插进去")
			assert.Less(t, prerender, root, "正文在 #root 前面")
			// #root 必须保持为空，main.tsx 才会挂载应用
			assert.Contains(t, body, `<div id="root" translate="no" class="notranslate"></div>`)
			assert.Contains(t, body, "<!--umami-->", "统计脚本的占位不能丢")
		})
	}
	rec := newSEOFixture().serve(t, "adtyai.com", "/zh-cn")
	assert.Contains(t, rec.Body.String(), "<h1>中文首屏</h1>")
	ko := newSEOFixture().serve(t, "adtyai.com", "/ko")
	assert.Contains(t, ko.Body.String(), "<h1>한국어 첫 화면</h1>")
}

func TestHomeTitleFallsBackToBrand(t *testing.T) {
	f := newSEOFixture()
	f.title = nil
	f.description = nil
	body := f.serve(t, "adtyai.com", "/fr").Body.String()

	assert.Contains(t, body, "<title>"+common.AdtyBrandName+"</title>")
	assert.NotContains(t, body, `name="description"`)
}

func TestHomeCanonicalDropsQueryAndSlash(t *testing.T) {
	f := newSEOFixture()

	root := f.serve(t, "adtyai.com", "/?ref=abc").Body.String()
	assert.Contains(t, root, `<link rel="canonical" href="https://adtyai.com/" />`)
	assert.NotContains(t, root, "ref=abc")

	zh := f.serve(t, "adtyai.com", "/zh-cn/")
	require.Equal(t, http.StatusOK, zh.Code)
	assert.Contains(t, zh.Body.String(), `<link rel="canonical" href="https://adtyai.com/zh-cn" />`)
}

func TestNonHomePagesKeepNoindex(t *testing.T) {
	f := newSEOFixture()
	for _, path := range []string{"/pricing", "/sign-in", "/dashboard", "/xx", "/zh-cn/pricing"} {
		body := f.serve(t, "adtyai.com", path).Body.String()
		assert.Contains(t, body, `<meta name="robots" content="noindex" />`, path)
		assert.NotContains(t, body, "data-adty-seo", path)
	}
	assert.NotContains(t, f.serve(t, "adtyai.com", "/").Body.String(), "noindex")
}

func TestHomePagesOnlyHandleGetAndHead(t *testing.T) {
	f := newSEOFixture()
	f.serve(t, "adtyai.com", "/") // 先设好配置
	engine := gin.New()
	engine.NoRoute(AdtySEOHomePages([]byte(seoIndexPage), f.files), func(c *gin.Context) {
		c.String(http.StatusTeapot, "fallthrough")
	})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Equal(t, http.StatusTeapot, rec.Code)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	req.Host = "adtyai.com"
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSEOValuesEscaped(t *testing.T) {
	f := newSEOFixture()
	f.title = map[string]string{"en": `</title><script>alert(1)</script>`}
	f.description = map[string]string{"en": `"><img src=x onerror=alert(1)>`}
	body := f.serve(t, "adtyai.com", "/").Body.String()

	assert.NotContains(t, body, "<script>alert(1)</script>")
	assert.NotContains(t, body, `<img src=x`)
	assert.Contains(t, body, "&lt;/title&gt;&lt;script&gt;")
	assert.Contains(t, body, `content="&#34;&gt;&lt;img src=x onerror=alert(1)&gt;"`)
	// 结构化数据里的站名和网址也不能带出能闭合 script 的字符
	jsonld := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(body)
	require.Len(t, jsonld, 2)
	assert.NotContains(t, jsonld[1], "<")
}

func TestNoServerAddressOmitsCanonical(t *testing.T) {
	for _, address := range []string{"", "http://localhost:3000"} {
		f := newSEOFixture()
		f.serverAddress = address
		body := f.serve(t, "adtyai.com", "/").Body.String()

		assert.Contains(t, body, "<title>ADTY AI - one key</title>", address)
		assert.NotContains(t, body, `rel="canonical"`, address)
		assert.NotContains(t, body, `hreflang=`, address)
		assert.NotContains(t, body, `og:url`, address)
		assert.NotContains(t, body, `application/ld+json`, address)
	}
}

func TestOtherHostNoindex(t *testing.T) {
	f := newSEOFixture()
	body := f.serve(t, "api.adtyai.com", "/").Body.String()
	assert.Contains(t, body, `<meta name="robots" content="noindex" />`)

	// 带端口和大小写不同不算另一个域名
	f.serverAddress = "https://AdtyAI.com:443"
	body = f.serve(t, "adtyai.com:443", "/").Body.String()
	assert.NotContains(t, body, "noindex")
}

func TestCustomHomeSkipsPrerender(t *testing.T) {
	f := newSEOFixture()
	f.homeContent = "# Custom"
	body := f.serve(t, "adtyai.com", "/").Body.String()

	assert.Contains(t, body, "<title>ADTY AI - one key</title>")
	assert.NotContains(t, body, "adty-prerender")
}

func TestWebsiteJSONLDOnlyOnRoot(t *testing.T) {
	f := newSEOFixture()
	root := f.serve(t, "adtyai.com", "/").Body.String()
	assert.Contains(t, root, `<script type="application/ld+json">`)
	assert.Contains(t, root, `"@type":"WebSite"`)
	assert.Contains(t, root, `"name":"ADTY AI"`)
	assert.Contains(t, root, `"alternateName":["adtyai","adtyai.com"]`)
	assert.Contains(t, root, `"url":"https://adtyai.com/"`)

	assert.NotContains(t, f.serve(t, "adtyai.com", "/ja").Body.String(), "application/ld+json")
}

func TestMissingPrerenderFileStillServesHead(t *testing.T) {
	f := newSEOFixture()
	f.files = fstest.MapFS{}
	rec := f.serve(t, "adtyai.com", "/")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<title>ADTY AI - one key</title>")
	assert.NotContains(t, rec.Body.String(), "adty-prerender")
}
