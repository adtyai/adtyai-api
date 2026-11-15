package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdtyNotificationPublication keeps withdrawn publications and their original
// attribution. IssuerSiteId=0 is the Root publishing domain, never a recipient.
type AdtyNotificationPublication struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	IssuerSiteId   int    `json:"issuer_site_id" gorm:"not null;index"`
	PublisherId    int    `json:"publisher_id" gorm:"not null"`
	PublisherName  string `json:"publisher_name" gorm:"type:varchar(80);not null"`
	Audience       string `json:"audience" gorm:"type:varchar(16);not null"`
	TargetSiteId   int    `json:"target_site_id" gorm:"not null;index"`
	TargetSiteName string `json:"target_site_name" gorm:"type:varchar(255);not null"`
	Kind           string `json:"kind" gorm:"type:varchar(16);not null"`
	Title          string `json:"title" gorm:"type:varchar(120);not null"`
	Content        string `json:"content" gorm:"type:text;not null"`
	Business       string `json:"business" gorm:"type:varchar(200);not null"`
	Status         string `json:"status" gorm:"type:varchar(16);not null;index"`
	Revision       int64  `json:"revision" gorm:"not null"`
	ReadKey        string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	PublishedAt    int64  `json:"published_at" gorm:"not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"not null"`
	UpdatedBy      int    `json:"updated_by" gorm:"not null"`
	// TelegramChannel 是这条公告要同步到的 TG 频道（`@name` 或 `-100…`）。
	// 空表示不同步。存在公告上而不是每次请求里带，编辑时才知道原来发到哪。
	TelegramChannel string `json:"telegram_channel" gorm:"type:varchar(128);not null;default:''"`
	// TelegramAt 是这条公告推送 TG 的时刻；0 表示没推过。
	//
	// 它只回答「推过没有」，不记「最近一次入队时间」：公告要推的是
	// 「新通知」，不是「持续编辑」。同一条公告改三次就是三条推送，
	// 用户会把它当骚扰——改错别字的收益远小于这个代价。
	// 所以非 0 之后就不再推，哪怕内容改了。
	TelegramAt int64 `json:"telegram_at" gorm:"not null;default:0"`
}

var ErrAdtyNotificationConflict = common.NewMessage("This notification was changed elsewhere. Reload and try again")

// UpdateAdtyNotificationPublication uses a conditional write so a stale editor
// cannot overwrite a revision or restore a withdrawn publication.
func UpdateAdtyNotificationPublication(row *AdtyNotificationPublication, revision int64) error {
	result := DB.Model(&AdtyNotificationPublication{}).
		// 草稿也能编辑：`draft` 存在的意义就是「还没发出去、随便改」。
		// 已撤回的不能编辑——那不是编辑对象，是历史记录。
		Where("id = ? AND issuer_site_id = ? AND revision = ? AND status IN ?", row.Id, row.IssuerSiteId, revision, []string{"published", "draft"}).
		Updates(map[string]any{
			"audience": row.Audience, "target_site_id": row.TargetSiteId, "target_site_name": row.TargetSiteName,
			"kind": row.Kind, "title": row.Title, "content": row.Content, "business": row.Business,
			"status": row.Status, "revision": revision + 1, "updated_at": row.UpdatedAt, "updated_by": row.UpdatedBy,
			"read_key": row.ReadKey, "telegram_channel": row.TelegramChannel,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAdtyNotificationConflict
	}
	row.Revision = revision + 1
	return nil
}

// AdtyNotificationRead records the first read of a content version by one account
// on one site. Public browser history never becomes an account receipt.
type AdtyNotificationRead struct {
	UserId  int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	SiteId  int    `json:"site_id" gorm:"primaryKey;autoIncrement:false"`
	ReadKey string `json:"read_key" gorm:"type:char(64);primaryKey"`
	ReadAt  int64  `json:"read_at" gorm:"not null"`
}

func GetAdtyNotificationReadKeys(userID, siteID int, keys []string) ([]string, error) {
	read := []string{}
	if len(keys) == 0 {
		return read, nil
	}
	err := DB.Model(&AdtyNotificationRead{}).Where("user_id = ? AND site_id = ? AND read_key IN ?", userID, siteID, keys).
		Order("read_key").Pluck("read_key", &read).Error
	return read, err
}

func MarkAdtyNotificationsRead(userID, siteID int, keys []string, now int64) error {
	if len(keys) == 0 {
		return nil
	}
	rows := make([]AdtyNotificationRead, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, AdtyNotificationRead{UserId: userID, SiteId: siteID, ReadKey: key, ReadAt: now})
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// ClaimAdtyNotificationTelegramSlot 原子地抢下这条公告的 TG 推送权。
//
// 条件写在 SQL 里（`telegram_at = 0`）而不是「先读再判断再写」：并发保存同一条
// 公告时只有一个能拿到 RowsAffected=1，另一个直接跳过。应用层 check-then-write
// 在这里会漏——两个请求都能读到 0。
func ClaimAdtyNotificationTelegramSlot(id int, at int64) (bool, error) {
	result := DB.Model(&AdtyNotificationPublication{}).
		Where("id = ? AND telegram_at = 0", id).
		Update("telegram_at", at)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ReleaseAdtyNotificationTelegramSlot 在入队失败后把推送权还回去，
// 否则这条公告会被永久标成「推过」，而实际一条都没发出去。
func ReleaseAdtyNotificationTelegramSlot(id int) error {
	return DB.Model(&AdtyNotificationPublication{}).
		Where("id = ?", id).
		Update("telegram_at", 0).Error
}

// ImportLegacyAnnouncement 把一条旧 `console_setting.announcements` 落成一条
// IssuerSiteId=0、Audience=all_users 的发布记录，返回是否真的插入了一行。
//
// 幂等靠 ReadKey 的唯一索引，不靠「先查一遍再插」：重复导入会撞索引，
// OnConflict DoNothing 让它安静跳过，row 数不变。这条属性是可重跑的前提——
// 启动时每次都会调，不能每次多一批。
func ImportLegacyAnnouncement(row *AdtyNotificationPublication) (bool, error) {
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ListAdtyNotificationsForSite 返回某个站点上「已发布」的公告，供公开面板读。
//
// siteID=0 是主站。可见性与 `adtyVisibleNotificationPublications` 同一套规则，
// 但这里没有登录用户，所以只按站点判断——这正是分站架构下对的依据。
// 主站只看到 all_users（以及 all_sites，那本来就包含主站）；分站看到
// all_users + all_sites + 自己那条。
func ListAdtyNotificationsForSite(siteID int, limit int) ([]AdtyNotificationPublication, error) {
	items := []AdtyNotificationPublication{}
	// 调用方是 `/api/status`——那条路由在服务启动的最前面就会被访问。
	// 库还没初始化时这里是 nil，直接解引用会把整份 status 打成 panic，
	// 而公告只是 status 里的一个可选字段，不能拖垮站点开关和支付方式。
	if DB == nil {
		return items, nil
	}
	query := DB.Model(&AdtyNotificationPublication{}).Where("status = ?", "published")
	// 与 `adtyVisibleNotificationPublications` 同一张表：分站看不到 `all_users`。
	// 两处规则必须一起改，否则公开面板和站内收件箱会给出不同的可见范围。
	if siteID == 0 {
		query = query.Where("audience IN ?", []string{"all_users", "all_sites"})
	} else {
		query = query.Where("audience = ? OR (audience = ? AND target_site_id = ?)",
			"all_sites", "site", siteID)
	}
	err := query.Order("published_at DESC, id DESC").Limit(limit).Find(&items).Error
	return items, err
}

var _ = gorm.ErrRecordNotFound
