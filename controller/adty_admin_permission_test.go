package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAdtyPermissionAudit 准备 PostgreSQL 测试库、打开后台权限检查，并建一个管理员作为被改的人。
func setupAdtyPermissionAudit(t *testing.T) (*gorm.DB, model.User) {
	t.Helper()
	db := adtyTestDB(t)
	oldRedis, oldSecret := common.RedisEnabled, common.SessionSecret
	common.RedisEnabled = false
	common.SessionSecret = "adty-permission-audit-secret"
	t.Cleanup(func() { common.RedisEnabled, common.SessionSecret = oldRedis, oldSecret })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}, &model.AuditLog{}, &model.Log{},
		&model.CasbinRule{}, &model.AuthzRole{}, &model.UserAccessToken{}, &model.TwoFA{}, &model.PasskeyCredential{}))
	require.NoError(t, model.MigrateAdtyTables())
	require.NoError(t, model.SeedAdtyGroups())
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	restore := middleware.EnableAdtyAdminPermissions()
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		restore()
		require.NoError(t, authz.Init(db))
		common.IsMasterNode = wasMaster
	})
	target := model.User{Username: "perm-target", Password: "hashed-password", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&target).Error)
	return db, target
}

func updateUserPermissionsAsRoot(t *testing.T, db *gorm.DB, target model.User, permissions string) *httptest.ResponseRecorder {
	t.Helper()
	identity, proof := manageUserProof(t, db, service.VerificationOperation{
		Scope:   service.VerificationScopeAdminUserUpdate,
		Context: []byte(fmt.Sprintf(`{"user_id":%d}`, target.Id)),
	})
	body := fmt.Sprintf(`{"id":%d,"username":%q,"group":"default","admin_permissions":%s}`, target.Id, target.Username, permissions)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Security-Proof", proof)
	c.Set("id", identity.UserID)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
	UpdateUser(c)
	return recorder
}

func TestPermissionChangeAuditDiff(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	require.NoError(t, authz.SetUserPermissions(target.Id, authz.PermissionsMap{"adty_log": {"read": true}}))

	resp := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true,"quota":true},"adty_log":{"read":false}}`)
	require.Contains(t, resp.Body.String(), `"success":true`, resp.Body.String())
	assert.True(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserQuota))
	assert.False(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyLogRead))

	var logs []model.AuditLog
	require.NoError(t, db.Where("action = ?", "adty.admin_permissions.update").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].Other.Op)
	params := logs[0].Other.Op.Params
	added, err := common.Marshal(params["added"])
	require.NoError(t, err)
	removed, err := common.Marshal(params["removed"])
	require.NoError(t, err)
	assert.JSONEq(t, `["adty_user:quota","adty_user:read"]`, string(added))
	assert.JSONEq(t, `["adty_log:read"]`, string(removed))
}

func TestPermissionAuditNotWrittenOnRollback(t *testing.T) {
	db, target := setupAdtyPermissionAudit(t)
	// 用户名超长让 EditWithTx 之前的校验或事务失败，权限不能变，也不能留下变更审计。
	target.Username = strings.Repeat("x", 30)
	resp := updateUserPermissionsAsRoot(t, db, target, `{"adty_user":{"read":true}}`)
	assert.Contains(t, resp.Body.String(), `"success":false`)
	assert.False(t, authz.Can(target.Id, common.RoleAdminUser, authz.AdtyUserRead))
	var count int64
	require.NoError(t, db.Model(&model.AuditLog{}).Where("action = ?", "adty.admin_permissions.update").Count(&count).Error)
	assert.Zero(t, count)
}
