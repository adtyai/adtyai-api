package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

var (
	ErrGroupGrantUserMissing = errors.New("grant user not found")
	ErrGroupGrantNotHidden   = errors.New("group is not a hidden group")
	ErrGroupGrantNoRatio     = errors.New("group ratio is missing")
	ErrGroupGrantExists      = errors.New("group grant already exists")
	ErrGroupGrantNotFound    = errors.New("group grant not found")
)

// UserGroupGrant authorizes one user to use a hidden group code.
// It does not change users.group.
type UserGroupGrant struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int    `json:"user_id" gorm:"not null;uniqueIndex:ux_user_group_grants_user_code"`
	GroupCode string `json:"group_code" gorm:"type:varchar(64);not null;uniqueIndex:ux_user_group_grants_user_code"`
	CreatedBy int    `json:"created_by" gorm:"not null;default:0"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (UserGroupGrant) TableName() string { return "user_group_grants" }

type UserGroupGrantView struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	GroupCode   string `json:"group_code"`
	CreatedAt   int64  `json:"created_at"`
}

func EnsureUserGroupGrantSchema(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if db == nil {
		return errors.New("user_group_grants: database is not initialized")
	}
	if err := db.AutoMigrate(&UserGroupGrant{}); err != nil {
		return fmt.Errorf("user_group_grants migrate: %w", err)
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return nil
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_user_group_grants_user_code ON user_group_grants (user_id, group_code)`).Error; err != nil {
		return fmt.Errorf("user_group_grants unique index: %w", err)
	}
	return nil
}

func ListUserGroupGrantCodes(userID int) ([]string, error) {
	codes := make([]string, 0)
	if userID <= 0 || DB == nil {
		return codes, nil
	}
	err := DB.Model(&UserGroupGrant{}).Where("user_id = ?", userID).Pluck("group_code", &codes).Error
	return codes, err
}

func ListGroupGrantViews(code string) ([]UserGroupGrantView, error) {
	rows := make([]UserGroupGrantView, 0)
	err := DB.Table("user_group_grants AS g").
		Select("g.user_id AS user_id, COALESCE(u.username, '') AS username, COALESCE(u.display_name, '') AS display_name, g.group_code AS group_code, g.created_at AS created_at").
		Joins("LEFT JOIN users AS u ON u.id = g.user_id AND u.deleted_at IS NULL").
		Where("g.group_code = ?", code).
		Order("g.user_id ASC").
		Scan(&rows).Error
	return rows, err
}

func GrantHiddenGroup(userID int, code string, createdBy int) error {
	code, err := validateHiddenGrantCode(code)
	if err != nil {
		return err
	}
	if userID <= 0 {
		return ErrGroupGrantUserMissing
	}
	var count int64
	if err := DB.Model(&User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrGroupGrantUserMissing
	}
	row := UserGroupGrant{UserId: userID, GroupCode: code, CreatedBy: createdBy}
	if err := DB.Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrGroupGrantExists
		}
		return err
	}
	return nil
}

func RevokeHiddenGroup(userID int, code string) error {
	if err := ValidateGroupMetaCode(code); err != nil {
		return err
	}
	if userID <= 0 {
		return ErrGroupGrantNotFound
	}
	res := DB.Where("user_id = ? AND group_code = ?", userID, code).Delete(&UserGroupGrant{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrGroupGrantNotFound
	}
	return nil
}

func validateHiddenGrantCode(code string) (string, error) {
	if err := ValidateGroupMetaCode(code); err != nil {
		return "", err
	}
	if code == "auto" {
		return "", ErrGroupGrantNotHidden
	}
	if !ratio_setting.ContainsGroupRatio(code) {
		return "", ErrGroupGrantNoRatio
	}
	if _, public := setting.GetUserUsableGroupsCopy()[code]; public {
		return "", ErrGroupGrantNotHidden
	}
	return code, nil
}
