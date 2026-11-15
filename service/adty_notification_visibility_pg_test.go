package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 可见性规则必须在服务层单独验一遍。
//
// 路由层那两条 `TestNotification*Isolation` 现在红在 401（`adtySessionToken`
// 不写 IssuedSiteID，和 0183643 同根因，总管明令不单独修），所以它们承载的
// 可见性断言**目前没有被任何绿测试守着**。我把规则本身在这一层钉住：
// 不经过认证，直接问 `adtyVisibleNotificationPublications` 谁能看到什么。
func TestNotificationVisibilityRulePG(t *testing.T) {
	db := announcementTestDB(t)
	var version int
	require.NoError(t, db.Raw("SHOW server_version_num").Scan(&version).Error)
	require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")

	siteA := model.AdtySite{Subdomain: "vis-a", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&siteA).Error)
	siteB := model.AdtySite{Subdomain: "vis-b", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&siteB).Error)

	ownerA := ticketTestUser(t, db, "vis-owner-a", 1)
	memberA := ticketTestUser(t, db, "vis-member-a", 1)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", siteA.Id).Update("owner_user_id", ownerA.Id).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", []int{ownerA.Id, memberA.Id}).Update("site_id", siteA.Id).Error)
	// ownerA 同时是 A 站所有人；memberA 只是 A 站成员。
	outsider := ticketTestUser(t, db, "vis-outsider", 1) // site_id = 0，主站账号
	mainUser := ticketTestUser(t, db, "vis-main", 1)

	for _, row := range []model.AdtyNotificationPublication{
		{IssuerSiteId: 0, Audience: "all_users", Kind: "system", Title: "主站公告",
			Content: "主站公告", Status: "published", Revision: 1, ReadKey: "vis-all-users", PublishedAt: 1},
		{IssuerSiteId: 0, Audience: "all_sites", Kind: "system", Title: "平台公告",
			Content: "平台公告", Status: "published", Revision: 1, ReadKey: "vis-all-sites", PublishedAt: 2},
		{IssuerSiteId: siteA.Id, Audience: "site", TargetSiteId: siteA.Id, Kind: "system", Title: "A 站公告",
			Content: "A 站公告", Status: "published", Revision: 1, ReadKey: "vis-site-a", PublishedAt: 3},
		{IssuerSiteId: siteB.Id, Audience: "site", TargetSiteId: siteB.Id, Kind: "system", Title: "B 站公告",
			Content: "B 站公告", Status: "published", Revision: 1, ReadKey: "vis-site-b", PublishedAt: 4},
	} {
		row := row
		_, err := model.ImportLegacyAnnouncement(&row)
		require.NoError(t, err)
	}

	visible := func(userID int, site *model.AdtySite) []string {
		t.Helper()
		query, err := adtyVisibleNotificationPublications(userID, site)
		require.NoError(t, err)
		var rows []model.AdtyNotificationPublication
		require.NoError(t, query.Order("id ASC").Find(&rows).Error)
		return contentsOf(rows)
	}

	// 主站：看得到主站公告和平台公告，看不到任何分站定向公告。
	mainView := visible(mainUser.Id, nil)
	assert.Contains(t, mainView, "主站公告")
	assert.Contains(t, mainView, "平台公告")
	assert.NotContains(t, mainView, "A 站公告")
	assert.NotContains(t, mainView, "B 站公告")

	// A 站成员：**看不到主站公告**（老板明确否掉的形态），看得到平台公告和自己的。
	aView := visible(memberA.Id, &siteA)
	assert.NotContains(t, aView, "主站公告", "主站公告不同步到分站")
	assert.Contains(t, aView, "平台公告")
	assert.Contains(t, aView, "A 站公告")
	assert.NotContains(t, aView, "B 站公告")

	// A 站所有人：同上，多的是管理权限，不是可见范围。
	ownerView := visible(ownerA.Id, &siteA)
	assert.NotContains(t, ownerView, "主站公告")
	assert.Contains(t, ownerView, "A 站公告")

	// 只是访问了这个域名、并不属于这个站：要代码又给了它一份站内定向内容。
	outsiderView := visible(outsider.Id, &siteA)
	assert.NotContains(t, outsiderView, "主站公告")
	assert.Contains(t, outsiderView, "平台公告", "平台级公告对所有人可见")
	assert.NotContains(t, outsiderView, "A 站公告", "不属于这个站就拿不到站内定向内容")

	_ = gorm.ErrRecordNotFound
}
