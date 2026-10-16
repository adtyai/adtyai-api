package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func optionSecretSetup(t *testing.T, key string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))
	t.Setenv("ADTY_SECRET_KEY", "unit-test-option-secret")
	common.InitAdtySecretKeys()
	origMap := common.OptionMap
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	t.Cleanup(func() {
		defer func() { common.OptionMap = origMap }()
		DB.Where(commonKeyCol+" = ?", "UnitTestSecret").Delete(&Option{})
		DB.Where(commonKeyCol+" = ?", "UnitPlainOpt").Delete(&Option{})
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, "UnitTestSecret")
		delete(common.OptionMap, "UnitPlainOpt")
		common.OptionMapRWMutex.Unlock()
		common.InitAdtySecretKeys()
	})
}

func dbOptionValue(t *testing.T, key string) string {
	var o Option
	require.NoError(t, DB.Where(commonKeyCol+" = ?", key).First(&o).Error)
	return o.Value
}

func TestIsSensitiveOptionKey(t *testing.T) {
	for _, k := range []string{"BepusdtApiToken", "ChatwootHmacSecret", "AdtyTelegramBotToken", "x.api_key", "SomeKey"} {
		assert.True(t, IsSensitiveOptionKey(k), k)
	}
	assert.False(t, IsSensitiveOptionKey(OptChatwootWebsiteToken))
	assert.False(t, IsSensitiveOptionKey("SystemName"))
}

func TestUpdateOptionEncryptsSensitive(t *testing.T) {
	optionSecretSetup(t, "UnitTestSecret")
	require.NoError(t, UpdateOption("UnitTestSecret", "plain-s3cret"))
	raw := dbOptionValue(t, "UnitTestSecret")
	assert.True(t, strings.HasPrefix(raw, OptionSecretPrefix))
	assert.NotContains(t, raw, "plain-s3cret")
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "plain-s3cret", common.OptionMap["UnitTestSecret"])
	common.OptionMapRWMutex.RUnlock()

	// reload from DB decrypts
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, "UnitTestSecret")
	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabase()
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "plain-s3cret", common.OptionMap["UnitTestSecret"])
	common.OptionMapRWMutex.RUnlock()

	// empty clears
	require.NoError(t, UpdateOption("UnitTestSecret", ""))
	assert.Equal(t, "", dbOptionValue(t, "UnitTestSecret"))

	// non-sensitive stays plaintext
	require.NoError(t, UpdateOption("UnitPlainOpt", "hello"))
	assert.Equal(t, "hello", dbOptionValue(t, "UnitPlainOpt"))
}

func TestLegacyPlaintextMigrated(t *testing.T) {
	optionSecretSetup(t, "UnitTestSecret")
	require.NoError(t, DB.Create(&Option{Key: "UnitTestSecret", Value: "legacy-plain"}).Error)
	loadOptionsFromDatabase()
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "legacy-plain", common.OptionMap["UnitTestSecret"])
	common.OptionMapRWMutex.RUnlock()
	raw := dbOptionValue(t, "UnitTestSecret")
	assert.True(t, strings.HasPrefix(raw, OptionSecretPrefix))
	p, _, err := decodeOptionValueFromDB("UnitTestSecret", raw)
	require.NoError(t, err)
	assert.Equal(t, "legacy-plain", p)
}

func TestSensitiveWriteFailsClosedWithoutKey(t *testing.T) {
	optionSecretSetup(t, "UnitTestSecret")
	t.Setenv("ADTY_SECRET_KEY", "")
	common.InitAdtySecretKeys()
	assert.Error(t, UpdateOption("UnitTestSecret", "x"))
	assert.Error(t, UpdateOptionsBulk(map[string]string{"UnitTestSecret": "x"}))
	assert.NoError(t, UpdateOption("UnitPlainOpt", "ok"))
}

func TestExternalOptionSecretPrefixRejected(t *testing.T) {
	optionSecretSetup(t, "UnitTestSecret")
	forged := OptionSecretPrefix + "Zm9yZ2Vk"
	for _, k := range []string{"UnitTestSecret", "UnitPlainOpt"} {
		err := UpdateOption(k, forged)
		require.ErrorIs(t, err, ErrOptionSecretPrefixInput, k)
		err = UpdateOptionsBulk(map[string]string{k: forged})
		require.ErrorIs(t, err, ErrOptionSecretPrefixInput, k)
		var n int64
		DB.Model(&Option{}).Where(commonKeyCol+" = ?", k).Count(&n)
		assert.Zero(t, n, k)
	}
	require.ErrorIs(t, ApplyBusinessParamUpdates(map[string]string{"FirstTopupBonusRate": forged}), ErrOptionSecretPrefixInput)
	_, err := encodeOptionValueForDB("UnitTestSecret", forged)
	require.ErrorIs(t, err, ErrOptionSecretPrefixInput)

	// plaintext sensitive value still encrypted on write
	require.NoError(t, UpdateOption("UnitTestSecret", "plain-ok"))
	raw := dbOptionValue(t, "UnitTestSecret")
	assert.True(t, strings.HasPrefix(raw, OptionSecretPrefix))
	assert.NotContains(t, raw, "plain-ok")
}
