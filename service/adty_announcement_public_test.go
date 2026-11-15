package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 匿名公开面板（`/api/status`）从新表读公告，且按 Host 分站点。
//
// 这条路没有认证中间件，所以只能按站点判断；下面用 gin 上下文把站点中间件
// 放进去的东西摆好，直接验函数本身。
func TestPublicAnnouncementsReadFromPublicationsPG(t *testing.T) {
	db := announcementTestDB(t)
	pgtest.RecordServerVersion(t, db)

	siteA := model.AdtySite{Subdomain: "pub-a", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&siteA).Error)

	published := time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)
	for _, row := range []model.AdtyNotificationPublication{
		{IssuerSiteId: 0, Audience: "all_users", Kind: "platform", Title: "维护通知",
			Content: "主站维护", Business: "legacy:type=warning", Status: "published",
			Revision: 1, ReadKey: "pub-all-users", PublishedAt: published.Unix()},
		{IssuerSiteId: 0, Audience: "all_sites", Kind: "system", Title: "", Content: "平台通知",
			Status: "published", Revision: 1, ReadKey: "pub-all-sites", PublishedAt: published.Add(time.Hour).Unix()},
		{IssuerSiteId: siteA.Id, Audience: "site", TargetSiteId: siteA.Id, Kind: "system",
			Title: "A 站通知", Content: "A 站内容", Status: "published", Revision: 1,
			ReadKey: "pub-site-a", PublishedAt: published.Add(2 * time.Hour).Unix()},
		{IssuerSiteId: 0, Audience: "all_users", Kind: "platform", Title: "草稿",
			Content: "不该出现", Status: "withdrawn", Revision: 2,
			ReadKey: "pub-withdrawn", PublishedAt: published.Unix()},
	} {
		row := row
		_, err := model.ImportLegacyAnnouncement(&row)
		require.NoError(t, err)
	}

	gin.SetMode(gin.TestMode)
	request := func(site *model.AdtySite) []AdtyPublicAnnouncement {
		t.Helper()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if site != nil {
			c.Set(AdtySiteContextKey, site)
		}
		return AdtyPublicAnnouncementsForRequest(c)
	}

	main := request(nil)
	mainContents := contentsOfPublic(main)
	assert.Contains(t, mainContents, "主站维护")
	assert.Contains(t, mainContents, "平台通知")
	assert.NotContains(t, mainContents, "A 站内容", "主站不读分站定向公告")
	assert.NotContains(t, mainContents, "不该出现", "撤回的公告不出现")

	// 字段形状必须和旧 option 一致：面板读 publishDate/type/extra，换数据源不换形状。
	var maintenance AdtyPublicAnnouncement
	for _, item := range main {
		if item.Content == "主站维护" {
			maintenance = item
		}
	}
	assert.Equal(t, "2026-11-01T02:00:00Z", maintenance.PublishDate)
	assert.Equal(t, "warning", maintenance.Type, "旧 type 是颜色语义，从 Business 里读回来")
	assert.Equal(t, "维护通知", maintenance.Extra)
	assert.Positive(t, maintenance.Id, "面板用它当 key")

	siteView := request(&siteA)
	siteContents := contentsOfPublic(siteView)
	assert.Contains(t, siteContents, "A 站内容")
	assert.Contains(t, siteContents, "平台通知")
	assert.NotContains(t, siteContents, "主站维护", "主站公告不同步到分站")
}

func contentsOfPublic(rows []AdtyPublicAnnouncement) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Content)
	}
	return out
}
