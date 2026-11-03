package common

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 品牌守卫（任务 8.9）：访客、调用方、上游服务商和浏览器开发者工具能看到的
// 上游名字已经换成 ADTY AI / adtyai。合并上游时新带进来的这些写法会在这里报出来，
// 改法见方案任务 8.9 的表，「不改」的范围见 docs/adty/conventions.md。
var adtyForbiddenBrandValues = []string{
	"new_api_error",
	"new_api_panic",
	"new_api_refresh",
	"new_api_has_session",
	"new-api-dashboard",
	"X-New-Api-Other-Ratios",
	"Calcium-Ion/new-api",
	"docs.newapi.pro",
	"newapi:",
	"new-api:",
	"NEW_API_KEY",
}

// 只放「不改」清单里的文件：Redis 键和锁的名字只在服务器上。
var adtyBrandGuardAllowed = map[string][]string{
	"pkg/wsmanager/wsmanager.go":  {"new-api:"},
	"service/channel_affinity.go": {"new-api:"},
}

func TestAdtyBrandGuard(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "main.go"))

	var violations []string
	scanned := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "web", "node_modules", "vendor", "electron":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for lineNo, line := range strings.Split(string(content), "\n") {
			for _, value := range adtyForbiddenBrandValues {
				if !strings.Contains(line, value) {
					continue
				}
				if !slices.Contains(adtyBrandGuardAllowed[rel], value) {
					violations = append(violations, rel+":"+strconv.Itoa(lineNo+1)+" "+value)
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, scanned, 100, "扫描的 Go 文件太少，根目录可能不对")
	assert.Empty(t, violations, "出现了上游的对外名字，按方案任务 8.9 的表改掉")

	mainGo, err := os.ReadFile(filepath.Join(root, "main.go"))
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(mainGo), "middleware.Version()"), "main.go 不再挂 middleware.Version()，不发版本号响应头（任务 8.9）")
}
