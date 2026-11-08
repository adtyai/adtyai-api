package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestLateTokenSnapshotCannotRestoreRevokedAuthorization(t *testing.T) {
	for _, mutation := range []string{"disable", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, DB.AutoMigrate(&AdtyGroup{}))
			server := useUserCacheMiniRedis(t)
			token := Token{UserId: 73, Key: "late-snapshot-" + mutation, Name: "late-snapshot", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto"}
			require.NoError(t, token.Insert())
			snapshot, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			if mutation == "delete" {
				require.NoError(t, token.Delete())
			} else {
				token.Status = common.TokenStatusDisabled
				require.NoError(t, token.Update())
			}
			// A database reader can be paused longer than any cache fence TTL.
			server.FastForward(time.Minute)
			_, err = cacheInitToken(*snapshot)
			require.NoError(t, err)
			_, err = ValidateUserToken(token.Key)
			assert.ErrorIs(t, err, ErrTokenInvalid)
		})
	}
}

func TestTokenMutationRollbackAllowsFreshCacheHydration(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	token := Token{UserId: 74, Key: "rollback-token-metadata", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto"}
	require.NoError(t, token.Insert())
	snapshot, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	forced := errors.New("rollback token metadata")
	err = token.mutateMetadata(func(tx *gorm.DB) error {
		if err := tx.Model(&token).Update("status", common.TokenStatusDisabled).Error; err != nil {
			return err
		}
		return forced
	})
	require.ErrorIs(t, err, forced)
	_, err = cacheInitToken(*snapshot)
	require.NoError(t, err)
	validated, err := ValidateUserToken(token.Key)
	require.NoError(t, err)
	assert.Equal(t, common.TokenStatusEnabled, validated.Status)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err, "rollback must not leave a permanent fence")
	assert.Equal(t, common.TokenStatusEnabled, cached.Status)
}

func TestTokenCommittedMutationSurvivesRedisFailure(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	token := Token{UserId: 75, Key: "commit-redis-failure", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto"}
	require.NoError(t, token.Insert())
	snapshot, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register("test:token-postcommit-redis", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			server.SetError("READONLY")
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove("test:token-postcommit-redis")) })
	token.Status = common.TokenStatusDisabled
	require.NoError(t, token.Update(), "post-commit cache cleanup must not report an uncommitted mutation")
	_, err = ValidateUserToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "Redis failure must fall back to committed database state")
	server.SetError("")
	server.FastForward(time.Minute)
	_, err = cacheInitToken(*snapshot)
	require.NoError(t, err)
	_, err = ValidateUserToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid)
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err, "recovery must permit fresh hydration without clearing a fence")
	assert.Equal(t, common.TokenStatusDisabled, cached.Status)
}

func TestTokenCacheRejectsGenerationChangedDuringDatabaseRead(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	token := Token{UserId: 76, Key: "generation-before-database", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "auto"}
	require.NoError(t, token.Insert())
	advanced := false
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register("test:token-generation-change", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" && !advanced {
			advanced = true
			tx.AddError(invalidateTokenCacheForMutation(token.Key))
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Query().Remove("test:token-generation-change")) })
	loaded, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	assert.Equal(t, token.Id, loaded.Id)
	_, err = cacheGetTokenByKey(token.Key)
	require.Error(t, err, "a generation change after capture invalidates this publication")
	_, err = ValidateUserToken(token.Key)
	require.NoError(t, err)
	_, err = cacheGetTokenByKey(token.Key)
	require.NoError(t, err, "the next read must recover using a fresh generation")
}
