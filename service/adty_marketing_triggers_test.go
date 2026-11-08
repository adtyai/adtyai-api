package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// prepareTriggers 在营销测试库上打开三条规则，写好英文模板。
func prepareTriggers(t *testing.T) (*gorm.DB, *marketingFakeResend) {
	t.Helper()
	db, fake := prepareMarketingDB(t)
	oldRemind, oldThreshold := common.QuotaRemindEnabled, common.QuotaRemindThreshold
	t.Cleanup(func() { common.QuotaRemindEnabled, common.QuotaRemindThreshold = oldRemind, oldThreshold })
	common.QuotaRemindEnabled = false
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	cfg := adty_setting.GetMarketingSetting()
	cfg.TriggerUnpaidEnabled, cfg.TriggerUnpaidDays = true, 3
	cfg.TriggerUnpaidSubjects, cfg.TriggerUnpaidBodies = map[string]string{"en": "Need help?"}, map[string]string{"en": "<p>Hi {{username}}</p>"}
	cfg.TriggerLowBalanceEnabled, cfg.TriggerLowBalanceUSD = true, "1"
	cfg.TriggerLowBalanceSubjects, cfg.TriggerLowBalanceBodies = map[string]string{"en": "Balance low"}, map[string]string{"en": "<p>Balance {{balance}}</p>"}
	cfg.TriggerWinbackEnabled, cfg.TriggerWinbackDays = true, 30
	cfg.TriggerWinbackSubjects, cfg.TriggerWinbackBodies = map[string]string{"en": "We miss you"}, map[string]string{"en": "<p>Come back</p>"}
	cfg.TriggerBatchSize = 100
	return db, fake
}

func consumeLog(t *testing.T, db *gorm.DB, userId int, at time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&model.Log{UserId: userId, Type: model.LogTypeConsume, CreatedAt: at.Unix(), Quota: 10}).Error)
}

func triggerTopUp(t *testing.T, db *gorm.DB, userId int, tradeNo string) {
	t.Helper()
	require.NoError(t, db.Create(&model.TopUp{UserId: userId, TradeNo: tradeNo, Status: common.TopUpStatusSuccess}).Error)
}

func backdateUser(t *testing.T, db *gorm.DB, userId int, at time.Time) {
	t.Helper()
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", userId).Update("created_at", at.Unix()).Error)
}

func emailsTo(fake *marketingFakeResend) []string {
	var to []string
	for _, call := range fake.callsMatching(http.MethodPost, "/emails") {
		var body struct {
			To []string `json:"to"`
		}
		_ = common.UnmarshalJsonStr(call.Body, &body)
		to = append(to, body.To...)
	}
	return to
}

func TestTriggerSendsOncePerUser(t *testing.T) {
	db, fake := prepareTriggers(t)
	now := time.Now()
	unpaid := marketingUser(t, db, "trg-unpaid", true)
	backdateUser(t, db, unpaid.Id, now.Add(-4*24*time.Hour))
	consumeLog(t, db, unpaid.Id, now.Add(-time.Hour))
	tooNew := marketingUser(t, db, "trg-new", true)
	consumeLog(t, db, tooNew.Id, now.Add(-time.Hour))
	payer := marketingUser(t, db, "trg-payer", true)
	backdateUser(t, db, payer.Id, now.Add(-4*24*time.Hour))
	consumeLog(t, db, payer.Id, now.Add(-time.Hour))
	triggerTopUp(t, db, payer.Id, "trg-p1")

	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, []string{"trg-unpaid@example.com"}, emailsTo(fake))
	call := fake.callsMatching(http.MethodPost, "/emails")[0]
	var body struct {
		From    string            `json:"from"`
		Subject string            `json:"subject"`
		HTML    string            `json:"html"`
		Headers map[string]string `json:"headers"`
	}
	require.NoError(t, common.UnmarshalJsonStr(call.Body, &body))
	assert.Equal(t, "ADTY AI <news@news.example.com>", body.From)
	assert.Equal(t, "Need help?", body.Subject)
	assert.Contains(t, body.HTML, "Hi trg-unpaid")
	assert.Contains(t, body.HTML, "1 Example Road")
	assert.Contains(t, body.Headers["List-Unsubscribe"], "/api/adty/marketing/unsubscribe?token=")
	assert.Equal(t, "List-Unsubscribe=One-Click", body.Headers["List-Unsubscribe-Post"])
	assert.NotEmpty(t, call.Idempotency)

	// 第二轮不再发给同一个人。
	fake.reset()
	sent, err = RunAdtyMarketingTriggersOnce(context.Background(), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Zero(t, sent)
	assert.Empty(t, emailsTo(fake))
}

func TestStaleTriggerSendIsRetried(t *testing.T) {
	db, fake := prepareTriggers(t)
	now := time.Now()
	stale := marketingUser(t, db, "trg-stale", true)
	backdateUser(t, db, stale.Id, now.Add(-10*24*time.Hour))
	consumeLog(t, db, stale.Id, now.Add(-time.Hour))
	require.NoError(t, db.Create(&model.AdtyMarketingTriggerSend{
		Rule: model.AdtyTriggerUnpaid, UserId: stale.Id, Status: "sending", CreatedAt: now.Add(-time.Hour).Unix(),
	}).Error)
	fresh := marketingUser(t, db, "trg-fresh", true)
	backdateUser(t, db, fresh.Id, now.Add(-10*24*time.Hour))
	consumeLog(t, db, fresh.Id, now.Add(-time.Hour))
	require.NoError(t, db.Create(&model.AdtyMarketingTriggerSend{
		Rule: model.AdtyTriggerUnpaid, UserId: fresh.Id, Status: "sending", CreatedAt: now.Unix(),
	}).Error)

	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, []string{"trg-stale@example.com"}, emailsTo(fake))

	var retried model.AdtyMarketingTriggerSend
	require.NoError(t, db.Where("user_id = ? AND rule = ?", stale.Id, model.AdtyTriggerUnpaid).First(&retried).Error)
	assert.Equal(t, "sent", retried.Status)
	var held model.AdtyMarketingTriggerSend
	require.NoError(t, db.Where("user_id = ? AND rule = ?", fresh.Id, model.AdtyTriggerUnpaid).First(&held).Error)
	assert.Equal(t, "sending", held.Status)
}

func TestTriggerSkipsUnsubscribed(t *testing.T) {
	db, fake := prepareTriggers(t)
	now := time.Now()
	for _, name := range []string{"trg-unsub", "trg-noconsent"} {
		user := marketingUser(t, db, name, name == "trg-unsub")
		backdateUser(t, db, user.Id, now.Add(-10*24*time.Hour))
		consumeLog(t, db, user.Id, now.Add(-time.Hour))
		if name == "trg-unsub" {
			_, err := model.MarkAdtyMarketingByEmail(user.Email, model.AdtyMarketingFlagUnsubscribed)
			require.NoError(t, err)
		}
	}
	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Zero(t, sent)
	assert.Empty(t, emailsTo(fake))
}

func TestTriggerUnsubscribeLink(t *testing.T) {
	db, _ := prepareTriggers(t)
	user := marketingUser(t, db, "trg-link", true)
	token := AdtyMarketingUnsubscribeToken(user.Id)
	require.NoError(t, AdtyUnsubscribeByToken(token))
	assert.True(t, marketingContact(t, user.Id).Unsubscribed)
	require.Error(t, AdtyUnsubscribeByToken(token+"x"))
	require.Error(t, AdtyUnsubscribeByToken(""))
}

func TestLowBalanceDedupWithUpstreamNotice(t *testing.T) {
	db, fake := prepareTriggers(t)
	now := time.Now()
	low := marketingUser(t, db, "trg-low", true)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", low.Id).Updates(map[string]any{
		"quota": 2000000, "quota_pending": -1750000,
	}).Error) // logical $0.50 after an unflushed spend
	triggerTopUp(t, db, low.Id, "trg-l1")
	consumeLog(t, db, low.Id, now.Add(-time.Hour))
	rich := marketingUser(t, db, "trg-rich", true)
	triggerTopUp(t, db, rich.Id, "trg-r1")
	consumeLog(t, db, rich.Id, now.Add(-time.Hour))
	free := marketingUser(t, db, "trg-free", true)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", free.Id).Update("quota", 0).Error)

	// 上游的额度不足通知开着、并且这个人会收到（没关提醒、阈值不低于营销阈值）时，上游那封优先，营销不发。
	common.QuotaRemindEnabled, common.QuotaRemindThreshold = true, 1*500000
	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Zero(t, sent)
	assert.Empty(t, emailsTo(fake))

	// 这个人自己关了上游提醒：营销这封照发。
	off := false
	setting := low.GetSetting()
	setting.QuotaWarningEnabled = &off
	low.SetSetting(setting)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", low.Id).Update("setting", low.Setting).Error)
	sent, err = RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, []string{"trg-low@example.com"}, emailsTo(fake))
}

func TestTriggerWinbackPaidAndIdle(t *testing.T) {
	db, fake := prepareTriggers(t)
	adty_setting.GetMarketingSetting().TriggerLowBalanceEnabled = false
	now := time.Now()
	idle := marketingUser(t, db, "trg-idle", true)
	triggerTopUp(t, db, idle.Id, "trg-i1")
	consumeLog(t, db, idle.Id, now.Add(-40*24*time.Hour))
	busy := marketingUser(t, db, "trg-busy", true)
	triggerTopUp(t, db, busy.Id, "trg-b1")
	consumeLog(t, db, busy.Id, now.Add(-2*24*time.Hour))
	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, sent)
	assert.Equal(t, []string{"trg-idle@example.com"}, emailsTo(fake))
}

func TestTriggerDisabledSendsNothing(t *testing.T) {
	db, fake := prepareTriggers(t)
	now := time.Now()
	user := marketingUser(t, db, "trg-off", true)
	backdateUser(t, db, user.Id, now.Add(-10*24*time.Hour))
	consumeLog(t, db, user.Id, now.Add(-time.Hour))

	cfg := adty_setting.GetMarketingSetting()
	cfg.TriggerUnpaidEnabled, cfg.TriggerLowBalanceEnabled, cfg.TriggerWinbackEnabled = false, false, false
	sent, err := RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Zero(t, sent)

	// 规则开着、营销总开关关着也不发。
	cfg.TriggerUnpaidEnabled, cfg.Enabled = true, false
	sent, err = RunAdtyMarketingTriggersOnce(context.Background(), now)
	require.NoError(t, err)
	assert.Zero(t, sent)
	assert.Empty(t, emailsTo(fake))
}
