package smartrouting

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrPreviewRequired = errors.New("preview_required: call POST /weight-schemes/preview first and pass preview_token")
	ErrPreviewInvalid  = errors.New("preview_token invalid or expired")
	ErrPreviewMismatch = errors.New("preview_token does not match requested change")
)

// PreviewIntent describes a weight/scheme change that must be previewed first (P0 server gate).
type PreviewIntent struct {
	Kind       string      `json:"kind"` // activate_scheme | update_weights | put_config
	SchemeID   int64       `json:"scheme_id,omitempty"`
	SchemeName string      `json:"scheme_name,omitempty"`
	Weights    WeightParts `json:"weights,omitempty"`
	OperatorID int         `json:"operator_id"`
}

// Fingerprint 审查 N-P1-3：只绑定“变更后的目标”（方案名 + 权重），不绑定 kind / scheme_id。
// 前端 preview 声明的 kind（put_config / activate_scheme / update_weights）与后端按字段推断的 kind
// 不一致时不再误判 mismatch；token 仍单次有效、10 分钟过期、绑定操作人，且目标不同即拒绝。
func (i PreviewIntent) Fingerprint() string {
	b, _ := json.Marshal(struct {
		SchemeName string      `json:"scheme_name"`
		Weights    WeightParts `json:"weights"`
	}{i.SchemeName, i.Weights})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type previewEntry struct {
	Intent    PreviewIntent
	ExpiresAt time.Time
	Used      bool
}

var (
	previewMu sync.Mutex
	previewStore = map[string]*previewEntry{}
)

const previewTTL = 10 * time.Minute

func newPreviewToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// IssuePreviewToken stores intent and returns an opaque token (TTL 10m, single-use on confirm).
func IssuePreviewToken(intent PreviewIntent) (token string, expiresAt time.Time) {
	previewMu.Lock()
	defer previewMu.Unlock()
	// purge expired
	now := time.Now()
	for k, v := range previewStore {
		if now.After(v.ExpiresAt) || v.Used {
			delete(previewStore, k)
		}
	}
	token = newPreviewToken()
	expiresAt = now.Add(previewTTL)
	previewStore[token] = &previewEntry{Intent: intent, ExpiresAt: expiresAt}
	return token, expiresAt
}

// ConsumePreviewToken validates token matches intent and marks it used.
func ConsumePreviewToken(token string, intent PreviewIntent) error {
	if token == "" {
		return ErrPreviewRequired
	}
	previewMu.Lock()
	defer previewMu.Unlock()
	ent, ok := previewStore[token]
	if !ok || ent.Used || time.Now().After(ent.ExpiresAt) {
		if ok {
			delete(previewStore, token)
		}
		return ErrPreviewInvalid
	}
	// 审查重审 P2-3：preview_token 必须绑定 operator_id；PUT 校验必须一致（任一方为 0 或不等均拒绝）。
	if ent.Intent.OperatorID == 0 || intent.OperatorID == 0 || ent.Intent.OperatorID != intent.OperatorID {
		return ErrPreviewMismatch
	}
	if ent.Intent.Fingerprint() != intent.Fingerprint() {
		return ErrPreviewMismatch
	}
	ent.Used = true
	delete(previewStore, token)
	return nil
}

// RequirePreviewForConfigChange returns nil if no weight/scheme change, else validates token.
func RequirePreviewForConfigChange(old, neu Config, operatorID int, previewToken string) error {
	weightsChanged := old.Weights != neu.Weights
	schemeChanged := old.ActiveWeightScheme != neu.ActiveWeightScheme && neu.ActiveWeightScheme != ""
	if !weightsChanged && !schemeChanged {
		return nil
	}
	intent := PreviewIntent{
		Kind:       "put_config",
		SchemeName: neu.ActiveWeightScheme,
		Weights:    neu.Weights,
		OperatorID: operatorID,
	}
	if schemeChanged && !weightsChanged {
		intent.Kind = "activate_scheme"
	} else if weightsChanged && !schemeChanged {
		intent.Kind = "update_weights"
	}
	return ConsumePreviewToken(previewToken, intent)
}

// RequirePreviewForSchemeUpdate validates token for scheme weight edit.
func RequirePreviewForSchemeUpdate(schemeID int64, name string, weights WeightParts, operatorID int, previewToken string) error {
	intent := PreviewIntent{
		Kind:       "update_weights",
		SchemeID:   schemeID,
		SchemeName: name,
		Weights:    weights,
		OperatorID: operatorID,
	}
	return ConsumePreviewToken(previewToken, intent)
}

// RequirePreviewForActivate validates token for switching active scheme by name.
func RequirePreviewForActivate(schemeName string, weights WeightParts, operatorID int, previewToken string) error {
	intent := PreviewIntent{
		Kind:       "activate_scheme",
		SchemeName: schemeName,
		Weights:    weights,
		OperatorID: operatorID,
	}
	return ConsumePreviewToken(previewToken, intent)
}

// ParseWeightsJSON parses scheme weights JSON into WeightParts.
// ValidateWeights 审查 R-P2-1：各分量 >=0 且总和 >0。
func ValidateWeights(w WeightParts) error {
	if w.Conn < 0 || w.Price < 0 || w.Speed < 0 || w.Stab < 0 || w.Auth < 0 {
		return errors.New("weights must be >= 0")
	}
	if w.Sum() <= 0 {
		return errors.New("weights sum must be > 0")
	}
	return nil
}

func ParseWeightsJSON(raw string) (WeightParts, error) {
	var w WeightParts
	if raw == "" {
		return DefaultWeights(), nil
	}
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return w, fmt.Errorf("bad weights json: %w", err)
	}
	// 审查 N-P2-4：各分量不得为负
	if w.Conn < 0 || w.Price < 0 || w.Speed < 0 || w.Stab < 0 || w.Auth < 0 {
		return w, errors.New("weights must be >= 0")
	}
	if w.Sum() <= 0 {
		return w, errors.New("weights sum must be > 0")
	}
	return w, nil
}
