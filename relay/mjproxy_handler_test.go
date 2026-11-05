package relay

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 上游 code 21 表示任务已存在。重试不能再扣钱包，也不能为同一 mj_id 再插一行。
func TestMidjourneyCode21DoesNotChargeOrDuplicate(t *testing.T) {
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Midjourney{}, &model.Log{}))

	oldRedis := common.RedisEnabled
	oldBatch := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		common.BatchUpdateEnabled = oldBatch
	})
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	const startQuota = 1000000
	const alreadyCharged = 50000
	user := &model.User{
		Username: fmt.Sprintf("mj21%d", time.Now().UnixNano()%100000000),
		Password: "x",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    startQuota,
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         fmt.Sprintf("sk-mj21-%d", user.Id),
		Name:        "mj21",
		Status:      common.TokenStatusEnabled,
		RemainQuota: startQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)
	existing := &model.Midjourney{
		UserId:    user.Id,
		MjId:      "mj-exists-21",
		Action:    constant.MjActionImagine,
		Prompt:    "a cat",
		Status:    "SUBMITTED",
		Progress:  "0%",
		ChannelId: 2,
		Quota:     alreadyCharged,
	}
	require.NoError(t, model.DB.Create(existing).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		id := "mj-new-21"
		if strings.Contains(string(raw), "exists") {
			id = "mj-exists-21"
		}
		_, _ = fmt.Fprintf(w, `{"code":21,"description":"task exists","result":"%s","properties":{"status":"SUCCESS","imageUrl":"https://example.test/cat.png"}}`, id)
	}))
	t.Cleanup(upstream.Close)

	submit := func(t *testing.T, prompt string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		body := fmt.Sprintf(`{"prompt":%q}`, prompt)
		req := httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		ctx.Request = req
		ctx.Set("id", user.Id)
		ctx.Set("original_model", "mj_imagine")
		ctx.Set("group", "default")
		ctx.Set("user_group", "default")
		ctx.Set("base_url", upstream.URL)
		ctx.Set("channel_id", 2)
		ctx.Set("channel_type", constant.ChannelTypeMidjourney)
		ctx.Set("token_id", token.Id)
		ctx.Set("token_key", token.Key)
		ctx.Set("username", user.Username)

		info, infoErr := relaycommon.GenRelayInfo(ctx, kittypes.RelayFormatMjProxy, nil, nil)
		require.NoError(t, infoErr)
		price, priceErr := helper.ModelPriceHelperPerCall(ctx, info)
		require.NoError(t, priceErr)
		require.Greater(t, price.Quota, 0, "mj_imagine 必须有正价，否则测不到多扣")

		mjErr := RelayMidjourneySubmit(ctx, info)
		require.Nil(t, mjErr)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"code":1`)
		require.NotContains(t, recorder.Body.String(), `"code":21`)
	}

	submit(t, "a cat exists")
	quota, quotaErr := model.GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	require.Equal(t, startQuota, quota)
	var rows []model.Midjourney
	require.NoError(t, model.DB.Where("user_id = ? AND mj_id = ?", user.Id, "mj-exists-21").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, alreadyCharged, rows[0].Quota)

	submit(t, "a cat new")
	quota, quotaErr = model.GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	require.Equal(t, startQuota, quota)
	var created model.Midjourney
	require.NoError(t, model.DB.Where("user_id = ? AND mj_id = ?", user.Id, "mj-new-21").First(&created).Error)
	require.Zero(t, created.Quota)
	var remain int
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Select("remain_quota").Scan(&remain).Error)
	require.Equal(t, startQuota, remain)
}

func TestSimpleChangeUsesContentWhenTaskIdIsAbsent(t *testing.T) {
	mode := relayconstant.Path2RelayMode("/mj/submit/simple-change")
	require.Equal(t, relayconstant.RelayModeMidjourneySimpleChange, mode)
	modelName, mjErr, ok := service.GetMjRequestModel(mode, &dto.MidjourneyRequest{Content: "origin-simple U2"})
	require.True(t, ok)
	require.Nil(t, mjErr)
	require.Equal(t, "mj_upscale", modelName)

	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, opened.Close())
	})
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Midjourney{}, &model.Log{}))

	oldRedis := common.RedisEnabled
	oldBatch := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedis
		common.BatchUpdateEnabled = oldBatch
	})
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	var seenPath, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		seenBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"description":"submitted","result":"mj-simple-new"}`))
	}))
	t.Cleanup(upstream.Close)

	const startQuota = 1000000
	user := &model.User{
		Username: fmt.Sprintf("mjsc%d", time.Now().UnixNano()%100000000),
		Password: "x",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Quota:    startQuota,
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         fmt.Sprintf("sk-mjsc-%d", user.Id),
		Name:        "mjsc",
		Status:      common.TokenStatusEnabled,
		RemainQuota: startQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)
	baseURL := upstream.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeMidjourney,
		Key:     "mj-simple-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "mj-simple",
		BaseURL: &baseURL,
		Models:  "mj_upscale",
	}
	require.NoError(t, model.DB.Create(channel).Error)
	origin := &model.Midjourney{
		UserId:    user.Id,
		MjId:      "origin-simple",
		Action:    constant.MjActionImagine,
		Prompt:    "a cat",
		Status:    "SUCCESS",
		Progress:  "100%",
		ChannelId: channel.Id,
	}
	require.NoError(t, model.DB.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/mj/submit/simple-change", strings.NewReader(`{"content":"origin-simple U2"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("id", user.Id)
	ctx.Set("original_model", "mj_upscale")
	ctx.Set("group", "default")
	ctx.Set("user_group", "default")
	ctx.Set("token_id", token.Id)
	ctx.Set("token_key", token.Key)
	ctx.Set("username", user.Username)

	info, infoErr := relaycommon.GenRelayInfo(ctx, kittypes.RelayFormatMjProxy, nil, nil)
	require.NoError(t, infoErr)
	require.Equal(t, relayconstant.RelayModeMidjourneySimpleChange, info.RelayMode)
	submitErr := RelayMidjourneySubmit(ctx, info)
	require.Nil(t, submitErr)
	require.Equal(t, "/mj/submit/simple-change", seenPath)
	require.Contains(t, seenBody, "origin-simple U2")
	require.NotContains(t, seenBody, "task_id_is_required")

	var created model.Midjourney
	require.NoError(t, model.DB.Where("user_id = ? AND mj_id = ?", user.Id, "mj-simple-new").First(&created).Error)
	require.Equal(t, "UPSCALE", created.Action)
	quota, quotaErr := model.GetUserQuota(user.Id, true)
	require.NoError(t, quotaErr)
	require.Less(t, quota, startQuota)
}

func TestRelayMidjourneyImageRejectsNonImageBodies(t *testing.T) {
	previousDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Midjourney{}))
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<script>alert("mj-xss")</script>`))
	}))
	t.Cleanup(htmlServer.Close)
	svgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert("mj-svg")</script></svg>`))
	}))
	t.Cleanup(svgServer.Close)
	pngServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(pngServer.Close)

	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	fetchSetting.ApplyIPFilterForDomain = false
	fetchSetting.AllowedPorts = []string{
		strconv.Itoa(portOfTestServer(t, htmlServer)),
		strconv.Itoa(portOfTestServer(t, svgServer)),
		strconv.Itoa(portOfTestServer(t, pngServer)),
	}
	service.InitHttpClient()

	require.NoError(t, database.Create(&model.Midjourney{MjId: "mj-html", ImageUrl: htmlServer.URL, Status: "SUCCESS"}).Error)
	require.NoError(t, database.Create(&model.Midjourney{MjId: "mj-svg", ImageUrl: svgServer.URL, Status: "SUCCESS"}).Error)
	require.NoError(t, database.Create(&model.Midjourney{MjId: "mj-png", ImageUrl: pngServer.URL, Status: "SUCCESS"}).Error)

	htmlRecorder := httptest.NewRecorder()
	htmlCtx, _ := gin.CreateTestContext(htmlRecorder)
	htmlCtx.Params = gin.Params{{Key: "id", Value: "mj-html"}}
	htmlCtx.Request = signedMidjourneyImageRequest(t, "mj-html")
	RelayMidjourneyImage(htmlCtx)
	require.Equal(t, http.StatusUnsupportedMediaType, htmlRecorder.Code)
	require.NotContains(t, htmlRecorder.Body.String(), "mj-xss")
	require.NotContains(t, htmlRecorder.Header().Get("Content-Type"), "text/html")

	svgRecorder := httptest.NewRecorder()
	svgCtx, _ := gin.CreateTestContext(svgRecorder)
	svgCtx.Params = gin.Params{{Key: "id", Value: "mj-svg"}}
	svgCtx.Request = signedMidjourneyImageRequest(t, "mj-svg")
	RelayMidjourneyImage(svgCtx)
	require.Equal(t, http.StatusUnsupportedMediaType, svgRecorder.Code)
	require.NotContains(t, svgRecorder.Body.String(), "mj-svg")

	pngRecorder := httptest.NewRecorder()
	pngCtx, _ := gin.CreateTestContext(pngRecorder)
	pngCtx.Params = gin.Params{{Key: "id", Value: "mj-png"}}
	pngCtx.Request = signedMidjourneyImageRequest(t, "mj-png")
	RelayMidjourneyImage(pngCtx)
	require.Equal(t, http.StatusOK, pngRecorder.Code)
	require.Equal(t, "image/png", pngRecorder.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", pngRecorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, []byte{0x89, 'P', 'N', 'G'}, pngRecorder.Body.Bytes())
}

func TestRelayMidjourneyImagePinsProxyTargetAfterDNSCheck(t *testing.T) {
	previousDB := model.DB
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Midjourney{}, &model.Channel{}))
	model.DB = database
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
		lookupMidjourneyImageIPs = net.LookupIP
	})

	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.RequestURI)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(proxy.Close)

	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	fetchSetting.ApplyIPFilterForDomain = false
	fetchSetting.AllowedPorts = []string{"80", "443"}

	proxySetting := `{"proxy":"` + proxy.URL + `"}`
	channel := model.Channel{Type: 1, Key: "proxy-key", Name: "mj-proxy", Status: common.ChannelStatusEnabled, Setting: &proxySetting}
	require.NoError(t, database.Create(&channel).Error)
	require.NoError(t, database.Create(&model.Midjourney{
		MjId: "mj-pin", ChannelId: channel.Id, ImageUrl: "http://cdn.example.test/pic", Status: "SUCCESS",
	}).Error)

	lookupMidjourneyImageIPs = func(host string) ([]net.IP, error) {
		if host == "cdn.example.test" {
			return []net.IP{net.ParseIP("1.1.1.1")}, nil
		}
		return net.LookupIP(host)
	}
	okRecorder := httptest.NewRecorder()
	okCtx, _ := gin.CreateTestContext(okRecorder)
	okCtx.Params = gin.Params{{Key: "id", Value: "mj-pin"}}
	okCtx.Request = signedMidjourneyImageRequest(t, "mj-pin")
	RelayMidjourneyImage(okCtx)
	require.Equal(t, http.StatusOK, okRecorder.Code, okRecorder.Body.String())
	require.Equal(t, []byte{0x89, 'P', 'N', 'G'}, okRecorder.Body.Bytes())
	require.NotEmpty(t, seen)
	require.Contains(t, seen[0], "1.1.1.1")
	require.NotContains(t, seen[0], "cdn.example.test")

	seen = nil
	lookupMidjourneyImageIPs = func(host string) ([]net.IP, error) {
		if host == "cdn.example.test" {
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		}
		return net.LookupIP(host)
	}
	blocked := httptest.NewRecorder()
	blockedCtx, _ := gin.CreateTestContext(blocked)
	blockedCtx.Params = gin.Params{{Key: "id", Value: "mj-pin"}}
	blockedCtx.Request = signedMidjourneyImageRequest(t, "mj-pin")
	RelayMidjourneyImage(blockedCtx)
	require.Equal(t, http.StatusForbidden, blocked.Code, blocked.Body.String())
	require.Empty(t, seen)
	require.NotContains(t, blocked.Body.String(), "PNG")
}

func signedMidjourneyImageRequest(t *testing.T, id string) *http.Request {
	t.Helper()
	path, err := service.MidjourneyImagePath(id, time.Now())
	require.NoError(t, err)
	return httptest.NewRequest(http.MethodGet, path, nil)
}

func TestRelayMidjourneyImageRejectsUnsignedRequest(t *testing.T) {
	previousDB := model.DB
	previousCache := common.MemoryCacheEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Midjourney{}))
	model.DB = database
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
	})
	var hits int
	pngServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(pngServer.Close)
	require.NoError(t, database.Create(&model.Midjourney{MjId: "mj-private", ImageUrl: pngServer.URL, Status: "SUCCESS"}).Error)
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.AllowedPorts = []string{strconv.Itoa(portOfTestServer(t, pngServer))}
	service.InitHttpClient()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "mj-private"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/mj/image/mj-private", nil)
	RelayMidjourneyImage(ctx)

	require.NotEqual(t, http.StatusOK, recorder.Code)
	require.NotEqual(t, []byte{0x89, 'P', 'N', 'G'}, recorder.Body.Bytes())
	require.Zero(t, hits)

	expiredAt := time.Now().Add(-time.Minute).Unix()
	expired, err := service.IssueMidjourneyImageAccess("mj-private", expiredAt)
	require.NoError(t, err)
	expiredRecorder := httptest.NewRecorder()
	expiredCtx, _ := gin.CreateTestContext(expiredRecorder)
	expiredCtx.Params = gin.Params{{Key: "id", Value: "mj-private"}}
	expiredCtx.Request = httptest.NewRequest(http.MethodGet, "/mj/image/mj-private?sig="+url.QueryEscape(expired)+"&exp="+strconv.FormatInt(expiredAt, 10), nil)
	RelayMidjourneyImage(expiredCtx)
	require.NotEqual(t, http.StatusOK, expiredRecorder.Code)
	require.Zero(t, hits)
}

func portOfTestServer(t *testing.T, server *httptest.Server) int {
	t.Helper()
	_, portText, ok := strings.Cut(server.Listener.Addr().String(), ":")
	require.True(t, ok)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return port
}
