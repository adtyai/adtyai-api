package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// A29 Chatwoot option keys (adtyai-tech A15 naming).
const (
	OptChatwootScript       = "ChatwootScript"
	OptChatwootWebsiteToken = "ChatwootWebsiteToken" // public exception — readable
	OptChatwootHmacSecret   = "ChatwootHmacSecret"   // sensitive — fingerprint only
	OptChatwootBaseUrl      = "ChatwootBaseUrl"      // optional probe target
)

// ChatwootHmacSecretMeta for GET: secret_set + hm_ fingerprint (no plaintext).
func ChatwootHmacSecretMeta() (secretSet bool, fingerprint string) {
	sec := chatwootOption(OptChatwootHmacSecret)
	if strings.TrimSpace(sec) == "" {
		return false, ""
	}
	fp, err := common.AdtySecretFingerprint(sec)
	if err != nil || fp == "" {
		return true, ""
	}
	return true, "hm_" + fp
}

func chatwootOption(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap[key])
}

// ChatwootConfigView is GET /api/admin/chatwoot payload (never includes HMAC plaintext).
type ChatwootConfigView struct {
	Script                string `json:"script"`
	WebsiteToken          string `json:"website_token"`
	BaseURL               string `json:"base_url"`
	HmacSecretSet         bool   `json:"hmac_secret_set"`
	HmacSecretFingerprint string `json:"hmac_secret_fingerprint,omitempty"`
	BackupContactType     string `json:"backup_contact_type"`
	BackupContact         string `json:"backup_contact"`
	BackupContactNote     string `json:"backup_contact_note"`
	// 审查 R-P2-9：配置了挂件但没配 HMAC 密钥时给出提示（身份校验无法生效）
	Warnings []string `json:"warnings"`
}

// BuildChatwootConfigView assembles safe GET payload; backup contact is read-only here (A45 owns writes).
func BuildChatwootConfigView() ChatwootConfigView {
	set, fp := ChatwootHmacSecretMeta()
	typ, contact := GetBackupContact()
	return ChatwootConfigView{
		Script:                chatwootOption(OptChatwootScript),
		WebsiteToken:          chatwootOption(OptChatwootWebsiteToken),
		BaseURL:               chatwootOption(OptChatwootBaseUrl),
		HmacSecretSet:         set,
		HmacSecretFingerprint: fp,
		BackupContactType:     typ,
		BackupContact:         contact,
		BackupContactNote:     "Read-only from A45 business-params (BackupContactType / BackupContact). Change via PUT /api/admin/business-params.",
		Warnings:              chatwootWarnings(set),
	}
}

func chatwootWarnings(hmacSet bool) []string {
	out := []string{}
	if chatwootOption(OptChatwootWebsiteToken) != "" && !hmacSet {
		out = append(out, "hmac_secret_missing: identity verification disabled; set the inbox HMAC secret and enable identity validation in Chatwoot")
	}
	if base := strings.TrimSpace(chatwootOption(OptChatwootBaseUrl)); base != "" && !ChatwootBaseURLAllowed(base) {
		out = append(out, "base_url_invalid: public widget disabled until base_url is an https URL without credentials")
	}
	return out
}

// ChatwootBaseURLAllowed 只接受无凭据的 https 地址。本机 http 仅用于本地 Chatwoot。
// 空字符串表示不加载挂件，允许保存。javascript/data/file 和用户信息一律拒绝。
func ChatwootBaseURLAllowed(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "\r\n\t\\ ") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || !u.IsAbs() || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(u.Path, "..") || strings.Contains(u.EscapedPath(), "..") {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return true
	case "http":
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		return false
	}
}

// ChatwootPutInput is the PUT body (pointers = present). Empty/omitted hmac_secret leaves secret unchanged.
type ChatwootPutInput struct {
	Script       *string `json:"script"`
	WebsiteToken *string `json:"website_token"`
	BaseURL      *string `json:"base_url"`
	HmacSecret   *string `json:"hmac_secret"`
	Confirm      bool    `json:"confirm"`
}

// ChatwootPutNeedsConfirm reports whether the request changes script or sets a new HMAC secret.
func ChatwootPutNeedsConfirm(in ChatwootPutInput) bool {
	if in.Script != nil && *in.Script != chatwootOption(OptChatwootScript) {
		return true
	}
	if in.HmacSecret != nil && strings.TrimSpace(*in.HmacSecret) != "" {
		return true
	}
	return false
}

// ApplyChatwootPut persists chatwoot options. Empty/omitted hmac_secret leaves secret unchanged.
func ApplyChatwootPut(in ChatwootPutInput) (oldSnap, newSnap map[string]any, err error) {
	oldSnap = chatwootAuditSnap()
	updates := map[string]string{}
	if in.Script != nil {
		updates[OptChatwootScript] = *in.Script
	}
	if in.WebsiteToken != nil {
		updates[OptChatwootWebsiteToken] = strings.TrimSpace(*in.WebsiteToken)
	}
	if in.BaseURL != nil {
		base := strings.TrimSpace(*in.BaseURL)
		if !ChatwootBaseURLAllowed(base) {
			return oldSnap, nil, fmt.Errorf("base_url must be an absolute https URL without credentials")
		}
		updates[OptChatwootBaseUrl] = base
	}
	if in.HmacSecret != nil {
		sec := strings.TrimSpace(*in.HmacSecret)
		if sec != "" {
			updates[OptChatwootHmacSecret] = sec
		}
	}
	if len(updates) == 0 {
		newSnap = chatwootAuditSnap()
		return oldSnap, newSnap, nil
	}
	for k, v := range updates {
		if err = UpdateOption(k, v); err != nil {
			return oldSnap, nil, fmt.Errorf("%s: %w", k, err)
		}
	}
	newSnap = chatwootAuditSnap()
	return oldSnap, newSnap, nil
}

func chatwootAuditSnap() map[string]any {
	set, fp := ChatwootHmacSecretMeta()
	return map[string]any{
		"script":                  chatwootOption(OptChatwootScript),
		"website_token":           chatwootOption(OptChatwootWebsiteToken),
		"base_url":                chatwootOption(OptChatwootBaseUrl),
		"hmac_secret_set":         set,
		"hmac_secret_fingerprint": fp,
	}
}

// SeedChatwootOptionsIntoOptionMap registers defaults (called from InitOptionMap).
func SeedChatwootOptionsIntoOptionMap() {
	common.OptionMap[OptChatwootScript] = ""
	common.OptionMap[OptChatwootWebsiteToken] = ""
	common.OptionMap[OptChatwootHmacSecret] = ""
	common.OptionMap[OptChatwootBaseUrl] = ""
}

// ChatwootPublicConfig 审查 N-P2-10：前端加载挂件所需的公开配置（不含 HMAC 密钥）。
type ChatwootPublicConfig struct {
	Enabled           bool   `json:"enabled"`
	BaseURL           string `json:"base_url"`
	WebsiteToken      string `json:"website_token"`
	IdentityVerified  bool   `json:"identity_verification"`
	BackupContactType string `json:"backup_contact_type"`
	BackupContact     string `json:"backup_contact"`
}

func GetChatwootPublicConfig() ChatwootPublicConfig {
	base := strings.TrimRight(strings.TrimSpace(chatwootOption(OptChatwootBaseUrl)), "/")
	if !ChatwootBaseURLAllowed(base) {
		base = ""
	}
	tok := strings.TrimSpace(chatwootOption(OptChatwootWebsiteToken))
	typ, contact := GetBackupContact()
	return ChatwootPublicConfig{
		Enabled:           base != "" && tok != "",
		BaseURL:           base,
		WebsiteToken:      tok,
		IdentityVerified:  chatwootOption(OptChatwootHmacSecret) != "",
		BackupContactType: typ,
		BackupContact:     contact,
	}
}

// ChatwootIdentityHash HMAC-SHA256(ChatwootHmacSecret, identifier) 的 hex，对应 Chatwoot setUser 的 identifier_hash。
func ChatwootIdentityHash(identifier string) (string, bool) {
	sec := chatwootOption(OptChatwootHmacSecret)
	if sec == "" {
		return "", false
	}
	m := hmac.New(sha256.New, []byte(sec))
	m.Write([]byte(identifier))
	return hex.EncodeToString(m.Sum(nil)), true
}
