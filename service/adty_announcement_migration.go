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
	// 同一段「发布时间 + 正文」出现过几次。用它当序号，而不是数组下标：
	// 序号只随「有没有重复」变化，不随位置变化。
	seen := map[string]int{}
	for _, entry := range entries {
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
		// `extra` 不是标题，是**正文旁边的短备注**（「2025 年起生效」这类），
		// 和正文是一体的。所以按原文顺序接在正文后面，Title 留空。
		//
		// 两个理由，一个是顺序、一个是可逆性：
		// 放进 Title 会让前端把一条公告拆成「标题」和「正文」两段，原来的阅读
		// 顺序就没了；而 Title 是 varchar(120)，`extra` 有 100 字上限又被截断，
		// 截掉的正是正文的一部分。
		// 标题为空在界面上最多多一个空行（改一行数据就能补），正文被截断则原文
		// 就没了——两种错误的可逆性完全不同。前端不渲染空标题是前端该处理的事。
		if extra, ok := entry["extra"].(string); ok {
			if trimmed := strings.TrimSpace(extra); trimmed != "" {
				content = content + "\n\n" + extra
			}
		}
		dedupeKey := publishDate + "\n" + content
		ordinal := seen[dedupeKey]
		seen[dedupeKey] = ordinal + 1
		row := model.AdtyNotificationPublication{
			IssuerSiteId:  0, // 主站发布域；旧公告都是平台级的
			PublisherId:   0,
			PublisherName: "legacy",
			Audience:      "all_users",
			Kind:          kind,
			Title:         "",
			Content:       content, // 正文逐字节保留；备注按原文顺序接在后面
			Business:      business,
			Status:        "published",
			Revision:      1,
			ReadKey:       legacyAnnouncementReadKey(publishDate, content, ordinal),
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

// legacyAnnouncementReadKey 由「发布时间 + 正文 + 同内容出现序号」决定。
//
// **不含数组下标**，这是关键：旧面板按数组编辑，删一条会让后面所有条目换位置。
// 下标进哈希的话，重排一次就会给后面每一条算出新键、全部重插一遍——公告在面板上
// 就重复了。而「重排后仍同一行」才是迁移该有的行为。
//
// `ordinal` 只用来区分**真正重复**的条目（旧面板里复制一条、连日期都没改）。
// 它是「同样内容里的第几条」，不随位置变化：删掉前面一条，后面那条的序号不变。
func legacyAnnouncementReadKey(publishDate, content string, ordinal int) string {
	sum := sha256.Sum256([]byte(legacyAnnouncementMarker + publishDate + "\n" + content + "\n" + strconv.Itoa(ordinal)))
	return hex.EncodeToString(sum[:])
}

// migrateAdtyLegacyAnnouncementsOnStart 在启动时跑一次，失败只记日志。
//
// 不需要额外的「这个 option 迁过没有」标记：ReadKey 由内容决定（不含数组下标），
// 重复执行会撞唯一索引，第二次插入 0 行。**重排数组也不会重插**——这正是把下标
// 从哈希里拿掉的原因。
func migrateAdtyLegacyAnnouncementsOnStart() {
	created, err := MigrateLegacyAnnouncements(console_setting.GetConsoleSetting().Announcements)
	if err != nil {
		// 坏数据不该让服务起不来，但必须留痕。
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
