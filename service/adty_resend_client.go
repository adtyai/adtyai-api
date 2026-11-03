package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Resend API 客户端（P11、P12 共用）。按官方 API 文档的示例写，没有实测（11.0 推迟到上线前），
// 原文在 adtyai-design/research/verify-p11/resend_*.txt。测试替换 BaseURL 和 HTTP，指向 httptest 假服务器。
// API Key 复用上游的 ResendApiKey（common/email.go:81，后台「系统设置 → 邮件」），不另设。
// 响应原文和 API Key 都不打日志。

const adtyResendDefaultBaseURL = "https://api.resend.com"

// AdtyResendClient 调 Resend 的 REST API。字段都可以在测试里换掉。
type AdtyResendClient struct {
	BaseURL string
	HTTP    *http.Client
	APIKey  func() string
}

// NewAdtyResendClient 返回用正式地址和上游 ResendApiKey 的客户端。
func NewAdtyResendClient() *AdtyResendClient {
	return &AdtyResendClient{
		BaseURL: adtyResendDefaultBaseURL,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		APIKey: func() string {
			common.OptionMapRWMutex.RLock()
			defer common.OptionMapRWMutex.RUnlock()
			return common.OptionMap["ResendApiKey"]
		},
	}
}

// AdtyResend 是业务代码用的客户端，测试里整个换掉。
var AdtyResend = NewAdtyResendClient()

var (
	// ErrAdtyResendNotConfigured：后台还没填 Resend API Key，不发请求。
	ErrAdtyResendNotConfigured = errors.New("resend api key is not configured")
	// ErrAdtyResendRateLimited：429 限流（默认每个团队每秒 10 个请求），按 RetryAfter 退避后重试。
	ErrAdtyResendRateLimited = errors.New("resend rate limited")
	// ErrAdtyResendQuotaExceeded：429 的 daily_quota_exceeded、monthly_quota_exceeded，重试没用。
	ErrAdtyResendQuotaExceeded = errors.New("resend quota exceeded")
)

// AdtyResendError 是 Resend 返回的错误 {statusCode, name, message}。
type AdtyResendError struct {
	StatusCode int
	Name       string
	Message    string
	RetryAfter time.Duration // 429 时取 Retry-After 头，没有就是 0
}

func (e *AdtyResendError) Error() string {
	return fmt.Sprintf("resend api error: status=%d name=%s message=%s", e.StatusCode, e.Name, e.Message)
}

// Is 让 errors.Is 认出限流和额度用完。
func (e *AdtyResendError) Is(target error) bool {
	if e.StatusCode != http.StatusTooManyRequests {
		return false
	}
	quota := e.Name == "daily_quota_exceeded" || e.Name == "monthly_quota_exceeded"
	switch target {
	case ErrAdtyResendQuotaExceeded:
		return quota
	case ErrAdtyResendRateLimited:
		return !quota
	}
	return false
}

// do 发一个请求。body 为 nil 时不带请求体；out 为 nil 时丢弃响应。
func (c *AdtyResendClient) do(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	apiKey := strings.TrimSpace(c.APIKey())
	if apiKey == "" {
		return ErrAdtyResendNotConfigured
	}
	var reader io.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 1<<20)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &AdtyResendError{StatusCode: resp.StatusCode}
		var failure struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		if common.DecodeJson(limited, &failure) == nil {
			apiErr.Name, apiErr.Message = failure.Name, failure.Message
		}
		if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
			apiErr.RetryAfter = time.Duration(seconds) * time.Second
		}
		return apiErr
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}
	return common.DecodeJson(limited, out)
}

type adtyResendID struct {
	ID string `json:"id"`
}

// AdtyResendContact 是新建联系人的请求。Properties 的键要先用 CreateContactProperty 建好，否则 Resend 拒绝整个请求。
type AdtyResendContact struct {
	Email        string         `json:"email"`
	FirstName    string         `json:"first_name,omitempty"`
	LastName     string         `json:"last_name,omitempty"`
	Unsubscribed bool           `json:"unsubscribed"`
	Properties   map[string]any `json:"properties,omitempty"`
}

// AdtyResendContactUpdate 是修改联系人的请求，nil 的字段不改。
type AdtyResendContactUpdate struct {
	FirstName    *string        `json:"first_name,omitempty"`
	Unsubscribed *bool          `json:"unsubscribed,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

// CreateContact：POST /contacts，返回联系人 id。
func (c *AdtyResendClient) CreateContact(ctx context.Context, contact AdtyResendContact) (string, error) {
	var out adtyResendID
	err := c.do(ctx, http.MethodPost, "/contacts", contact, "", &out)
	return out.ID, err
}

// UpdateContact：PATCH /contacts/{id 或 email}。
func (c *AdtyResendClient) UpdateContact(ctx context.Context, idOrEmail string, update AdtyResendContactUpdate) error {
	return c.do(ctx, http.MethodPatch, "/contacts/"+url.PathEscape(idOrEmail), update, "", nil)
}

// DeleteContact：DELETE /contacts/{id 或 email}。
func (c *AdtyResendClient) DeleteContact(ctx context.Context, idOrEmail string) error {
	return c.do(ctx, http.MethodDelete, "/contacts/"+url.PathEscape(idOrEmail), nil, "", nil)
}

// CreateContactProperty：POST /contact-properties，type 是 string 或 number；fallback 为空时不带。
func (c *AdtyResendClient) CreateContactProperty(ctx context.Context, key, kind, fallback string) (string, error) {
	body := map[string]any{"key": key, "type": kind}
	if fallback != "" {
		body["fallback_value"] = fallback
	}
	var out adtyResendID
	err := c.do(ctx, http.MethodPost, "/contact-properties", body, "", &out)
	return out.ID, err
}

// CreateSegment：POST /segments，返回 segment id。
func (c *AdtyResendClient) CreateSegment(ctx context.Context, name string) (string, error) {
	var out adtyResendID
	err := c.do(ctx, http.MethodPost, "/segments", map[string]string{"name": name}, "", &out)
	return out.ID, err
}

// DeleteSegment：DELETE /segments/{id}。删 segment 不删联系人。
func (c *AdtyResendClient) DeleteSegment(ctx context.Context, segmentID string) error {
	return c.do(ctx, http.MethodDelete, "/segments/"+url.PathEscape(segmentID), nil, "", nil)
}

// AddContactToSegment：POST /contacts/{contact}/segments/{segment}。
func (c *AdtyResendClient) AddContactToSegment(ctx context.Context, contactIDOrEmail, segmentID string) error {
	return c.do(ctx, http.MethodPost, "/contacts/"+url.PathEscape(contactIDOrEmail)+"/segments/"+url.PathEscape(segmentID), nil, "", nil)
}

// AdtyResendBroadcast 是新建 broadcast 的请求。ScheduledAt 要和 Send=true 一起用（ISO 8601）。
type AdtyResendBroadcast struct {
	SegmentID   string `json:"segment_id"`
	From        string `json:"from"`
	Subject     string `json:"subject"`
	HTML        string `json:"html"`
	Text        string `json:"text,omitempty"`
	ReplyTo     string `json:"reply_to,omitempty"`
	Name        string `json:"name,omitempty"`
	Send        bool   `json:"send,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
}

// CreateBroadcast：POST /broadcasts，返回 broadcast id。
func (c *AdtyResendClient) CreateBroadcast(ctx context.Context, broadcast AdtyResendBroadcast) (string, error) {
	var out adtyResendID
	err := c.do(ctx, http.MethodPost, "/broadcasts", broadcast, "", &out)
	return out.ID, err
}

// SendBroadcast：POST /broadcasts/{id}/send，scheduledAt 为空时立即发。
func (c *AdtyResendClient) SendBroadcast(ctx context.Context, broadcastID, scheduledAt string) error {
	body := map[string]string{}
	if scheduledAt != "" {
		body["scheduled_at"] = scheduledAt
	}
	return c.do(ctx, http.MethodPost, "/broadcasts/"+url.PathEscape(broadcastID)+"/send", body, "", nil)
}

// CancelBroadcast：POST /broadcasts/{id}/cancel，取消排队或定时的 broadcast。
// 已对照官方文档：无请求体；scheduled 取消后回到 draft，queued 取消后变 canceled，已发出的不撤回。
func (c *AdtyResendClient) CancelBroadcast(ctx context.Context, broadcastID string) error {
	return c.do(ctx, http.MethodPost, "/broadcasts/"+url.PathEscape(broadcastID)+"/cancel", nil, "", nil)
}

// AdtyResendBroadcastInfo 是 GET /broadcasts/{id} 里用到的字段。
type AdtyResendBroadcastInfo struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ScheduledAt string `json:"scheduled_at"`
	SentAt      string `json:"sent_at"`
}

// GetBroadcast：GET /broadcasts/{id}。
func (c *AdtyResendClient) GetBroadcast(ctx context.Context, broadcastID string) (AdtyResendBroadcastInfo, error) {
	var out AdtyResendBroadcastInfo
	err := c.do(ctx, http.MethodGet, "/broadcasts/"+url.PathEscape(broadcastID), nil, "", &out)
	return out, err
}

// AdtyResendTag 是邮件标签，webhook 事件里会原样带回。
type AdtyResendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AdtyResendEmail 是逐封发送的请求（POST /emails）。Headers 用来带 List-Unsubscribe。
type AdtyResendEmail struct {
	From           string            `json:"from"`
	To             []string          `json:"to"`
	Subject        string            `json:"subject"`
	HTML           string            `json:"html"`
	Text           string            `json:"text,omitempty"`
	ReplyTo        string            `json:"reply_to,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Tags           []AdtyResendTag   `json:"tags,omitempty"`
	IdempotencyKey string            `json:"-"` // 放进 Idempotency-Key 头，24 小时内同一个键只发一次
}

// SendEmail：POST /emails，返回邮件 id。
func (c *AdtyResendClient) SendEmail(ctx context.Context, email AdtyResendEmail) (string, error) {
	var out adtyResendID
	err := c.do(ctx, http.MethodPost, "/emails", email, email.IdempotencyKey, &out)
	return out.ID, err
}

// AdtyResendDomainCreate 是新建发信域名的请求（11.3）。
type AdtyResendDomainCreate struct {
	Name              string `json:"name"`
	Region            string `json:"region,omitempty"`
	OpenTracking      *bool  `json:"open_tracking,omitempty"`
	ClickTracking     *bool  `json:"click_tracking,omitempty"`
	TrackingSubdomain string `json:"tracking_subdomain,omitempty"`
}

// AdtyResendDomainRecord 是要加到 DNS 的一条记录。
type AdtyResendDomainRecord struct {
	Record   string `json:"record"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      string `json:"ttl"`
	Status   string `json:"status"`
	Priority int    `json:"priority,omitempty"`
}

// AdtyResendDomain 是域名详情。Status 是 not_started、pending、verified、failed 等。
type AdtyResendDomain struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Status        string                   `json:"status"`
	Region        string                   `json:"region"`
	OpenTracking  bool                     `json:"open_tracking"`
	ClickTracking bool                     `json:"click_tracking"`
	Records       []AdtyResendDomainRecord `json:"records"`
}

// CreateDomain：POST /domains，返回域名和要加的 DNS 记录。
func (c *AdtyResendClient) CreateDomain(ctx context.Context, domain AdtyResendDomainCreate) (AdtyResendDomain, error) {
	var out AdtyResendDomain
	err := c.do(ctx, http.MethodPost, "/domains", domain, "", &out)
	return out, err
}

// GetDomain：GET /domains/{id}。
func (c *AdtyResendClient) GetDomain(ctx context.Context, domainID string) (AdtyResendDomain, error) {
	var out AdtyResendDomain
	err := c.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(domainID), nil, "", &out)
	return out, err
}

// DeleteDomain：DELETE /domains/{id}。
func (c *AdtyResendClient) DeleteDomain(ctx context.Context, domainID string) error {
	return c.do(ctx, http.MethodDelete, "/domains/"+url.PathEscape(domainID), nil, "", nil)
}

// adtySvixTolerance 是 webhook 时间戳允许的偏差，超出的当成重放拒绝。
const adtySvixTolerance = 5 * time.Minute

var errAdtySvixInvalid = errors.New("invalid webhook signature")

// AdtyVerifySvixSignature 校验 Resend webhook（Svix）的签名：
// 签名内容是「svix-id.svix-timestamp.原始 body」，密钥去掉 whsec_ 前缀后 base64 解码作 HMAC-SHA256 的 key，
// 结果 base64；svix-signature 可能有多个（空格分开，每个 v1,<sig>），任意一个常量时间比对相等即通过。
// 时间戳和 now 相差超过 5 分钟拒绝。body 必须是收到的原始字节，不能重新序列化。
func AdtyVerifySvixSignature(secret string, headers http.Header, body []byte, now time.Time) error {
	id := headers.Get("svix-id")
	timestamp := headers.Get("svix-timestamp")
	signatures := headers.Get("svix-signature")
	if id == "" || timestamp == "" || signatures == "" {
		return errAdtySvixInvalid
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(secret), "whsec_"))
	if err != nil || len(key) == 0 {
		return errAdtySvixInvalid
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errAdtySvixInvalid
	}
	if skew := now.Sub(time.Unix(seconds, 0)); skew > adtySvixTolerance || skew < -adtySvixTolerance {
		return errAdtySvixInvalid
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	expected := mac.Sum(nil)
	for candidate := range strings.FieldsSeq(signatures) {
		version, encoded, ok := strings.Cut(candidate, ",")
		if !ok || version != "v1" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && hmac.Equal(decoded, expected) {
			return nil
		}
	}
	return errAdtySvixInvalid
}
