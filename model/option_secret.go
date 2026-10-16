package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// OptionSecretPrefix marks an options.value that is AES-GCM encrypted with ADTY_SECRET_KEY.
const OptionSecretPrefix = "adtyenc:"

// ErrOptionSecretPrefixInput rejects external option values that already carry the
// ciphertext prefix, so a client cannot forge "adtyenc:..." and bypass encryption.
var ErrOptionSecretPrefixInput = errors.New("配置值不能以保留前缀 adtyenc: 开头")

// RejectOptionSecretPrefix validates external option writes: any value starting with
// OptionSecretPrefix is refused. Only encodeOptionValueForDB itself produces the prefix.
func RejectOptionSecretPrefix(values map[string]string) error {
	for k, v := range values {
		if strings.HasPrefix(v, OptionSecretPrefix) {
			return fmt.Errorf("%w: %s", ErrOptionSecretPrefixInput, k)
		}
	}
	return nil
}

// IsSensitiveOptionKey mirrors the GetOptions masking rule: keys ending with
// Token/Secret/Key/secret/api_key are secrets, except the public Chatwoot website token.
func IsSensitiveOptionKey(k string) bool {
	if k == OptChatwootWebsiteToken {
		return false
	}
	return strings.HasSuffix(k, "Token") ||
		strings.HasSuffix(k, "Secret") ||
		strings.HasSuffix(k, "Key") ||
		strings.HasSuffix(k, "secret") ||
		strings.HasSuffix(k, "api_key")
}

// encodeOptionValueForDB returns what should be stored in DB for key/value.
// Sensitive non-empty values are encrypted; missing ADTY_SECRET_KEY fails closed.
func encodeOptionValueForDB(key, value string) (string, error) {
	if strings.HasPrefix(value, OptionSecretPrefix) {
		return "", fmt.Errorf("%w: %s", ErrOptionSecretPrefixInput, key)
	}
	if !IsSensitiveOptionKey(key) || value == "" {
		return value, nil
	}
	enc, err := common.EncryptAdtySecret(value)
	if err != nil {
		return "", errors.New("敏感配置落库加密失败（检查 ADTY_SECRET_KEY）: " + err.Error())
	}
	return OptionSecretPrefix + enc, nil
}

// decodeOptionValueFromDB returns plaintext for runtime; needsMigrate is true for
// legacy plaintext sensitive values that should be re-encrypted in DB.
func decodeOptionValueFromDB(key, value string) (plain string, needsMigrate bool, err error) {
	if strings.HasPrefix(value, OptionSecretPrefix) {
		p, err := common.DecryptAdtySecret(strings.TrimPrefix(value, OptionSecretPrefix))
		if err != nil {
			return "", false, err
		}
		return p, false, nil
	}
	return value, IsSensitiveOptionKey(key) && value != "", nil
}

// encodeOptionValuesForDB encodes a batch, failing closed before any write.
func encodeOptionValuesForDB(values map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for k, v := range values {
		e, err := encodeOptionValueForDB(k, v)
		if err != nil {
			return nil, err
		}
		out[k] = e
	}
	return out, nil
}

// migrateLegacyOptionSecrets lazily rewrites legacy plaintext sensitive options as ciphertext.
func migrateLegacyOptionSecrets(values map[string]string) {
	for k, v := range values {
		enc, err := encodeOptionValueForDB(k, v)
		if err != nil {
			common.SysError(common.LogText("option secret migrate skipped for %s: %s", k, err.Error()))
			continue
		}
		if err := DB.Model(&Option{}).Where(commonKeyCol+" = ?", k).Where("value = ?", v).Update("value", enc).Error; err != nil {
			common.SysError(common.LogText("option secret migrate failed for %s: %s", k, err.Error()))
		}
	}
}
