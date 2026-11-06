package router

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 人工调额留痕（任务 7.2）：原因必填、幂等、同一事务写流水、原因不进用户日志。

func adjustQuota(t *testing.T, f *adtyGuardFixture, userID int, extra string) (bool, string) {
	t.Helper()
	body := fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":500%s}`, userID, extra)
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", body, f.root)
	require.Equal(t, http.StatusOK, resp.Code)
	var envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(resp.Body.Bytes(), &envelope))
	return envelope.Success, envelope.Message
}

func ledgerUser(t *testing.T, f *adtyGuardFixture, name string) model.User {
	t.Helper()
	adtySessionToken(t, f.db, name, common.RoleCommonUser)
	var user model.User
	require.NoError(t, f.db.Where("username = ?", name).First(&user).Error)
	require.NoError(t, f.db.Model(&user).Update("quota", 1000).Error)
	return user
}

func TestManualAdjustRequiresReason(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-reason")
	ok, message := adjustQuota(t, f, user.Id, "")
	assert.False(t, ok)
	assert.Equal(t, "Enter a reason for the adjustment", message)
	ok, message = adjustQuota(t, f, user.Id, fmt.Sprintf(`,"reason":%q`, strings.Repeat("长", 201)))
	assert.False(t, ok, "201 characters")
	assert.Equal(t, "The reason must be at most 200 characters", message)
	ok, _ = adjustQuota(t, f, user.Id, fmt.Sprintf(`,"reason":%q,"idempotency_key":"reason-200"`, strings.Repeat("长", 200)))
	assert.True(t, ok, "200 characters, counted as characters not bytes")
	ok, message = adjustQuota(t, f, user.Id, `,"reason":"missing key"`)
	assert.False(t, ok)
	assert.Equal(t, "Invalid idempotency key", message)
}

func TestManualAdjustIdempotent(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-idem")
	for range 2 {
		ok, _ := adjustQuota(t, f, user.Id, `,"reason":"compensation","idempotency_key":"k-1"`)
		require.True(t, ok)
	}
	var stored model.User
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Equal(t, 1500, stored.Quota, "the retry does not add again")
	var ledgers, logs int64
	f.db.Model(&model.AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers)
	model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Count(&logs)
	assert.EqualValues(t, 1, ledgers)
	assert.EqualValues(t, 1, logs, "no second user log for the replay")
}

func TestManualAdjustIdempotentKeyMismatch(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-mismatch")
	ok, _ := adjustQuota(t, f, user.Id, `,"reason":"r","idempotency_key":"k-2"`)
	require.True(t, ok)
	body := fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":900,"reason":"r","idempotency_key":"k-2"}`, user.Id)
	resp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", body, f.root)
	assert.Contains(t, resp.Body.String(), "This request was already used for another operation")
}

func TestManualAdjustWritesLedgerWithBalances(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-ledger")
	ok, _ := adjustQuota(t, f, user.Id, `,"reason":"goodwill credit","idempotency_key":"goodwill-1"`)
	require.True(t, ok)

	resp := adtyJSON(t, f.engine, http.MethodGet, fmt.Sprintf("/api/adty/admin/users/%d/ledger", user.Id), "", f.root)
	require.Equal(t, http.StatusOK, resp.Code)
	page := decodeAdtyData[struct {
		Items []model.AdtyBalanceLedger `json:"items"`
	}](t, resp.Body)
	require.Len(t, page.Items, 1)
	entry := page.Items[0]
	assert.EqualValues(t, 1000, entry.BalanceBefore)
	assert.EqualValues(t, 1500, entry.BalanceAfter)
	assert.EqualValues(t, 500, entry.DeltaQuota)
	assert.Equal(t, "goodwill credit", entry.Reason)
	assert.Equal(t, model.AdtyLedgerTypeManualAdjust, entry.Type)

	assert.Equal(t, http.StatusForbidden,
		adtyJSON(t, f.engine, http.MethodGet, fmt.Sprintf("/api/adty/admin/users/%d/ledger", user.Id), "", f.token).Code,
		"needs user read")
}

func TestManualAdjustFloorRejectsNegativeBalance(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-floor")

	subtract := fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":1500,"reason":"too much","idempotency_key":"floor-sub"}`, user.Id)
	subtractResp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", subtract, f.root)
	require.Equal(t, http.StatusOK, subtractResp.Code)
	assert.Contains(t, subtractResp.Body.String(), `"success":false`)
	assert.Contains(t, subtractResp.Body.String(), "Adjustment cannot make the balance negative")

	override := fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"override","value":-1,"reason":"negative set","idempotency_key":"floor-over"}`, user.Id)
	overrideResp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", override, f.root)
	require.Equal(t, http.StatusOK, overrideResp.Code)
	assert.Contains(t, overrideResp.Body.String(), `"success":false`)
	assert.Contains(t, overrideResp.Body.String(), "Adjustment cannot make the balance negative")

	var stored model.User
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Equal(t, 1000, stored.Quota)
	var ledgers int64
	require.NoError(t, f.db.Model(&model.AdtyBalanceLedger{}).Where("user_id = ?", user.Id).Count(&ledgers).Error)
	assert.Zero(t, ledgers, "a rejected adjustment does not write a balance row")

	exact := fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"subtract","value":1000,"reason":"to zero","idempotency_key":"floor-zero"}`, user.Id)
	exactResp := adtyJSON(t, f.engine, http.MethodPost, "/api/user/manage", exact, f.root)
	require.Contains(t, exactResp.Body.String(), `"success":true`)
	require.NoError(t, f.db.First(&stored, user.Id).Error)
	assert.Equal(t, 0, stored.Quota)
}

func TestManualAdjustReasonNotInUserLog(t *testing.T) {
	f := newAdtyGuardFixture(t)
	user := ledgerUser(t, f, "adj-private")
	ok, _ := adjustQuota(t, f, user.Id, `,"reason":"internal note for staff","idempotency_key":"private-1"`)
	require.True(t, ok)
	logs, _, err := model.GetUserLogs(user.Id, model.LogFilter{Type: model.LogTypeTopup}, 0, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.NotContains(t, logs[0].Other, "internal note for staff")
	assert.NotContains(t, logs[0].Content, "internal note for staff")
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "user.quota_add").Find(&audits).Error)
	require.NotEmpty(t, audits)
	params, err := common.Marshal(audits[len(audits)-1].Other.Op.Params)
	require.NoError(t, err)
	assert.Contains(t, string(params), `"reason":"internal note for staff"`, "the admin audit keeps the reason")
}
