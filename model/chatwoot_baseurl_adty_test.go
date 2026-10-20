package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestChatwootBaseURLAllowed(t *testing.T) {
	require.True(t, ChatwootBaseURLAllowed(""))
	require.True(t, ChatwootBaseURLAllowed("https://cw.example.com"))
	require.True(t, ChatwootBaseURLAllowed("http://127.0.0.1:3000"))
	require.True(t, ChatwootBaseURLAllowed("http://localhost"))
	require.False(t, ChatwootBaseURLAllowed("javascript:alert(1)"))
	require.False(t, ChatwootBaseURLAllowed("http://evil.example"))
	require.False(t, ChatwootBaseURLAllowed("https://user:pass@cw.example.com"))
	require.False(t, ChatwootBaseURLAllowed("data:text/html,hi"))
}

func TestApplyChatwootPutRejectsUnsafeBaseURL(t *testing.T) {
	bad := "javascript:alert(1)"
	_, _, err := ApplyChatwootPut(ChatwootPutInput{BaseURL: &bad})
	require.Error(t, err)
}

func TestGetChatwootPublicConfigHidesUnsafeBaseURL(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldBase := common.OptionMap[OptChatwootBaseUrl]
	oldToken := common.OptionMap[OptChatwootWebsiteToken]
	common.OptionMap[OptChatwootBaseUrl] = "javascript:alert(1)"
	common.OptionMap[OptChatwootWebsiteToken] = "tok"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[OptChatwootBaseUrl] = oldBase
		common.OptionMap[OptChatwootWebsiteToken] = oldToken
		common.OptionMapRWMutex.Unlock()
	})

	cfg := GetChatwootPublicConfig()
	require.False(t, cfg.Enabled)
	require.Empty(t, cfg.BaseURL)
}
