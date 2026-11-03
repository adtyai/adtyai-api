package middleware

import "fmt"

// DeclareAdtyAccessTokenRoute 给二开路由登记令牌规则。
// 键已在静态表里，或同一个键登记了不同规则时 panic；同一规则再登记一次则忽略。
func DeclareAdtyAccessTokenRoute(method, fullPath, kind, scope string) {
	key := method + " " + fullPath
	rule, err := adtyAccessTokenRule(kind, scope)
	if err != nil {
		panic(err)
	}
	if _, ok := accessTokenRouteRules[key]; ok {
		panic(fmt.Sprintf("access token route %s is already declared", key))
	}
	permissionRouteRulesMu.Lock()
	defer permissionRouteRulesMu.Unlock()
	if existing, ok := permissionRouteRules[key]; ok {
		if existing != rule {
			panic(fmt.Sprintf("access token route %s is declared with %s and %s", key, existing.Kind(), rule.Kind()))
		}
		return
	}
	permissionRouteRules[key] = rule
}

func adtyAccessTokenRule(kind, scope string) (accessTokenRouteRule, error) {
	switch kind {
	case "scope":
		if scope == "" {
			return accessTokenRouteRule{}, fmt.Errorf("access token scope is required")
		}
		return accessTokenScopeRule(scope), nil
	case "any":
		return accessTokenAnyRule, nil
	case "session":
		return accessTokenSessionRule, nil
	default:
		return accessTokenRouteRule{}, fmt.Errorf("unknown access token rule kind %q", kind)
	}
}
