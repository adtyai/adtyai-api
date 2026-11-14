package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuthCacheMACCoversForkedIdentityFields(t *testing.T) {
	user := UserBase{
		Id: 11, Group: "default", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Username: "mac-user", AuthVersion: 1, CacheSchema: userCacheSchemaVersion,
		SiteId: 7, StaffSiteId: 9, CacheExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	baseMAC, err := user.cacheMAC()
	require.NoError(t, err)

	tamperedSite := user
	tamperedSite.SiteId = 8
	mac, err := tamperedSite.cacheMAC()
	require.NoError(t, err)
	assert.NotEqual(t, baseMAC, mac)

	tamperedStaff := user
	tamperedStaff.StaffSiteId = 3
	mac, err = tamperedStaff.cacheMAC()
	require.NoError(t, err)
	assert.NotEqual(t, baseMAC, mac)

	tamperedSchema := user
	tamperedSchema.CacheSchema = 4
	mac, err = tamperedSchema.cacheMAC()
	require.NoError(t, err)
	assert.NotEqual(t, baseMAC, mac)

	token := Token{
		Id: 21, UserId: 11, Key: "mac-token", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, QuotaPending: 15, CacheExpiresAt: user.CacheExpiresAt,
	}
	tokenMAC, err := token.cacheMAC()
	require.NoError(t, err)
	token.QuotaPending = 16
	changed, err := token.cacheMAC()
	require.NoError(t, err)
	assert.NotEqual(t, tokenMAC, changed)

	session := newTestUserSession("mac-session", 11, time.Now().Unix())
	session.BindIssuedSite(7)
	entry := session.cacheEntry()
	entry.CacheExpiresAt = user.CacheExpiresAt
	sessionMAC, err := authCacheMAC(userSessionCacheKey(session.SID), entry)
	require.NoError(t, err)
	entry.IssuedSiteID = 8
	changed, err = authCacheMAC(userSessionCacheKey(session.SID), entry)
	require.NoError(t, err)
	assert.NotEqual(t, sessionMAC, changed)
}

func TestAuthCacheIntegrityRebuildsUnsignedAndRejectsTampering(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}, &UserSession{}, &AdtyGroup{}))
	useUserCacheMiniRedis(t)

	user := User{
		Username: "cache-integrity-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AuthVersion: 1, Group: "default",
		SiteId: 7, StaffSiteId: 9,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, populateUserCache(user))

	t.Run("user_site_and_role_tampering", func(t *testing.T) {
		for _, fields := range []map[string]any{
			{"Role": common.RoleRootUser},
			{"SiteId": 1},
			{"StaffSiteId": 2},
			{"CacheSchema": 4},
		} {
			require.NoError(t, common.RDB.HSet(t.Context(), getUserCacheKey(user.Id), fields).Err())
			_, err := cacheGetUserBase(user.Id)
			require.Error(t, err)
			loaded, err := GetUserCache(user.Id)
			require.NoError(t, err)
			assert.Equal(t, user.Role, loaded.Role)
			assert.Equal(t, 7, loaded.SiteId)
			assert.Equal(t, 9, loaded.StaffSiteId)
			assert.Equal(t, userCacheSchemaVersion, loaded.CacheSchema)
			cached, err := cacheGetUserBase(user.Id)
			require.NoError(t, err)
			assert.Equal(t, 7, cached.SiteId)
			assert.Equal(t, 9, cached.StaffSiteId)
		}
	})

	token := Token{
		UserId: user.Id, Key: "cache-integrity-token", Status: common.TokenStatusEnabled,
		Name: "integrity", ExpiredTime: -1, RemainQuota: 100, QuotaPending: 15,
		ModelLimitsEnabled: true, ModelLimits: "allowed-model", Group: "default",
	}
	require.NoError(t, DB.Create(&token).Error)
	_, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	cachedToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, 15, cachedToken.QuotaPending)

	t.Run("token_identity_and_pending", func(t *testing.T) {
		for _, fields := range []map[string]any{
			{"UserId": user.Id + 1},
			{"QuotaPending": 99},
			{"Status": common.TokenStatusDisabled, "ExpiredTime": 1},
			{"DeletedAt": time.Now().Format(time.RFC3339)},
		} {
			require.NoError(t, common.RDB.HSet(t.Context(), getTokenCacheKey(token.Key), fields).Err())
			_, err := cacheGetTokenByKey(token.Key)
			require.Error(t, err)
			loaded, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			assert.Equal(t, token.UserId, loaded.UserId)
			assert.Equal(t, token.Status, loaded.Status)
			// GetTokenByKey projects QuotaPending into RemainQuota for callers.
			cached, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			assert.Equal(t, token.UserId, cached.UserId)
			assert.Equal(t, 15, cached.QuotaPending)
			assert.Equal(t, token.Status, cached.Status)
			assert.Equal(t, token.ExpiredTime, cached.ExpiredTime)
		}
	})

	session := newTestUserSession("cache-integrity-session", user.Id, time.Now().Unix())
	session.BindIssuedSite(7)
	require.NoError(t, CreateUserSession(session))
	t.Cleanup(func() { assert.NoError(t, DB.Delete(session).Error) })

	t.Run("session_owner_and_issued_site", func(t *testing.T) {
		require.NoError(t, common.RDB.HSet(t.Context(), userSessionCacheKey(session.SID),
			"UserID", user.Id+1, "IssuedSiteID", 8).Err())
		_, err := getUserSessionCache(session.SID)
		require.Error(t, err)
		loaded, err := GetUserSessionCached(session.SID)
		require.NoError(t, err)
		assert.Equal(t, session.UserID, loaded.UserID)
		issued, ok := loaded.IssuedSite()
		require.True(t, ok)
		assert.Equal(t, 7, issued)
	})

	t.Run("unsigned_cache_is_rebuilt", func(t *testing.T) {
		for _, key := range []string{getUserCacheKey(user.Id), getTokenCacheKey(token.Key), userSessionCacheKey(session.SID)} {
			require.NoError(t, common.RDB.HDel(t.Context(), key, "CacheMAC", "CacheExpiresAt").Err())
		}
		_, err := cacheGetUserBase(user.Id)
		assert.Error(t, err)
		_, err = cacheGetTokenByKey(token.Key)
		assert.Error(t, err)
		_, err = getUserSessionCache(session.SID)
		assert.Error(t, err)

		_, err = GetUserCache(user.Id)
		require.NoError(t, err)
		_, err = GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		_, err = GetUserSessionCached(session.SID)
		require.NoError(t, err)

		cachedUser, err := cacheGetUserBase(user.Id)
		require.NoError(t, err)
		assert.NotEmpty(t, cachedUser.CacheMAC)
		assert.Equal(t, 7, cachedUser.SiteId)
		assert.Equal(t, 9, cachedUser.StaffSiteId)
		assert.Equal(t, userCacheSchemaVersion, cachedUser.CacheSchema)
		cachedToken, err := cacheGetTokenByKey(token.Key)
		require.NoError(t, err)
		assert.NotEmpty(t, cachedToken.CacheMAC)
		assert.Equal(t, 15, cachedToken.QuotaPending)
		cachedSession, err := getUserSessionCache(session.SID)
		require.NoError(t, err)
		assert.NotEmpty(t, cachedSession.CacheMAC)
		assert.Equal(t, 7, cachedSession.IssuedSiteID)
		assert.True(t, cachedSession.HasIssuedSiteID)

		expired := time.Now().Add(-time.Second).UnixMilli()
		cachedUser.CacheExpiresAt = expired
		cachedToken.CacheExpiresAt = expired
		cachedSession.CacheExpiresAt = expired
		userMAC, err := cachedUser.cacheMAC()
		require.NoError(t, err)
		tokenMAC, err := cachedToken.cacheMAC()
		require.NoError(t, err)
		sessionMAC, err := authCacheMAC(userSessionCacheKey(session.SID), cachedSession)
		require.NoError(t, err)
		for key, mac := range map[string]string{
			getUserCacheKey(user.Id):         userMAC,
			getTokenCacheKey(token.Key):      tokenMAC,
			userSessionCacheKey(session.SID): sessionMAC,
		} {
			require.NoError(t, common.RDB.HSet(t.Context(), key, "CacheExpiresAt", expired, "CacheMAC", mac).Err())
			require.NoError(t, common.RDB.Persist(t.Context(), key).Err())
		}
		_, err = cacheGetUserBase(user.Id)
		assert.Error(t, err)
		_, err = cacheGetTokenByKey(token.Key)
		assert.Error(t, err)
		_, err = getUserSessionCache(session.SID)
		assert.Error(t, err)
		_, err = GetUserCache(user.Id)
		require.NoError(t, err)
		_, err = GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		_, err = GetUserSessionCached(session.SID)
		require.NoError(t, err)
	})

	t.Run("cross_identity_copy_is_rejected", func(t *testing.T) {
		for source, destination := range map[string]string{
			getUserCacheKey(user.Id):         getUserCacheKey(user.Id + 1000),
			getTokenCacheKey(token.Key):      getTokenCacheKey("unknown-token"),
			userSessionCacheKey(session.SID): userSessionCacheKey("unknown-session"),
		} {
			fields, err := common.RDB.HGetAll(t.Context(), source).Result()
			require.NoError(t, err)
			require.NoError(t, common.RDB.HSet(t.Context(), destination, fields).Err())
		}
		require.NoError(t, common.RDB.HSet(t.Context(), getUserCacheKey(user.Id+1000), "Id", user.Id+1000).Err())
		require.NoError(t, common.RDB.HSet(t.Context(), userSessionCacheKey("unknown-session"), "SID", "unknown-session").Err())
		_, err := cacheGetUserBase(user.Id + 1000)
		assert.Error(t, err)
		_, err = GetUserCache(user.Id + 1000)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
		_, err = cacheGetTokenByKey("unknown-token")
		assert.Error(t, err)
		_, err = GetTokenByKey("unknown-token", false)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
		_, err = getUserSessionCache("unknown-session")
		assert.Error(t, err)
		_, err = GetUserSessionCached("unknown-session")
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("session_revocation_cannot_be_cleared", func(t *testing.T) {
		revoked, err := RevokeUserSession(user.Id, session.SID, "test")
		require.NoError(t, err)
		require.True(t, revoked)
		require.NoError(t, common.RDB.HSet(t.Context(), userSessionCacheKey(session.SID),
			"Status", UserSessionStatusActive, "RevokedAt", 0, "RevokedReason", "").Err())
		_, err = GetUserSessionCached(session.SID)
		assert.ErrorIs(t, err, ErrUserSessionInactive)
	})

	t.Run("unreadable_quota_counters_are_repaired", func(t *testing.T) {
		require.NoError(t, common.RDB.HSet(t.Context(), getUserCacheKey(user.Id), "Quota", "garbage").Err())
		require.NoError(t, common.RDB.HSet(t.Context(), getTokenCacheKey(token.Key), "RemainQuota", "garbage").Err())
		_, err := GetUserCache(user.Id)
		require.NoError(t, err)
		_, err = GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		cachedUser, err := cacheGetUserBase(user.Id)
		require.NoError(t, err)
		assert.Equal(t, 0, cachedUser.Quota)
		cachedToken, err := cacheGetTokenByKey(token.Key)
		require.NoError(t, err)
		assert.Equal(t, token.RemainQuota, cachedToken.RemainQuota)
		assert.Equal(t, 15, cachedToken.QuotaPending)
	})
}
