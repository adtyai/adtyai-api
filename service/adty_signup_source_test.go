package service

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareSignupSourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	cleanupOnce.Do(startCleanupTask)
	workerBaseline := gopool.WorkerCount()
	db := adtyTestDB(t)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() <= workerBaseline }, 5*time.Second, time.Millisecond,
			"background quota jobs did not stop before restoring signup fixture globals")
		common.RedisEnabled = oldRedis
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.Option{}))
	require.NoError(t, model.MigrateAdtyTables())
	return db
}

// signupContext 是带 adty_src cookie 和 Referer 的注册请求。
func signupContext(cookie, referer string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", nil)
	if cookie != "" {
		c.Request.AddCookie(&http.Cookie{Name: AdtySignupSourceCookie, Value: cookie})
	}
	if referer != "" {
		c.Request.Header.Set("Referer", referer)
	}
	return c
}

func signupCookie(values map[string]string) string {
	query := url.Values{}
	for key, value := range values {
		query.Set(key, value)
	}
	return url.QueryEscape(query.Encode())
}

func signupSource(t *testing.T, userId int) model.AdtySignupSource {
	t.Helper()
	var row model.AdtySignupSource
	require.NoError(t, model.DB.Where("user_id = ?", userId).First(&row).Error)
	return row
}

func TestSignupSourceRecordsUtm(t *testing.T) {
	db := prepareSignupSourceDB(t)
	user := &model.User{Username: "src-utm", Password: "x", Group: "default", AuthVersion: 1, SiteId: 4}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: user.Id, InviterUserId: 77, AffiliateId: 1}).Error)

	c := signupContext(signupCookie(map[string]string{
		"utm_source": "google", "utm_medium": "cpc", "utm_campaign": "launch_2026", "landing": "/pricing",
		"ref_host": "news.ycombinator.com", "extra": "dropped",
	}), "https://www.adtyai.example/sign-up")
	recordAdtySignupSource(c, user, time.Unix(1_800_000_000, 0))

	row := signupSource(t, user.Id)
	assert.Equal(t, model.AdtySignupSource{
		UserId: user.Id, InviterUserId: 77, SiteId: 4, UtmSource: "google", UtmMedium: "cpc", UtmCampaign: "launch_2026",
		LandingPath: "/pricing", ReferrerHost: "news.ycombinator.com", CreatedAt: 1_800_000_000,
	}, row)

	// 没有 cookie 时用请求的 Referer 主机名，其余为空。
	other := &model.User{Username: "src-direct", Password: "x", Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(other).Error)
	recordAdtySignupSource(signupContext("", "https://t.co/abc"), other, time.Now())
	direct := signupSource(t, other.Id)
	assert.Equal(t, "t.co", direct.ReferrerHost)
	assert.Empty(t, direct.UtmSource)
	assert.Empty(t, direct.LandingPath)
}

func TestSignupSourceSanitized(t *testing.T) {
	cases := []struct {
		name   string
		cookie string
		want   AdtySignupSourceFields
	}{
		{"oversized value is dropped", signupCookie(map[string]string{"utm_source": strings.Repeat("a", 65), "utm_medium": "email"}),
			AdtySignupSourceFields{UtmMedium: "email"}},
		{"bad characters are dropped", signupCookie(map[string]string{"utm_source": "<script>", "utm_campaign": "x y", "utm_medium": "social-1.0"}),
			AdtySignupSourceFields{UtmMedium: "social-1.0"}},
		{"values are lower-cased", signupCookie(map[string]string{"utm_source": "Google"}), AdtySignupSourceFields{UtmSource: "google"}},
		{"landing must be a site path", signupCookie(map[string]string{"landing": "https://evil.example/x"}), AdtySignupSourceFields{}},
		{"protocol-relative landing is dropped", signupCookie(map[string]string{"landing": "//evil.example"}), AdtySignupSourceFields{}},
		{"landing drops the query", signupCookie(map[string]string{"landing": "/zh-cn?utm_source=x"}), AdtySignupSourceFields{LandingPath: "/zh-cn"}},
		{"bad host is dropped", signupCookie(map[string]string{"ref_host": "evil host"}), AdtySignupSourceFields{}},
		{"garbage cookie", "%%%", AdtySignupSourceFields{}},
		{"cookie too long", strings.Repeat("a", 2049), AdtySignupSourceFields{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseAdtySignupSourceCookie(tc.cookie))
		})
	}
}

func TestSourceStatsSums(t *testing.T) {
	db := prepareSignupSourceDB(t)
	from := time.Unix(1_800_000_000, 0)
	add := func(name, source string, at time.Time, usedUSD int, topups ...int) {
		user := &model.User{Username: name, Password: "x", Group: "default", AuthVersion: 1, UsedQuota: usedUSD * 500000}
		require.NoError(t, db.Create(user).Error)
		require.NoError(t, db.Create(&model.AdtySignupSource{UserId: user.Id, UtmSource: source, CreatedAt: at.Unix()}).Error)
		for i, amount := range topups {
			require.NoError(t, db.Create(&model.TopUp{UserId: user.Id, TradeNo: name + "-" + string(rune('a'+i)), Amount: int64(amount), Status: common.TopUpStatusSuccess}).Error)
		}
	}
	add("ss-g1", "google", from.Add(time.Hour), 3, 10, 20)
	add("ss-g2", "google", from.Add(2*time.Hour), 1)
	add("ss-t1", "twitter", from.Add(3*time.Hour), 0, 50)
	add("ss-old", "google", from.Add(-time.Hour), 9, 100)
	require.NoError(t, db.Create(&model.TopUp{UserId: 999, TradeNo: "ss-pending", Amount: 70, Status: "pending"}).Error)

	stats, err := model.AdtySignupSourceStats(model.AdtySignupSourceFilter{Dimension: "utm_source", From: from.Unix(), To: from.Add(24 * time.Hour).Unix(), Limit: 50})
	require.NoError(t, err)
	byKey := map[string]model.AdtySignupSourceStat{}
	for _, stat := range stats {
		byKey[stat.Key] = stat
	}
	require.Len(t, byKey, 2)
	assert.EqualValues(t, 2, byKey["google"].Signups)
	assert.EqualValues(t, 1, byKey["google"].Paying)
	assert.Equal(t, "30", byKey["google"].TopUpUSD.String())
	assert.Equal(t, "4", byKey["google"].ConsumeUSD.String())
	assert.EqualValues(t, 1, byKey["twitter"].Signups)
	assert.EqualValues(t, 1, byKey["twitter"].Paying)
	assert.Equal(t, "50", byKey["twitter"].TopUpUSD.String())
	assert.Equal(t, "0", byKey["twitter"].ConsumeUSD.String())
}
