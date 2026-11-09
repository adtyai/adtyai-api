package model

// AdtySiteStaff is a dedicated staff account. Deleted memberships remain as
// authorization tombstones; they must never turn into ordinary customer accounts.
type AdtySiteStaff struct {
	Id          int    `json:"id"`
	SiteId      int    `json:"site_id" gorm:"index"`
	OwnerUserId int    `json:"-"`
	UserId      int    `json:"user_id" gorm:"uniqueIndex"`
	Username    string `json:"username" gorm:"type:varchar(20)"`
	DisplayName string `json:"display_name" gorm:"type:varchar(20)"`
	Role        string `json:"role" gorm:"type:varchar(16)"`
	Permissions string `json:"-" gorm:"type:text"`
	Status      string `json:"status" gorm:"type:varchar(16);index"`
	Version     int64  `json:"version"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySiteStaff) TableName() string { return "adty_site_staff" }
