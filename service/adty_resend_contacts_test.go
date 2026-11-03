package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 假的 Resend：记下每个请求，POST 新建的东西按顺序给 id。
type marketingFakeResend struct {
	mu    sync.Mutex
	calls []resendCall
	next  int
	fail  func(r *http.Request) (int, string) // 返回非 0 时用它作响应
}

func (f *marketingFakeResend) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, resendCall{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(body), r.Header.Get("Idempotency-Key")})
	f.next++
	id := fmt.Sprintf("id-%d", f.next)
	fail := f.fail
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if fail != nil {
		if status, response := fail(r); status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, response)
			return
		}
	}
	_, _ = io.WriteString(w, `{"object":"x","id":"`+id+`"}`)
}

func (f *marketingFakeResend) callsMatching(method, prefix string) []resendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []resendCall
	for _, call := range f.calls {
		if call.Method == method && strings.HasPrefix(call.Path, prefix) {
			out = append(out, call)
		}
	}
	return out
}

func (f *marketingFakeResend) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// prepareMarketingDB 建库、换上假 Resend、设好营销设置，结束时全部恢复。
func prepareMarketingDB(t *testing.T) (*gorm.DB, *marketingFakeResend) {
	t.Helper()
	db := adtyTestDB(t)
	oldRedis, oldSecret := common.RedisEnabled, common.SessionSecret
	common.RedisEnabled, common.SessionSecret = false, "marketing-test-secret"
	saved := *adty_setting.GetMarketingSetting()
	oldClient, oldSleep := AdtyResend, adtyMarketingSleep
	fake := &marketingFakeResend{}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	AdtyResend = &AdtyResendClient{BaseURL: server.URL, HTTP: server.Client(), APIKey: func() string { return "re_test" }}
	adtyMarketingSleep = func(time.Duration) {}
	adtyResendPropertiesReady.Store(false)
	t.Cleanup(func() {
		server.Close()
		common.RedisEnabled, common.SessionSecret = oldRedis, oldSecret
		*adty_setting.GetMarketingSetting() = saved
		AdtyResend, adtyMarketingSleep = oldClient, oldSleep
		adtyResendPropertiesReady.Store(false)
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.AuditLog{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTables())
	cfg := adty_setting.GetMarketingSetting()
	cfg.Enabled = true
	cfg.FromAddress = "ADTY AI <news@news.example.com>"
	cfg.FooterCompany, cfg.FooterAddress = "ADTY Ltd", "1 Example Road"
	cfg.WebhookSecret = svixTestSecret
	cfg.SyncBatchSize = 100
	cfg.APIRatePerSecond = 1000
	return db, fake
}

func marketingUser(t *testing.T, db *gorm.DB, name string, consent bool) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com", Quota: 5 * 500000}
	require.NoError(t, db.Create(user).Error)
	_, err := model.SaveAdtyMarketingConsent(model.AdtyMarketingConsentInput{UserId: user.Id, Email: user.Email, Consent: consent, Lang: "en", Version: "1", Source: "register"})
	require.NoError(t, err)
	return user
}

func marketingContact(t *testing.T, userId int) model.AdtyMarketingContact {
	t.Helper()
	contact, err := model.GetAdtyMarketingContact(userId)
	require.NoError(t, err)
	require.NotNil(t, contact)
	return *contact
}

// signedWebhook 按 Svix 的规则给 body 签名。
func signedWebhook(t *testing.T, id string, now time.Time, body string) http.Header {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(svixTestSecret, "whsec_"))
	require.NoError(t, err)
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + body))
	return svixHeaders(id, ts, "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func TestConsentDefaultByRegion(t *testing.T) {
	saved := *adty_setting.GetMarketingSetting()
	t.Cleanup(func() { *adty_setting.GetMarketingSetting() = saved })
	cfg := adty_setting.GetMarketingSetting()
	for _, nonEU := range []bool{false, true} {
		cfg.ConsentDefaultNonEU = nonEU
		for _, country := range []string{"", "XX", "T1", "DE", "fr", " GB "} {
			assert.False(t, AdtyConsentDefault(country), "country=%q nonEU=%v", country, nonEU)
		}
		assert.Equal(t, nonEU, AdtyConsentDefault("US"), "US nonEU=%v", nonEU)
		assert.Equal(t, nonEU, AdtyConsentDefault("jp"), "JP nonEU=%v", nonEU)
	}
}

func TestOnlyConsentedContactsSynced(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	yes := marketingUser(t, db, "mk-yes", true)
	marketingUser(t, db, "mk-no", false)
	bounced := marketingUser(t, db, "mk-bounced", true)
	_, err := model.MarkAdtyMarketingByEmail(bounced.Email, model.AdtyMarketingFlagBounced)
	require.NoError(t, err)

	synced, err := SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, synced)
	created := fake.callsMatching(http.MethodPost, "/contacts")
	require.Len(t, created, 1)
	var body map[string]any
	require.NoError(t, common.Unmarshal([]byte(created[0].Body), &body))
	assert.Equal(t, "mk-yes@example.com", body["email"])
	assert.Equal(t, false, body["unsubscribed"])
	props, _ := body["properties"].(map[string]any)
	assert.Equal(t, "mk-yes", props["username"])
	assert.Equal(t, "$5.00", props["balance"])
	assert.NotEmpty(t, fake.callsMatching(http.MethodPost, "/contact-properties"), "contact properties must exist before contacts use them")

	contact := marketingContact(t, yes.Id)
	assert.NotEmpty(t, contact.ResendContactId)
	assert.GreaterOrEqual(t, contact.SyncedAt, contact.UpdatedAt)

	// 没有变化时第二轮什么都不发。
	fake.reset()
	synced, err = SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, synced)
	assert.Empty(t, fake.callsMatching(http.MethodPost, "/contacts"))

	// 营销总开关关着时不同步。
	adty_setting.GetMarketingSetting().Enabled = false
	marketingUser(t, db, "mk-later", true)
	synced, err = SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, synced)
}

func TestContactSyncFailureDoesNotBlockTheRest(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	bad := marketingUser(t, db, "mk-bad", true)
	good := marketingUser(t, db, "mk-good", true)
	fake.fail = func(r *http.Request) (int, string) {
		if r.Method != http.MethodPost || r.URL.Path != "/contacts" {
			return 0, ""
		}
		fake.mu.Lock()
		body := ""
		if n := len(fake.calls); n > 0 {
			body = fake.calls[n-1].Body
		}
		fake.mu.Unlock()
		if strings.Contains(body, "mk-bad@example.com") {
			return http.StatusUnprocessableEntity, `{"name":"validation_error","message":"rejected"}`
		}
		return 0, ""
	}

	synced, err := SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, synced)
	assert.Empty(t, marketingContact(t, bad.Id).ResendContactId)
	assert.NotEmpty(t, marketingContact(t, good.Id).ResendContactId)

	fake.fail = func(r *http.Request) (int, string) {
		if r.Method == http.MethodPost && r.URL.Path == "/contacts" {
			return http.StatusConflict, `{"name":"conflict","message":"already exists"}`
		}
		return 0, ""
	}
	synced, err = SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, synced, 1)
	assert.NotEmpty(t, fake.callsMatching(http.MethodPatch, "/contacts/"))
	assert.Equal(t, bad.Email, marketingContact(t, bad.Id).ResendContactId)
	_ = good
}

func TestWebhookUnsubscribeStopsSync(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	user := marketingUser(t, db, "mk-unsub", true)
	_, err := SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	contactID := marketingContact(t, user.Id).ResendContactId
	require.NotEmpty(t, contactID)

	now := time.Now()
	body := `{"type":"contact.updated","created_at":"2026-11-01T00:00:00.000Z","data":{"id":"` + contactID + `","email":"MK-UNSUB@example.com","unsubscribed":true}}`
	require.NoError(t, HandleAdtyResendWebhook(signedWebhook(t, "msg_unsub_1", now, body), []byte(body), now))
	contact := marketingContact(t, user.Id)
	assert.True(t, contact.Unsubscribed)
	assert.False(t, contact.Sendable())

	// 同一个 svix-id 重发时不再处理。
	require.NoError(t, HandleAdtyResendWebhook(signedWebhook(t, "msg_unsub_1", now, body), []byte(body), now))

	fake.reset()
	_, err = SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	deleted := fake.callsMatching(http.MethodDelete, "/contacts/")
	require.Len(t, deleted, 1)
	assert.Equal(t, "/contacts/"+contactID, deleted[0].Path)
	assert.Empty(t, fake.callsMatching(http.MethodPost, "/contacts"))
	assert.Empty(t, marketingContact(t, user.Id).ResendContactId)

	// 用户在个人设置里重新勾选同意，可以再收（Resend 上重新建联系人）。
	_, err = AdtySetMarketingConsent(user.Id, true, "", "en", "profile")
	require.NoError(t, err)
	assert.True(t, marketingContact(t, user.Id).Sendable())
}

func TestBounceAndComplaintBlacklist(t *testing.T) {
	db, fake := prepareMarketingDB(t)
	bounced := marketingUser(t, db, "mk-bounce", true)
	complained := marketingUser(t, db, "mk-complain", true)
	_, err := SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)

	now := time.Now()
	bounce := `{"type":"email.bounced","created_at":"2026-11-01T00:00:00.000Z","data":{"email_id":"e1","broadcast_id":"b1","from":"news@news.example.com","to":["mk-bounce@example.com"],"bounce":{"type":"Permanent","subType":"General","message":"no such user"}}}`
	complaint := `{"type":"email.complained","created_at":"2026-11-01T00:00:00.000Z","data":{"email_id":"e2","from":"news@news.example.com","to":["mk-complain@example.com"]}}`
	require.NoError(t, HandleAdtyResendWebhook(signedWebhook(t, "msg_bounce", now, bounce), []byte(bounce), now))
	require.NoError(t, HandleAdtyResendWebhook(signedWebhook(t, "msg_complain", now, complaint), []byte(complaint), now))

	assert.True(t, marketingContact(t, bounced.Id).Bounced)
	assert.True(t, marketingContact(t, complained.Id).Complained)

	// 个人设置里再勾同意也不解除退信和投诉。
	for _, user := range []*model.User{bounced, complained} {
		_, err := AdtySetMarketingConsent(user.Id, true, "", "en", "profile")
		require.NoError(t, err)
		contact := marketingContact(t, user.Id)
		assert.True(t, contact.Consent)
		assert.False(t, contact.Sendable(), user.Username)
	}

	fake.reset()
	_, err = SyncAdtyMarketingContactsOnce(context.Background())
	require.NoError(t, err)
	assert.Len(t, fake.callsMatching(http.MethodDelete, "/contacts/"), 2)
	assert.Empty(t, fake.callsMatching(http.MethodPost, "/contacts"))
}

func TestWebhookRejectsBadSignature(t *testing.T) {
	db, _ := prepareMarketingDB(t)
	user := marketingUser(t, db, "mk-sig", true)
	now := time.Now()
	body := `{"type":"email.complained","created_at":"2026-11-01T00:00:00.000Z","data":{"to":["mk-sig@example.com"]}}`

	headers := signedWebhook(t, "msg_sig", now, body)
	tampered := strings.Replace(body, "complained", "bounced", 1)
	require.ErrorIs(t, HandleAdtyResendWebhook(headers, []byte(tampered), now), ErrAdtyMarketingWebhookSignature)

	stale := signedWebhook(t, "msg_sig_old", now.Add(-10*time.Minute), body)
	require.ErrorIs(t, HandleAdtyResendWebhook(stale, []byte(body), now), ErrAdtyMarketingWebhookSignature)

	adty_setting.GetMarketingSetting().WebhookSecret = ""
	require.ErrorIs(t, HandleAdtyResendWebhook(headers, []byte(body), now), ErrAdtyMarketingWebhookSignature)

	contact := marketingContact(t, user.Id)
	assert.False(t, contact.Complained)
	assert.False(t, contact.Bounced)
}
