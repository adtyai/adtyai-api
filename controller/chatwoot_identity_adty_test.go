package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// 审查 R-P2-9：未配置 HMAC 密钥时 identity_verified=false，不下发 email。
func TestChatwootIdentityWithoutHmac(t *testing.T) {
	database := adtyTestDB(t)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	prev, had := common.OptionMap[model.OptChatwootHmacSecret]
	common.OptionMap[model.OptChatwootHmacSecret] = ""
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if had {
			common.OptionMap[model.OptChatwootHmacSecret] = prev
		} else {
			delete(common.OptionMap, model.OptChatwootHmacSecret)
		}
		common.OptionMapRWMutex.Unlock()
	})
	u := model.User{Username: "cw1", Password: "unused-password", Email: "cw1@example.com", AffCode: "cw1a"}
	require.NoError(t, database.Create(&u).Error)
	rec := adtyCall(GetChatwootIdentity, http.MethodGet, "/api/chatwoot/identity", "", u.Id, common.RoleCommonUser, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, false, out.Data["identity_verified"])
	require.Equal(t, "", out.Data["email"])
	require.Equal(t, "", out.Data["identifier_hash"])
}
