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
	if id > 0 {
		return &row, model.UpdateAdtyNotificationPublication(&row, input.Revision)
	}
	return &row, model.DB.Create(&row).Error
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
	return inbox, err
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
