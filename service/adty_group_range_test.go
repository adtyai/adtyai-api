package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupModelAllowListRejectsModelsOutsideTheLiveOption(t *testing.T) {
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previous })

	assert.True(t, GroupModelAllowed("vip", "gpt-4"))
	common.OptionMap["GroupModelAllow"] = `{"vip":["gpt-4","gpt-4o"]}`
	assert.True(t, GroupModelAllowed("vip", "gpt-4"))
	assert.False(t, GroupModelAllowed("vip", "hidden-model"))
	assert.True(t, GroupModelAllowed("default", "hidden-model"), "a group with no list keeps the live catalog")
	assert.True(t, GroupModelAllowed("auto", "hidden-model"))
}

func TestSiteGroupAuthorizationUsesTheSiteRecord(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	site := &model.AdtySite{OwnerUserId: 1, Subdomain: "group-auth", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(site).Error)
	assert.True(t, SiteGroupAllowed(site.Id, "vip"))

	require.NoError(t, db.Model(site).Update("allowed_groups", `["default","vip"]`).Error)
	assert.True(t, SiteGroupAllowed(site.Id, "vip"))
	assert.False(t, SiteGroupAllowed(site.Id, "svip"))
	assert.True(t, SiteGroupAllowed(0, "svip"))
}
