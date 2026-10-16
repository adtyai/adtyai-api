package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/smartrouting"
	"github.com/stretchr/testify/require"
)

// 终审 P2-2：回滚到官方路由不需要 preview/确认，但必须写审计：操作人、时间、前后配置。
func TestRoutingRollbackOfficialWritesAudit(t *testing.T) {
	before := setupRoutingConfigTest(t)
	require.True(t, before.Enabled)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.AuditLog{}))
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	root := model.User{Username: "rbroot", Password: "unused-password", Role: common.RoleRootUser, AffCode: "rbr1"}
	require.NoError(t, model.DB.Create(&root).Error)

	rec := adtyCall(AdtyRoutingSchemeRollback, http.MethodPost, "/api/adty/routing/weight-schemes/rollback", `{"to":"official"}`, root.Id, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.False(t, smartrouting.GetConfig().Enabled)

	var a model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "routing.rollback").Order("id desc").First(&a).Error)
	require.Equal(t, root.Id, a.UserId)
	require.Equal(t, "rbroot", a.Username)
	require.NotZero(t, a.CreatedAt)
	require.NotNil(t, a.Other.AdminInfo)
	require.Equal(t, root.Id, a.Other.AdminInfo.AdminID)
	require.Equal(t, "rbroot", a.Other.AdminInfo.AdminUsername)
	require.NotNil(t, a.Other.Op)
	raw, err := json.Marshal(a.Other.Op.Params)
	require.NoError(t, err)
	var p map[string]any
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Equal(t, "official", p["to"])
	require.Equal(t, true, p["before_enabled"])
	require.Equal(t, false, p["after_enabled"])
	require.Equal(t, before.ActiveWeightScheme, p["before_scheme"])
	require.NotNil(t, p["before_weights"])
	require.NotNil(t, p["after_weights"])
}
