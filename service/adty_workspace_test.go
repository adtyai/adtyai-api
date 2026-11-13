package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyTeamPageMembersKeysAndFixedLeader(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-page-leader")
	member := referralUser(t, db, "team-page-member")
	created, err := CreateAdtyTeam(leader.Id, 0, "Desk", decimal.RequireFromString("1"))
	require.NoError(t, err)
	code, err := CreateAdtyTeamInvite(leader.Id, created.Team.Id, time.Hour)
	require.NoError(t, err)
	_, err = AcceptAdtyTeamInvite(member.Id, code)
	require.NoError(t, err)
	token := model.Token{UserId: member.Id, Name: "member-key", Key: "team-page-secret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, BindAdtyTeamKey(member.Id, token.Id))
	require.NoError(t, CreditAdtyTeam(int64(leader.Id), created.Team.Id, 800000, "page-credit"))

	page, err := GetAdtyTeamMine(leader.Id)
	require.NoError(t, err)
	require.Len(t, page.Members, 2)
	require.Len(t, page.Keys, 1)
	assert.Equal(t, "member-key", page.Keys[0].Name)
	assert.EqualValues(t, 800000, page.SharedQuota)

	memberPage, err := GetAdtyTeamMine(member.Id)
	require.NoError(t, err)
	require.Len(t, memberPage.Members, 1)
	assert.Equal(t, member.Id, memberPage.Members[0].UserId)

	require.ErrorIs(t, TransferAdtyTeamLeader(leader.Id, created.Team.Id, member.Id), ErrAdtyTeamLeaderFixed)
	require.ErrorIs(t, LeaveAdtyTeam(leader.Id, created.Team.Id), ErrAdtyTeamLeaderFixed)
	require.NoError(t, DisableAdtyTeamMember(leader.Id, created.Team.Id, member.Id))
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusDisabled, stored.Status)
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	assert.Equal(t, leader.Id, team.LeaderUserId)
	assert.EqualValues(t, 800000, team.SharedQuota)
}

func TestAdtyTelegramBindIsNotifyOnlyAndDoesNotDial(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	t.Setenv("ADTY_TELEGRAM_ALLOW_NETWORK", "")
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("telegram test dialed %s", request.URL.String())
		return nil, errors.New("dial")
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	oldGet := AdtyTelegramGetMe
	AdtyTelegramGetMe = func(string) (int64, string, error) { return 0, "", ErrAdtyTelegramUnverified }
	t.Cleanup(func() { AdtyTelegramGetMe = oldGet })

	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "tg-user")
	other := referralUser(t, db, "tg-other")
	token := "424242:ABCDEFGHIJKLMNOPQRSTUV"
	saved, err := SaveAdtyTelegramBot(model.AdtyTelegramPurposeUser, token, true)
	require.NoError(t, err)
	assert.NotContains(t, saved.View.TokenMask, "ABCDEF")
	assert.NotEmpty(t, saved.WebhookSecret)
	assert.False(t, saved.View.IdentityVerified)

	code, _, _, err := StartAdtyTelegramBind(user.Id, model.AdtyTelegramPurposeUser)
	require.NoError(t, err)
	reply, err := HandleAdtyTelegramUpdate(model.AdtyTelegramPurposeUser, saved.WebhookSecret, telegramUpdate("/balance please-steal", 77, false))
	require.NoError(t, err)
	assert.Contains(t, reply, "此机器人仅用于通知")
	var leaked int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM adty_telegram_deliveries WHERE body LIKE ?", "%please-steal%").Scan(&leaked).Error)
	assert.Zero(t, leaked)

	_, err = HandleAdtyTelegramUpdate(model.AdtyTelegramPurposeUser, saved.WebhookSecret, telegramUpdate("/start "+code, 88, true))
	require.NoError(t, err)
	var forwarded int64
	require.NoError(t, db.Model(&model.AdtyTelegramChallenge{}).Where("telegram_user_id = ?", 88).Count(&forwarded).Error)
	assert.Zero(t, forwarded)

	reply, err = HandleAdtyTelegramUpdate(model.AdtyTelegramPurposeUser, saved.WebhookSecret, telegramUpdate("/start "+code, 77, false))
	require.NoError(t, err)
	assert.Contains(t, reply, "请回到网站确认")
	_, err = HandleAdtyTelegramUpdate(model.AdtyTelegramPurposeUser, saved.WebhookSecret, telegramUpdate("/start "+code, 99, false))
	require.NoError(t, err)
	var staged model.AdtyTelegramChallenge
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("id DESC").First(&staged).Error)
	assert.EqualValues(t, 77, staged.TelegramUserId)

	_, err = ConfirmAdtyTelegramBind(other.Id, model.AdtyTelegramPurposeUser, code, 77)
	require.ErrorIs(t, err, ErrAdtyTelegramForbidden)
	require.NoError(t, db.Model(user).Update("site_id", 9).Error)
	_, err = ConfirmAdtyTelegramBind(user.Id, model.AdtyTelegramPurposeUser, code, 77)
	require.ErrorIs(t, err, ErrAdtyTelegramForbidden)
	require.NoError(t, db.Model(user).Update("site_id", 0).Error)
	binding, err := ConfirmAdtyTelegramBind(user.Id, model.AdtyTelegramPurposeUser, code, 77)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTelegramBindActive, binding.Status)
	_, err = ConfirmAdtyTelegramBind(user.Id, model.AdtyTelegramPurposeUser, code, 77)
	require.Error(t, err)

	require.NoError(t, QueueAdtyTelegramNotice(user.Id, model.AdtyTelegramPurposeUser, "余额已入账\n\nBalance credited", "notice-1"))
	require.Error(t, QueueAdtyTelegramNotice(user.Id, model.AdtyTelegramPurposeUser, "sk-secretvalueplease", "notice-secret"))
	require.Error(t, QueueAdtyPublicTelegram("channel", "订单 123 已退款", true))
	var count int64
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).Where("event_key = ?", "notice-1").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	_, err = RetryAdtyTelegram()
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).Where("event_key = ?", "notice-1").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestAdtyTicketNotesImagesAndRemovedStaff(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	restore := SetAdtyTicketImageDirForTest(t.TempDir())
	t.Cleanup(restore)
	customer := referralUser(t, db, "ticket-customer")
	owner := referralUser(t, db, "ticket-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "ticket-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Updates(map[string]any{"site_id": site.Id, "quota": 500000}).Error)
	staffUser := referralUser(t, db, "ticket-staff")
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staffUser.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)

	ticket, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Charge", Body: "api_key=sk-livesecretvalue please", RelatedRef: "T100001", Idempotency: "same"})
	require.NoError(t, err)
	again, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Charge", Body: "again", Idempotency: "same"})
	require.NoError(t, err)
	assert.Equal(t, ticket.Id, again.Id)
	var stored model.AdtyTicketMessage
	require.NoError(t, db.Where("ticket_id = ?", ticket.Id).First(&stored).Error)
	assert.NotContains(t, stored.Body, "sk-livesecretvalue")

	_, err = NoteAdtyTicket(staffUser.Id, ticket.Id, "internal only", "note-1")
	require.NoError(t, err)
	customerView, err := GetAdtyTicket(customer.Id, ticket.Id)
	require.NoError(t, err)
	assert.False(t, customerView.CanNote)
	for _, message := range customerView.Messages {
		assert.NotEqual(t, model.AdtyTicketVisibilityNote, message.Visibility)
		assert.NotContains(t, message.Body, "internal only")
	}
	ownerView, err := GetAdtyTicket(owner.Id, ticket.Id)
	require.NoError(t, err)
	assert.True(t, ownerView.CanNote)
	require.GreaterOrEqual(t, len(ownerView.Messages), 2)
	assert.Contains(t, ownerView.Messages[1].Body, "internal only")

	imageRow, err := AddAdtyTicketImage(customer.Id, ticket.Id, jpegWithMarker(t))
	require.NoError(t, err)
	_, _, err = OpenAdtyTicketImage(staffUser.Id, ticket.Id, imageRow.Id)
	require.NoError(t, err)
	_, err = AddAdtyTicketImage(customer.Id, ticket.Id, []byte("%PDF-1.4"))
	require.ErrorIs(t, err, ErrAdtyTicketImage)
	_, err = AddAdtyTicketImage(customer.Id, ticket.Id, tinyPNG(t))
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdtySiteStaff{}).Where("user_id = ?", staffUser.Id).Update("status", "deleted").Error)
	_, _, err = OpenAdtyTicketImage(staffUser.Id, ticket.Id, imageRow.Id)
	require.ErrorIs(t, err, ErrAdtyTicketForbidden)
	_, _, err = OpenAdtyTicketImage(customer.Id, ticket.Id, imageRow.Id)
	require.NoError(t, err)
	var quota int
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Select("quota").Scan(&quota).Error)
	assert.Equal(t, 500000, quota)
}

func TestAdtyTranslationReuseVersionAndNoDebit(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "tr-user")
	require.NoError(t, db.Model(user).Update("quota", 500000).Error)
	calls := 0
	old := AdtyTranslateFunc
	AdtyTranslateFunc = func(_, text string) (string, error) {
		calls++
		if strings.Contains(text, "失败") {
			return "missing facts", nil
		}
		return "Amount 12.50 USD order A123456 is not ready", nil
	}
	t.Cleanup(func() { AdtyTranslateFunc = old })

	_, err := SaveAdtyTranslationConfig(AdtyTranslationInput{Mode: model.AdtyTranslationModeOwn, BaseURL: "https://api.adty.test/v1/chat/completions", ModelName: "translator", RequestHost: "api.adty.test"})
	require.ErrorIs(t, err, ErrAdtyTranslationSelf)
	token := model.Token{UserId: user.Id, Key: "customer-key-secret", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	_, err = SaveAdtyTranslationConfig(AdtyTranslationInput{Mode: model.AdtyTranslationModeOwn, BaseURL: "https://translator.example/translate", ModelName: "translator", Token: token.Key, RequestHost: "api.adty.test"})
	require.ErrorIs(t, err, ErrAdtyTranslationSelf)
	saved, err := SaveAdtyTranslationConfig(AdtyTranslationInput{Mode: model.AdtyTranslationModeSet, ModelName: "translator", Enabled: true, DailyQuota: 10, RetryLimit: 1})
	require.NoError(t, err)
	assert.NotZero(t, saved.SavedAt)
	assert.Zero(t, saved.TestedAt)

	source := "金额 12.50 USD，订单 A123456，不退款"
	first, err := TranslateAdtyDynamic("notice:1", source, 0)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTranslationReady, first.Status)
	assert.Equal(t, 1, calls)
	second, err := TranslateAdtyDynamic("notice:1", source, 0)
	require.NoError(t, err)
	assert.True(t, second.Reused)
	assert.Equal(t, 1, calls)

	require.NoError(t, EditAdtyTranslation("notice:1", "Human amount 12.50 USD order A123456 is not changed"))
	require.Error(t, CompleteAdtyTranslationJob("notice:1", 1, "late amount 12.50 USD order A123456 is not applied"))
	var head model.AdtyTranslationHead
	require.NoError(t, db.Where("object_key = ?", "notice:1").First(&head).Error)
	assert.Contains(t, head.English, "Human")
	assert.True(t, head.HumanEdited)

	failed, err := TranslateAdtyDynamic("notice:2", "失败 12.50 USD 订单 A123456 不退", 0)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyTranslationFailed, failed.Status)
	assert.Equal(t, AdtyTranslationNotReady, failed.Notice)
	assert.Empty(t, failed.English)
	quota, err := AdtyTranslationCallsDoNotDebit(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 500000, quota)
}

func TestAdtyGroupHealthMarksExpiredSamplesStale(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	require.NoError(t, PutAdtyGroupHealthConfig("0.95", 3600))
	sampleAt := now.Add(-2 * time.Hour)
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthSuccess, sampleAt))
	require.NoError(t, db.Model(&model.AdtyGroupHealthState{}).Where("id = ?", 1).Update("heartbeat_at", now.Unix()).Error)

	rows, err := listAdtyGroupHealth(0, "", now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.AdtyGroupHealthStale, rows[0].Status)
	assert.Equal(t, sampleAt.Unix(), rows[0].UpdatedAt)
	assert.Equal(t, 1, rows[0].RequestSamples)
}

func TestAdtyGroupHealthSeriesUses96FifteenMinuteWindows(t *testing.T) {
	prepareReferralDialectDB(t, "sqlite")
	now := time.Date(2026, 11, 13, 12, 7, 0, 0, time.UTC)
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthSuccess, now))
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthUpstream, now.Add(-15*time.Minute)))

	rows, err := listAdtyGroupHealth(0, "", now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].Series, 96)
	starts := AdtyHealthWindowStarts(now)
	assert.Equal(t, starts[0], rows[0].Series[0].Ts)
	assert.Equal(t, starts[95], rows[0].Series[95].Ts)
	assert.Equal(t, 1, rows[0].Series[95].Success)
	assert.Equal(t, 1, rows[0].Series[94].Fail)
	assert.Equal(t, 2, rows[0].RequestSamples)
}

func TestAdtyGroupHealthKeepsProbeAndCustomerRefusalApart(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	user := referralUser(t, db, "health-user")
	token := model.Token{UserId: user.Id, Key: "health-key", Status: common.TokenStatusEnabled, Group: "vip"}
	require.NoError(t, db.Create(&token).Error)
	now := time.Now()
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthSuccess, now))
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthCustomer, now))
	require.NoError(t, RecordAdtyGroupHealth(0, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthUpstream, now))
	require.NoError(t, RecordAdtyGroupProbe(0, "vip", "gpt", true, now))
	require.NoError(t, RecordAdtyGroupHealth(7, "vip", "gpt", model.AdtyGroupHealthRequest, model.AdtyGroupHealthSuccess, now))

	rows, err := listAdtyGroupHealth(0, "gpt", now)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 2, rows[0].RequestSamples)
	require.NotNil(t, rows[0].RequestSuccessRate)
	assert.InDelta(t, 0.5, *rows[0].RequestSuccessRate, 0.001)
	require.NotNil(t, rows[0].ProbeRate)
	assert.InDelta(t, 1, *rows[0].ProbeRate, 0.001)
	assert.Equal(t, "platform", rows[0].Scope)

	siteRows, err := listAdtyGroupHealth(7, "gpt", now)
	require.NoError(t, err)
	require.Len(t, siteRows, 1)
	assert.Equal(t, "site", siteRows[0].Scope)
	assert.Equal(t, 1, siteRows[0].RequestSamples)

	empty, err := ListAdtyGroupHealthForUser(user.Id, "", "gpt", []string{"fresh"}, now)
	require.NoError(t, err)
	var fresh *AdtyGroupHealthView
	for i := range empty {
		if empty[i].Group == "fresh" {
			fresh = &empty[i]
		}
	}
	require.NotNil(t, fresh)
	assert.Equal(t, model.AdtyGroupHealthNoData, fresh.Status)
	assert.Nil(t, fresh.RequestSuccessRate)

	require.NoError(t, db.Model(&model.AdtyGroupHealthState{}).Where("id = ?", 1).Update("heartbeat_at", now.Add(-7*time.Hour).Unix()).Error)
	stale, err := listAdtyGroupHealth(0, "gpt", now)
	require.NoError(t, err)
	assert.Equal(t, model.AdtyGroupHealthInterrupted, stale[0].Status)
	assert.Nil(t, stale[0].RequestSuccessRate)

	group, err := AdtyGroupHealthDoesNotReroute(token.Id)
	require.NoError(t, err)
	assert.Equal(t, "vip", group)
	outcome := perfmetrics.ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{}, types.NewError(errors.New("quota"), types.ErrorCodeInsufficientUserQuota))
	kind, record := adtyGroupHealthFromOutcome(string(outcome))
	assert.False(t, record)
	assert.NotEqual(t, model.AdtyGroupHealthUpstream, kind)
}

func telegramUpdate(text string, userID int64, forwarded bool) AdtyTelegramUpdate {
	message := &AdtyTelegramMessage{Text: text}
	message.From.Id = userID
	message.From.Username = "alice"
	message.Chat.Id = userID
	message.Chat.Type = "private"
	if forwarded {
		message.ForwardFrom = map[string]int64{"id": 1}
	}
	return AdtyTelegramUpdate{Message: message}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return buf.Bytes()
}

func jpegWithMarker(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), &jpeg.Options{Quality: 80}))
	raw := buf.Bytes()
	marker := []byte{0xFF, 0xE1, 0x00, 0x10, 'A', 'D', 'T', 'Y', 'E', 'X', 'I', 'F', 'M', 'A', 'R', 'K'}
	out := append([]byte{}, raw[:2]...)
	out = append(out, marker...)
	out = append(out, raw[2:]...)
	require.Contains(t, string(out), "ADTYEXIFMARK")
	stripped, _, _, _, err := sanitizeAdtyTicketImage(out, adtyTicketPolicy())
	require.NoError(t, err)
	assert.NotContains(t, string(stripped), "ADTYEXIFMARK")
	return stripped
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
