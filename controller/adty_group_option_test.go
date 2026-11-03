package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedAdtyGuardGroups 建分组表，只留 default、vip、svip 三个未删除的分组和一个已删除的 gone。
func seedAdtyGuardGroups(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, model.MigrateAdtyTables())
	require.NoError(t, db.Unscoped().Where("1 = 1").Delete(&model.AdtyGroup{}).Error)
	for _, id := range []string{"default", "vip", "svip", "gone"} {
		require.NoError(t, db.Create(&model.AdtyGroup{ID: id, Names: model.AdtyLangText{"en": id}, Descs: model.AdtyLangText{}}).Error)
	}
	require.NoError(t, db.Delete(&model.AdtyGroup{}, "id = ?", "gone").Error)
}

func TestValidateAdtyGroupOption(t *testing.T) {
	db := adtyTestDB(t)
	seedAdtyGuardGroups(t, db)
	cases := []struct {
		name, key, value string
		errContains      string
	}{
		{"all groups present", "GroupRatio", `{"default":1,"vip":1,"svip":0.9}`, ""},
		{"unknown id", "GroupRatio", `{"default":1,"vip":1,"svip":1,"vipp":1}`, "vipp"},
		{"deleted id", "TopupGroupRatio", `{"gone":1}`, "gone"},
		{"missing active group", "GroupRatio", `{"default":1,"vip":1}`, "svip"},
		{"nested inner key", "GroupGroupRatio", `{"vip":{"nope":0.9}}`, "nope"},
		{"nested outer key", "GroupGroupRatio", `{"nope":{"vip":0.9}}`, "nope"},
		{"nested ok", "GroupGroupRatio", `{"vip":{"default":0.9}}`, ""},
		{"special prefixes ok", "group_ratio_setting.group_special_usable_group", `{"vip":{"+:svip":"x","-:default":"","auto":"Auto"}}`, ""},
		{"special prefix unknown", "group_ratio_setting.group_special_usable_group", `{"vip":{"+:nope":"x"}}`, "nope"},
		{"special owner auto", "group_ratio_setting.group_special_usable_group", `{"auto":{"vip":"x"}}`, "auto"},
		{"usable auto ok", "UserUsableGroups", `{"default":"Default","auto":"Auto"}`, ""},
		{"auto groups rejects auto", "AutoGroups", `["default","auto"]`, "auto"},
		{"auto groups unknown", "AutoGroups", `["default","x1"]`, "x1"},
		{"group ratio rejects auto", "GroupRatio", `{"default":1,"vip":1,"svip":1,"auto":1}`, "auto"},
		{"rate limit group", "ModelRequestRateLimitGroup", `{"vip":[10,5],"ghost":[1,1]}`, "ghost"},
		{"other key ignored", "ModelRatio", `{"anything":1}`, ""},
		{"bad json left to upstream", "GroupRatio", `not json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := service.ValidateAdtyGroupOption(tc.key, tc.value)
			if tc.errContains == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestUpdateOptionRejectsUnknownGroupID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	seedAdtyGuardGroups(t, db)
	require.NoError(t, db.Create(&model.Option{Key: "TopupGroupRatio", Value: `{"default":1}`}).Error)

	body, err := common.Marshal(OptionUpdateRequest{Key: "TopupGroupRatio", Value: `{"default":1,"typo":1}`})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))

	UpdateOption(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"message_key":"Unknown group ID {{id}} in {{key}}"`)
	var stored model.Option
	require.NoError(t, db.Where(&model.Option{Key: "TopupGroupRatio"}).First(&stored).Error)
	assert.Equal(t, `{"default":1}`, stored.Value)
}
