package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdtyGroupAdminRequiresRoot(t *testing.T) {
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AdtyGroup{}))
	engine := newAdtyGroupEngine()

	anonymous := httptest.NewRequest(http.MethodGet, "/api/adty/admin/groups", nil)
	anonymousResp := httptest.NewRecorder()
	engine.ServeHTTP(anonymousResp, anonymous)
	assert.Equal(t, http.StatusUnauthorized, anonymousResp.Code)

	commonToken := adtySessionToken(t, db, "adty-common", common.RoleCommonUser)
	commonReq := httptest.NewRequest(http.MethodGet, "/api/adty/admin/groups", nil)
	commonReq.Header.Set("Authorization", "Bearer "+commonToken)
	commonResp := httptest.NewRecorder()
	engine.ServeHTTP(commonResp, commonReq)
	assert.Equal(t, http.StatusForbidden, commonResp.Code)

	rootToken := adtySessionToken(t, db, "adty-root", common.RoleRootUser)
	rootReq := httptest.NewRequest(http.MethodGet, "/api/adty/admin/groups", nil)
	rootReq.Header.Set("Authorization", "Bearer "+rootToken)
	rootResp := httptest.NewRecorder()
	engine.ServeHTTP(rootResp, rootReq)
	assert.Equal(t, http.StatusOK, rootResp.Code)
	assert.Contains(t, rootResp.Body.String(), `"success":true`)
}

func TestAdtyGroupDisplayAnonymous(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, prepareAdtyDisplayGroups(t, db))
	engine := newAdtyGroupEngine()

	req := httptest.NewRequest(http.MethodGet, "/api/adty/groups/display", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	body := decodeAdtyData[[]adtyDisplayItem](t, resp.Body)
	ids := map[string]adtyDisplayItem{}
	for _, item := range body {
		ids[item.ID] = item
	}
	// 访客只看到可选分组，hidden 不在 UserUsableGroups 里，ID 不能露出来。
	assert.Contains(t, ids, "default")
	assert.Contains(t, ids, "vip")
	assert.NotContains(t, ids, "hidden")
	assert.True(t, ids["vip"].Usable)
	assert.True(t, ids["vip"].Visible)
	assert.Equal(t, "VIP group", ids["vip"].Descs["en"])
}

func TestAdtyGroupDisplayLoggedInGetsAllNames(t *testing.T) {
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, prepareAdtyDisplayGroups(t, db))
	token := adtySessionToken(t, db, "adty-member", common.RoleCommonUser)
	require.NoError(t, db.Model(&model.AdtyGroup{}).Where("id = ?", "hidden").Update("names", model.AdtyLangText{"en": "Hidden", "fr": "Caché"}).Error)
	engine := newAdtyGroupEngine()

	req := httptest.NewRequest(http.MethodGet, "/api/adty/groups/display", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	body := decodeAdtyData[[]adtyDisplayItem](t, resp.Body)
	byID := map[string]adtyDisplayItem{}
	for _, item := range body {
		byID[item.ID] = item
		assert.NotEmpty(t, item.Names["en"], item.ID)
	}
	require.Contains(t, byID, "default")
	require.Contains(t, byID, "vip")
	require.Contains(t, byID, "hidden")
	assert.True(t, byID["default"].Usable)
	assert.True(t, byID["vip"].Usable)
	assert.False(t, byID["hidden"].Usable)
	assert.True(t, byID["hidden"].Visible)
	assert.Equal(t, "Caché", byID["hidden"].Names["fr"])
	assert.Equal(t, "VIP group", byID["vip"].Descs["en"])
}

func TestAdtyGroupListIncludesRefs(t *testing.T) {
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.Channel{}, &model.AuditLog{}))
	require.NoError(t, model.MigrateAdtyTables())
	clearRouterAdtyGroups(t, db)
	token := adtySessionToken(t, db, "adty-root2", common.RoleRootUser)
	engine := newAdtyGroupEngine()

	create := adtyJSON(t, engine, http.MethodPost, "/api/adty/admin/groups", `{"name":"展示","desc":"说明"}`, token)
	require.Equal(t, http.StatusOK, create.Code)
	created := decodeAdtyData[model.AdtyGroup](t, create.Body)
	require.Regexp(t, `^g[a-z0-9]{4}$`, created.ID)

	user := &model.User{Username: "listed", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: created.ID, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&model.Token{UserId: user.Id, Key: "listed-token", Name: "listed", Group: created.ID, Status: 1}).Error)
	require.NoError(t, db.Create(&model.Channel{Key: "listed-ch", Name: "listed", Group: created.ID + ",other", Status: 1}).Error)

	list := adtyJSON(t, engine, http.MethodGet, "/api/adty/admin/groups", "", token)
	require.Equal(t, http.StatusOK, list.Code)
	rows := decodeAdtyData[[]adtyAdminGroup](t, list.Body)
	var found *adtyAdminGroup
	for i := range rows {
		if rows[i].ID == created.ID {
			found = &rows[i]
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, int64(1), found.Refs.Users)
	assert.Equal(t, int64(1), found.Refs.Tokens)
	assert.Equal(t, int64(1), found.Refs.Channels)
	assert.Equal(t, "展示", found.Names["zh-CN"])
	assert.Equal(t, "说明", found.Descs["en"])
}

func TestAdtyGroupCreateAndUpdateAudit(t *testing.T) {
	disableAdtyRedis(t)
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuditLog{}))
	require.NoError(t, model.MigrateAdtyTables())
	clearRouterAdtyGroups(t, db)
	token := adtySessionToken(t, db, "adty-root3", common.RoleRootUser)
	engine := newAdtyGroupEngine()

	create := adtyJSON(t, engine, http.MethodPost, "/api/adty/admin/groups", `{"name":"旧名","desc":"旧说明"}`, token)
	require.Equal(t, http.StatusOK, create.Code)
	created := decodeAdtyData[model.AdtyGroup](t, create.Body)

	names := fillAdtyLangs("新名")
	names["en"] = ""
	rejected := adtyJSON(t, engine, http.MethodPut, "/api/adty/admin/groups/"+created.ID, mustJSON(t, map[string]any{
		"names": names,
		"descs": fillAdtyLangs("新说明"),
	}), token)
	require.Equal(t, http.StatusOK, rejected.Code)
	assert.Contains(t, rejected.Body.String(), `"success":false`)
	var stored model.AdtyGroup
	require.NoError(t, db.First(&stored, "id = ?", created.ID).Error)
	assert.Equal(t, "旧名", stored.Names["en"])

	updated := adtyJSON(t, engine, http.MethodPut, "/api/adty/admin/groups/"+created.ID, mustJSON(t, map[string]any{
		"names": fillAdtyLangs("新名"),
		"descs": fillAdtyLangs("新说明"),
	}), token)
	require.Equal(t, http.StatusOK, updated.Code)
	assert.Contains(t, updated.Body.String(), `"success":true`)

	var logs []model.AuditLog
	require.NoError(t, db.Where("action IN ?", []string{"adty.group.create", "adty.group.update"}).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, "adty.group.create", logs[0].Action)
	assert.Contains(t, auditParamJSON(t, logs[0], "new"), `"en":"旧名"`)
	assert.Equal(t, "adty.group.update", logs[1].Action)
	assert.Contains(t, auditParamJSON(t, logs[1], "old"), `"en":"旧名"`)
	assert.Contains(t, auditParamJSON(t, logs[1], "new"), `"en":"新名"`)
}

type adtyDisplayItem struct {
	ID      string            `json:"id"`
	Names   map[string]string `json:"names"`
	Descs   map[string]string `json:"descs"`
	Usable  bool              `json:"usable"`
	Visible bool              `json:"visible"`
}

type adtyAdminGroup struct {
	ID    string              `json:"id"`
	Names map[string]string   `json:"names"`
	Descs map[string]string   `json:"descs"`
	Refs  model.AdtyGroupRefs `json:"refs"`
}

func newAdtyGroupEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetAdtyRouter(engine.Group("/api"))
	return engine
}

func prepareAdtyDisplayGroups(t *testing.T, db *gorm.DB) error {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	saved := routerGroupSettings()
	t.Cleanup(func() {
		require.NoError(t, applyRouterGroupSettings(saved))
	})
	require.NoError(t, applyRouterGroupSettings(routerGroupSettingSnapshot{
		groupRatio: `{"default":1,"vip":1,"hidden":1}`,
		usable:     `{"default":"Default group","vip":"VIP group"}`,
		topup:      `{}`,
		auto:       `["default"]`,
		groupGroup: `{}`,
		special:    `{}`,
		rateLimit:  `{}`,
	}))
	if err := model.MigrateAdtyTables(); err != nil {
		return err
	}
	return model.SeedAdtyGroups()
}

func adtySessionToken(t *testing.T, db *gorm.DB, username string, role int) string {
	t.Helper()
	user := &model.User{
		Username:    username,
		Password:    "hashed-password",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	session := &model.UserSession{
		SID:             "sid-" + username,
		UserID:          user.Id,
		Version:         1,
		UserAuthVersion: 1,
		Status:          model.UserSessionStatusActive,
		RefreshHash:     "refresh-" + username,
		LoginMethod:     "password",
		LastActiveAt:    now,
		ExpiresAt:       now + 3600,
	}
	require.NoError(t, model.CreateUserSession(session))
	token, _, err := service.IssueAccessToken(service.AuthIdentity{
		UserID:          user.Id,
		SessionID:       session.SID,
		UserAuthVersion: 1,
		SessionVersion:  1,
	})
	require.NoError(t, err)
	return token
}

func adtyJSON(t *testing.T, engine *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	return adtyJSONWithProof(t, engine, method, path, body, token, "")
}

// adtyJSONOnHost is adtyJSON with a trusted request Host so dashboard
// sessions can be exercised on a specific site after login.
func adtyJSONOnHost(t *testing.T, engine *gin.Engine, host, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	return adtyJSONOnHostWithProof(t, engine, host, method, path, body, token, "")
}

// adtyJSONWithProof 和 adtyJSON 一样，另带二次验证的 X-Security-Proof（站长敏感操作用）。
func adtyJSONWithProof(t *testing.T, engine *gin.Engine, method, path, body, token, proof string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if proof != "" {
		req.Header.Set("X-Security-Proof", proof)
	}
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func adtyJSONOnHostWithProof(t *testing.T, engine *gin.Engine, host, method, path, body, token, proof string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if host != "" {
		req.Host = host
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if proof != "" {
		req.Header.Set("X-Security-Proof", proof)
	}
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func decodeAdtyData[T any](t *testing.T, body io.Reader) T {
	t.Helper()
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var envelope struct {
		Success bool `json:"success"`
		Data    T    `json:"data"`
	}
	require.NoError(t, common.Unmarshal(raw, &envelope))
	require.True(t, envelope.Success, string(raw))
	return envelope.Data
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func auditParamJSON(t *testing.T, entry model.AuditLog, key string) string {
	t.Helper()
	require.NotNil(t, entry.Other.Op)
	value, ok := entry.Other.Op.Params[key]
	require.True(t, ok, key)
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func clearRouterAdtyGroups(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Unscoped().Where("1 = 1").Delete(&model.AdtyGroup{}).Error)
}

type routerGroupSettingSnapshot struct {
	groupRatio string
	usable     string
	topup      string
	auto       string
	groupGroup string
	special    string
	rateLimit  string
}

func routerGroupSettings() routerGroupSettingSnapshot {
	return routerGroupSettingSnapshot{
		groupRatio: ratio_setting.GroupRatio2JSONString(),
		usable:     setting.UserUsableGroups2JSONString(),
		topup:      common.TopupGroupRatio2JSONString(),
		auto:       setting.AutoGroups2JsonString(),
		groupGroup: ratio_setting.GroupGroupRatio2JSONString(),
		special:    ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString(),
		rateLimit:  setting.ModelRequestRateLimitGroup2JSONString(),
	}
}

func applyRouterGroupSettings(saved routerGroupSettingSnapshot) error {
	if err := ratio_setting.UpdateGroupRatioByJSONString(saved.groupRatio); err != nil {
		return err
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(saved.usable); err != nil {
		return err
	}
	if err := common.UpdateTopupGroupRatioByJSONString(saved.topup); err != nil {
		return err
	}
	if err := setting.UpdateAutoGroupsByJsonString(saved.auto); err != nil {
		return err
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(saved.groupGroup); err != nil {
		return err
	}
	if err := types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, saved.special); err != nil {
		return err
	}
	return setting.UpdateModelRequestRateLimitGroupByJSONString(saved.rateLimit)
}

func fillAdtyLangs(value string) map[string]string {
	out := make(map[string]string, len(model.AdtyContentLangs))
	for _, lang := range model.AdtyContentLangs {
		out[lang] = value
	}
	return out
}

func disableAdtyRedis(t *testing.T) {
	t.Helper()
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = old
	})
}

func TestDeletedGroupNameStillDisplayed(t *testing.T) {
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.Channel{}, &model.Option{}, &model.AuditLog{}))
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = oldOptions })
	require.NoError(t, prepareAdtyDisplayGroups(t, db))
	require.NoError(t, db.Model(&model.AdtyGroup{}).Where("id = ?", "hidden").Update("names", model.AdtyLangText{"en": "Old plan"}).Error)
	rootToken := adtySessionToken(t, db, "adty-root-del", common.RoleRootUser)
	memberToken := adtySessionToken(t, db, "adty-member-del", common.RoleCommonUser)
	engine := newAdtyGroupEngine()

	memberDelete := adtyJSON(t, engine, http.MethodDelete, "/api/adty/admin/groups/hidden", "", memberToken)
	assert.Equal(t, http.StatusForbidden, memberDelete.Code)
	memberMigrate := adtyJSON(t, engine, http.MethodPost, "/api/adty/admin/groups/hidden/migrate", `{"to":"vip"}`, memberToken)
	assert.Equal(t, http.StatusForbidden, memberMigrate.Code)

	deleted := adtyJSON(t, engine, http.MethodDelete, "/api/adty/admin/groups/hidden", "", rootToken)
	require.Equal(t, http.StatusOK, deleted.Code)
	require.Contains(t, deleted.Body.String(), `"success":true`, deleted.Body.String())

	list := adtyJSON(t, engine, http.MethodGet, "/api/adty/groups/display", "", memberToken)
	require.Equal(t, http.StatusOK, list.Code)
	byID := map[string]adtyDisplayItem{}
	for _, item := range decodeAdtyData[[]adtyDisplayItem](t, list.Body) {
		byID[item.ID] = item
	}
	require.Contains(t, byID, "hidden")
	assert.Equal(t, "Old plan", byID["hidden"].Names["en"])
	assert.False(t, byID["hidden"].Usable)
	assert.False(t, byID["hidden"].Visible)

	anonymous := adtyJSON(t, engine, http.MethodGet, "/api/adty/groups/display", "", "")
	assert.NotContains(t, anonymous.Body.String(), `"hidden"`)

	var logs []model.AuditLog
	require.NoError(t, db.Where("action = ?", "adty.group.delete").Find(&logs).Error)
	assert.Len(t, logs, 1)
}
