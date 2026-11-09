package service

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
)

type AdtyNotificationInbox struct {
	Notice        string           `json:"notice"`
	NoticeKey     string           `json:"notice_key"`
	Announcements []map[string]any `json:"announcements"`
	ReadKeys      []string         `json:"read_keys"`
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

func GetAdtyNotificationInbox(userID int, site *model.AdtySite) (AdtyNotificationInbox, error) {
	inbox, keys, err := adtyCurrentNotificationInbox(site)
	if err != nil {
		return inbox, err
	}
	siteID := 0
	if site != nil {
		siteID = site.Id
	}
	inbox.ReadKeys, err = model.GetAdtyNotificationReadKeys(userID, siteID, keys)
	return inbox, err
}

func MarkAdtyNotificationInboxRead(userID int, site *model.AdtySite, keys []string) error {
	if len(keys) == 0 || len(keys) > 21 {
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
