package model

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	userOAuthBindingSiteSubjectIndex   = "ux_provider_site_userid"
	legacyUserOAuthBindingSubjectIndex = "ux_provider_userid"
)

// UserOAuthBinding stores the binding relationship between users and custom OAuth providers
type UserOAuthBinding struct {
	Id             int       `json:"id" gorm:"primaryKey"`
	UserId         int       `json:"user_id" gorm:"not null;uniqueIndex:ux_user_provider"`                                                    // User ID - one binding per user per provider
	SiteId         int       `json:"site_id" gorm:"not null;default:0;uniqueIndex:ux_provider_site_userid,priority:2"`                        // Bound user's site; occupancy is provider + site + subject
	ProviderId     int       `json:"provider_id" gorm:"not null;uniqueIndex:ux_user_provider;uniqueIndex:ux_provider_site_userid,priority:1"` // Custom OAuth provider ID
	ProviderUserId string    `json:"provider_user_id" gorm:"type:varchar(256);not null;uniqueIndex:ux_provider_site_userid,priority:3"`       // User ID from OAuth provider
	CreatedAt      time.Time `json:"created_at"`
}

func (UserOAuthBinding) TableName() string {
	return "user_oauth_bindings"
}

// EnsureUserOAuthBindingSiteSubjectIndex copies each binding's site from its
// user, drops the platform-wide (provider, subject) unique index, and keeps a
// unique scope of provider + site + subject.
func EnsureUserOAuthBindingSiteSubjectIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("user oauth binding site subject index: database is nil")
	}
	if !db.Migrator().HasTable(&UserOAuthBinding{}) {
		return nil
	}
	if db.Migrator().HasTable(&User{}) {
		if err := db.Exec(`
			UPDATE user_oauth_bindings
			SET site_id = COALESCE((SELECT site_id FROM users WHERE users.id = user_oauth_bindings.user_id), site_id)
		`).Error; err != nil {
			return fmt.Errorf("backfill user oauth binding site_id: %w", err)
		}
	}
	if db.Migrator().HasIndex(&UserOAuthBinding{}, legacyUserOAuthBindingSubjectIndex) {
		if err := db.Migrator().DropIndex(&UserOAuthBinding{}, legacyUserOAuthBindingSubjectIndex); err != nil {
			return fmt.Errorf("drop legacy user oauth binding subject index: %w", err)
		}
	}
	if db.Migrator().HasIndex(&UserOAuthBinding{}, userOAuthBindingSiteSubjectIndex) {
		return nil
	}
	statement := &gorm.Statement{DB: db}
	return db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON %s (%s, %s, %s)",
		statement.Quote(userOAuthBindingSiteSubjectIndex),
		statement.Quote("user_oauth_bindings"),
		statement.Quote("provider_id"),
		statement.Quote("site_id"),
		statement.Quote("provider_user_id"),
	)).Error
}

// GetUserOAuthBindingsByUserId returns all OAuth bindings for a user
func GetUserOAuthBindingsByUserId(userId int) ([]*UserOAuthBinding, error) {
	var bindings []*UserOAuthBinding
	err := DB.Where("user_id = ?", userId).Find(&bindings).Error
	return bindings, err
}

// GetUserOAuthBinding returns a specific binding for a user and provider
func GetUserOAuthBinding(userId, providerId int) (*UserOAuthBinding, error) {
	var binding UserOAuthBinding
	err := DB.Where("user_id = ? AND provider_id = ?", userId, providerId).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// GetUserByOAuthBinding finds a user by provider ID, provider user ID, and site.
func GetUserByOAuthBinding(providerId int, providerUserId string, siteID int) (*User, error) {
	var binding UserOAuthBinding
	err := DB.Where("provider_id = ? AND provider_user_id = ? AND site_id = ?", providerId, providerUserId, siteID).First(&binding).Error
	if err != nil {
		return nil, err
	}

	var user User
	err = DB.First(&user, binding.UserId).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// IsProviderUserIdTaken checks if a provider user ID is already bound on this site.
func IsProviderUserIdTaken(providerId int, providerUserId string, siteID int) bool {
	var count int64
	DB.Model(&UserOAuthBinding{}).Where("provider_id = ? AND provider_user_id = ? AND site_id = ?", providerId, providerUserId, siteID).Count(&count)
	return count > 0
}

func assignBindingSite(tx *gorm.DB, binding *UserOAuthBinding) error {
	if binding == nil {
		return errors.New("user ID is required")
	}
	if binding.UserId == 0 {
		return errors.New("user ID is required")
	}
	siteID, err := siteIDForUser(tx, binding.UserId)
	if err != nil {
		return err
	}
	binding.SiteId = siteID
	return nil
}

// CreateUserOAuthBinding creates a new OAuth binding
func CreateUserOAuthBinding(binding *UserOAuthBinding) error {
	if binding.UserId == 0 {
		return errors.New("user ID is required")
	}
	if binding.ProviderId == 0 {
		return errors.New("provider ID is required")
	}
	if binding.ProviderUserId == "" {
		return errors.New("provider user ID is required")
	}

	if err := assignBindingSite(DB, binding); err != nil {
		return err
	}
	if IsProviderUserIdTaken(binding.ProviderId, binding.ProviderUserId, binding.SiteId) {
		return errors.New("this OAuth account is already bound to another user")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		return CreateUserOAuthBindingWithTx(tx, binding)
	})
}

// CreateUserOAuthBindingWithTx creates a new OAuth binding within a transaction
func CreateUserOAuthBindingWithTx(tx *gorm.DB, binding *UserOAuthBinding) error {
	if binding.UserId == 0 {
		return errors.New("user ID is required")
	}
	if binding.ProviderId == 0 {
		return errors.New("provider ID is required")
	}
	if binding.ProviderUserId == "" {
		return errors.New("provider user ID is required")
	}

	if err := assignBindingSite(tx, binding); err != nil {
		return err
	}
	if err := lockCustomOAuthProviderTx(tx, binding.ProviderId); err != nil {
		return err
	}
	var count int64
	tx.Model(&UserOAuthBinding{}).Where("provider_id = ? AND provider_user_id = ? AND site_id = ?", binding.ProviderId, binding.ProviderUserId, binding.SiteId).Count(&count)
	if count > 0 {
		return errors.New("this OAuth account is already bound to another user")
	}

	binding.CreatedAt = time.Now()
	err := tx.Create(binding).Error
	if isAdtyUniqueViolation(err) {
		return errors.New("this OAuth account is already bound to another user")
	}
	return err
}

// UpdateUserOAuthBinding updates an existing OAuth binding (e.g., rebind to different OAuth account)
func UpdateUserOAuthBinding(userId, providerId int, newProviderUserId string) error {
	siteID, err := siteIDForUser(DB, userId)
	if err != nil {
		return err
	}
	// Check if the new provider user ID is already taken by another user on this site
	var existingBinding UserOAuthBinding
	err = DB.Where("provider_id = ? AND provider_user_id = ? AND site_id = ?", providerId, newProviderUserId, siteID).First(&existingBinding).Error
	if err == nil && existingBinding.UserId != userId {
		return errors.New("this OAuth account is already bound to another user")
	}

	// Check if user already has a binding for this provider
	var binding UserOAuthBinding
	err = DB.Where("user_id = ? AND provider_id = ?", userId, providerId).First(&binding).Error
	if err != nil {
		// No existing binding, create new one
		return CreateUserOAuthBinding(&UserOAuthBinding{
			UserId:         userId,
			ProviderId:     providerId,
			ProviderUserId: newProviderUserId,
		})
	}

	// Update existing binding
	return DB.Model(&binding).Updates(map[string]any{
		"provider_user_id": newProviderUserId,
		"site_id":          siteID,
	}).Error
}

// DeleteUserOAuthBinding deletes an OAuth binding
func DeleteUserOAuthBinding(userId, providerId int) error {
	return DB.Where("user_id = ? AND provider_id = ?", userId, providerId).Delete(&UserOAuthBinding{}).Error
}

func deleteUserOAuthBindingsByUserId(tx *gorm.DB, userId int) error {
	return tx.Where("user_id = ?", userId).Delete(&UserOAuthBinding{}).Error
}

// GetBindingCountByProviderId returns the number of bindings for a provider
func GetBindingCountByProviderId(providerId int) (int64, error) {
	var count int64
	err := DB.Model(&UserOAuthBinding{}).Where("provider_id = ?", providerId).Count(&count).Error
	return count, err
}
