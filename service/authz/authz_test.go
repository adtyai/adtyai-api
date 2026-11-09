package authz

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAuthzTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() {
		common.IsMasterNode = wasMaster
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	return db
}

func TestInitSeedsBuiltInRolesAndPoliciesOnce(t *testing.T) {
	db := newAuthzTestDB(t)

	require.NoError(t, Init(db))
	require.NoError(t, Init(db))

	// root is a superuser role and is granted everything implicitly, so only the
	// admin baseline is written as explicit policy rows.
	var count int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Count(&count).Error)
	assert.Equal(t, int64(len(PermissionsForRole(BuiltInRoleAdmin))), count)

	var roles []model.AuthzRole
	require.NoError(t, db.Order("sort asc").Find(&roles).Error)
	require.Len(t, roles, 2)
	assert.Equal(t, BuiltInRoleRoot, roles[0].Key)
	assert.Equal(t, BuiltInRoleAdmin, roles[1].Key)

	assert.True(t, Can(1, common.RoleRootUser, ChannelSensitiveWrite))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelRead))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelOperate))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelWrite))
	assert.False(t, Can(2, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.False(t, Can(3, common.RoleCommonUser, ChannelRead))
}

func TestInitOnSlaveOnlyLoadsPolicies(t *testing.T) {
	wasMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() {
		common.IsMasterNode = wasMaster
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}))

	require.NoError(t, Init(db))

	var roleCount int64
	require.NoError(t, db.Model(&model.AuthzRole{}).Count(&roleCount).Error)
	assert.Equal(t, int64(0), roleCount)
	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Count(&policyCount).Error)
	assert.Equal(t, int64(0), policyCount)
	assert.False(t, Can(2, common.RoleAdminUser, ChannelRead))
}

func TestLegacyScopedPoliciesDoNotExpandPermissions(t *testing.T) {
	for _, master := range []bool{true, false} {
		name := "master"
		if !master {
			name = "slave"
		}
		t.Run(name, func(t *testing.T) {
			db := newAuthzTestDB(t)
			common.IsMasterNode = master
			rules := []model.CasbinRule{
				{Ptype: "p", V0: "role:vendor", V1: "channel", V2: "read", V3: "allow", V4: "own"},
				{Ptype: "p", V0: UserSubject(42), V1: "channel", V2: "read", V3: "allow", V4: "own"},
				{Ptype: "p", V0: UserSubject(43), V1: "channel", V2: "sensitive_write", V3: "allow", V4: "all"},
				{Ptype: "p", V0: UserSubject(44), V1: "channel", V2: "secret_view", V3: "deny", V4: "all"},
				{Ptype: "p", V0: UserSubject(45), V1: "channel", V2: "read"},
				{Ptype: "p", V0: UserSubject(46), V1: "channel", V2: "read", V3: "allow", V4: "unknown-scope"},
				{Ptype: "p", V0: UserSubject(47), V1: "channel", V2: "read", V3: "allow", V5: "own"},
				{Ptype: "p", V0: UserSubject(48), V1: "channel", V2: "read", V3: "allow", V4: "own"},
				{Ptype: "p", V0: UserSubject(48), V1: "channel", V2: "read", V3: "allow", V4: "all"},
				{Ptype: "p", V0: RoleSubject(BuiltInRoleAdmin), V1: "channel", V2: "operate", V3: "allow", V4: "own"},
				{Ptype: "p", V0: UserSubject(50), V1: "channel", V2: "sensitive_write", V3: "allow"},
				{Ptype: "g", V0: UserSubject(99), V1: RoleSubject(BuiltInRoleAdmin)},
			}
			require.NoError(t, db.Create(&rules).Error)
			ids := make([]uint, len(rules))
			for i := range rules {
				ids[i] = rules[i].Id
			}
			for range 2 {
				require.NoError(t, Init(db))
				require.NoError(t, ReloadPolicy())
				assert.False(t, Can(42, common.RoleAdminUser, ChannelRead), "own must not fall back to the admin allow baseline")
				assert.True(t, Can(43, common.RoleAdminUser, ChannelSensitiveWrite))
				assert.False(t, Can(44, common.RoleAdminUser, ChannelSecretView))
				assert.True(t, Can(45, common.RoleAdminUser, ChannelRead))
				for _, userID := range []int{46, 47, 48} {
					assert.False(t, Can(userID, common.RoleAdminUser, ChannelRead))
				}
				assert.False(t, Can(51, common.RoleAdminUser, ChannelOperate), "reseed must not erase a scoped role restriction")
				assert.True(t, Can(50, common.RoleAdminUser, ChannelSensitiveWrite))
				assert.False(t, Can(99, common.RoleCommonUser, ChannelRead))
				var stored []model.CasbinRule
				require.NoError(t, db.Where("id IN ?", ids).Order("id").Find(&stored).Error)
				assert.Equal(t, rules, stored, "legacy rows must remain available for administrator review")
			}
		})
	}
}

func TestSetUserPermissionsStoresExplicitActions(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	require.NoError(t, SetUserPermissions(42, PermissionsMap{
		ResourceChannel: {
			ActionRead:           true,
			ActionOperate:        true,
			ActionWrite:          false,
			ActionSensitiveWrite: true,
			ActionSecretView:     false,
		},
	}))

	assert.True(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.False(t, Can(42, common.RoleAdminUser, ChannelWrite))
	assert.Equal(t, PermissionsMap{
		ResourceChannel: {
			ActionRead:           true,
			ActionOperate:        true,
			ActionWrite:          false,
			ActionSensitiveWrite: true,
			ActionSecretView:     false,
			ActionCreate:         false,
			ActionDelete:         false,
		},
		ResourceTaskPlugin: {
			ActionBind: false,
		},
		ResourceAudit: {ActionRead: false},
	}, ExplicitUserPermissions(42))
	assert.Equal(t, PermissionsMap{
		ResourceChannel: {
			ActionRead:           true,
			ActionOperate:        true,
			ActionSensitiveWrite: true,
			ActionWrite:          false,
			ActionSecretView:     false,
		},
	}, ExplicitUserOverrides(42))

	var userPolicyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ?", UserSubject(42)).Count(&userPolicyCount).Error)
	assert.Equal(t, int64(5), userPolicyCount)

	require.NoError(t, SetUserPermissions(42, PermissionsMap{ResourceChannel: {
		ActionRead:           true,
		ActionOperate:        true,
		ActionWrite:          true,
		ActionSensitiveWrite: false,
		ActionSecretView:     false,
	}}))
	assert.False(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.Equal(t, PermissionsMap{
		ResourceChannel: {
			ActionRead:           true,
			ActionOperate:        true,
			ActionWrite:          true,
			ActionSensitiveWrite: false,
			ActionSecretView:     false,
			ActionCreate:         false,
			ActionDelete:         false,
		},
		ResourceTaskPlugin: {
			ActionBind: false,
		},
		ResourceAudit: {ActionRead: false},
	}, ExplicitUserPermissions(42))
	assert.Equal(t, PermissionsMap{ResourceChannel: {
		ActionRead: true, ActionOperate: true, ActionWrite: true,
		ActionSensitiveWrite: false, ActionSecretView: false,
	}}, ExplicitUserOverrides(42))
}

func TestClearUserAuthorizationRemovesOverrides(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	require.NoError(t, SetUserPermissions(90, PermissionsMap{ResourceChannel: {
		ActionWrite:          false,
		ActionSensitiveWrite: true,
	}}))

	assert.True(t, Can(90, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.False(t, Can(90, common.RoleAdminUser, ChannelWrite))

	require.NoError(t, ClearUserAuthorization(90))

	assert.Empty(t, ExplicitUserOverrides(90))
	assert.True(t, Can(90, common.RoleAdminUser, ChannelRead))
	assert.True(t, Can(90, common.RoleAdminUser, ChannelWrite))
	assert.False(t, Can(90, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.False(t, Can(90, common.RoleCommonUser, ChannelRead))
}

func TestSetUserPermissionsInTxDoesNotMutateEnforcerBeforeReload(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return SetUserPermissionsInTx(tx, 42, PermissionsMap{ResourceChannel: {
			ActionRead:           true,
			ActionOperate:        true,
			ActionWrite:          true,
			ActionSensitiveWrite: true,
			ActionSecretView:     false,
		}})
	}))

	assert.False(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
	require.NoError(t, ReloadPolicy())
	assert.True(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
}

func TestSetUserPermissionsInTxRollbackLeavesNoPolicy(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, SetUserPermissionsInTx(tx, 43, PermissionsMap{ResourceChannel: {
		ActionSensitiveWrite: true,
	}}))
	require.NoError(t, tx.Rollback().Error)
	require.NoError(t, ReloadPolicy())

	assert.False(t, Can(43, common.RoleAdminUser, ChannelSensitiveWrite))
	var count int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ?", UserSubject(43)).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestExplicitActionPatchPreservesOtherAndScopedRules(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))
	require.NoError(t, SetUserPermissions(71, PermissionsMap{ResourceChannel: {ActionWrite: false}}))
	require.NoError(t, SetUserPermissions(71, PermissionsMap{ResourceChannel: {ActionSensitiveWrite: true}}))
	assert.False(t, Can(71, common.RoleAdminUser, ChannelWrite), "a partial patch must preserve an earlier explicit false")
	assert.True(t, Can(71, common.RoleAdminUser, ChannelSensitiveWrite))

	scoped := model.CasbinRule{Ptype: "p", V0: UserSubject(72), V1: ResourceChannel, V2: ActionSensitiveWrite, V3: EffectAllow, V4: "own"}
	require.NoError(t, db.Create(&scoped).Error)
	require.NoError(t, ReloadPolicy())
	require.NoError(t, SetUserPermissions(72, PermissionsMap{ResourceChannel: {ActionSensitiveWrite: true}}))
	assert.False(t, Can(72, common.RoleAdminUser, ChannelSensitiveWrite), "an unscoped editor must not remove a legacy own restriction")
	var retained model.CasbinRule
	require.NoError(t, db.First(&retained, scoped.Id).Error)
	assert.Equal(t, scoped, retained)
}

func TestExplicitActionPatchRejectsUnknownCellsBeforeWriting(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))
	for _, matrix := range []PermissionsMap{
		{ResourceChannel: {ActionRead: false, "unknown": true}},
		{ResourceChannel: {ActionRead: false}, "unknown": {ActionRead: true}},
	} {
		assert.Error(t, SetUserPermissions(73, matrix))
		assert.True(t, Can(73, common.RoleAdminUser, ChannelRead))
		var count int64
		require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ?", UserSubject(73)).Count(&count).Error)
		assert.Zero(t, count, "validation must precede the first write")
	}
}

func TestAdapterAddPolicyIsIdempotent(t *testing.T) {
	db := newAuthzTestDB(t)
	adapter := newGormAdapter(db)
	rule := []string{UserSubject(55), ResourceChannel, ActionSensitiveWrite, EffectAllow}

	require.NoError(t, adapter.AddPolicy("p", "p", rule))
	require.NoError(t, adapter.AddPolicy("p", "p", rule))

	var count int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where(
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ?",
		"p",
		UserSubject(55),
		ResourceChannel,
		ActionSensitiveWrite,
		EffectAllow,
	).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestCapabilitiesUseCatalogShape(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	capabilities := Capabilities(7, common.RoleAdminUser)

	assert.True(t, capabilities[ResourceChannel][ActionRead])
	assert.True(t, capabilities[ResourceChannel][ActionOperate])
	assert.True(t, capabilities[ResourceChannel][ActionWrite])
	assert.False(t, capabilities[ResourceChannel][ActionSensitiveWrite])
	assert.False(t, capabilities[ResourceChannel][ActionSecretView])
	assert.False(t, capabilities[ResourceTaskPlugin][ActionBind])
}

func TestTaskPluginBindIsRootOnlyUntilGranted(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))

	var bindAction *ActionDefinition
	for _, resource := range Catalog() {
		if resource.Resource != ResourceTaskPlugin {
			continue
		}
		assert.Equal(t, "Task Plugin", resource.LabelKey)
		for i := range resource.Actions {
			if resource.Actions[i].Action == ActionBind {
				bindAction = &resource.Actions[i]
			}
		}
	}
	require.NotNil(t, bindAction)
	assert.Equal(t, "Bind task plugins", bindAction.LabelKey)
	assert.Equal(t, "List registered task plugins and bind them when creating or editing task plugin channels.", bindAction.DescriptionKey)
	assert.Empty(t, bindAction.DefaultRoles)

	assert.False(t, Can(2, common.RoleAdminUser, TaskPluginBind))
	assert.True(t, Can(1, common.RoleRootUser, TaskPluginBind))

	enforcer := currentEnforcer()
	require.NotNil(t, enforcer)
	_, err := enforcer.AddPolicy(RoleSubject(BuiltInRoleAdmin), ResourceTaskPlugin, ActionBind, EffectAllow)
	require.NoError(t, err)
	assert.True(t, Can(2, common.RoleAdminUser, TaskPluginBind))

	_, err = enforcer.RemovePolicy(RoleSubject(BuiltInRoleAdmin), ResourceTaskPlugin, ActionBind, EffectAllow)
	require.NoError(t, err)
	assert.False(t, Can(2, common.RoleAdminUser, TaskPluginBind))
}

func TestAdtyActionCatalogStartupBoundary(t *testing.T) {
	for _, scenario := range []string{"master", "slave", "migration_failure", "revoked_restart"} {
		t.Run(scenario, func(t *testing.T) {
			db := newAuthzTestDB(t)
			saved := registry
			AdtyRegisterResources()
			restore := AdtyClearAdminBaseline()
			t.Cleanup(func() { restore(); registry = saved })
			legacy := model.CasbinRule{Ptype: "p", V0: UserSubject(81), V1: ResourceAdtyUser, V2: ActionWrite, V3: EffectAllow}
			require.NoError(t, db.Create(&legacy).Error)
			switch scenario {
			case "master":
				require.NoError(t, Init(db))
				assert.NoError(t, RequireAdtyActionCatalogVersion(db))
				assert.True(t, Can(81, common.RoleAdminUser, AdtyUserCreate), "migration must finish before policies are loaded")
			case "slave":
				common.IsMasterNode = false
				before := currentEnforcer()
				assert.Error(t, Init(db))
				assert.Same(t, before, currentEnforcer())
				var count int64
				require.NoError(t, db.Model(&model.AuthzActionCatalogState{}).Count(&count).Error)
				assert.Zero(t, count)
				require.NoError(t, db.Model(&model.CasbinRule{}).Count(&count).Error)
				assert.EqualValues(t, 1, count)
			case "migration_failure":
				before := currentEnforcer()
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_action_catalog_startup", func(tx *gorm.DB) {
					if tx.Statement.Table == "casbin_rule" {
						tx.AddError(fmt.Errorf("injected action catalog startup failure"))
					}
				}))
				err := Init(db)
				require.NoError(t, db.Callback().Create().Remove("fail_action_catalog_startup"))
				assert.Error(t, err)
				assert.Same(t, before, currentEnforcer(), "failed migration cannot publish an enforcer")
				assert.Error(t, RequireAdtyActionCatalogVersion(db))
			case "revoked_restart":
				require.NoError(t, MigrateAdtyActionCatalog(db))
				require.NoError(t, Init(db))
				require.True(t, Can(81, common.RoleAdminUser, AdtyUserCreate))
				require.NoError(t, SetUserPermissions(81, PermissionsMap{ResourceAdtyUser: {ActionCreate: false}}))
				for range 2 {
					require.NoError(t, Init(db))
					assert.False(t, Can(81, common.RoleAdminUser, AdtyUserCreate))
					assert.True(t, Can(81, common.RoleAdminUser, AdtyUserWrite))
				}
			}
		})
	}
}

func newActionCatalogDatabase(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	if dialect == "sqlite" {
		db := newAuthzTestDB(t)
		var version string
		require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
		t.Logf("database version: SQLite %s", version)
		return db
	}
	opened, err := model.OpenAdtyTestDatabase()
	if err == model.ErrAdtyTestDSNUnset {
		t.Skip("ADTY_TEST_PG_DSN is required for the PostgreSQL action catalog matrix")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	wasMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = wasMaster })
	require.NoError(t, opened.DB.AutoMigrate(&model.CasbinRule{}, &model.AuthzRole{}, &model.AuthzActionCatalogState{}))
	var version string
	require.NoError(t, opened.DB.Raw("SELECT version()").Scan(&version).Error)
	t.Logf("database version: %s", version)
	return opened.DB
}

func TestAdtyActionCatalogMigrationDatabaseMatrix(t *testing.T) {
	saved := registry
	AdtyRegisterResources()
	restore := AdtyClearAdminBaseline()
	t.Cleanup(func() { restore(); registry = saved })
	for _, dialect := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"fresh", "legacy_and_revoke", "rollback", "concurrent"} {
			t.Run(dialect+"/"+scenario, func(t *testing.T) {
				db := newActionCatalogDatabase(t, dialect)
				if scenario == "fresh" {
					assert.Error(t, RequireAdtyActionCatalogVersion(db))
					common.IsMasterNode = false
					before := currentEnforcer()
					assert.Error(t, Init(db))
					assert.Same(t, before, currentEnforcer())
					var stateCount int64
					require.NoError(t, db.Model(&model.AuthzActionCatalogState{}).Count(&stateCount).Error)
					assert.Zero(t, stateCount, "a replica readiness check cannot create a marker")
					common.IsMasterNode = true
					for range 2 {
						require.NoError(t, Init(db))
						require.NoError(t, RequireAdtyActionCatalogVersion(db))
						for _, action := range AllPermissions() {
							assert.False(t, Can(100, common.RoleAdminUser, action), action)
							assert.True(t, Can(1, common.RoleRootUser, action), action)
						}
					}
					return
				}
				legacy := model.CasbinRule{Ptype: "p", V0: UserSubject(101), V1: ResourceAdtyModel, V2: ActionWrite, V3: EffectAllow}
				require.NoError(t, db.Create(&legacy).Error)
				if scenario == "rollback" {
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail_action_catalog_commit", func(tx *gorm.DB) {
						if tx.Statement.Table == "authz_action_catalog_states" {
							tx.AddError(fmt.Errorf("injected action catalog commit failure"))
						}
					}))
					assert.Error(t, MigrateAdtyActionCatalog(db))
					require.NoError(t, db.Callback().Update().Remove("fail_action_catalog_commit"))
					var rows []model.CasbinRule
					require.NoError(t, db.Find(&rows).Error)
					assert.Equal(t, []model.CasbinRule{legacy}, rows)
					var count int64
					require.NoError(t, db.Model(&model.AuthzActionCatalogState{}).Count(&count).Error)
					assert.Zero(t, count, "a failed migration must not leave a completed marker")
					require.NoError(t, Init(db))
					assert.True(t, Can(101, common.RoleAdminUser, AdtyModelCreate))
					return
				}
				if scenario == "concurrent" {
					start := make(chan struct{})
					results := make(chan error, 2)
					var wg sync.WaitGroup
					for range 2 {
						wg.Go(func() { <-start; results <- MigrateAdtyActionCatalog(db) })
					}
					close(start)
					wg.Wait()
					close(results)
					for err := range results {
						require.NoError(t, err)
					}
					var count int64
					require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 = ?", legacy.V0).Count(&count).Error)
					assert.EqualValues(t, 3, count, "the two new actions are expanded once")
					require.NoError(t, Init(db))
					assert.True(t, Can(101, common.RoleAdminUser, AdtyModelDelete))
					return
				}
				mappings := []struct {
					source  Permission
					targets []Permission
				}{
					{AdtyUserWrite, []Permission{AdtyUserCreate}},
					{AdtyModelWrite, []Permission{AdtyModelCreate, AdtyModelDelete}},
					{AdtyLogRead, []Permission{AdtyLogExport}},
					{ChannelSensitiveWrite, []Permission{ChannelCreate, ChannelDelete}},
					{AdtySiteAdjust, []Permission{AdtySiteCreate, AdtySiteDelete}},
					{AdtySiteSettings, []Permission{AdtySiteSettingsRead, AdtySiteSettingsCreate, AdtySiteSettingsDelete}},
					{AdtyReferralSettings, []Permission{AdtyReferralSettingsRead}},
				}
				var rules []model.CasbinRule
				for i, mapping := range mappings {
					rules = append(rules, model.CasbinRule{Ptype: "p", V0: UserSubject(110 + i), V1: mapping.source.Resource, V2: mapping.source.Action, V3: EffectAllow})
				}
				rules = append(rules,
					model.CasbinRule{Ptype: "p", V0: UserSubject(201), V1: ResourceAdtyUser, V2: ActionWrite, V3: EffectAllow},
					model.CasbinRule{Ptype: "p", V0: UserSubject(201), V1: ResourceAdtyUser, V2: ActionCreate, V3: EffectDeny},
					model.CasbinRule{Ptype: "p", V0: UserSubject(202), V1: ResourceAdtyUser, V2: ActionWrite, V3: EffectDeny},
					model.CasbinRule{Ptype: "p", V0: UserSubject(202), V1: ResourceAdtyUser, V2: ActionCreate, V3: EffectAllow},
					model.CasbinRule{Ptype: "p", V0: UserSubject(203), V1: ResourceAdtyModel, V2: ActionWrite, V3: EffectAllow},
					model.CasbinRule{Ptype: "p", V0: UserSubject(203), V1: ResourceAdtyModel, V2: ActionWrite, V3: EffectDeny},
					model.CasbinRule{Ptype: "p", V0: UserSubject(204), V1: ResourceAdtyLog, V2: ActionRead, V3: EffectAllow, V4: "own"},
					model.CasbinRule{Ptype: "p", V0: UserSubject(205), V1: "adty_marketing", V2: ActionWrite, V3: EffectAllow, V5: "own"},
					model.CasbinRule{Ptype: "p", V0: UserSubject(206), V1: ResourceChannel, V2: ActionSensitiveWrite, V3: EffectAllow, V4: "all"},
					model.CasbinRule{Ptype: "p", V0: UserSubject(207), V1: ResourceAdtySite, V2: ActionAdjust},
					model.CasbinRule{Ptype: "p", V0: UserSubject(208), V1: ResourceAdtySite, V2: ActionSettings, V3: EffectAllow},
					model.CasbinRule{Ptype: "p", V0: UserSubject(208), V1: ResourceAdtySite, V2: ActionSettingsRead, V3: EffectAllow, V4: "own"},
				)
				require.NoError(t, db.Create(&rules).Error)
				require.NoError(t, Init(db))
				for i, mapping := range mappings {
					for _, target := range mapping.targets {
						assert.True(t, Can(110+i, common.RoleAdminUser, target), target)
					}
				}
				for _, check := range []struct {
					id    int
					p     Permission
					allow bool
				}{
					{201, AdtyUserCreate, false}, {202, AdtyUserCreate, true},
					{203, AdtyModelCreate, false}, {203, AdtyModelDelete, false},
					{204, AdtyLogExport, false}, {205, Permission{Resource: "adty_marketing", Action: ActionCreate}, false},
					{206, ChannelCreate, true}, {206, ChannelDelete, true},
					{207, AdtySiteCreate, true}, {208, AdtySiteSettingsRead, false},
					{208, AdtySiteSettingsCreate, true},
				} {
					assert.Equal(t, check.allow, Can(check.id, common.RoleAdminUser, check.p), check)
				}
				for _, rule := range rules {
					var retained model.CasbinRule
					require.NoError(t, db.First(&retained, rule.Id).Error)
					assert.Equal(t, rule, retained)
				}
				require.NoError(t, SetUserPermissions(101, PermissionsMap{ResourceAdtyModel: {ActionCreate: false}}))
				// A versionless legacy matrix contains only the old cell; it must
				// neither erase nor re-expand a subsequently revoked target action.
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					return SetUserPermissionsInTx(tx, 101, PermissionsMap{ResourceAdtyModel: {ActionWrite: true}})
				}))
				// Absence is the empty admin baseline, too. A durable marker must
				// prevent filling a removed destination from the retained old cell.
				require.NoError(t, db.Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?", "p", UserSubject(110), ResourceAdtyUser, ActionCreate).Delete(&model.CasbinRule{}).Error)
				for range 2 {
					require.NoError(t, Init(db))
					assert.False(t, Can(101, common.RoleAdminUser, AdtyModelCreate))
					assert.True(t, Can(101, common.RoleAdminUser, AdtyModelWrite))
					assert.True(t, Can(101, common.RoleAdminUser, AdtyModelDelete))
					assert.False(t, Can(110, common.RoleAdminUser, AdtyUserCreate))
					assert.True(t, Can(110, common.RoleAdminUser, AdtyUserWrite))
				}
			})
		}
	}
}
