package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ExternalIdentityProviderTelegram = "telegram"

var ErrExternalIdentityAlreadyClaimed = errors.New("external identity is already claimed")

// ExternalIdentityClaim is the durable ownership record for an identity issued
// by an external provider. The two unique indexes make both the provider
// subject and the user's provider slot single-owner without relying on a
// check-then-update sequence.
const (
	externalIdentitySubjectIndex       = "idx_external_identity_site_subject"
	legacyExternalIdentitySubjectIndex = "idx_external_identity_subject"
)

type ExternalIdentityClaim struct {
	Id        int64     `json:"id" gorm:"primaryKey"`
	Provider  string    `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_external_identity_site_subject,priority:1;uniqueIndex:idx_external_identity_user,priority:1"`
	SiteId    int       `json:"site_id" gorm:"not null;default:0;uniqueIndex:idx_external_identity_site_subject,priority:2"`
	Subject   string    `json:"subject" gorm:"type:varchar(128);not null;uniqueIndex:idx_external_identity_site_subject,priority:3"`
	UserId    int       `json:"user_id" gorm:"not null;index;uniqueIndex:idx_external_identity_user,priority:2"`
	CreatedAt time.Time `json:"created_at"`
}

func (ExternalIdentityClaim) TableName() string {
	return "external_identity_claims"
}

// ClaimExternalIdentityWithTx atomically claims a provider subject for one
// user. Repeating the exact mapping is idempotent; every competing subject or
// user is rejected. Ownership is read back instead of trusting RowsAffected,
// whose duplicate-key semantics differ between supported databases.
func ClaimExternalIdentityWithTx(tx *gorm.DB, provider, subject string, userId int) error {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if tx == nil || provider == "" || subject == "" || userId == 0 {
		return errors.New("external identity claim is invalid")
	}

	var owner User
	if err := tx.Select("id", "site_id").First(&owner, userId).Error; err != nil {
		return err
	}
	claim := ExternalIdentityClaim{Provider: provider, Subject: subject, UserId: userId, SiteId: owner.SiteId}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
	if result.Error != nil {
		return result.Error
	}
	var subjectOwner ExternalIdentityClaim
	if err := tx.Where("provider = ? AND subject = ? AND site_id = ?", provider, subject, owner.SiteId).First(&subjectOwner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrExternalIdentityAlreadyClaimed
		}
		return err
	}
	if subjectOwner.UserId != userId {
		return ErrExternalIdentityAlreadyClaimed
	}

	var userClaim ExternalIdentityClaim
	if err := tx.Where("provider = ? AND user_id = ?", provider, userId).First(&userClaim).Error; err != nil {
		return err
	}
	if userClaim.Subject != subject {
		return ErrExternalIdentityAlreadyClaimed
	}
	return nil
}

func ReleaseExternalIdentityWithTx(tx *gorm.DB, provider string, userId int) error {
	provider = strings.TrimSpace(provider)
	if tx == nil || provider == "" || userId == 0 {
		return errors.New("external identity release is invalid")
	}
	return tx.Where("provider = ? AND user_id = ?", provider, userId).
		Delete(&ExternalIdentityClaim{}).Error
}

func GetUserByTelegramID(telegramID string, siteID int) (*User, error) {
	var user User
	err := DB.Where("telegram_id = ? AND site_id = ?", telegramID, siteID).First(&user).Error
	return &user, err
}

// BindTelegramForSessionWithTx preserves single ownership and the session that
// started the binding. The caller consumes its OAuth flow in this transaction.
func BindTelegramForSessionWithTx(tx *gorm.DB, identity AuthSessionIdentity, telegramID string) error {
	if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
		return err
	}
	var user User
	if err := tx.Select("id", "telegram_id").First(&user, identity.UserID).Error; err != nil {
		return err
	}
	if user.TelegramId != "" {
		return ErrExternalIdentityAlreadyClaimed
	}
	if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, telegramID, user.Id); err != nil {
		return err
	}
	result := tx.Model(&User{}).Where("id = ? AND (telegram_id = ? OR telegram_id IS NULL)", user.Id, "").Update("telegram_id", telegramID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrExternalIdentityAlreadyClaimed
	}
	return nil
}

func releaseAllExternalIdentitiesWithTx(tx *gorm.DB, userId int) error {
	if tx == nil || userId == 0 {
		return errors.New("external identity release is invalid")
	}
	return tx.Where("user_id = ?", userId).Delete(&ExternalIdentityClaim{}).Error
}

// InitializeExternalIdentityClaims imports legacy Telegram bindings after the
// claim table is migrated. Existing duplicate ownership fails migration rather
// than preserving an ambiguous login identity.
func InitializeExternalIdentityClaims() error {
	var users []User
	if err := DB.Unscoped().Select("id", "telegram_id").
		Where("telegram_id <> ?", "").Find(&users).Error; err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, user := range users {
			if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.TelegramId, user.Id); err != nil {
				return fmt.Errorf("backfill Telegram identity for user %d: %w", user.Id, err)
			}
		}
		return nil
	})
}

// EnsureExternalIdentityClaimSiteIndexes replaces the platform-wide subject
// unique index with provider + site_id + subject, after copying each claim's
// site from its user. Same-site duplicates still fail; cross-site subjects
// become legal once the old index is gone.
func EnsureExternalIdentityClaimSiteIndexes(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("external identity claim indexes: database is nil")
	}
	if !db.Migrator().HasTable(&ExternalIdentityClaim{}) {
		return nil
	}
	if db.Migrator().HasTable(&User{}) {
		if err := db.Exec(`
			UPDATE external_identity_claims
			SET site_id = COALESCE((SELECT site_id FROM users WHERE users.id = external_identity_claims.user_id), site_id)
		`).Error; err != nil {
			return fmt.Errorf("backfill external identity claim site_id: %w", err)
		}
	}
	if db.Migrator().HasIndex(&ExternalIdentityClaim{}, legacyExternalIdentitySubjectIndex) {
		if err := db.Migrator().DropIndex(&ExternalIdentityClaim{}, legacyExternalIdentitySubjectIndex); err != nil {
			return fmt.Errorf("drop legacy external identity subject index: %w", err)
		}
	}
	if db.Migrator().HasIndex(&ExternalIdentityClaim{}, externalIdentitySubjectIndex) {
		return nil
	}
	statement := &gorm.Statement{DB: db}
	return db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON %s (%s, %s, %s)",
		statement.Quote(externalIdentitySubjectIndex),
		statement.Quote("external_identity_claims"),
		statement.Quote("provider"),
		statement.Quote("site_id"),
		statement.Quote("subject"),
	)).Error
}
