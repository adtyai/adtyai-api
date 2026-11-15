package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"gorm.io/gorm"
)

type AdtyNotificationInbox struct {
	Notice             string           `json:"notice"`
	NoticeKey          string           `json:"notice_key"`
	Announcements      []map[string]any `json:"announcements"`
	ReadKeys           []string         `json:"read_keys"`
	NextBeforeId       int              `json:"next_before_id"`
	UnreadPublications int64            `json:"unread_publications"`
}

type AdtyNotificationInput struct {
	Title        string `json:"title"`
	Content      string `json:"content"`
	Kind         string `json:"kind"`
	Audience     string `json:"audience"`
	TargetSiteId int    `json:"target_site_id"`
	Business     string `json:"business"`
	Revision     int64  `json:"revision"`
	// SyncTelegram 勾了才把这条公告排进 TG 队列；TelegramChannel 是目标频道
	// （`@name` 或 `-100…`）。都不填就是只发站内。
	SyncTelegram    bool   `json:"sync_telegram"`
	TelegramChannel string `json:"telegram_channel"`
}

func ListAdtyManagedNotifications(issuerSiteID, start, limit int) ([]model.AdtyNotificationPublication, int64, error) {
	query := model.DB.Model(&model.AdtyNotificationPublication{}).Where("issuer_site_id = ?", issuerSiteID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.AdtyNotificationPublication{}
	err := query.Order("id DESC").Offset(start).Limit(limit).Find(&items).Error
	return items, total, err
}

func SaveAdtyNotificationPublication(actorID int, issuer *model.AdtySite, id int, input AdtyNotificationInput, withdraw bool) (*model.AdtyNotificationPublication, error) {
	issuerID := 0
	if issuer != nil {
		issuerID = issuer.Id
		if issuer.Status == model.AdtySiteStatusClosing {
			return nil, ErrAdtySiteClosing
		}
	}
	row := model.AdtyNotificationPublication{IssuerSiteId: issuerID}
	if id > 0 {
		if err := model.DB.Where("id = ? AND issuer_site_id = ?", id, issuerID).First(&row).Error; err != nil {
			return nil, err
		}
		if input.Revision <= 0 {
			return nil, common.NewMessage("Invalid parameters")
		}
	} else {
		if withdraw || input.Revision != 0 {
			return nil, common.NewMessage("Invalid parameters")
		}
		var publisher model.User
		if err := model.DB.First(&publisher, actorID).Error; err != nil {
			return nil, err
		}
		row.PublisherId, row.PublisherName = actorID, publisher.Username
		row.PublishedAt, row.Revision = time.Now().Unix(), 1
	}
	row.UpdatedAt, row.UpdatedBy = time.Now().Unix(), actorID
	if withdraw {
		row.Status = "withdrawn"
		return &row, model.UpdateAdtyNotificationPublication(&row, input.Revision)
	}
	input.Title, input.Content, input.Business = strings.TrimSpace(input.Title), strings.TrimSpace(input.Content), strings.TrimSpace(input.Business)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 120 || input.Content == "" || utf8.RuneCountInString(input.Content) > 10000 || utf8.RuneCountInString(input.Business) > 200 ||
		!slices.Contains([]string{"platform", "system", "maintenance", "rules"}, input.Kind) || !slices.Contains([]string{"all_users", "all_sites", "site"}, input.Audience) {
		return nil, common.NewMessage("Invalid parameters")
	}
	if issuer != nil && (input.Audience != "site" || input.TargetSiteId != issuerID) {
		return nil, common.NewMessage("Invalid parameters")
	}
	row.TargetSiteName = ""
	if input.Audience == "site" {
		var target model.AdtySite
		if input.TargetSiteId <= 0 {
			return nil, common.NewMessage("Invalid parameters")
		}
		if err := model.DB.Where("id = ? AND status IN ?", input.TargetSiteId, []string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused}).First(&target).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, common.NewMessage("Invalid parameters")
			}
			return nil, err
		}
		row.TargetSiteName = AdtySiteDisplayHost(&target)
	} else if input.TargetSiteId != 0 {
		return nil, common.NewMessage("Invalid parameters")
	}
	row.Title, row.Content, row.Business = input.Title, input.Content, input.Business
	row.Kind, row.Audience, row.TargetSiteId, row.Status = input.Kind, input.Audience, input.TargetSiteId, "published"
	row.ReadKey = adtyNotificationVersion("publication:" + common.GetUUID())
	// 不勾同步就把频道一起清掉：留着上一轮的频道名，下次勾上会安静地发到
	// 一个没人预期的目标。
	if input.SyncTelegram {
		row.TelegramChannel = strings.TrimSpace(input.TelegramChannel)
	} else {
		row.TelegramChannel = ""
	}
	if id > 0 {
		if err := model.UpdateAdtyNotificationPublication(&row, input.Revision); err != nil {
			return nil, err
		}
	} else if err := model.DB.Create(&row).Error; err != nil {
		return nil, err
	}
	// 入队失败不回滚公告：公告落库是主事实，TG 只是通知。
	pushAdtyNotificationToTelegram(&row, issuer)
	return &row, nil
}

// pushAdtyNotificationToTelegram 尝试把公告推一次到频道，并保证「一条公告最多推一次」。
//
// 「新建才推、编辑不推」这种按操作类型分流的写法不够：公告先发布、后勾上同步，
// 就永远没机会推了。这里按「推过没有」分流（`telegram_at`），既让后勾的能推出去，
// 也不让改了错别字的公告再推一遍。抢名额靠 SQL 条件写，不靠先读后写。
func pushAdtyNotificationToTelegram(row *model.AdtyNotificationPublication, issuer *model.AdtySite) {
	channel := strings.TrimSpace(row.TelegramChannel)
	if channel == "" {
		return // 没配频道就不发，也不报错
	}
	purpose := model.AdtyTelegramPurposeAdmin
	if issuer != nil {
		purpose = model.AdtyTelegramPurposeUser
	}
	at := time.Now().Unix()
	won, err := model.ClaimAdtyNotificationTelegramSlot(row.Id, at)
	if err != nil {
		common.SysError(common.LogText("claim telegram slot for announcement %d failed: %s", row.Id, err.Error()))
		return
	}
	if !won {
		return // 推过了，这次编辑只更新站内内容
	}
	row.TelegramAt = at
	if err := QueueAdtyPublicTelegram(channel, purpose, announceTelegramBody(*row)); err != nil {
		// 名额要还回去：`telegram_at` 非 0 的意思是「发过了」，而这次一条都没发。
		row.TelegramAt = 0
		if releaseErr := model.ReleaseAdtyNotificationTelegramSlot(row.Id); releaseErr != nil {
			common.SysError(common.LogText("release telegram slot for announcement %d failed: %s", row.Id, releaseErr.Error()))
		}
		common.SysError(common.LogText("queue announcement %d for telegram failed: %s", row.Id, err.Error()))
	}
}

// announceTelegramBody 把公告压成一条频道消息：标题 + 正文。
// 正文已经在上面的校验里 TrimSpace 过，这里只做拼接，不重新解释内容。
func announceTelegramBody(row model.AdtyNotificationPublication) string {
	if row.Title == "" {
		return row.Content
	}
	return row.Title + "\n\n" + row.Content
}

// A matching Host alone does not entitle an unrelated account to site content.
// Account membership is read from SQL rather than client parameters or caches.
func adtyVisibleNotificationPublications(userID int, site *model.AdtySite) (*gorm.DB, error) {
	var user model.User
	if err := model.DB.First(&user, userID).Error; err != nil {
		return nil, err
	}
	query := model.DB.Model(&model.AdtyNotificationPublication{}).Where("status = ?", "published")
	if site == nil {
		return query.Where("audience = ?", "all_users"), nil
	}
	var current model.AdtySite
	if err := model.DB.First(&current, site.Id).Error; err != nil {
		return nil, err
	}
	if user.SiteId != site.Id && current.OwnerUserId != userID {
		return query.Where("audience = ?", "all_users"), nil
	}
	return query.Where("audience = ? OR audience = ? OR (audience = ? AND target_site_id = ?)", "all_users", "all_sites", "site", site.Id), nil
}

func adtyNotificationVersion(source string) string {
	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:])
}

// adtyCurrentNotificationInbox uses the same Host boundary as public Notice and
// status. Version keys bind reads to the actual content shown by this response.
func adtyCurrentNotificationInbox(site *model.AdtySite) (AdtyNotificationInbox, []string, error) {
	inbox := AdtyNotificationInbox{Announcements: []map[string]any{}, ReadKeys: []string{}}
	keys := []string{}
	if site != nil {
		return inbox, keys, nil
	}
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	inbox.Notice = strings.TrimSpace(common.OptionMap["Notice"])
	if inbox.Notice != "" {
		inbox.NoticeKey = adtyNotificationVersion("notice:" + inbox.Notice)
		keys = append(keys, inbox.NoticeKey)
	}
	if console_setting.GetConsoleSetting().AnnouncementsEnabled {
		announcements := console_setting.GetAnnouncements()
		for _, item := range announcements[:min(len(announcements), 20)] {
			raw, err := common.Marshal(item)
			if err != nil {
				return inbox, nil, err
			}
			key := adtyNotificationVersion("announcement:" + string(raw))
			item["read_key"] = key
			inbox.Announcements = append(inbox.Announcements, item)
			keys = append(keys, key)
		}
	}
	return inbox, keys, nil
}

func GetAdtyNotificationInbox(userID int, site *model.AdtySite, before int) (AdtyNotificationInbox, error) {
	inbox, keys, err := adtyCurrentNotificationInbox(site)
	if err != nil {
		return inbox, err
	}
	query, err := adtyVisibleNotificationPublications(userID, site)
	if err != nil {
		return inbox, err
	}
	siteID := 0
	if site != nil {
		siteID = site.Id
	}
	receipts := model.DB.Model(&model.AdtyNotificationRead{}).Select("read_key").Where("user_id = ? AND site_id = ?", userID, siteID)
	var unread int64
	if err := query.Session(&gorm.Session{}).Where("read_key NOT IN (?)", receipts).Count(&unread).Error; err != nil {
		return inbox, err
	}
	inbox.UnreadPublications = unread
	if before > 0 {
		query = query.Where("id < ?", before)
		inbox = AdtyNotificationInbox{Announcements: []map[string]any{}, ReadKeys: []string{}, UnreadPublications: unread}
		keys = []string{}
	}
	var rows []model.AdtyNotificationPublication
	if err := query.Order("id DESC").Limit(21).Find(&rows).Error; err != nil {
		return inbox, err
	}
	if len(rows) > 20 {
		rows = rows[:20]
		inbox.NextBeforeId = rows[19].Id
	}
	for _, row := range rows {
		key := row.ReadKey
		inbox.Announcements = append(inbox.Announcements, map[string]any{
			"id": fmt.Sprintf("publication:%d", row.Id), "read_key": key, "title": row.Title, "content": row.Content,
			"type": "default", "publishDate": time.Unix(row.PublishedAt, 0).UTC().Format(time.RFC3339),
			"publisher_name": row.PublisherName, "publisher_id": row.PublisherId, "issuer_site_id": row.IssuerSiteId,
			"audience": row.Audience, "target_site_id": row.TargetSiteId, "target_site_name": row.TargetSiteName,
			"kind": row.Kind, "business": row.Business,
		})
		keys = append(keys, key)
	}
	inbox.ReadKeys, err = model.GetAdtyNotificationReadKeys(userID, siteID, keys)
	if err != nil {
		return inbox, err
	}
	return hideAdtyNotifications(userID, inbox)
}

func hideAdtyNotifications(userID int, inbox AdtyNotificationInbox) (AdtyNotificationInbox, error) {
	if userID <= 0 || model.DB == nil || len(inbox.Announcements) == 0 {
		return inbox, nil
	}
	keys := make([]string, 0, len(inbox.Announcements))
	for _, item := range inbox.Announcements {
		if key, ok := item["read_key"].(string); ok && key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return inbox, nil
	}
	var hidden []string
	if err := model.DB.Model(&model.AdtyNotificationHide{}).Where("user_id = ? AND key IN ?", userID, keys).Pluck("key", &hidden).Error; err != nil {
		return inbox, err
	}
	if len(hidden) == 0 {
		return inbox, nil
	}
	skip := map[string]bool{}
	for _, key := range hidden {
		skip[key] = true
	}
	visible := inbox.Announcements[:0]
	for _, item := range inbox.Announcements {
		key, _ := item["read_key"].(string)
		if !skip[key] {
			visible = append(visible, item)
		}
	}
	inbox.Announcements = visible
	return inbox, nil
}

func MarkAdtyNotificationInboxRead(userID int, site *model.AdtySite, keys []string) error {
	if len(keys) == 0 || len(keys) > 41 {
		return common.NewMessage("Invalid parameters")
	}
	_, visible, err := adtyCurrentNotificationInbox(site)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(visible))
	for _, key := range visible {
		allowed[key] = true
	}
	query, err := adtyVisibleNotificationPublications(userID, site)
	if err != nil {
		return err
	}
	// Version keys select candidates; the visibility predicate must also hold,
	// including for publications on later pages.
	var publicationKeys []string
	if err := query.Where("read_key IN ?", keys).Pluck("read_key", &publicationKeys).Error; err != nil {
		return err
	}
	for _, key := range publicationKeys {
		allowed[key] = true
	}
	unique := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !allowed[key] {
			return common.NewMessage("Invalid parameters")
		}
		unique[key] = true
	}
	keys = make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	siteID := 0
	if site != nil {
		siteID = site.Id
	}
	return model.MarkAdtyNotificationsRead(userID, siteID, keys, time.Now().Unix())
}
