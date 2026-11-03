package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 群发接口按营销的三个权限分开：查看能看列表和预计人数，编辑能存草稿，发送才能发、测试发、取消（任务 12.2）。
func TestCampaignPermissions(t *testing.T) {
	f := newAdtyGuardFixture(t)
	campaign := model.AdtyMarketingCampaign{Name: "draft", Status: model.AdtyCampaignStatusDraft,
		Subjects: model.AdtyLangText{"en": "Hi"}, Bodies: model.AdtyLangText{"en": "<p>Hi</p>"}, CreatedBy: f.admin.Id}
	require.NoError(t, f.db.Create(&campaign).Error)
	id := common.Interface2String(campaign.Id)
	body := `{"name":"n","subjects":{"en":"s"},"bodies":{"en":"<p>b</p>"},"audience":{}}`
	requests := []struct {
		method, path, body string
		need               authz.Permission
	}{
		{http.MethodGet, "/api/adty/admin/marketing/campaigns", "", authz.AdtyMarketingRead},
		{http.MethodGet, "/api/adty/admin/marketing/campaigns/" + id, "", authz.AdtyMarketingRead},
		{http.MethodPost, "/api/adty/admin/marketing/audience/estimate", `{}`, authz.AdtyMarketingRead},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns", body, authz.AdtyMarketingWrite},
		{http.MethodPut, "/api/adty/admin/marketing/campaigns/" + id, body, authz.AdtyMarketingWrite},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/test", `{"to":"me@example.com","lang":"en"}`, authz.AdtyMarketingSend},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/send", `{"confirm":true}`, authz.AdtyMarketingSend},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/cancel", `{}`, authz.AdtyMarketingSend},
	}
	others := map[authz.Permission][]authz.Permission{
		authz.AdtyMarketingRead:  {authz.AdtyMarketingWrite, authz.AdtyMarketingSend},
		authz.AdtyMarketingWrite: {authz.AdtyMarketingRead, authz.AdtyMarketingSend},
		authz.AdtyMarketingSend:  {authz.AdtyMarketingRead, authz.AdtyMarketingWrite},
	}
	for _, req := range requests {
		f.grant(t, others[req.need]...)
		resp := adtyJSON(t, f.engine, req.method, req.path, req.body, f.token)
		assert.Equal(t, http.StatusForbidden, resp.Code, "%s %s without %s", req.method, req.path, req.need.Action)
		f.grant(t, req.need)
		resp = adtyJSON(t, f.engine, req.method, req.path, req.body, f.token)
		assert.NotEqual(t, http.StatusForbidden, resp.Code, "%s %s with %s", req.method, req.path, req.need.Action)
	}
}
