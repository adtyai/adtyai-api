package relay

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMjImageDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Midjourney{}))
	model.DB = database
	t.Cleanup(func() {
		model.DB = previous
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func callMjImage(id, rawQuery string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/mj/image/"+id+"?"+rawQuery, nil)
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	RelayMidjourneyImage(ctx)
	return rec
}

// 未带签名时不能拉取上游，也不能把上游正文或 SSRF 细节返回给调用方。
func TestRelayMidjourneyImageRejectsUnsignedRequest(t *testing.T) {
	database := setupMjImageDB(t)
	service.InitHttpClient()
	const secret = "upstream-secret-body-do-not-leak"
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, secret, http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	previousFetch := *system_setting.GetFetchSetting()
	t.Cleanup(func() {
		*system_setting.GetFetchSetting() = previousFetch
	})
	parsed, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	fetch := system_setting.GetFetchSetting()
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = true
	fetch.AllowedPorts = append(append([]string{}, previousFetch.AllowedPorts...), parsed.Port())

	require.NoError(t, database.Create(&model.Midjourney{
		MjId:     "mj-open",
		ImageUrl: upstream.URL + "/secret.png",
		Status:   "SUCCESS",
	}).Error)

	rec := callMjImage("mj-open", "")
	body := rec.Body.String()
	require.Equal(t, http.StatusNotFound, rec.Code, body)
	require.NotContains(t, body, secret)
	require.NotContains(t, body, "request blocked")
	require.NotContains(t, body, "midjourney_task_not_found")
	require.Zero(t, hits)
}

func TestRelayMidjourneyImageSignedRequestDoesNotLeakUpstreamBody(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "mj-image-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	database := setupMjImageDB(t)
	service.InitHttpClient()
	const secret = "upstream-secret-body-do-not-leak"
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, secret, http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	previousFetch := *system_setting.GetFetchSetting()
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previousFetch })
	parsed, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	fetch := system_setting.GetFetchSetting()
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = true
	fetch.AllowedPorts = append(append([]string{}, previousFetch.AllowedPorts...), parsed.Port())

	require.NoError(t, database.Create(&model.Midjourney{
		MjId: "mj-signed", ImageUrl: upstream.URL + "/secret.png", Status: "SUCCESS",
	}).Error)
	access, err := service.IssueMidjourneyImageAccess("mj-signed")
	require.NoError(t, err)

	bad := callMjImage("mj-signed", "access=not-the-signature")
	require.Equal(t, http.StatusNotFound, bad.Code, bad.Body.String())
	require.NotContains(t, bad.Body.String(), secret)
	require.Zero(t, hits)

	rec := callMjImage("mj-signed", "access="+url.QueryEscape(access))
	body := rec.Body.String()
	require.Equal(t, http.StatusBadGateway, rec.Code, body)
	require.Contains(t, body, "upstream_image_unavailable")
	require.NotContains(t, body, secret)
	require.Equal(t, 1, hits)
}

func TestCoverMidjourneyTaskDtoSignsForwardImageURL(t *testing.T) {
	previousSecret := common.CryptoSecret
	previousForward := setting.MjForwardUrlEnabled
	previousAddress := system_setting.ServerAddress
	common.CryptoSecret = "mj-image-test-secret"
	setting.MjForwardUrlEnabled = true
	system_setting.ServerAddress = "https://example.test"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		setting.MjForwardUrlEnabled = previousForward
		system_setting.ServerAddress = previousAddress
	})

	signed := coverMidjourneyTaskDto(nil, &model.Midjourney{
		MjId: "mj-1", ImageUrl: "https://cdn.example/a.png", Status: "SUCCESS",
	})
	parsed, err := url.Parse(signed.ImageUrl)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "/mj/image/mj-1", parsed.Path)
	require.True(t, service.VerifyMidjourneyImageAccess("mj-1", parsed.Query().Get(service.TaskArtifactAccessQueryParameter)))
	require.False(t, service.VerifyMidjourneyImageAccess("mj-2", parsed.Query().Get(service.TaskArtifactAccessQueryParameter)))

	pending := coverMidjourneyTaskDto(nil, &model.Midjourney{
		MjId: "mj-1", ImageUrl: "https://cdn.example/a.png", Status: "IN_PROGRESS",
	})
	require.Contains(t, pending.ImageUrl, "&rand=")

	common.CryptoSecret = ""
	empty := coverMidjourneyTaskDto(nil, &model.Midjourney{
		MjId: "mj-1", ImageUrl: "https://cdn.example/a.png", Status: "SUCCESS",
	})
	require.Empty(t, empty.ImageUrl)
}
