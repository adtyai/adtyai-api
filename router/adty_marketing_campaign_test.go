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

// 群发的查看、创建、编辑、删除和发送独立授权；其他动作不能代替所需动作。
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
		{http.MethodPost, "/api/adty/admin/marketing/campaigns", body, authz.AdtyMarketingCreate},
		{http.MethodPut, "/api/adty/admin/marketing/campaigns/" + id, body, authz.AdtyMarketingWrite},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/test", `{"to":"me@example.com","lang":"en"}`, authz.AdtyMarketingSend},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/send", `{"confirm":true}`, authz.AdtyMarketingSend},
		{http.MethodPost, "/api/adty/admin/marketing/campaigns/" + id + "/cancel", `{}`, authz.AdtyMarketingSend},
		{http.MethodDelete, "/api/adty/admin/marketing/campaigns/" + id, "", authz.AdtyMarketingDelete},
	}
	for _, req := range requests {
		var others []authz.Permission
		for _, candidate := range []authz.Permission{authz.AdtyMarketingRead, authz.AdtyMarketingCreate, authz.AdtyMarketingWrite, authz.AdtyMarketingDelete, authz.AdtyMarketingSend} {
			if candidate != req.need {
				others = append(others, candidate)
			}
		}
		f.grant(t, others...)
		resp := adtyJSON(t, f.engine, req.method, req.path, req.body, f.token)
		assert.Equal(t, http.StatusForbidden, resp.Code, "%s %s without %s", req.method, req.path, req.need.Action)
		f.grant(t, req.need)
		resp = adtyJSON(t, f.engine, req.method, req.path, req.body, f.token)
		assert.NotEqual(t, http.StatusForbidden, resp.Code, "%s %s with %s", req.method, req.path, req.need.Action)
	}
}
