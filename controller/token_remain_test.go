package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUpdateDoesNotRestoreSpentRemainQuota(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "limited", "limited-key-secret")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"remain_quota":    1000,
		"unlimited_quota": false,
		"used_quota":      0,
	}).Error)
	ok, err := model.TryReserveTokenQuota(token.Id, token.Key, 200, false)
	require.NoError(t, err)
	require.True(t, ok)

	save := func(target string, body map[string]any) {
		t.Helper()
		ctx, recorder := newAuthenticatedContext(t, http.MethodPut, target, body, 1)
		UpdateToken(ctx)
		response := decodeAPIResponse(t, recorder)
		require.True(t, response.Success, response.Message)
	}

	save("/api/token/", map[string]any{
		"id": token.Id, "name": "limited", "expired_time": -1,
		"unlimited_quota": false, "group": "default",
	})
	var kept model.Token
	require.NoError(t, db.First(&kept, token.Id).Error)
	assert.Equal(t, 800, kept.RemainQuota)
	assert.Equal(t, 200, kept.UsedQuota)

	save("/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusDisabled, "remain_quota": 1000,
	})
	require.NoError(t, db.First(&kept, token.Id).Error)
	assert.Equal(t, 800, kept.RemainQuota)
	assert.Equal(t, common.TokenStatusDisabled, kept.Status)

	save("/api/token/", map[string]any{
		"id": token.Id, "name": "limited", "expired_time": -1,
		"remain_quota": 250, "unlimited_quota": false, "group": "default",
	})
	require.NoError(t, db.First(&kept, token.Id).Error)
	assert.Equal(t, 250, kept.RemainQuota)
	assert.Equal(t, common.TokenStatusDisabled, kept.Status)
}
