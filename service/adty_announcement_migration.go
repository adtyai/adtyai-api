package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
)

// legacyAnnouncementMarker 是旧公告的幂等键前缀。ReadKey 是 char(64) 的唯一索引，
// 用它兜住重复导入：同一条旧公告无论导入几次都只有一行。
const legacyAnnouncementMarker = "legacy-announcement:"

// adtyLegacyAnnouncementKind 是所有旧公告落到新表时用的类别。
//
// 旧结构只有「这是公告」一种语义（type 是颜色，不是类别），所以这里不硬塞映射；
// 颜色语义另存在 Business 里，回滚时能还原。
const adtyLegacyAnnouncementKind = "platform"

// MigrateLegacyAnnouncements 把 `console_setting.announcements` 的每一条搬进
// `adty_notification_publications`，返回本次新插入的行数。
//
// 可重跑：幂等由 ReadKey 唯一索引兜底，第二次调用插入 0 行。
// 旧 option **不删除**——它是回滚源，删了就没法退回去。
func MigrateLegacyAnnouncements(raw string) (created int, err error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		// 旧 option 是用户手写的 JSON，坏数据不该挡住启动：记一条日志，跳过迁移。
		return 0, err
	}
	now := time.Now().Unix()
	for index, entry := range entries {
		content, _ := entry["content"].(string)
		if strings.TrimSpace(content) == "" {
			continue
		}
		publishDate, _ := entry["publishDate"].(string)
		publishedAt := now
		if parsed, parseErr := time.Parse(time.RFC3339, publishDate); parseErr == nil {
			publishedAt = parsed.Unix()
		}
		kind := adtyLegacyAnnouncementKind
		business := ""
		if legacyType, ok := entry["type"].(string); ok && legacyType != "" && legacyType != "default" {
			// 颜色是旧面板的展示语义，新表没有这一列。存在 Business 里是为了
			// 回滚时能还原，而不是给它编一个新的类别。
			business = "legacy:type=" + legacyType
		}
		title := ""
		if extra, ok := entry["extra"].(string); ok {
			// 旧结构没有标题，`extra` 是紧跟在正文旁的短备注，最接近标题的位置。
			title = truncateAdtyRunes(strings.TrimSpace(extra), 120)
		}
		row := model.AdtyNotificationPublication{
			IssuerSiteId:  0, // 主站发布域；旧公告都是平台级的
			PublisherId:   0,
			PublisherName: "legacy",
			Audience:      "all_users",
			Kind:          kind,
			Title:         title,
			Content:       content, // 逐字节保留，不 trim 不改写
			Business:      business,
			Status:        "published",
			Revision:      1,
			ReadKey:       legacyAnnouncementReadKey(index, publishDate, content),
			PublishedAt:   publishedAt,
			UpdatedAt:     publishedAt,
			UpdatedBy:     0,
		}
		inserted, insertErr := model.ImportLegacyAnnouncement(&row)
		if insertErr != nil {
			return created, insertErr
		}
		if inserted {
			created++
		}
	}
	return created, nil
}

// legacyAnnouncementReadKey 由「位置 + 发布时间 + 正文」决定。改正文就是另一条记录了，
// 这是想要的：迁移不是同步，它只负责把当时那份搬过去一次。
func legacyAnnouncementReadKey(index int, publishDate, content string) string {
	sum := sha256.Sum256([]byte(legacyAnnouncementMarker + strconv.Itoa(index) + "\n" + publishDate + "\n" + content))
	return hex.EncodeToString(sum[:])
}

// migrateAdtyLegacyAnnouncementsOnStart 在启动时跑一次。失败只记日志：
// 公告迁移失败不该让服务起不来，但必须留下可查的痕迹。
func migrateAdtyLegacyAnnouncementsOnStart() {
	created, err := MigrateLegacyAnnouncements(console_setting.GetConsoleSetting().Announcements)
	if err != nil {
		common.SysError(common.LogText("migrate legacy announcements failed: %s", err.Error()))
		return
	}
	if created > 0 {
		common.SysLog(common.LogText("migrated %d legacy announcements into notification publications", created))
	}
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, migrateAdtyLegacyAnnouncementsOnStart)
}

func truncateAdtyRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
