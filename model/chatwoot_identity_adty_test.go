package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 审查 N-P2-10
func TestChatwootIdentityHashAndPublicConfig(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	old := map[string]string{}
	for _, k := range []string{OptChatwootHmacSecret, OptChatwootBaseUrl, OptChatwootWebsiteToken} {
		old[k] = common.OptionMap[k]
	}
	common.OptionMap[OptChatwootHmacSecret] = "s3cret"
	common.OptionMap[OptChatwootBaseUrl] = "https://cw.example.com/"
	common.OptionMap[OptChatwootWebsiteToken] = "tok"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		for k, v := range old {
			common.OptionMap[k] = v
		}
		common.OptionMapRWMutex.Unlock()
	})
	m := hmac.New(sha256.New, []byte("s3cret"))
	m.Write([]byte("42"))
	h, ok := ChatwootIdentityHash("42")
	require.True(t, ok)
	require.Equal(t, hex.EncodeToString(m.Sum(nil)), h)
	cfg := GetChatwootPublicConfig()
	require.True(t, cfg.Enabled)
	require.Equal(t, "https://cw.example.com", cfg.BaseURL)
	require.True(t, cfg.IdentityVerified)
}
