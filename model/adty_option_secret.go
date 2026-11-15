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
	"slices"
	"strings"
	"sync"

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

// adtySecretClearedValues 是「这一条读不出来」时写回内存的值：token 是空串，
// widget secret 是 JSON 对象（配置解析把 `{}` 读成空表）。
var adtySecretClearedValues = map[string]string{
	adty_setting.ResellerSettingModule + "." + adty_setting.ResellerKeyCloudflareTok: "",
	adty_setting.ResellerSettingModule + "." + adty_setting.ResellerKeyWidgetSecret:  "{}",
}

var adtyClearedSecretKeys struct {
	sync.RWMutex
	keys []string
}

// ClearedAdtyOptionSecrets 返回最近一次装载时因为解不开而被清空的密钥键名。
// 后台设置页据此告诉管理员要重填哪一条；空表示没有。
func ClearedAdtyOptionSecrets() []string {
	adtyClearedSecretKeys.RLock()
	defer adtyClearedSecretKeys.RUnlock()
	return slices.Clone(adtyClearedSecretKeys.keys)
}

func recordAdtyClearedOptionSecrets(keys []string) {
	adtyClearedSecretKeys.Lock()
	adtyClearedSecretKeys.keys = slices.Clone(keys)
	adtyClearedSecretKeys.Unlock()
}

// adtyOptionSecretPlaintext 把一行密钥解成明文。历史明文行原样返回（封装成密文是
// AllOption 的事，这里只读）；没有可用的加密密钥时明文行同样算读不出来，否则它会
// 在后续写入里悄悄退化成未加密存储。
func adtyOptionSecretPlaintext(key, stored string) (string, bool) {
	if stored == "" {
		return "", true
	}
	if strings.HasPrefix(stored, adtySecretPrefix) {
		plain, err := decodeAdtyOptionSecret(key, stored)
		return plain, err == nil
	}
	if _, err := encodeAdtyOptionSecret(key, stored); err != nil {
		return "", false
	}
	return stored, true
}

// reloadAdtyOptionSecrets 重新装载两条站点密钥，只清掉解不开的那几条，返回被清的键名。
//
// 原来这里是「AllOption 一出错就把两条一起清」，而 AllOption 是逐行解密的循环，
// 遇到第一条解不开就整体返回错误（model/option.go:29-52）：「一条密文坏了」被当成
// 「全部坏了」，另一条完好的密钥被连带清空。运营看到的是「分站注册全挂了」，
// 线索只有一行 SysError，根因却在另一条完全无关的密钥上（ISS-R4-0122）。
//
// 读库本身失败时一条都不清：那时谁坏了还无从判断，把还好的密钥一起抹掉只会让
// 一次瞬时故障变成一次配置丢失——下次同步周期才恢复。
func reloadAdtyOptionSecrets() []string {
	var rows []*Option
	if err := DB.Where(commonKeyCol+" IN ?", adtySecretOptionKeys).Find(&rows).Error; err != nil {
		common.SysError(common.LogText("could not read site credentials; leaving them untouched: %s", err.Error()))
		return nil
	}
	stored := make(map[string]string, len(rows))
	for _, row := range rows {
		stored[row.Key] = row.Value
	}
	var cleared []string
	for _, key := range adtySecretOptionKeys {
		plain, readable := adtyOptionSecretPlaintext(key, stored[key])
		if !readable {
			plain = adtySecretClearedValues[key]
			cleared = append(cleared, key)
		} else if plain == "" {
			// 库里没有这一行：按「没配」装载，widget secret 要的是 `{}` 而不是空串。
			plain = adtySecretClearedValues[key]
		}
		if err := updateOptionMap(key, plain); err != nil {
			common.SysLog(common.LogText("failed to update option map: %s", err.Error()))
		}
	}
	if len(cleared) > 0 {
		common.SysError(common.LogText("site credentials %s could not be decrypted and were cleared; the other credential is unchanged", strings.Join(cleared, ", ")))
	}
	recordAdtyClearedOptionSecrets(cleared)
	return cleared
}
