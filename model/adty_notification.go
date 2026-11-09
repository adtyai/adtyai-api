package model

import (
	"github.com/QuantumNous/new-api/common"
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
}

var ErrAdtyNotificationConflict = common.NewMessage("This notification was changed elsewhere. Reload and try again")

// UpdateAdtyNotificationPublication uses a conditional write so a stale editor
// cannot overwrite a revision or restore a withdrawn publication.
func UpdateAdtyNotificationPublication(row *AdtyNotificationPublication, revision int64) error {
	result := DB.Model(&AdtyNotificationPublication{}).
		Where("id = ? AND issuer_site_id = ? AND revision = ? AND status = ?", row.Id, row.IssuerSiteId, revision, "published").
		Updates(map[string]any{
			"audience": row.Audience, "target_site_id": row.TargetSiteId, "target_site_name": row.TargetSiteName,
			"kind": row.Kind, "title": row.Title, "content": row.Content, "business": row.Business,
			"status": row.Status, "revision": revision + 1, "updated_at": row.UpdatedAt, "updated_by": row.UpdatedBy,
			"read_key": row.ReadKey,
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
