/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"gorm.io/gorm"
)

const adtySecretPrefix = "adty:secret:v1:"

var adtySecretOptionKeys = []string{
	adty_setting.ResellerSettingModule + "." + adty_setting.ResellerKeyCloudflareTok,
	adty_setting.ResellerSettingModule + "." + adty_setting.ResellerKeyWidgetSecret,
}

func isAdtySecretOption(key string) bool {
	return key == adty_setting.ResellerSettingModule+"."+adty_setting.ResellerKeyCloudflareTok ||
		key == adty_setting.ResellerSettingModule+"."+adty_setting.ResellerKeyWidgetSecret
}

// The deployment key stays outside the database and must survive restarts and restores.
func adtyOptionSecretCipher() (cipher.AEAD, error) {
	secret := os.Getenv("CRYPTO_SECRET")
	if len(secret) < 32 {
		return nil, common.NewMessage("Operation failed")
	}
	// Extract before expanding: other business flows store root-key HMACs of
	// attacker-controlled input, which must never disclose this encryption key.
	key, err := hkdf.Key(sha256.New, []byte(secret), []byte("ADTY AI encrypted options"), adtySecretPrefix, 32)
	if err != nil {
		return nil, common.NewMessage("Operation failed")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, common.NewMessage("Operation failed")
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func encodeAdtyOptionSecret(key, value string) (string, error) {
	for _, canonical := range adtySecretOptionKeys {
		if key != canonical && strings.EqualFold(strings.TrimSpace(key), canonical) {
			return "", common.NewMessage("Operation failed")
		}
	}
	if !isAdtySecretOption(key) || value == "" {
		return value, nil
	}
	if strings.HasPrefix(value, "adty:secret:") {
		return "", common.NewMessage("Operation failed")
	}
	box, err := adtyOptionSecretCipher()
	if err != nil {
		return "", err
	}
	sealed := box.Seal(nil, nil, []byte(value), []byte(key))
	return adtySecretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decodeAdtyOptionSecret(key, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	encoded, ok := strings.CutPrefix(stored, adtySecretPrefix)
	if !ok {
		return "", common.NewMessage("Operation failed")
	}
	box, err := adtyOptionSecretCipher()
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", common.NewMessage("Operation failed")
	}
	plain, err := box.Open(nil, nil, sealed, []byte(key))
	if err != nil {
		return "", common.NewMessage("Operation failed")
	}
	return string(plain), nil
}

// Caller owns the transaction. Lock the current row before upgrading legacy
// plaintext so a concurrent credential replacement cannot be overwritten.
func readAdtyOptionSecret(tx *gorm.DB, key string) (string, bool, error) {
	var row Option
	err := lockForUpdate(tx).Where(&Option{Key: key}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, common.NewMessage("Operation failed")
	}
	if row.Value == "" || strings.HasPrefix(row.Value, "adty:secret:") {
		plain, err := decodeAdtyOptionSecret(key, row.Value)
		return plain, true, err
	}
	sealed, err := encodeAdtyOptionSecret(key, row.Value)
	if err != nil {
		return "", true, err
	}
	// Only ciphertext is bound to the UPDATE. Plaintext never appears in SQL logs.
	if err := tx.Model(&Option{}).Where(&Option{Key: key}).Update("value", sealed).Error; err != nil {
		return "", true, common.NewMessage("Operation failed")
	}
	return row.Value, true, nil
}

// SealAdtyPurposeSecret encrypts a credential for one named purpose. The purpose is the AEAD data.
func SealAdtyPurposeSecret(purpose, value string) (string, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" || strings.ContainsAny(purpose, "\r\n") {
		return "", common.NewMessage("Invalid parameters")
	}
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "adty:secret:") {
		return "", common.NewMessage("Operation failed")
	}
	box, err := adtyOptionSecretCipher()
	if err != nil {
		return "", err
	}
	sealed := box.Seal(nil, nil, []byte(value), []byte("purpose:"+purpose))
	return adtySecretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// OpenAdtyPurposeSecret decrypts a value sealed for the same purpose.
func OpenAdtyPurposeSecret(purpose, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	encoded, ok := strings.CutPrefix(stored, adtySecretPrefix)
	if !ok {
		return "", common.NewMessage("Operation failed")
	}
	box, err := adtyOptionSecretCipher()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", common.NewMessage("Operation failed")
	}
	plain, err := box.Open(nil, nil, raw, []byte("purpose:"+strings.TrimSpace(purpose)))
	if err != nil {
		return "", common.NewMessage("Operation failed")
	}
	return string(plain), nil
}

func clearAdtyOptionSecrets() {
	_ = updateOptionMap(adty_setting.ResellerSettingModule+"."+adty_setting.ResellerKeyCloudflareTok, "")
	_ = updateOptionMap(adty_setting.ResellerSettingModule+"."+adty_setting.ResellerKeyWidgetSecret, "{}")
}
