package service

import (
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
)

func TestAdtyAccessTokenCatalogOnlyOffersCurrentActions(t *testing.T) {
	previous := AdtyAdminPermissionsEnabled()
	SetAdtyAdminPermissionsEnabled(true)
	authz.AdtyRegisterResources()
	t.Cleanup(func() { SetAdtyAdminPermissionsEnabled(previous) })
	var resources []string
	for _, group := range AccessTokenCatalog(1, common.RoleRootUser) {
		for _, resource := range group.Resources {
			resources = append(resources, resource.Resource)
		}
	}
	for _, obsolete := range []string{"user", "billing", "model", "log", "group"} {
		assert.NotContains(t, resources, obsolete, "new grants cannot use the old admin catalog")
		assert.True(t, slices.ContainsFunc(AccessTokenScopeDictionary(), func(resource AccessTokenResource) bool { return resource.Resource == obsolete }), "old grants keep their display labels")
	}
	for _, scope := range []string{"user:write", "billing:write", "model:write", "log:read", "group:read"} {
		_, err := NormalizeAccessTokenScopes(1, common.RoleRootUser, []string{scope})
		assert.ErrorIs(t, err, ErrAccessTokenScopeForbidden, scope)
	}
	assert.NotContains(t, resources, "deployment")
	_, err := NormalizeAccessTokenScopes(1, common.RoleRootUser, []string{"deployment:write"})
	assert.Error(t, err, "a retired resource cannot be newly granted")
	for _, group := range AccessTokenCatalog(42, common.RoleAdminUser) {
		for _, resource := range group.Resources {
			assert.NotEqual(t, "deployment", resource.Resource, "a Root-only capability cannot be newly granted by an administrator")
		}
	}
}
