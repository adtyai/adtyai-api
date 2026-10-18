package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useMenuVisibilityMergeDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)

	common.OptionMapRWMutex.Lock()
	prevMap := common.OptionMap
	common.OptionMap = map[string]string{
		OptSidebarModulesAdmin:    "",
		OptSidebarUserVisible:     "{}",
		OptSidebarAdminVisible:    "{}",
		OptMenuVisibilityMergedAt: "",
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = prevMap
		common.OptionMapRWMutex.Unlock()
	})
	return db
}

func setOptionMapKV(key, value string) {
	common.OptionMapRWMutex.Lock()
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
}

func TestApplyMenuVisibilityFalse_OnlyFalseDisables(t *testing.T) {
	modules := SidebarModulesAdmin{
		"personal": {"enabled": true, "topup": true, "personal": true},
		"admin":    {"enabled": true, "channel": true},
	}
	user := map[string]bool{"wallet": false, "profile": true} // profile true ignored
	admin := map[string]bool{"channels": false}
	ApplyMenuVisibilityFalseToModules(modules, user, admin)
	assert.False(t, modules["personal"]["topup"])
	assert.True(t, modules["personal"]["personal"]) // not flipped by profile:true
	assert.False(t, modules["admin"]["channel"])
}

func TestApplyMenuVisibilityFalse_WalletOrAffiliateDisablesTopup(t *testing.T) {
	cases := []struct {
		name string
		user map[string]bool
	}{
		{"wallet_false", map[string]bool{"wallet": false, "affiliate": true}},
		{"affiliate_false", map[string]bool{"wallet": true, "affiliate": false}},
		{"both_false", map[string]bool{"wallet": false, "affiliate": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modules := SidebarModulesAdmin{"personal": {"enabled": true, "topup": true}}
			ApplyMenuVisibilityFalseToModules(modules, tc.user, nil)
			assert.False(t, modules["personal"]["topup"])
		})
	}
}

func TestApplyMenuVisibilityFalse_DefaultVisibleDoesNotWriteTrue(t *testing.T) {
	// Existing false must stay false when Visible is empty / all true (禁止 silent 全开).
	modules := SidebarModulesAdmin{
		"admin": {"enabled": true, "channel": false, "models": true},
	}
	ApplyMenuVisibilityFalseToModules(modules, map[string]bool{}, map[string]bool{"models": true})
	assert.False(t, modules["admin"]["channel"])
	assert.True(t, modules["admin"]["models"])
}

func TestApplyMenuVisibilityFalse_UnknownKeySnakeCase(t *testing.T) {
	modules := SidebarModulesAdmin{}
	ApplyMenuVisibilityFalseToModules(modules, map[string]bool{"my-custom": false}, map[string]bool{"weird-admin": false})
	assert.False(t, modules["personal"]["my_custom"])
	assert.False(t, modules["admin"]["weird_admin"])
}

func TestApplyMenuVisibilityFalse_PluginsAlias(t *testing.T) {
	modules := SidebarModulesAdmin{"admin": {"enabled": true, "task_plugins": true}}
	ApplyMenuVisibilityFalseToModules(modules, nil, map[string]bool{"plugins": false})
	assert.False(t, modules["admin"]["task_plugins"])
}

func TestMigrateMenuVisibility_FalseToDisableAndClear(t *testing.T) {
	db := useMenuVisibilityMergeDB(t)
	existing := `{"admin":{"enabled":true,"channel":false,"models":true},"personal":{"enabled":true,"topup":true}}`
	require.NoError(t, db.Create(&[]Option{
		{Key: OptSidebarModulesAdmin, Value: existing},
		{Key: OptSidebarUserVisible, Value: `{"wallet":false,"profile":true}`},
		{Key: OptSidebarAdminVisible, Value: `{"channels":false,"users":false}`},
		{Key: OptMenuVisibilityMergedAt, Value: ""},
	}).Error)
	setOptionMapKV(OptSidebarModulesAdmin, existing)
	setOptionMapKV(OptSidebarUserVisible, `{"wallet":false,"profile":true}`)
	setOptionMapKV(OptSidebarAdminVisible, `{"channels":false,"users":false}`)
	setOptionMapKV(OptMenuVisibilityMergedAt, "")

	require.NoError(t, MigrateMenuVisibilityIntoSidebarModules())

	var modulesOpt, userOpt, adminOpt, mergedOpt Option
	require.NoError(t, db.Where(&Option{Key: OptSidebarModulesAdmin}).First(&modulesOpt).Error)
	require.NoError(t, db.Where(&Option{Key: OptSidebarUserVisible}).First(&userOpt).Error)
	require.NoError(t, db.Where(&Option{Key: OptSidebarAdminVisible}).First(&adminOpt).Error)
	require.NoError(t, db.Where(&Option{Key: OptMenuVisibilityMergedAt}).First(&mergedOpt).Error)

	assert.Equal(t, "{}", userOpt.Value)
	assert.Equal(t, "{}", adminOpt.Value)
	assert.NotEmpty(t, mergedOpt.Value)

	parsed, err := ParseSidebarModulesAdmin(modulesOpt.Value)
	require.NoError(t, err)
	assert.False(t, parsed["personal"]["topup"], "wallet false → personal.topup false")
	assert.False(t, parsed["admin"]["channel"], "existing false preserved + Visible channels false")
	assert.False(t, parsed["admin"]["user"], "users false → admin.user false")
	assert.True(t, parsed["admin"]["models"], "untouched stays true")
	_, hasPersonal := parsed["personal"]["personal"]
	assert.False(t, hasPersonal, "profile:true must not introduce personal.personal")
}

func TestMigrateMenuVisibility_DefaultVisibleNoSilentOpen(t *testing.T) {
	db := useMenuVisibilityMergeDB(t)
	existing := `{"admin":{"enabled":true,"channel":false}}`
	require.NoError(t, db.Create(&[]Option{
		{Key: OptSidebarModulesAdmin, Value: existing},
		{Key: OptSidebarUserVisible, Value: `{}`},
		{Key: OptSidebarAdminVisible, Value: `{}`},
	}).Error)
	setOptionMapKV(OptSidebarModulesAdmin, existing)
	setOptionMapKV(OptSidebarUserVisible, "{}")
	setOptionMapKV(OptSidebarAdminVisible, "{}")
	setOptionMapKV(OptMenuVisibilityMergedAt, "")

	require.NoError(t, MigrateMenuVisibilityIntoSidebarModules())

	var modulesOpt Option
	require.NoError(t, db.Where(&Option{Key: OptSidebarModulesAdmin}).First(&modulesOpt).Error)
	parsed, err := ParseSidebarModulesAdmin(modulesOpt.Value)
	require.NoError(t, err)
	assert.False(t, parsed["admin"]["channel"], "empty Visible must not silent-open existing false")
}

func TestMigrateMenuVisibility_Idempotent(t *testing.T) {
	db := useMenuVisibilityMergeDB(t)
	require.NoError(t, db.Create(&[]Option{
		{Key: OptSidebarModulesAdmin, Value: `{}`},
		{Key: OptSidebarUserVisible, Value: `{"wallet":false}`},
		{Key: OptSidebarAdminVisible, Value: `{}`},
	}).Error)
	setOptionMapKV(OptSidebarModulesAdmin, `{}`)
	setOptionMapKV(OptSidebarUserVisible, `{"wallet":false}`)
	setOptionMapKV(OptSidebarAdminVisible, `{}`)
	setOptionMapKV(OptMenuVisibilityMergedAt, "")

	require.NoError(t, MigrateMenuVisibilityIntoSidebarModules())
	var firstMerged, firstModules Option
	require.NoError(t, db.Where(&Option{Key: OptMenuVisibilityMergedAt}).First(&firstMerged).Error)
	require.NoError(t, db.Where(&Option{Key: OptSidebarModulesAdmin}).First(&firstModules).Error)

	// Simulate someone writing Visible again after merge — second run must skip.
	require.NoError(t, db.Model(&Option{}).Where("key = ?", OptSidebarUserVisible).Update("value", `{"wallet":false,"profile":false}`).Error)
	setOptionMapKV(OptSidebarUserVisible, `{"wallet":false,"profile":false}`)
	// Keep MergedAt in OptionMap as written by UpdateOptionsBulk
	require.NoError(t, MigrateMenuVisibilityIntoSidebarModules())

	var secondMerged, secondModules, userOpt Option
	require.NoError(t, db.Where(&Option{Key: OptMenuVisibilityMergedAt}).First(&secondMerged).Error)
	require.NoError(t, db.Where(&Option{Key: OptSidebarModulesAdmin}).First(&secondModules).Error)
	require.NoError(t, db.Where(&Option{Key: OptSidebarUserVisible}).First(&userOpt).Error)
	assert.Equal(t, firstMerged.Value, secondMerged.Value)
	assert.JSONEq(t, firstModules.Value, secondModules.Value)
	assert.Equal(t, `{"wallet":false,"profile":false}`, userOpt.Value, "idempotent skip must not clear again")
}

func TestParseAndMergeSidebarModulesAdmin_Defaults(t *testing.T) {
	empty, err := ParseSidebarModulesAdmin("")
	require.NoError(t, err)
	merged := MergeSidebarModulesAdmin(empty)
	assert.True(t, merged["chat"]["playground"])
	assert.True(t, merged["admin"]["smart_routing"])
	assert.True(t, merged["admin"]["system_info"])

	partial, err := ParseSidebarModulesAdmin(`{"admin":{"channel":false}}`)
	require.NoError(t, err)
	merged2 := MergeSidebarModulesAdmin(partial)
	assert.False(t, merged2["admin"]["channel"])
	assert.True(t, merged2["admin"]["models"]) // default filled
}

func TestMarshalRoundTrip(t *testing.T) {
	m := DefaultSidebarModulesAdmin()
	m["admin"]["orders"] = false
	raw, err := MarshalSidebarModulesAdmin(m)
	require.NoError(t, err)
	var check map[string]map[string]bool
	require.NoError(t, json.Unmarshal([]byte(raw), &check))
	assert.False(t, check["admin"]["orders"])
}
