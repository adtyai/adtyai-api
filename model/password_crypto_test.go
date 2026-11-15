package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testLoginEncryptionCryptoSecret = "login-encryption-key-at-rest-test-secret"

func prepareLoginEncryptionKeyTable(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&LoginEncryptionKey{}))
	t.Cleanup(func() {
		_ = DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&LoginEncryptionKey{}).Error
	})
}

func loadStoredLoginEncryptionKey(t *testing.T) LoginEncryptionKey {
	t.Helper()
	var stored LoginEncryptionKey
	require.NoError(t, DB.Where("slot = ?", activeLoginEncryptionKeySlot).First(&stored).Error)
	return stored
}

// The login-encryption RSA key must never reach the database as PKCS#8 PEM: a
// backup or a read replica would otherwise hand out the key that makes every
// stored password hash recoverable from captured login traffic.
func TestInitPasswordEncryptionSealsPrivateKeyAtRest(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", testLoginEncryptionCryptoSecret)
	prepareLoginEncryptionKeyTable(t)

	require.NoError(t, InitPasswordEncryption())

	stored := loadStoredLoginEncryptionKey(t)
	assert.True(t, strings.HasPrefix(stored.PrivateKeyPEM, adtySecretPrefix), "the private key must be sealed at rest")
	assert.NotContains(t, stored.PrivateKeyPEM, "PRIVATE KEY")

	keyID, publicKeyPEM := common.PasswordEncryptionPublicKey()
	assert.NotEmpty(t, keyID)
	assert.Contains(t, publicKeyPEM, "BEGIN PUBLIC KEY")

	// A reload reuses the stored key instead of re-encrypting or replacing it.
	require.NoError(t, InitPasswordEncryption())
	assert.Equal(t, stored.PrivateKeyPEM, loadStoredLoginEncryptionKey(t).PrivateKeyPEM)
	reloadedKeyID, _ := common.PasswordEncryptionPublicKey()
	assert.Equal(t, keyID, reloadedKeyID)
}

// Rows written before this change hold a plaintext PEM. They are upgraded in
// place, and the same key stays active so in-flight login pages keep working.
func TestInitPasswordEncryptionUpgradesLegacyPlaintextRow(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", testLoginEncryptionCryptoSecret)
	prepareLoginEncryptionKeyTable(t)

	legacyPEM, err := common.GeneratePasswordEncryptionPrivateKey()
	require.NoError(t, err)
	require.NoError(t, DB.Create(&LoginEncryptionKey{
		Slot:          activeLoginEncryptionKeySlot,
		PrivateKeyPEM: legacyPEM,
	}).Error)
	require.NoError(t, common.LoadPasswordEncryptionPrivateKey(legacyPEM))
	legacyKeyID, _ := common.PasswordEncryptionPublicKey()

	require.NoError(t, InitPasswordEncryption())

	stored := loadStoredLoginEncryptionKey(t)
	require.True(t, strings.HasPrefix(stored.PrivateKeyPEM, adtySecretPrefix), "the legacy row must be sealed after the upgrade")
	opened, err := OpenAdtyPurposeSecret(loginEncryptionKeyPurpose, stored.PrivateKeyPEM)
	require.NoError(t, err)
	assert.Equal(t, legacyPEM, opened)

	upgradedKeyID, _ := common.PasswordEncryptionPublicKey()
	assert.Equal(t, legacyKeyID, upgradedKeyID, "the upgrade must keep the same key active")
}

// Without a deployment key there is nothing to seal with: fail instead of
// falling back to plaintext.
func TestInitPasswordEncryptionRefusesToPersistUnsealedKey(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	prepareLoginEncryptionKeyTable(t)

	require.Error(t, InitPasswordEncryption())

	var count int64
	require.NoError(t, DB.Model(&LoginEncryptionKey{}).Count(&count).Error)
	assert.Zero(t, count, "no key material may be persisted when it cannot be sealed")
}
