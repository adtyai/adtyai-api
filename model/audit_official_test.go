package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOfficialAuditTestDB(t *testing.T) *AdtyTestDatabase {
	t.Helper()
	opened, err := OpenAdtyTestDatabase()
	if errors.Is(err, ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	return opened
}

func TestPrepareOfficialAuditLogRejectsOmittedRequiredFields(t *testing.T) {
	siteID := 0
	base := OfficialAuditWrite{
		UserId: 1, Username: "root", ActorRole: common.RoleRootUser,
		SiteID: &siteID, ObjectType: "user", ObjectID: "2", Action: "user.update",
		Result: AuditResultSuccess, Success: true,
	}
	t.Run("missing site", func(t *testing.T) {
		rec := base
		rec.SiteID = nil
		_, err := PrepareOfficialAuditLog(nil, rec)
		require.ErrorIs(t, err, ErrOfficialAuditIncomplete)
	})
	t.Run("missing object type", func(t *testing.T) {
		rec := base
		rec.ObjectType = "  "
		_, err := PrepareOfficialAuditLog(nil, rec)
		require.ErrorIs(t, err, ErrOfficialAuditIncomplete)
	})
	t.Run("missing object id", func(t *testing.T) {
		rec := base
		rec.ObjectID = ""
		_, err := PrepareOfficialAuditLog(nil, rec)
		require.ErrorIs(t, err, ErrOfficialAuditIncomplete)
	})
	t.Run("missing action", func(t *testing.T) {
		rec := base
		rec.Action = ""
		_, err := PrepareOfficialAuditLog(nil, rec)
		require.ErrorIs(t, err, ErrOfficialAuditIncomplete)
	})
	t.Run("missing result", func(t *testing.T) {
		rec := base
		rec.Result = ""
		_, err := PrepareOfficialAuditLog(nil, rec)
		require.ErrorIs(t, err, ErrOfficialAuditIncomplete)
	})
	t.Run("approval may be empty", func(t *testing.T) {
		rec := base
		rec.ApprovalRef = ""
		entry, err := PrepareOfficialAuditLog(nil, rec)
		require.NoError(t, err)
		assert.Equal(t, 0, *entry.SiteId)
		assert.Equal(t, "user", entry.ObjectType)
		assert.Equal(t, "2", entry.ObjectId)
		assert.Equal(t, "user.update", entry.Action)
		assert.Equal(t, AuditResultSuccess, entry.Result)
		assert.Empty(t, entry.ApprovalRef)
		assert.True(t, entry.Success)
		assert.NotEmpty(t, entry.RequestId)
	})
}

func TestOfficialAuditWritePersistsFirstClassFieldsAndRedactsSecrets(t *testing.T) {
	setupOfficialAuditTestDB(t)
	require.NoError(t, MigrateAuditLogs())

	siteID := 7
	require.NoError(t, WriteOfficialAuditLog(nil, OfficialAuditWrite{
		UserId: 11, Username: "admin", ActorRole: common.RoleAdminUser,
		SiteID: &siteID, ObjectType: "user", ObjectID: "22", Action: "user.quota_add",
		Category: AuditCategoryOperation, Content: "Increased user quota",
		Before:      map[string]any{"quota": 100, "api_key": "sk-live-secretvalue", "password": "hunter2"},
		After:       map[string]any{"quota": 150, "token": "bot123456:ABCDEFGHIJKLmnopqr"},
		Reason:      "support adjustment password=supersecret",
		ApprovalRef: "",
		Result:      AuditResultSuccess, Success: true,
		RequestId: "req-official-1",
		Other: AuditOther{Op: &AuditOperation{Action: "user.quota_add", Params: AuditFields{
			"reason":  "support adjustment",
			"api_key": "sk-live-secretvalue",
		}}},
	}))

	var stored AuditLog
	require.NoError(t, LOG_DB.Where("request_id = ?", "req-official-1").Take(&stored).Error)
	require.NotNil(t, stored.SiteId)
	assert.Equal(t, 7, *stored.SiteId)
	assert.Equal(t, "user", stored.ObjectType)
	assert.Equal(t, "22", stored.ObjectId)
	assert.Equal(t, "user.quota_add", stored.Action)
	assert.Equal(t, AuditResultSuccess, stored.Result)
	assert.True(t, stored.Success)
	assert.Equal(t, "req-official-1", stored.RequestId)
	assert.Equal(t, 11, stored.UserId)
	assert.Equal(t, "admin", stored.Username)
	assert.Equal(t, common.RoleAdminUser, stored.ActorRole)
	assert.Empty(t, stored.ApprovalRef)
	assert.Contains(t, stored.BeforeValue, `"quota":100`)
	assert.Contains(t, stored.AfterValue, `"quota":150`)
	assert.NotContains(t, stored.BeforeValue, "sk-live-secretvalue")
	assert.NotContains(t, stored.BeforeValue, "hunter2")
	assert.NotContains(t, stored.AfterValue, "bot123456:ABCDEFGHIJKLmnopqr")
	assert.NotContains(t, stored.Reason, "supersecret")
	assert.Contains(t, stored.BeforeValue, "[redacted]")
	assert.Contains(t, stored.AfterValue, "[redacted]")
	assert.Contains(t, stored.Reason, "[redacted]")
	require.NotNil(t, stored.Other.Op)
	assert.NotContains(t, stringifyOfficialAuditID(stored.Other.Op.Params["api_key"]), "sk-live-secretvalue")
}

func TestHistoricalAuditLogsWithoutOfficialColumnsStillLoad(t *testing.T) {
	setupOfficialAuditTestDB(t)
	require.NoError(t, LOG_DB.Exec(`
CREATE TABLE audit_logs (
	id bigserial PRIMARY KEY,
	event_id varchar(64),
	user_id int,
	username varchar(64),
	actor_role int,
	created_at bigint,
	category varchar(24),
	action varchar(128),
	token_ref varchar(64),
	auth_method varchar(24),
	ip varchar(64),
	user_agent varchar(512),
	method varchar(16),
	route varchar(255),
	status int,
	success boolean,
	request_id varchar(64),
	content text,
	other json
)`).Error)
	require.NoError(t, LOG_DB.Exec(`CREATE UNIQUE INDEX uni_audit_logs_event_id ON audit_logs (event_id)`).Error)
	require.NoError(t, LOG_DB.Exec(`
INSERT INTO audit_logs (event_id, user_id, username, actor_role, created_at, category, action, success, request_id, content, other)
VALUES ('hist-event-1', 3, 'legacy', 1, 100, 'operation', 'option.update', true, 'hist-req-1', 'legacy row', '{}')
`).Error)

	require.NoError(t, MigrateAuditLogs())

	var historical AuditLog
	require.NoError(t, LOG_DB.Where("event_id = ?", "hist-event-1").Take(&historical).Error)
	assert.Equal(t, "hist-event-1", historical.EventId)
	assert.Equal(t, "legacy", historical.Username)
	assert.Equal(t, "option.update", historical.Action)
	assert.Nil(t, historical.SiteId)
	assert.Empty(t, historical.ObjectType)
	assert.Empty(t, historical.ObjectId)
	assert.Empty(t, historical.BeforeValue)
	assert.Empty(t, historical.AfterValue)
	assert.Empty(t, historical.Reason)
	assert.Empty(t, historical.ApprovalRef)
	assert.Empty(t, historical.Result)

	logs, total, err := GetAuditLogs(AuditLogFilter{RequestId: "hist-req-1"}, 0, 10, common.RoleRootUser)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "hist-event-1", logs[0].EventId)
	assert.Nil(t, logs[0].SiteId)

	siteID := 0
	require.NoError(t, WriteOfficialAuditLog(nil, OfficialAuditWrite{
		UserId: 1, Username: "root", ActorRole: common.RoleRootUser,
		SiteID: &siteID, ObjectType: "option", ObjectID: "Notice",
		Action: "option.update", Result: AuditResultSuccess, Success: true,
		RequestId: "new-official-after-legacy",
		Reason:    "critical system config",
	}))
	var stored AuditLog
	require.NoError(t, LOG_DB.Where("request_id = ?", "new-official-after-legacy").Take(&stored).Error)
	require.NotNil(t, stored.SiteId)
	assert.Equal(t, 0, *stored.SiteId)
	assert.Equal(t, "option", stored.ObjectType)
	assert.Equal(t, "Notice", stored.ObjectId)
	assert.Equal(t, "critical system config", stored.Reason)
}

func TestRecordOperationAuditLogUsesOfficialWriter(t *testing.T) {
	setupOfficialAuditTestDB(t)
	require.NoError(t, MigrateAuditLogs())
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, DB.Create(&User{Id: 9, Username: "staff", Password: "password-for-test", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)

	RecordOperationAuditLog(9, common.RoleAdminUser, "Increased user quota by 50", "127.0.0.1", "user.quota_add",
		map[string]any{"target_user_id": 22, "from": 100, "to": 150, "quota": 50, "reason": "staff adjustment", "site_id": 4},
		&AuditAdminInfo{AdminID: 9, AdminUsername: "staff", AdminRole: common.RoleAdminUser, AuthMethod: "session"},
		&AuditRequestInfo{Method: "POST", Route: "/api/user/manage", Status: 200, Success: true},
		nil)

	var stored AuditLog
	require.NoError(t, LOG_DB.Where("action = ?", "user.quota_add").Order("id desc").Take(&stored).Error)
	require.NotNil(t, stored.SiteId)
	assert.Equal(t, 4, *stored.SiteId)
	assert.Equal(t, "user", stored.ObjectType)
	assert.Equal(t, "22", stored.ObjectId)
	assert.Equal(t, "user.quota_add", stored.Action)
	assert.Equal(t, AuditResultSuccess, stored.Result)
	assert.True(t, stored.Success)
	assert.NotEmpty(t, stored.RequestId)
	assert.Equal(t, 9, stored.UserId)
	assert.Equal(t, common.RoleAdminUser, stored.ActorRole)
	assert.Equal(t, "staff adjustment", stored.Reason)
	assert.Empty(t, stored.ApprovalRef)
	assert.Contains(t, stored.BeforeValue, "100")
	assert.Contains(t, stored.AfterValue, "150")
}

func TestWriteOfficialAuditLogRejectsIncompleteInputWithoutWriting(t *testing.T) {
	setupOfficialAuditTestDB(t)
	require.NoError(t, MigrateAuditLogs())

	err := WriteOfficialAuditLog(nil, OfficialAuditWrite{
		UserId: 1, Username: "root", ActorRole: common.RoleRootUser,
		ObjectType: "user", ObjectID: "2", Action: "user.update", Result: AuditResultSuccess,
	})
	require.ErrorIs(t, err, ErrOfficialAuditIncomplete)

	var count int64
	require.NoError(t, LOG_DB.Model(&AuditLog{}).Count(&count).Error)
	assert.Zero(t, count)
}
