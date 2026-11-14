package model

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	AuditResultSuccess = "success"
	AuditResultFailure = "failure"

	// officialAuditSiteContextKey matches the site already placed in the
	// request by site-resolution middleware. The model package cannot import
	// service, so the key is compared as a stable string.
	officialAuditSiteContextKey = "adty_site"
)

// ErrOfficialAuditIncomplete is returned when an official writer omits site,
// object, action, or result. Approval association may be empty.
var ErrOfficialAuditIncomplete = errors.New("official audit write requires site, object, action, and result")

// OfficialAuditWrite is the single official G.13.2 input. SiteID must be set
// even when it is 0 (the platform/main site). ApprovalRef may be empty when
// the action has no approval.
type OfficialAuditWrite struct {
	UserId      int
	Username    string
	ActorRole   int
	SiteID      *int
	ObjectType  string
	ObjectID    string
	Action      string
	Category    string
	Content     string
	Before      any
	After       any
	Reason      string
	ApprovalRef string
	Result      string
	Success     bool
	Status      int
	Ip          string
	TokenRef    string
	AuthMethod  string
	RequestId   string
	EventId     string
	CreatedAt   int64
	Other       AuditOther
}

var auditSecretText = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]{8,}|bot\d{6,}:[A-Za-z0-9_\-]{10,}|\d{6,12}:[A-Za-z0-9_\-]{20,}|api[_-]?key\s*[:=]\s*\S+|password\s*[:=]\s*\S+|验证码\s*[:=]\s*\S+)`)

var auditSecretKeyNames = map[string]struct{}{
	"password":          {},
	"passwd":            {},
	"secret":            {},
	"token":             {},
	"api_key":           {},
	"apikey":            {},
	"access_token":      {},
	"bot_token":         {},
	"authorization":     {},
	"private_key":       {},
	"client_secret":     {},
	"verification_code": {},
	"verify_code":       {},
	"otp":               {},
	"session_token":     {},
	"payment_secret":    {},
	"payment_key":       {},
	"merchant_key":      {},
	"webhook_secret":    {},
	"refresh_token":     {},
	"bearer":            {},
	"backup_codes":      {},
	"backup_code":       {},
}

// PrepareOfficialAuditLog validates G.13.2 first-class fields, redacts secrets,
// and returns a complete record ready to persist. It does not write the store.
func PrepareOfficialAuditLog(c *gin.Context, rec OfficialAuditWrite) (AuditLog, error) {
	if rec.SiteID == nil || strings.TrimSpace(rec.ObjectType) == "" || strings.TrimSpace(rec.ObjectID) == "" || strings.TrimSpace(rec.Action) == "" || strings.TrimSpace(rec.Result) == "" {
		return AuditLog{}, ErrOfficialAuditIncomplete
	}
	before, err := encodeOfficialAuditValue(rec.Before)
	if err != nil {
		return AuditLog{}, err
	}
	after, err := encodeOfficialAuditValue(rec.After)
	if err != nil {
		return AuditLog{}, err
	}
	success := rec.Success
	switch rec.Result {
	case AuditResultFailure:
		success = false
	case AuditResultSuccess:
		success = true
	}
	entry := AuditLog{
		UserId:      rec.UserId,
		Username:    rec.Username,
		ActorRole:   rec.ActorRole,
		SiteId:      rec.SiteID,
		ObjectType:  strings.TrimSpace(rec.ObjectType),
		ObjectId:    truncateOfficialAuditID(strings.TrimSpace(rec.ObjectID)),
		Action:      strings.TrimSpace(rec.Action),
		Category:    rec.Category,
		Content:     rec.Content,
		BeforeValue: before,
		AfterValue:  after,
		Reason:      rec.Reason,
		ApprovalRef: strings.TrimSpace(rec.ApprovalRef),
		Result:      strings.TrimSpace(rec.Result),
		Success:     success,
		Status:      rec.Status,
		Ip:          rec.Ip,
		TokenRef:    rec.TokenRef,
		AuthMethod:  rec.AuthMethod,
		RequestId:   rec.RequestId,
		EventId:     rec.EventId,
		CreatedAt:   rec.CreatedAt,
		Other:       rec.Other,
	}
	if entry.Category == "" {
		entry.Category = AuditCategoryOperation
	}
	entry = PrepareAuditLog(c, entry)
	redactAuditLog(&entry)
	return entry, nil
}

// WriteOfficialAuditLog is the single official writer for new G.13.2 records.
func WriteOfficialAuditLog(c *gin.Context, rec OfficialAuditWrite) error {
	entry, err := PrepareOfficialAuditLog(c, rec)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return writeAuditLog(context.WithoutCancel(ctx), entry, false)
}

func OfficialAuditSiteIDFromContext(c *gin.Context) int {
	if c == nil {
		return 0
	}
	value, ok := c.Get(officialAuditSiteContextKey)
	if !ok || value == nil {
		return 0
	}
	if site, ok := value.(*AdtySite); ok && site != nil {
		return site.Id
	}
	return 0
}

func OfficialAuditSiteIDPointer(id int) *int {
	siteID := id
	return &siteID
}

func OfficialAuditResult(success bool) string {
	if success {
		return AuditResultSuccess
	}
	return AuditResultFailure
}

// InferOfficialAuditObject derives a stable object type and identifier from an
// action and its structured parameters. The fallback IDs are used only when no
// parameter identifies the object.
func InferOfficialAuditObject(action string, params map[string]any, fallbackIDs ...string) (objectType, objectID string) {
	objectType = strings.TrimSpace(action)
	if i := strings.LastIndex(objectType, "."); i > 0 {
		objectType = objectType[:i]
	}
	if objectType == "" || objectType == "generic" {
		objectType = "request"
	}
	for _, key := range []string{
		"id", "target_user_id", "user_id", "site_id", "trade_no", "key", "model",
		"invite_id", "report_id", "domain_id", "code", "username", "channel_id",
		"ticket_id", "voucher_id", "withdrawal_id", "token_ref",
	} {
		if params == nil {
			break
		}
		if id := stringifyOfficialAuditID(params[key]); id != "" {
			return objectType, id
		}
	}
	for _, id := range fallbackIDs {
		if strings.TrimSpace(id) != "" {
			return objectType, strings.TrimSpace(id)
		}
	}
	if action != "" {
		return objectType, action
	}
	return objectType, "unknown"
}

func InferOfficialAuditBeforeAfter(params map[string]any) (before, after any) {
	if params == nil {
		return nil, nil
	}
	for _, key := range []string{"before", "old", "old_value", "from", "previous"} {
		if value, ok := params[key]; ok {
			before = value
			break
		}
	}
	for _, key := range []string{"after", "new", "new_value", "to"} {
		if value, ok := params[key]; ok {
			after = value
			break
		}
	}
	return before, after
}

func InferOfficialAuditReason(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"reason", "note"} {
		if text := stringifyOfficialAuditID(params[key]); text != "" {
			return text
		}
	}
	return ""
}

func InferOfficialAuditApprovalRef(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"approval_ref", "approval_id", "approval"} {
		if text := stringifyOfficialAuditID(params[key]); text != "" {
			return text
		}
	}
	return ""
}

func InferOfficialAuditSiteID(c *gin.Context, params map[string]any) int {
	if params != nil {
		if id, ok := intFromOfficialAuditValue(params["site_id"]); ok {
			return id
		}
	}
	return OfficialAuditSiteIDFromContext(c)
}

func encodeOfficialAuditValue(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	switch typed := value.(type) {
	case string:
		return redactAuditText(typed), nil
	case []byte:
		return redactAuditText(string(typed)), nil
	}
	encoded, err := common.Marshal(redactAuditValue(value))
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func truncateOfficialAuditID(id string) string {
	if len(id) <= 128 {
		return id
	}
	return id[:128]
}

func stringifyOfficialAuditID(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.RawMessage:
		return stringifyOfficialAuditID(decodeOfficialRaw(typed))
	case AuditFields:
		return ""
	}
	if text, err := common.Marshal(value); err == nil {
		s := strings.TrimSpace(string(text))
		s = strings.Trim(s, `"`)
		return s
	}
	return strings.TrimSpace(common.Interface2String(value))
}

func intFromOfficialAuditValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	case string:
		id, err := strconv.Atoi(strings.TrimSpace(typed))
		return id, err == nil
	case json.RawMessage:
		return intFromOfficialAuditValue(decodeOfficialRaw(typed))
	}
	return 0, false
}

func decodeOfficialRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if common.Unmarshal(raw, &decoded) != nil {
		return strings.TrimSpace(string(raw))
	}
	return decoded
}

func redactAuditLog(entry *AuditLog) {
	if entry == nil {
		return
	}
	entry.Content = redactAuditText(entry.Content)
	entry.BeforeValue = redactAuditText(entry.BeforeValue)
	entry.AfterValue = redactAuditText(entry.AfterValue)
	entry.Reason = redactAuditText(entry.Reason)
	entry.ApprovalRef = redactAuditText(entry.ApprovalRef)
	entry.Other = redactAuditOther(entry.Other)
}

func redactAuditOther(other AuditOther) AuditOther {
	if other.Op != nil {
		op := *other.Op
		op.Action = redactAuditText(op.Action)
		op.Params = redactAuditFields(op.Params)
		other.Op = &op
	}
	if other.AdminInfo != nil {
		info := *other.AdminInfo
		info.AdminUsername = redactAuditText(info.AdminUsername)
		info.AuthMethod = redactAuditText(info.AuthMethod)
		other.AdminInfo = &info
	}
	if other.AuditInfo != nil {
		info := *other.AuditInfo
		info.Params = redactStringMap(info.Params)
		other.AuditInfo = &info
	}
	other.RootInfo = redactAuditFields(other.RootInfo)
	other.LoginMethod = redactAuditText(other.LoginMethod)
	other.UserAgent = redactAuditText(other.UserAgent)
	return other
}

func redactAuditFields(fields AuditFields) AuditFields {
	if fields == nil {
		return nil
	}
	redacted := make(AuditFields, len(fields))
	for key, value := range fields {
		if isOfficialAuditSecretKey(key) {
			redacted[key] = "[redacted]"
			continue
		}
		redacted[key] = redactAuditValue(value)
	}
	return redacted
}

func redactStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if isOfficialAuditSecretKey(key) {
			redacted[key] = "[redacted]"
			continue
		}
		redacted[key] = redactAuditText(value)
	}
	return redacted
}

func redactAuditValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactAuditText(typed)
	case []byte:
		return redactAuditText(string(typed))
	case json.RawMessage:
		return redactAuditValue(decodeOfficialRaw(typed))
	case AuditFields:
		return redactAuditFields(typed)
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			if isOfficialAuditSecretKey(key) {
				redacted[key] = "[redacted]"
				continue
			}
			redacted[key] = redactAuditValue(item)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactAuditValue(item)
		}
		return redacted
	case []string:
		redacted := make([]string, len(typed))
		for i, item := range typed {
			redacted[i] = redactAuditText(item)
		}
		return redacted
	default:
		return value
	}
}

func redactAuditText(text string) string {
	if text == "" {
		return text
	}
	return auditSecretText.ReplaceAllString(text, "[redacted]")
}

func isOfficialAuditSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	_, ok := auditSecretKeyNames[normalized]
	return ok
}
