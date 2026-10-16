package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// frontendFormOptionKeys 前端以「表单字段名 = option key」方式写入 PUT /api/option 的 key。
// 静态扫描很难可靠地从表单里区分 option 与普通字段，所以这里显式列出，来源：
//   - web/src/features/system-settings/auth/oauth-section.tsx（LinuxDO*）
//   - web/src/features/system-settings/maintenance/{header-navigation,sidebar-modules}-section.tsx
//   - web/src/features/system-settings/integrations/ionet-deployment-settings-section.tsx
//   - web/src/features/system-settings/hooks/use-update-option.ts（STATUS_RELATED_KEYS）
// 新增前端设置页写新 key 时请同步加到这里，并在 model.InitOptionMap 初始化。
var frontendFormOptionKeys = []string{
	"LinuxDOOAuthEnabled", "LinuxDOClientId", "LinuxDOClientSecret", "LinuxDOMinimumTrustLevel",
	"HeaderNavModules", "SidebarModulesAdmin", "Notice", "LogConsumeEnabled", "QuotaPerUnit",
	"USDExchangeRate", "DisplayInCurrencyEnabled", "DisplayTokenStatEnabled", "ServerAddress",
	"model_deployment.ionet.enabled", "model_deployment.ionet.api_key",
	"GitHubOAuthEnabled", "GitHubClientId", "GitHubClientSecret", "TaskPluginEnabled",
	"log_setting.user_export_enabled", "general_setting.quota_display_type", "oidc.display_name", "passkey.rp_id",
}

var optionKeyLiteral = regexp.MustCompile(`key:\s*['"]([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*)['"]\s*,\s*value\b`)

// scanFrontendOptionKeyLiterals 扫描设置页源码里 `{ key: '...', value }` 形式写入的 option key（只读 web/）。
func scanFrontendOptionKeyLiterals(t *testing.T) []string {
	t.Helper()
	roots := []string{"../web/src/features/system-settings", "../web/src/features/task-plugins"}
	seen := map[string]struct{}{}
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			t.Skipf("frontend sources not present: %s", root)
		}
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.Contains(p, "__tests__") || strings.Contains(p, ".test.") ||
				!(strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx")) {
				return nil
			}
			b, _ := os.ReadFile(p)
			for _, m := range optionKeyLiteral.FindAllStringSubmatch(string(b), -1) {
				seen[m[1]] = struct{}{}
			}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// 审查 R-P2-5 补：全新库上，前端会写入的每个 option key 都必须被通用 option PUT 视为已知，
// 否则未知 key 校验会让设置页保存 400（例：LinuxDO OAuth）。
func TestFrontendWrittenOptionKeysAreKnownOnFreshDB(t *testing.T) {
	adtyTestDB(t)
	model.InitOptionMap()
	keys := append(scanFrontendOptionKeyLiterals(t), frontendFormOptionKeys...)
	require.NotEmpty(t, keys)
	var unknown []string
	for _, k := range keys {
		if !model.IsKnownOptionKey(k) {
			unknown = append(unknown, k)
		}
	}
	require.Empty(t, unknown, "frontend writes option keys that a fresh install rejects; initialize them in model.InitOptionMap")
}
