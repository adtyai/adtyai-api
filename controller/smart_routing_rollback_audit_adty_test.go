package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
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

// 终审 P2-2：回滚到快照（带合法 preview_token）同样写 routing.rollback 审计。
func TestRoutingRollbackSnapshotWritesAudit(t *testing.T) {
	before := setupRoutingConfigTest(t)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.AuditLog{}))
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	root := model.User{Username: "rbsnap", Password: "unused-password", Role: common.RoleRootUser, AffCode: "rbs1"}
	require.NoError(t, model.DB.Create(&root).Error)

	target := before
	target.Weights = smartrouting.WeightParts{Conn: 40, Price: 20, Speed: 20, Stab: 10, Auth: 10}
	require.NotEqual(t, before.Weights, target.Weights)
	require.NoError(t, model.SaveWeightSnapshot("manual", root.Id, map[string]any{}, target))
	var snap model.RoutingWeightSnapshot
	require.NoError(t, model.DB.Order("id desc").First(&snap).Error)

	rec := adtyCall(AdtyRoutingPreviewScheme, http.MethodPost, "/api/adty/routing/weight-schemes/preview",
		`{"weights":{"conn":40,"price":20,"speed":20,"stab":10,"auth":10}}`, root.Id, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var pv struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pv))
	require.NotEmpty(t, pv.Data.PreviewToken)

	body := `{"snapshot_id":` + strconv.FormatInt(snap.ID, 10) + `,"preview_token":"` + pv.Data.PreviewToken + `"}`
	rec = adtyCall(AdtyRoutingSchemeRollback, http.MethodPost, "/api/adty/routing/weight-schemes/rollback", body, root.Id, common.RoleRootUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, target.Weights, smartrouting.GetConfig().Weights)

	var a model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ? AND user_id = ?", "routing.rollback", root.Id).Order("id desc").First(&a).Error)
	require.Equal(t, "rbsnap", a.Username)
	require.NotZero(t, a.CreatedAt)
	require.NotNil(t, a.Other.AdminInfo)
	require.Equal(t, root.Id, a.Other.AdminInfo.AdminID)
	raw, err := json.Marshal(a.Other.Op.Params)
	require.NoError(t, err)
	var p map[string]any
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Equal(t, "snapshot", p["to"])
	require.EqualValues(t, snap.ID, p["snapshot_id"])
	require.EqualValues(t, root.Id, p["operator_id"])
	require.Equal(t, "rbsnap", p["operator_name"])
	require.Equal(t, before.Enabled, p["before_enabled"])
	require.Equal(t, target.Enabled, p["after_enabled"])
	require.Equal(t, before.ActiveWeightScheme, p["before_scheme"])
	require.Equal(t, target.ActiveWeightScheme, p["after_scheme"])
	bw := p["before_weights"].(map[string]any)
	aw := p["after_weights"].(map[string]any)
	require.EqualValues(t, before.Weights.Conn, bw["conn"])
	require.EqualValues(t, 40, aw["conn"])
	require.EqualValues(t, 10, aw["auth"])
}
