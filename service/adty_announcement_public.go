package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdtyPublicAnnouncementLimit 沿用旧 option 的上限（旧校验写的就是 100 条）。
const AdtyPublicAnnouncementLimit = 100

// AdtyPublicAnnouncement 是公开面板（首页/仪表盘）吃的形状。
//
// 它**故意**保持和旧 `console_setting.announcements` 一致：那个面板读的是
// `/api/status`，字段是 content/publishDate/type/extra。换数据源不换形状，
// 前端的未登录可见性就不会因为这次迁移而改变——这正是「读路径分叉」的要点。
type AdtyPublicAnnouncement struct {
	Id          int    `json:"id"`
	Content     string `json:"content"`
	PublishDate string `json:"publishDate"`
	Type        string `json:"type"`
	Extra       string `json:"extra"`
}

// AdtyPublicAnnouncementsForRequest 按请求的站点读已发布公告，供 `/api/status` 用。
//
// 这条路是**匿名**的：不经过任何认证中间件，所以只能用 Host（站点中间件早就
// 放进上下文了）判断可见性，不依赖登录用户。分站架构下这就是对的判断依据。
func AdtyPublicAnnouncementsForRequest(c *gin.Context) []AdtyPublicAnnouncement {
	out := []AdtyPublicAnnouncement{}
	siteID := 0
	if site := AdtySiteForRequest(c); site != nil {
		siteID = site.Id
	}
	rows, err := model.ListAdtyNotificationsForSite(siteID, AdtyPublicAnnouncementLimit)
	if err != nil {
		// 公告读失败不能让整个 /status 挂掉：站点开关、支付方式都在这份响应里。
		common.SysError(common.LogText("list public announcements for site %d failed: %s", siteID, err.Error()))
		return out
	}
	for _, row := range rows {
		out = append(out, AdtyPublicAnnouncement{
			Id:          row.Id,
			Content:     row.Content,
			PublishDate: time.Unix(row.PublishedAt, 0).UTC().Format(time.RFC3339),
			Type:        adtyLegacyAnnouncementColor(row.Business),
			Extra:       row.Title,
		})
	}
	return out
}

// adtyLegacyAnnouncementColor 从 Business 里解出旧面板的**颜色**语义。
//
// 迁移时旧公告的 type（default/ongoing/success/warning/error）没有别的地方可放，
// 就存在 `Business="legacy:type=warning"` 里——那时是为了能回滚，现在多一个用处：
// 公开面板还按颜色渲染状态点，读回来才不会让所有旧公告都变成灰点。
//
// 新发布的公告 Business 存的是「关联业务」文本，不是这个前缀，走 default。
func adtyLegacyAnnouncementColor(business string) string {
	const prefix = "legacy:type="
	if len(business) <= len(prefix) || business[:len(prefix)] != prefix {
		return "default"
	}
	switch color := business[len(prefix):]; color {
	case "default", "ongoing", "success", "warning", "error":
		return color
	}
	return "default"
}
