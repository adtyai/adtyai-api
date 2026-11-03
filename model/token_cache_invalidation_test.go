package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenMutationStopsWhenCacheInvalidationFails(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&AdtyGroup{}))
	server := useUserCacheMiniRedis(t)
	token := Token{
		UserId: 7, Key: "cache-fail-key", Name: "cache-fail",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto",
	}
	require.NoError(t, token.Insert())
	require.NoError(t, cacheSetTokenForTest(token))

	server.SetError("READONLY")
	token.Status = common.TokenStatusDisabled
	require.Error(t, token.Update())
	server.SetError("")

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, stored.Status)
	validated, err := ValidateUserToken(token.Key)
	require.NoError(t, err)
	assert.Equal(t, common.TokenStatusEnabled, validated.Status)

	server.SetError("READONLY")
	require.Error(t, token.Delete())
	server.SetError("")
	require.NoError(t, DB.First(&stored, token.Id).Error)

	server.SetError("READONLY")
	count, err := BatchDeleteTokens([]int{token.Id}, token.UserId)
	require.Error(t, err)
	assert.Zero(t, count)
	server.SetError("")
	require.NoError(t, DB.First(&stored, token.Id).Error)
	validated, err = ValidateUserToken(token.Key)
	require.NoError(t, err)
	assert.Equal(t, common.TokenStatusEnabled, validated.Status)
}

func TestTokenCacheMissesWhileMutationFenceIsSet(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&AdtyGroup{}))
	useUserCacheMiniRedis(t)
	token := Token{
		UserId: 8, Key: "cache-fence-key", Name: "cache-fence",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto",
	}
	require.NoError(t, token.Insert())
	require.NoError(t, cacheSetTokenForTest(token))
	require.NoError(t, common.RDB.Set(t.Context(), getTokenCacheFenceKey(token.Key), 1, time.Minute).Err())

	_, err := cacheGetTokenByKey(token.Key)
	require.Error(t, err)
	loaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, token.Id, loaded.Id)
	assert.Equal(t, common.TokenStatusEnabled, loaded.Status)
}
