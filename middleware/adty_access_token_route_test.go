package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclareAdtyAccessTokenRoute(t *testing.T) {
	const key = "GET /api/adty/test-declare"
	DeclareAdtyAccessTokenRoute("GET", "/api/adty/test-declare", "any", "")
	rule, ok := AccessTokenRouteRule(key)
	require.True(t, ok)
	assert.Equal(t, "any", rule.Kind())
	assert.Empty(t, rule.Scope())

	require.NotPanics(t, func() {
		DeclareAdtyAccessTokenRoute("GET", "/api/adty/test-declare", "any", "")
	})
	require.Panics(t, func() {
		DeclareAdtyAccessTokenRoute("GET", "/api/adty/test-declare", "session", "")
	})
	require.Panics(t, func() {
		DeclareAdtyAccessTokenRoute("GET", "/api/option/", "scope", "option:read")
	})

	DeclareAdtyAccessTokenRoute("POST", "/api/adty/test-declare-scope", "scope", "option:write")
	scoped, ok := AccessTokenRouteRule("POST /api/adty/test-declare-scope")
	require.True(t, ok)
	assert.Equal(t, "scope", scoped.Kind())
	assert.Equal(t, "option:write", scoped.Scope())
}
