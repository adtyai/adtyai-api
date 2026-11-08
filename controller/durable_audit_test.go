package controller

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This black-box fixture describes the durable storage contract independently
// of the producer: successful mutations must leave their frozen event here.
type durableAuditIntentRow struct {
	ID                int64  `gorm:"primaryKey"`
	EventID           string `gorm:"type:varchar(64);uniqueIndex"`
	Version           int
	Payload           string `gorm:"type:text"`
	CreatedAt         int64
	Attempts          int
	NextAttemptAt     int64
	DeliveredAt       int64
	LastErrorCategory string `gorm:"type:varchar(64)"`
}

func (durableAuditIntentRow) TableName() string { return "audit_intents" }

func TestDurablePermissionAuditSurvivesLogOutage(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	require.NoError(t, db.AutoMigrate(&durableAuditIntentRow{}))
	previous := model.LOG_DB
	model.LOG_DB = nil
	t.Cleanup(func() { model.LOG_DB = previous })

	response := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true}}`)
	require.Contains(t, response.Body.String(), `"success":true`)
	assert.True(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
	var intents []durableAuditIntentRow
	require.NoError(t, db.Find(&intents).Error)
	require.Len(t, intents, 2, "both the committed user mutation and its permission diff must survive the independent log outage")
	actions := make([]string, 0, len(intents))
	for _, intent := range intents {
		var event model.AuditLog
		require.NoError(t, common.UnmarshalJsonStr(intent.Payload, &event))
		assert.NotEmpty(t, event.EventId)
		assert.Equal(t, event.EventId, intent.EventID)
		assert.True(t, event.Success)
		assert.Equal(t, common.RoleRootUser, event.ActorRole)
		assert.Zero(t, intent.DeliveredAt)
		actions = append(actions, event.Action)
	}
	assert.ElementsMatch(t, []string{"user.update", "adty.admin_permissions.update"}, actions)
}

func TestDurablePermissionAuditFailureRollsBackMutation(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	require.NoError(t, db.AutoMigrate(&durableAuditIntentRow{}))
	const callback = "test:durable-permission-intent-failure"
	intentAttempted := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_intents" {
			intentAttempted = true
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callback)) })
	response := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true}}`)
	assert.True(t, intentAttempted)
	assert.Contains(t, response.Body.String(), `"success":false`)
	assert.False(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
	var rules int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ? AND v1 = ? AND v2 = ?", authz.UserSubject(target.Id), "adty_user", "read").Count(&rules).Error)
	assert.Zero(t, rules)
	var logs int64
	require.NoError(t, db.Model(&model.AuditLog{}).Where("action IN ?", []string{"user.update", "adty.admin_permissions.update"}).Count(&logs).Error)
	assert.Zero(t, logs)
}

func TestDurablePermissionAuditUsesTransactionSnapshot(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	// Another committed writer has changed policy before this node reloads it.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return authz.SetUserPermissionsInTx(tx, target.Id, authz.PermissionsMap{"adty_user": {"read": true}})
	}))
	require.False(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
	response := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":false}}`)
	require.Contains(t, response.Body.String(), `"success":true`)
	var event model.AuditLog
	require.NoError(t, db.Where("action = ?", "adty.admin_permissions.update").Take(&event).Error)
	removed, err := common.Marshal(event.Other.Op.Params["removed"])
	require.NoError(t, err)
	assert.JSONEq(t, `["adty_user:read"]`, string(removed), "the diff must use committed DB policy, not this node's stale enforcer")
}

func TestDurablePermissionAuditSurvivesPolicyReloadFailure(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	const createCallback, queryCallback = "test:audit-intent-committed", "test:audit-policy-reload-failure"
	intentWritten, injected := false, false
	require.NoError(t, db.Callback().Create().After("gorm:create").Register(createCallback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_intents" && tx.Error == nil {
			intentWritten = true
		}
	}))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if intentWritten && !injected && tx.Statement.Table == "casbin_rule" {
			injected = true
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Create().Remove(createCallback))
		require.NoError(t, db.Callback().Query().Remove(queryCallback))
	})
	response := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true}}`)
	require.True(t, injected)
	assert.Contains(t, response.Body.String(), `"success":false`, "the HTTP attempt can fail after the business commit")
	var rules int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ? AND v1 = ? AND v2 = ? AND v3 = ?", authz.UserSubject(target.Id), "adty_user", "read", authz.EffectAllow).Count(&rules).Error)
	assert.EqualValues(t, 1, rules)
	var intents []model.AuditIntent
	require.NoError(t, db.Find(&intents).Error)
	require.Len(t, intents, 2)
	for _, intent := range intents {
		var event model.AuditLog
		require.NoError(t, common.UnmarshalJsonStr(intent.Payload, &event))
		assert.True(t, event.Success, "a committed mutation is distinct from failed HTTP finalization")
		assert.Equal(t, common.RoleRootUser, event.ActorRole)
	}
	require.NoError(t, authz.ReloadPolicy())
	assert.True(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
}

func TestDurablePermissionAuditRechecksTransactionRole(t *testing.T) {
	for _, test := range []struct {
		name  string
		role  int
		rules int64
	}{
		{"promoted target audit uses root capabilities", common.RoleRootUser, 1},
		{"demoted target cannot retain admin overrides", common.RoleCommonUser, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, target := setupAdtyPermissionAudit(t)
			const callback = "test:role-changed-after-origin-read"
			changed := false
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				user, ok := tx.Statement.Dest.(*model.User)
				if !changed && ok && user.Id == target.Id && tx.Error == nil {
					changed = true
					require.NoError(t, db.Model(&model.User{}).Where("id = ?", target.Id).Update("role", test.role).Error)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
			response := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true}}`)
			require.True(t, changed)
			assert.Contains(t, response.Body.String(), `"success":true`)
			var rules int64
			require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ?", authz.UserSubject(target.Id)).Count(&rules).Error)
			assert.Equal(t, test.rules, rules, "the stale initial role cannot authorize persisted admin grants")
			var event model.AuditLog
			require.NoError(t, db.Where("action = ?", "user.update").Take(&event).Error)
			role, err := common.Marshal(event.Other.Op.Params["target_role"])
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprint(test.role), string(role))
			if test.role == common.RoleRootUser {
				event = model.AuditLog{}
				require.NoError(t, db.Where("action = ?", "adty.admin_permissions.update").Take(&event).Error)
				added, err := common.Marshal(event.Other.Op.Params["added"])
				require.NoError(t, err)
				var permissions []string
				require.NoError(t, common.Unmarshal(added, &permissions))
				assert.Empty(t, permissions, "a root target already has the permission independently of admin overrides")
			}
		})
	}
}

func TestDurableUserUpdateRechecksTargetAuthorizationInTransaction(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", target.Id).Update("role", common.RoleCommonUser).Error)
	const callback = "test:target-promoted-before-admin-update"
	changed := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		user, ok := tx.Statement.Dest.(*model.User)
		if !changed && ok && user.Id == target.Id && tx.Error == nil {
			changed = true
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", target.Id).Update("role", common.RoleAdminUser).Error)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
	body := fmt.Sprintf(`{"id":%d,"username":%q,"group":"default","display_name":"must-not-commit"}`, target.Id, target.Username)
	response := adminUserRequest(http.MethodPut, "/api/user/", body, "", service.AuthIdentity{UserID: 9999}, common.RoleAdminUser, nil, UpdateUser)
	require.True(t, changed)
	assert.Contains(t, response.Body.String(), `"success":false`)
	var stored model.User
	require.NoError(t, db.First(&stored, target.Id).Error)
	assert.Empty(t, stored.DisplayName)
	var count int64
	require.NoError(t, db.Model(&model.AuditIntent{}).Count(&count).Error)
	assert.Zero(t, count)
}

func durableOptionRequest() *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"SystemName","value":"durable-brand"}`))
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	UpdateOption(c)
	return response
}

func prepareDurableOption(t *testing.T) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous, previousName := common.OptionMap, common.SystemName
	common.OptionMap = maps.Clone(previous)
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap, common.SystemName = previous, previousName
		common.OptionMapRWMutex.Unlock()
	})
}

func TestDurableOptionAuditSurvivesLogOutage(t *testing.T) {
	db := setupManageUserTestDB(t)
	prepareDurableOption(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &durableAuditIntentRow{}))
	previous := model.LOG_DB
	model.LOG_DB = nil
	t.Cleanup(func() { model.LOG_DB = previous })
	response := durableOptionRequest()
	require.Contains(t, response.Body.String(), `"success":true`)
	var intent durableAuditIntentRow
	require.NoError(t, db.Take(&intent).Error)
	var event model.AuditLog
	require.NoError(t, common.UnmarshalJsonStr(intent.Payload, &event))
	assert.Equal(t, "option.update", event.Action)
	assert.Contains(t, intent.Payload, "SystemName")
	assert.NotContains(t, intent.Payload, "durable-brand", "configuration values must not enter audit storage")
}

func TestDurableOptionAuditFailureRollsBackMutation(t *testing.T) {
	db := setupManageUserTestDB(t)
	prepareDurableOption(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &durableAuditIntentRow{}))
	require.NoError(t, db.Create(&model.Option{Key: "SystemName", Value: "old-brand"}).Error)
	const callback = "test:durable-option-intent-failure"
	intentAttempted := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_intents" {
			intentAttempted = true
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(callback)) })
	response := durableOptionRequest()
	assert.True(t, intentAttempted)
	assert.Contains(t, response.Body.String(), `"success":false`)
	var option model.Option
	require.NoError(t, db.Where("key = ?", "SystemName").Take(&option).Error)
	assert.Equal(t, "old-brand", option.Value)
}
