package model

import (
	"testing"

	"os"
	"regexp"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAdtyContentLangsIncludeKorean(t *testing.T) {
	assert.Contains(t, AdtyContentLangs, "ko")
}

func TestAdtyTestDBUsesDistinctSchemasAndDropsThem(t *testing.T) {
	dsn := os.Getenv("ADTY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	var schemas []string
	for _, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			db := adtyTestDB(t)
			var schema string
			require.NoError(t, db.Raw("SELECT current_schema()").Scan(&schema).Error)
			require.NotEmpty(t, schema)
			require.NotEqual(t, "public", schema)
			schemas = append(schemas, schema)
			require.NoError(t, db.Exec("CREATE TABLE adty_probe (id int)").Error)
		})
	}
	require.Len(t, schemas, 2)
	assert.NotEqual(t, schemas[0], schemas[1])

	admin, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, sqlErr := admin.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	})
	var left int
	require.NoError(t, admin.Raw(
		"SELECT COUNT(*) FROM pg_namespace WHERE nspname IN (?, ?)",
		schemas[0], schemas[1],
	).Scan(&left).Error)
	assert.Zero(t, left)
}

func TestAdtyCheckPostgreSQL(t *testing.T) {
	oldExit := adtyExit
	t.Cleanup(func() { adtyExit = oldExit })
	exited := 0
	adtyExit = func(code int) {
		exited = code
		panic("adty-exit")
	}
	pg := "postgres://u:p@127.0.0.1:5432/db"
	cases := []struct {
		name, main, log string
		exit            bool
	}{
		{"both postgres", pg, pg, false},
		{"log shares main", "postgresql://u@h/db", "", false},
		{"sqlite main", "", "", true},
		{"mysql main", "root:p@tcp(127.0.0.1:3306)/db", "", true},
		{"mysql log", pg, "root:p@tcp(127.0.0.1:3306)/logs", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SQL_DSN", tc.main)
			t.Setenv("LOG_SQL_DSN", tc.log)
			exited = 0
			if tc.exit {
				require.Panics(t, CheckAdtyPostgreSQL)
				assert.Equal(t, 1, exited)
				return
			}
			require.NotPanics(t, CheckAdtyPostgreSQL)
			assert.Zero(t, exited)
		})
	}
}

func TestCIWorkflowRunsPostgreSQL(t *testing.T) {
	for _, fixture := range []struct {
		file string
		jobs []string
	}{
		{"ci.yml", []string{"backend", "e2e"}},
		{"adty-ghcr.yml", []string{"test"}},
	} {
		data, err := os.ReadFile("../.github/workflows/" + fixture.file)
		require.NoError(t, err)
		var workflow struct {
			Jobs map[string]struct {
				Services map[string]struct{ Image string }
				Env      map[string]string
			}
		}
		require.NoError(t, yaml.Unmarshal(data, &workflow))
		for _, name := range fixture.jobs {
			job, ok := workflow.Jobs[name]
			require.True(t, ok, "%s/%s", fixture.file, name)
			require.Equal(t, "postgres:17", job.Services["postgres"].Image, "%s/%s", fixture.file, name)
			require.NotEmpty(t, job.Env["ADTY_TEST_PG_DSN"], "%s/%s", fixture.file, name)
		}
	}
	for _, fixture := range []struct{ file, image string }{
		{"docker-compose.yml", "postgres:17"},
		{"docker-compose.dev.yml", "postgres:17-alpine"},
		{"deploy/adty/docker-compose.yml", "postgres:17"},
	} {
		data, err := os.ReadFile("../" + fixture.file)
		require.NoError(t, err)
		var compose struct {
			Services map[string]struct{ Image string }
		}
		require.NoError(t, yaml.Unmarshal(data, &compose))
		require.Equal(t, fixture.image, compose.Services["postgres"].Image, fixture.file)
	}
}

func TestSeedAdtyGroupsOnce(t *testing.T) {
	db := adtyTestDB(t)
	restore := snapshotAdtyGroupSettings(t)
	t.Cleanup(restore)
	require.NoError(t, applyAdtyGroupSettings(adtyGroupSettingSnapshot{
		groupRatio: `{"vip":1.5,"extra":2}`,
		usable:     `{"vip":"VIP group"}`,
		topup:      `{"wallet":1}`,
		auto:       `["auto-only"]`,
		groupGroup: `{"outer":{"inner":0.5}}`,
		special:    `{"owner":{"+:added":"Added","-:removed":"gone","plain":"Plain"}}`,
		rateLimit:  `{"limited":[1,2]}`,
	}))
	require.NoError(t, migrateAdtyTables())
	require.NoError(t, SeedAdtyGroups())

	var vip AdtyGroup
	require.NoError(t, db.First(&vip, "id = ?", "vip").Error)
	for _, lang := range AdtyContentLangs {
		assert.Equal(t, "vip", vip.Names[lang], lang)
		assert.Equal(t, "VIP group", vip.Descs[lang], lang)
	}
	var extra AdtyGroup
	require.NoError(t, db.First(&extra, "id = ?", "extra").Error)
	assert.Equal(t, "", extra.Descs["en"])
	for _, id := range []string{"default", "wallet", "auto-only", "outer", "inner", "owner", "added", "removed", "plain", "limited"} {
		assert.NoError(t, db.First(&AdtyGroup{}, "id = ?", id).Error, id)
	}

	vip.Names["en"] = "renamed"
	require.NoError(t, db.Save(&vip).Error)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":1.5,"brand-new":1}`))
	require.NoError(t, SeedAdtyGroups())

	require.NoError(t, db.First(&vip, "id = ?", "vip").Error)
	assert.Equal(t, "renamed", vip.Names["en"])
	var count int64
	require.NoError(t, db.Model(&AdtyGroup{}).Where("id = ?", "brand-new").Count(&count).Error)
	assert.Zero(t, count)
}

func TestNewAdtyGroupIDFormat(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, migrateAdtyTables())
	pattern := regexp.MustCompile(`^g[a-z0-9]{4}$`)
	for range 1000 {
		id, err := NewAdtyGroupID(db)
		require.NoError(t, err)
		require.Regexp(t, pattern, id)
		require.NotEqual(t, "auto", id)
		require.NotEqual(t, "default", id)
	}
}

func TestCreateAdtyGroupUniqueID(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, migrateAdtyTables())
	clearAdtyGroups(t, db)

	seen := map[string]struct{}{}
	for range 20 {
		created, err := CreateAdtyGroup("名字", "")
		require.NoError(t, err)
		assert.Regexp(t, `^g[a-z0-9]{4}$`, created.ID)
		_, dup := seen[created.ID]
		require.False(t, dup, created.ID)
		seen[created.ID] = struct{}{}
		assert.Equal(t, "名字", created.Names["ja"])
	}

	// 软删除的 ID 和设置里出现过的键都不能再生成。
	deleted, err := CreateAdtyGroup("删掉", "")
	require.NoError(t, err)
	require.NoError(t, db.Delete(&AdtyGroup{}, "id = ?", deleted.ID).Error)
	oldTaken := adtyGroupIDTaken
	t.Cleanup(func() { adtyGroupIDTaken = oldTaken })
	adtyGroupIDTaken = func() (map[string]string, error) { return map[string]string{"gabcd": ""}, nil }
	ids := map[string]struct{}{}
	for range 200 {
		id, err := NewAdtyGroupID(db)
		require.NoError(t, err)
		ids[id] = struct{}{}
	}
	assert.NotContains(t, ids, deleted.ID)
	assert.NotContains(t, ids, "gabcd")

	_, err = CreateAdtyGroup("  ", "说明")
	require.ErrorIs(t, err, ErrAdtyGroupTextRequired)
}

func TestUpdateAdtyGroupTextKeepsID(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, migrateAdtyTables())
	clearAdtyGroups(t, db)
	created, err := CreateAdtyGroup("旧名", "旧说明")
	require.NoError(t, err)

	names := adtyFillLangs("新名")
	names["en"] = " "
	_, err = UpdateAdtyGroupTexts(created.ID, names, adtyFillLangs("新说明"))
	require.ErrorIs(t, err, ErrAdtyGroupTextRequired)
	var stored AdtyGroup
	require.NoError(t, db.First(&stored, "id = ?", created.ID).Error)
	assert.Equal(t, "旧名", stored.Names["en"])

	updated, err := UpdateAdtyGroupTexts(created.ID, AdtyLangText{"en": "New", "zh-CN": " 新名 ", "ko": " 한국어 ", "xx": "ignored"}, AdtyLangText{})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "新名", updated.Names["zh-CN"])
	assert.Equal(t, "한국어", updated.Names["ko"])
	assert.Equal(t, "", updated.Names["fr"])
	assert.NotContains(t, updated.Names, "xx")

	_, err = UpdateAdtyGroupTexts("gnone", AdtyLangText{"en": "x"}, nil)
	require.ErrorIs(t, err, ErrAdtyGroupNotFound)
}

func TestClearAdtyPlaceholderGroupRatio(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = oldOptions })
	restore := snapshotAdtyGroupSettings(t)
	t.Cleanup(restore)
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"edit_this":0.9},"svip":{"vip":0.8,"edit_this":1}}`))

	require.NoError(t, ClearAdtyPlaceholderGroupRatio())
	assert.JSONEq(t, `{"svip":{"vip":0.8}}`, ratio_setting.GroupGroupRatio2JSONString())
	var stored Option
	require.NoError(t, db.First(&stored, commonKeyCol+" = ?", "GroupGroupRatio").Error)
	assert.JSONEq(t, `{"svip":{"vip":0.8}}`, stored.Value)

	// 已经清过就不再写库。
	require.NoError(t, db.Where("1 = 1").Delete(&Option{}).Error)
	require.NoError(t, ClearAdtyPlaceholderGroupRatio())
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCountAdtyGroupRefs(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, migrateAdtyTables())
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}))
	clearAdtyGroups(t, db)
	created, err := CreateAdtyGroup("被引用", "说明")
	require.NoError(t, err)
	id := created.ID

	user := &User{Username: "ref-user", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: id, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	deletedUser := &User{Username: "ref-gone", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: id, AuthVersion: 1}
	require.NoError(t, db.Create(deletedUser).Error)
	require.NoError(t, db.Delete(deletedUser).Error)

	require.NoError(t, db.Create(&Token{UserId: user.Id, Key: "tok-group", Name: "direct", Group: id, Status: 1}).Error)
	autoOnly := &Token{UserId: user.Id, Key: "tok-auto", Name: "auto", Group: "other", Status: 1}
	require.NoError(t, autoOnly.SetAutoGroups([]string{id, "other"}))
	require.NoError(t, db.Create(autoOnly).Error)
	both := &Token{UserId: user.Id, Key: "tok-both", Name: "both", Group: id, Status: 1}
	require.NoError(t, both.SetAutoGroups([]string{id}))
	require.NoError(t, db.Create(both).Error)
	deletedToken := &Token{UserId: user.Id, Key: "tok-del", Name: "del", Group: id, Status: 1}
	require.NoError(t, db.Create(deletedToken).Error)
	require.NoError(t, db.Delete(deletedToken).Error)
	lookalike := &Token{UserId: user.Id, Key: "tok-like", Name: "like", Group: id + "x", Status: 1}
	require.NoError(t, lookalike.SetAutoGroups([]string{id + "0"}))
	require.NoError(t, db.Create(lookalike).Error)

	require.NoError(t, db.Create(&Channel{Key: "ch-hit", Name: "hit", Group: id + ",other", Status: 1}).Error)
	require.NoError(t, db.Create(&Channel{Key: "ch-mid", Name: "mid", Group: "other," + id, Status: 1}).Error)
	require.NoError(t, db.Create(&Channel{Key: "ch-miss", Name: "miss", Group: id + "x,other", Status: 1}).Error)

	refs, err := CountAdtyGroupRefs(id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), refs.Users)
	assert.Equal(t, int64(3), refs.Tokens)
	assert.Equal(t, int64(2), refs.Channels)
}

type adtyGroupSettingSnapshot struct {
	groupRatio string
	usable     string
	topup      string
	auto       string
	groupGroup string
	special    string
	rateLimit  string
}

func snapshotAdtyGroupSettings(t *testing.T) func() {
	t.Helper()
	saved := adtyGroupSettingSnapshot{
		groupRatio: ratio_setting.GroupRatio2JSONString(),
		usable:     setting.UserUsableGroups2JSONString(),
		topup:      common.TopupGroupRatio2JSONString(),
		auto:       setting.AutoGroups2JsonString(),
		groupGroup: ratio_setting.GroupGroupRatio2JSONString(),
		special:    ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString(),
		rateLimit:  setting.ModelRequestRateLimitGroup2JSONString(),
	}
	return func() {
		require.NoError(t, applyAdtyGroupSettings(saved))
	}
}

func applyAdtyGroupSettings(saved adtyGroupSettingSnapshot) error {
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

func clearAdtyGroups(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Unscoped().Where("1 = 1").Delete(&AdtyGroup{}).Error)
}

func prepareAdtyMigrateGroups(t *testing.T, db *gorm.DB, ids ...string) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}, &Option{}))
	require.NoError(t, migrateAdtyTables())
	clearAdtyGroups(t, db)
	for _, id := range ids {
		require.NoError(t, db.Create(&AdtyGroup{ID: id, Names: AdtyLangText{"en": id}, Descs: AdtyLangText{}}).Error)
	}
}

func TestDeleteAdtyGroupRejectsAssignmentMadeWhileLocked(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "going")
	user := &User{Username: "grp-lock", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)

	started := make(chan struct{})
	deleted := make(chan error, 1)
	go func() {
		deleted <- DB.Transaction(func(tx *gorm.DB) error {
			if err := lockLiveAdtyGroupsTx(tx, "going"); err != nil {
				return err
			}
			close(started)
			time.Sleep(300 * time.Millisecond)
			refs, err := countAdtyGroupRefs(tx, "going")
			if err != nil {
				return err
			}
			if refs.Total() > 0 {
				return ErrAdtyGroupInUse
			}
			return tx.Where("id = ?", "going").Delete(&AdtyGroup{}).Error
		})
	}()
	<-started
	assignErr := DB.Transaction(func(tx *gorm.DB) error {
		user.Group = "going"
		return user.EditWithTx(tx, false)
	})
	require.NoError(t, <-deleted)
	require.ErrorIs(t, assignErr, ErrAdtyGroupNotFound)
	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "default", stored.Group)
}

func TestMigrateAdtyGroupMovesRefs(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "gfrom", "gto", "other")

	var users []*User
	for _, name := range []string{"mig-a", "mig-b"} {
		user := &User{Username: name, Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "gfrom", AuthVersion: 7}
		require.NoError(t, db.Create(user).Error)
		users = append(users, user)
	}
	direct := &Token{UserId: users[0].Id, Key: "mig-direct", Name: "direct", Group: "gfrom", Status: 1}
	require.NoError(t, db.Create(direct).Error)
	autoOnly := &Token{UserId: users[1].Id, Key: "mig-auto", Name: "auto", Group: "other", Status: 1}
	require.NoError(t, autoOnly.SetAutoGroups([]string{"gfrom", "gto", "other"}))
	require.NoError(t, db.Create(autoOnly).Error)
	untouched := &Token{UserId: users[1].Id, Key: "mig-none", Name: "none", Group: "other", Status: 1}
	require.NoError(t, db.Create(untouched).Error)
	channel := &Channel{Key: "mig-ch", Name: "mig", Group: "gfrom,other", Models: "gpt-x", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	moved, err := MigrateAdtyGroup("gfrom", "gto")
	require.NoError(t, err)
	assert.Equal(t, AdtyGroupRefs{Users: 2, Tokens: 2, Channels: 1}, moved)

	for _, user := range users {
		var stored User
		require.NoError(t, db.First(&stored, user.Id).Error)
		assert.Equal(t, "gto", stored.Group)
		assert.EqualValues(t, 7, stored.AuthVersion, "migration must not log users out")
	}
	var storedDirect, storedAuto Token
	require.NoError(t, db.First(&storedDirect, direct.Id).Error)
	assert.Equal(t, "gto", storedDirect.Group)
	require.NoError(t, db.First(&storedAuto, autoOnly.Id).Error)
	groups, err := storedAuto.GetAutoGroups()
	require.NoError(t, err)
	assert.Equal(t, []string{"gto", "other"}, groups, "duplicates are removed")
	var storedChannel Channel
	require.NoError(t, db.First(&storedChannel, channel.Id).Error)
	assert.Equal(t, "gto,other", storedChannel.Group)
	var abilityGroups []string
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Order(commonGroupCol).Pluck(commonGroupCol, &abilityGroups).Error)
	assert.Equal(t, []string{"gto", "other"}, abilityGroups)

	refs, err := CountAdtyGroupRefs("gfrom")
	require.NoError(t, err)
	assert.Zero(t, refs.Total())
}

func TestMigrateAdtyGroupRollsBack(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "gfrom")
	user := &User{Username: "mig-rb", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "gfrom", AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)

	_, err := MigrateAdtyGroup("gfrom", "gmissing")
	require.ErrorIs(t, err, ErrAdtyGroupNotFound)
	_, err = MigrateAdtyGroup("gfrom", "gfrom")
	require.ErrorIs(t, err, ErrAdtyGroupSameTarget)

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "gfrom", stored.Group)
}

func TestDeleteAdtyGroupKeepsRatioSavedInTheDatabase(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "vip", "gdel")
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = oldOptions })
	restore := snapshotAdtyGroupSettings(t)
	t.Cleanup(restore)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, db.Where(commonKeyCol+" = ?", "GroupRatio").Delete(&Option{}).Error)
	require.NoError(t, db.Create(&Option{Key: "GroupRatio", Value: `{"default":1,"vip":9,"gdel":0.5}`}).Error)

	require.NoError(t, DeleteAdtyGroup("gdel"))

	assert.JSONEq(t, `{"default":1,"vip":9}`, ratio_setting.GroupRatio2JSONString())
	var stored Option
	require.NoError(t, db.Where(commonKeyCol+" = ?", "GroupRatio").First(&stored).Error)
	assert.JSONEq(t, `{"default":1,"vip":9}`, stored.Value)
}

func TestDeleteAdtyGroupRequiresNoRefs(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "gbusy")
	require.NoError(t, db.Create(&Channel{Key: "del-ch", Name: "busy", Group: "gbusy", Status: 1}).Error)

	require.ErrorIs(t, DeleteAdtyGroup("gbusy"), ErrAdtyGroupInUse)
	_, err := GetAdtyGroup("gbusy")
	require.NoError(t, err)
	require.ErrorIs(t, DeleteAdtyGroup("default"), ErrAdtyGroupUndeletable)
	require.ErrorIs(t, DeleteAdtyGroup("gnone"), ErrAdtyGroupNotFound)
}

func TestDeleteAdtyGroupCleansOptions(t *testing.T) {
	db := adtyTestDB(t)
	prepareAdtyMigrateGroups(t, db, "default", "vip", "gdel")
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = oldOptions })
	restore := snapshotAdtyGroupSettings(t)
	t.Cleanup(restore)
	require.NoError(t, applyAdtyGroupSettings(adtyGroupSettingSnapshot{
		groupRatio: `{"default":1,"vip":1,"gdel":0.5}`,
		usable:     `{"default":"Default","gdel":"Gone"}`,
		topup:      `{"gdel":1.1}`,
		auto:       `["gdel","default"]`,
		groupGroup: `{"gdel":{"vip":0.9},"vip":{"gdel":0.8,"default":0.7}}`,
		special:    `{"gdel":{"+:vip":"x"},"vip":{"-:gdel":"","+:default":"y"}}`,
		rateLimit:  `{"gdel":[1,1],"vip":[5,5]}`,
	}))

	require.NoError(t, DeleteAdtyGroup("gdel"))

	assert.JSONEq(t, `{"default":1,"vip":1}`, ratio_setting.GroupRatio2JSONString())
	assert.JSONEq(t, `{"default":"Default"}`, setting.UserUsableGroups2JSONString())
	assert.JSONEq(t, `{}`, common.TopupGroupRatio2JSONString())
	assert.Equal(t, []string{"default"}, setting.GetAutoGroups())
	assert.JSONEq(t, `{"vip":{"default":0.7}}`, ratio_setting.GroupGroupRatio2JSONString())
	assert.JSONEq(t, `{"vip":{"+:default":"y"}}`, ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString())
	assert.JSONEq(t, `{"vip":[5,5]}`, setting.ModelRequestRateLimitGroup2JSONString())
	var stored Option
	require.NoError(t, db.First(&stored, commonKeyCol+" = ?", "ModelRequestRateLimitGroup").Error)
	assert.JSONEq(t, `{"vip":[5,5]}`, stored.Value)

	_, err := GetAdtyGroup("gdel")
	require.ErrorIs(t, err, ErrAdtyGroupNotFound)
	all, err := ListAdtyGroups(true)
	require.NoError(t, err)
	var kept bool
	for _, group := range all {
		if group.ID == "gdel" {
			kept = group.Deleted()
		}
	}
	assert.True(t, kept, "the deleted row stays for old logs")
	active, err := ActiveAdtyGroupIDs()
	require.NoError(t, err)
	assert.NotContains(t, active, "gdel")
}
