package service

import (
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
)

const (
	AccessTokenGroupPersonal = "personal"
	AccessTokenGroupAdmin    = "admin"
	AccessTokenGroupSystem   = "system"

	AccessTokenMaxPerUser        = 20
	AccessTokenDefaultExpiryDays = 30
	accessTokenActionReveal      = "reveal"
	accessTokenScopeSeparator    = ":"
)

var (
	ErrAccessTokenScopeInvalid   = errors.New("access token scope is invalid")
	ErrAccessTokenScopeForbidden = errors.New("access token scope cannot be granted")
)

// AccessTokenResource mirrors authz.ResourceDefinition so both catalogs share
// one JSON shape and one frontend matrix component.
type AccessTokenResource struct {
	Resource string                   `json:"resource"`
	LabelKey string                   `json:"label_key"`
	Actions  []authz.ActionDefinition `json:"actions"`
	group    string
	minRole  int
}

type AccessTokenCatalogGroup struct {
	Group     string                `json:"group"`
	Resources []AccessTokenResource `json:"resources"`
}

func accessTokenView(description string) authz.ActionDefinition {
	return authz.ActionDefinition{Action: authz.ActionRead, LabelKey: "View", DescriptionKey: description}
}

func accessTokenEdit(description string) authz.ActionDefinition {
	return authz.ActionDefinition{Action: authz.ActionWrite, LabelKey: "Edit", DescriptionKey: description}
}

// accessTokenStaticResources covers dashboard routes that are not guarded by a
// Casbin permission. Casbin resources come from authz.Catalog() at runtime and
// must never be duplicated here.
var accessTokenStaticResources = []AccessTokenResource{
	{Resource: "profile", LabelKey: "Profile", group: AccessTokenGroupPersonal, minRole: common.RoleCommonUser, Actions: []authz.ActionDefinition{
		accessTokenView("View your profile, groups, and available models."),
		accessTokenEdit("Change your display name and personal settings."),
	}},
	{Resource: "api_key", LabelKey: "API keys", group: AccessTokenGroupPersonal, minRole: common.RoleCommonUser, Actions: []authz.ActionDefinition{
		accessTokenView("View API keys and their settings without the full key."),
		accessTokenEdit("Create, edit, and delete API keys."),
		{Action: accessTokenActionReveal, LabelKey: "Reveal full keys", DescriptionKey: "View complete API keys."},
	}},
	{Resource: "usage", LabelKey: "Usage", group: AccessTokenGroupPersonal, minRole: common.RoleCommonUser, Actions: []authz.ActionDefinition{
		accessTokenView("View your usage logs, statistics, tasks, and audit logs."),
	}},
	{Resource: "wallet", LabelKey: "Wallet", group: AccessTokenGroupPersonal, minRole: common.RoleCommonUser, Actions: []authz.ActionDefinition{
		accessTokenView("View balance and top-up records."),
		accessTokenEdit("Top up and check in."),
	}},
	{Resource: "account_security", LabelKey: "Account security", group: AccessTokenGroupPersonal, minRole: common.RoleCommonUser, Actions: []authz.ActionDefinition{
		accessTokenView("View Passkey, two-factor, and sign-in binding status."),
		accessTokenEdit("Includes changes that require security verification, such as password, two-factor settings, and account deletion."),
	}},
	{Resource: "user", LabelKey: "Users", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View users, their sign-in bindings, and permission settings."),
		accessTokenEdit("Create, edit, disable, and delete users, and reset their security settings."),
	}},
	{Resource: "billing", LabelKey: "Billing management", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View all top-up records."),
		accessTokenEdit("Complete top-up orders."),
	}},
	{Resource: "model", LabelKey: "Models", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View models and vendors, and preview upstream sync."),
		accessTokenEdit("Edit models and vendors, and sync them from upstream."),
	}},
	{Resource: "deployment", LabelKey: "Deployments", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View deployments and price estimates."),
		accessTokenEdit("Create, edit, and delete deployments, and test connections."),
	}},
	{Resource: "log", LabelKey: "Logs", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View all users' usage logs, statistics, and tasks."),
	}},
	{Resource: "group", LabelKey: "Groups", group: AccessTokenGroupAdmin, minRole: common.RoleAdminUser, Actions: []authz.ActionDefinition{
		accessTokenView("View groups and prefill groups."),
		accessTokenEdit("Create, edit, and delete prefill groups."),
	}},
	{Resource: "option", LabelKey: "System settings", group: AccessTokenGroupSystem, minRole: common.RoleRootUser, Actions: []authz.ActionDefinition{
		accessTokenView("View system settings, custom OAuth providers, and price sync sources."),
		accessTokenEdit("Change system settings and custom OAuth providers, and sync prices."),
	}},
	{Resource: "plugin", LabelKey: "Task plugins", group: AccessTokenGroupSystem, minRole: common.RoleRootUser, Actions: []authz.ActionDefinition{
		accessTokenView("View installed task plugins and their settings."),
		accessTokenEdit("Install, configure, test, and remove task plugins."),
	}},
	{Resource: "ops", LabelKey: "Operations", group: AccessTokenGroupSystem, minRole: common.RoleRootUser, Actions: []authz.ActionDefinition{
		accessTokenView("View performance, system tasks, and system information."),
		accessTokenEdit("Run and manage maintenance tasks and performance settings."),
	}},
}

// AccessTokenScopeOf is the token scope key for a Casbin permission.
func AccessTokenScopeOf(permission authz.Permission) string {
	return permission.Resource + accessTokenScopeSeparator + permission.Action
}

// AccessTokenCatalog lists the scopes this user may grant right now, grouped
// for display. Resources without a grantable action and empty groups are
// omitted.
func AccessTokenCatalog(userID, role int) []AccessTokenCatalogGroup {
	resources := map[string][]AccessTokenResource{}
	for _, resource := range accessTokenStaticResources {
		if role < resource.minRole {
			continue
		}
		if AdtyAdminPermissionsEnabled() {
			if slices.Contains([]string{"user", "billing", "model", "log", "group"}, resource.Resource) || resource.Resource == "deployment" && role < common.RoleRootUser {
				continue
			}
		}
		resource.Actions = slices.Clone(resource.Actions)
		resources[resource.group] = append(resources[resource.group], resource)
	}
	if role >= common.RoleAdminUser {
		for _, definition := range authz.Catalog() {
			resource := AccessTokenResource{Resource: definition.Resource, LabelKey: definition.LabelKey}
			for _, action := range definition.Actions {
				if authz.Can(userID, role, authz.Permission{Resource: definition.Resource, Action: action.Action}) {
					resource.Actions = append(resource.Actions, action)
				}
			}
			if len(resource.Actions) > 0 {
				resources[AccessTokenGroupAdmin] = append(resources[AccessTokenGroupAdmin], resource)
			}
		}
	}
	groups := make([]AccessTokenCatalogGroup, 0, 3)
	for _, group := range []string{AccessTokenGroupPersonal, AccessTokenGroupAdmin, AccessTokenGroupSystem} {
		if len(resources[group]) > 0 {
			groups = append(groups, AccessTokenCatalogGroup{Group: group, Resources: resources[group]})
		}
	}
	return groups
}

// AccessTokenScopeDictionary lists every resource a token scope can name,
// whatever the viewer may grant, so stored grants keep their labels after the
// granting user loses a permission or when another user reads them.
func AccessTokenScopeDictionary() []AccessTokenResource {
	definitions := authz.Catalog()
	resources := make([]AccessTokenResource, 0, len(accessTokenStaticResources)+len(definitions))
	for _, resource := range accessTokenStaticResources {
		resource.Actions = slices.Clone(resource.Actions)
		resources = append(resources, resource)
	}
	for _, definition := range definitions {
		resources = append(resources, AccessTokenResource{Resource: definition.Resource, LabelKey: definition.LabelKey, Actions: definition.Actions})
	}
	return resources
}

// NormalizeAccessTokenScopes validates a requested grant against the catalog
// the user sees and returns it trimmed, de-duplicated and sorted.
func NormalizeAccessTokenScopes(userID, role int, scopes []string) ([]string, error) {
	normalized, ok := NormalizeAccessTokenScopeList(scopes)
	if !ok {
		return nil, ErrAccessTokenScopeInvalid
	}
	grantable := make(map[string]bool)
	for _, group := range AccessTokenCatalog(userID, role) {
		for _, resource := range group.Resources {
			for _, action := range resource.Actions {
				grantable[resource.Resource+accessTokenScopeSeparator+action.Action] = true
			}
		}
	}
	for _, scope := range normalized {
		if grantable[scope] {
			continue
		}
		if !knownAccessTokenScope(scope) {
			return nil, ErrAccessTokenScopeInvalid
		}
		return nil, ErrAccessTokenScopeForbidden
	}
	return normalized, nil
}

func knownAccessTokenScope(scope string) bool {
	resourceName, action, ok := strings.Cut(scope, accessTokenScopeSeparator)
	if !ok {
		return false
	}
	for _, resource := range accessTokenStaticResources {
		if resource.Resource == resourceName && slices.ContainsFunc(resource.Actions, func(definition authz.ActionDefinition) bool { return definition.Action == action }) {
			return true
		}
	}
	return slices.Contains(authz.AllPermissions(), authz.Permission{Resource: resourceName, Action: action})
}

func AccessTokenScopeGranted(scopes []string, scope string) bool {
	return scope != "" && slices.Contains(scopes, scope)
}

// AccessTokenScopeAllowsRoute applies old catalog aliases only to the original
// HTTP route and action. Callers must also check the user's live permission.
func AccessTokenScopeAllowsRoute(scopes []string, scopeVersion, role int, method, fullPath, requiredScope string) bool {
	if scopeVersion != 0 && scopeVersion != model.AccessTokenScopeVersion {
		return false
	}
	key := method + " " + fullPath
	if AdtyAdminPermissionsEnabled() && key == "GET /api/group/" && requiredScope == "group:read" {
		return scopeVersion == 0 && AccessTokenScopeGranted(scopes, "group:read") ||
			AccessTokenScopeGranted(scopes, "adty_user:read") || AccessTokenScopeGranted(scopes, "adty_log:read") ||
			AccessTokenScopeGranted(scopes, "adty_model:read") || AccessTokenScopeGranted(scopes, "channel:read")
	}
	if AccessTokenScopeGranted(scopes, requiredScope) {
		return true
	}
	if scopeVersion != 0 {
		return false
	}
	if AccessTokenScopeGranted(scopes, accessTokenLegacyActionScopes[key][requiredScope]) {
		return true
	}
	if role < common.RoleRootUser {
		return false
	}
	switch key {
	case "GET /api/adty/admin/reseller/settings":
		return requiredScope == AccessTokenScopeOf(authz.AdtySiteSettingsRead) && AccessTokenScopeGranted(scopes, "option:read")
	case "PUT /api/adty/admin/reseller/settings":
		return requiredScope == AccessTokenScopeOf(authz.AdtySiteSettings) && AccessTokenScopeGranted(scopes, "option:write")
	}
	return false
}

var accessTokenLegacyActionScopes = map[string]map[string]string{
	"GET /api/user/":                                   {"adty_user:read": "user:read"},
	"GET /api/user/search":                             {"adty_user:read": "user:read"},
	"GET /api/user/:id":                                {"adty_user:read": "user:read"},
	"GET /api/user/:id/oauth/bindings":                 {"adty_user:read": "user:read"},
	"GET /api/user/2fa/stats":                          {"adty_user:read": "user:read"},
	"GET /api/authz/catalog":                           {"adty_user:read": "user:read"},
	"PUT /api/user/":                                   {"adty_user:write": "user:write", "adty_user:group": "user:write"},
	"DELETE /api/user/:id/oauth/bindings/:provider_id": {"adty_user:write": "user:write"},
	"DELETE /api/user/:id/bindings/:binding_type":      {"adty_user:write": "user:write"},
	"DELETE /api/user/:id/reset_passkey":               {"adty_user:write": "user:write"},
	"DELETE /api/user/:id/2fa":                         {"adty_user:write": "user:write"},
	"GET /api/models/":                                 {"adty_model:read": "model:read"},
	"GET /api/models/search":                           {"adty_model:read": "model:read"},
	"GET /api/models/:id":                              {"adty_model:read": "model:read"},
	"GET /api/models/missing":                          {"adty_model:read": "model:read"},
	"GET /api/models/sync_upstream/preview":            {"adty_model:read": "model:read"},
	"GET /api/vendors/":                                {"adty_model:read": "model:read"},
	"GET /api/vendors/search":                          {"adty_model:read": "model:read"},
	"GET /api/vendors/:id":                             {"adty_model:read": "model:read"},
	"POST /api/vendors/operations/preview":             {"adty_model:read": "model:read"},
	"PUT /api/models/":                                 {"adty_model:write": "model:write"},
	"PUT /api/vendors/":                                {"adty_model:write": "model:write"},
	"GET /api/prefill_group/":                          {"adty_model:read": "group:read"},
	"PUT /api/prefill_group/":                          {"adty_model:write": "group:write"},
	"GET /api/status/test":                             {"adty_log:read": "log:read"},
	"GET /api/log/":                                    {"adty_log:read": "log:read"},
	"GET /api/log/stat":                                {"adty_log:read": "log:read"},
	"GET /api/log/channel_affinity_usage_cache":        {"adty_log:read": "log:read"},
	"GET /api/log/search":                              {"adty_log:read": "log:read"},
	"GET /api/data/":                                   {"adty_log:read": "log:read"},
	"GET /api/data/users":                              {"adty_log:read": "log:read"},
	"GET /api/data/flow":                               {"adty_log:read": "log:read"},
	"GET /api/mj/":                                     {"adty_log:read": "log:read"},
	"GET /api/task":                                    {"adty_log:read": "log:read"},
	"GET /api/user/topup":                              {"adty_finance:read": "billing:read"},
	"GET /api/group/":                                  {"adty_user:read": "group:read", "adty_log:read": "group:read", "adty_model:read": "group:read", "channel:read": "group:read"},
	"POST /api/models/sync_upstream":                   {"adty_model:create": "model:write", "adty_model:write": "model:write"},
	"POST /api/vendors/operations":                     {"adty_model:write": "model:write", "adty_model:delete": "model:write"},
	"POST /api/user/":                                  {"adty_user:create": "user:write"},
	"DELETE /api/user/:id":                             {"adty_user:delete": "user:write"},
	"GET /api/log/export":                              {"adty_log:export": "log:read"},
	"POST /api/models/":                                {"adty_model:create": "model:write"},
	"POST /api/vendors/":                               {"adty_model:create": "model:write"},
	"POST /api/prefill_group/":                         {"adty_model:create": "group:write"},
	"DELETE /api/models/:id":                           {"adty_model:delete": "model:write", "channel:write": "model:write"},
	"POST /api/models/delete":                          {"adty_model:delete": "model:write", "channel:write": "model:write"},
	"DELETE /api/vendors/:id":                          {"adty_model:delete": "model:write"},
	"DELETE /api/prefill_group/:id":                    {"adty_model:delete": "group:write"},
	"POST /api/channel/":                               {"channel:create": "channel:sensitive_write"},
	"POST /api/channel/copy/:id":                       {"channel:create": "channel:sensitive_write"},
	"DELETE /api/channel/disabled":                     {"channel:delete": "channel:sensitive_write"},
	"DELETE /api/channel/:id":                          {"channel:delete": "channel:sensitive_write"},
	"POST /api/channel/batch":                          {"channel:delete": "channel:sensitive_write"},
	"POST /api/adty/admin/sites":                       {"adty_site:create": "adty_site:adjust"},
	"DELETE /api/adty/admin/sites/:id/sender-domain":   {"adty_site:delete": "adty_site:adjust"},
	"DELETE /api/adty/admin/sites/domains/:id":         {"adty_site:delete": "adty_site:adjust"},
	"DELETE /api/adty/admin/sites/:id/notice":          {"adty_site:delete": "adty_site:adjust"},
	"GET /api/adty/admin/reseller/settings":            {"adty_site:settings_read": "adty_site:settings"},
	"GET /api/adty/admin/reseller/invite-codes":        {"adty_site:settings_read": "adty_site:settings"},
	"POST /api/adty/admin/reseller/invite-codes":       {"adty_site:settings_create": "adty_site:settings"},
	"DELETE /api/adty/admin/reseller/invite-codes/:id": {"adty_site:settings_delete": "adty_site:settings"},
	"GET /api/user/admin/referral/settings":            {"adty_referral:settings_read": "adty_referral:settings"},
}

// accessTokenVerificationScopes maps every verification scope to the token
// scope a PAT needs to obtain its proof. An empty value means PATs can never
// obtain it; scopes missing from this table are denied too.
var accessTokenVerificationScopes = map[string]string{
	VerificationScopeChannelKeyRead:        AccessTokenScopeOf(authz.ChannelSecretView),
	VerificationScopeAdminUserCreate:       "user:write",
	VerificationScopeAdminUserUpdate:       "user:write",
	VerificationScopeAdminUserDelete:       "user:write",
	VerificationScopeAdminUserManage:       "user:write",
	VerificationScopeAdminUserPasskeyReset: "user:write",
	VerificationScopeAdminUserTwoFADisable: "user:write",
	VerificationScopeAdminUserBindingClear: "user:write",
	VerificationScopePasskeyRegister:       "account_security:write",
	VerificationScopePasskeyDelete:         "account_security:write",
	VerificationScopeTwoFASetup:            "account_security:write",
	VerificationScopeTwoFADisable:          "account_security:write",
	VerificationScopeTwoFABackupCodes:      "account_security:write",
	VerificationScopePasswordSet:           "account_security:write",
	VerificationScopePasswordChange:        "account_security:write",
	VerificationScopeAccountBind:           "account_security:write",
	VerificationScopeAccountUnbind:         "account_security:write",
	VerificationScopeAccountDelete:         "account_security:write",
	VerificationScopeAccessTokenGenerate:   "",
	VerificationScopeAPIKeyRead:            "api_key:reveal",
	VerificationScopeAccessTokenUpdate:     "",
	VerificationScopeAccessTokenRevoke:     "",
	VerificationScopeLogin:                 "",
}

var adminUserVerificationPermissions = map[string]authz.Permission{
	VerificationScopeAdminUserCreate:       authz.AdtyUserCreate,
	VerificationScopeAdminUserDelete:       authz.AdtyUserDelete,
	VerificationScopeAdminUserUpdate:       authz.AdtyUserWrite,
	VerificationScopeAdminUserManage:       authz.AdtyUserWrite,
	VerificationScopeAdminUserPasskeyReset: authz.AdtyUserWrite,
	VerificationScopeAdminUserTwoFADisable: authz.AdtyUserWrite,
	VerificationScopeAdminUserBindingClear: authz.AdtyUserWrite,
}

// AccessTokenVerificationScope returns the token scope a PAT needs for a
// verification scope and whether the scope is listed at all.
func AccessTokenVerificationScope(verificationScope string) (scope string, listed bool) {
	if AdtyAdminPermissionsEnabled() {
		if permission, ok := adminUserVerificationPermissions[verificationScope]; ok {
			return AccessTokenScopeOf(permission), true
		}
	}
	scope, listed = accessTokenVerificationScopes[verificationScope]
	return scope, listed
}

// requireAccessTokenVerificationScope rejects a step-up request from a PAT
// whose grant does not cover the operation. Browser sessions pass through.
func requireAccessTokenVerificationScope(identity AuthIdentity, verificationScope string) error {
	tokenID, ok := model.ParseAccessTokenSessionID(identity.SessionID)
	if !ok {
		return nil
	}
	required, _ := AccessTokenVerificationScope(verificationScope)
	if required == "" {
		return ErrVerificationForbidden
	}
	token, err := model.GetUserAccessToken(identity.UserID, tokenID)
	if errors.Is(err, model.ErrAccessTokenNotFound) {
		return ErrAuthTokenInvalid
	}
	if err != nil {
		return err
	}
	if !AccessTokenScopeGranted(token.GetScopes(), required) {
		// Only an existing grant may retain the original admin proof scope.
		// The final mutation still rechecks its own route and action.
		if _, adminScope := adminUserVerificationPermissions[verificationScope]; AdtyAdminPermissionsEnabled() && adminScope && token.ScopeVersion == 0 && AccessTokenScopeGranted(token.GetScopes(), "user:write") {
			return nil
		}
		return ErrVerificationForbidden
	}
	return nil
}
