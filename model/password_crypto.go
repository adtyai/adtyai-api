package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	activeLoginEncryptionKeySlot = "active"
	// loginEncryptionKeyPurpose binds the sealed key material to this one use,
	// so a ciphertext copied from any other adty secret cannot be opened here.
	loginEncryptionKeyPurpose = "login-encryption-key"
)

// LoginEncryptionKey stores internal key material used by the browser login
// protocol. It is deliberately separate from administrator-facing options.
// PrivateKeyPEM holds an adty:secret:v1: ciphertext, never the PKCS#8 PEM: the
// column keeps its historical name so existing deployments migrate in place.
type LoginEncryptionKey struct {
	ID            uint   `json:"-" gorm:"primaryKey"`
	Slot          string `json:"-" gorm:"type:varchar(32);not null;uniqueIndex"`
	PrivateKeyPEM string `json:"-" gorm:"type:text;not null"`
}

// sealLoginEncryptionKeyPEM encrypts the generated private key before it
// reaches the database. The AEAD key is the deployment CRYPTO_SECRET, which
// must stay outside the database for a restore to be useless on its own.
func sealLoginEncryptionKeyPEM(privateKeyPEM string) (string, error) {
	sealed, err := SealAdtyPurposeSecret(loginEncryptionKeyPurpose, privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("password encryption key needs CRYPTO_SECRET of at least 32 bytes to be stored sealed: %w", err)
	}
	return sealed, nil
}

// openLoginEncryptionKeyPEM returns the PEM behind one stored row. A legacy
// plaintext row is upgraded in place; only the ciphertext is bound to the
// UPDATE so the PEM never shows up in SQL logs.
func openLoginEncryptionKeyPEM(stored *LoginEncryptionKey) (string, error) {
	if strings.HasPrefix(stored.PrivateKeyPEM, adtySecretPrefix) {
		privateKeyPEM, err := OpenAdtyPurposeSecret(loginEncryptionKeyPurpose, stored.PrivateKeyPEM)
		if err != nil {
			return "", fmt.Errorf("decrypt password encryption key: %w", err)
		}
		return privateKeyPEM, nil
	}
	if strings.HasPrefix(stored.PrivateKeyPEM, "adty:secret:") {
		return "", errors.New("password encryption key is sealed in an unsupported secret format")
	}
	privateKeyPEM := stored.PrivateKeyPEM
	sealed, err := sealLoginEncryptionKeyPEM(privateKeyPEM)
	if err != nil {
		return "", err
	}
	if err := DB.Model(&LoginEncryptionKey{}).Where("id = ?", stored.ID).Update("private_key_pem", sealed).Error; err != nil {
		return "", fmt.Errorf("encrypt password encryption key at rest: %w", err)
	}
	stored.PrivateKeyPEM = sealed
	return privateKeyPEM, nil
}

// InitPasswordEncryption loads the shared login-encryption key from its
// dedicated store. Concurrent replicas converge through the unique slot.
func InitPasswordEncryption() error {
	var stored LoginEncryptionKey
	queryErr := DB.Where("slot = ?", activeLoginEncryptionKeySlot).First(&stored).Error
	if queryErr == nil {
		privateKeyPEM, err := openLoginEncryptionKeyPEM(&stored)
		if err != nil {
			return err
		}
		if err := common.LoadPasswordEncryptionPrivateKey(privateKeyPEM); err != nil {
			return fmt.Errorf("load persisted password encryption key: %w", err)
		}
		return nil
	}
	if !errors.Is(queryErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read password encryption key: %w", queryErr)
	}

	privateKeyPEM, err := common.GeneratePasswordEncryptionPrivateKey()
	if err != nil {
		return err
	}
	sealed, err := sealLoginEncryptionKeyPEM(privateKeyPEM)
	if err != nil {
		return err
	}
	candidate := LoginEncryptionKey{
		Slot:          activeLoginEncryptionKeySlot,
		PrivateKeyPEM: sealed,
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "slot"}},
		DoNothing: true,
	}).Create(&candidate).Error; err != nil {
		return fmt.Errorf("persist password encryption key: %w", err)
	}

	if err := DB.Where("slot = ?", activeLoginEncryptionKeySlot).First(&stored).Error; err != nil {
		return fmt.Errorf("reload password encryption key: %w", err)
	}
	privateKeyPEM, err = openLoginEncryptionKeyPEM(&stored)
	if err != nil {
		return err
	}
	if err := common.LoadPasswordEncryptionPrivateKey(privateKeyPEM); err != nil {
		return fmt.Errorf("load persisted password encryption key: %w", err)
	}
	return nil
}
