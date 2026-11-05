package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Every adty key in keys_adty.go is in every loaded adty yaml and renders to
// real text; an adty yaml has no keys that keys_adty.go does not declare.
func TestAdtyLocalesRenderEveryKey(t *testing.T) {
	require.NoError(t, Init())
	keysFile, err := parser.ParseFile(token.NewFileSet(), "keys_adty.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	var declared []string
	ast.Inspect(keysFile, func(node ast.Node) bool {
		if spec, ok := node.(*ast.ValueSpec); ok {
			for _, value := range spec.Values {
				if literal, ok := value.(*ast.BasicLit); ok {
					key, err := strconv.Unquote(literal.Value)
					require.NoError(t, err)
					declared = append(declared, key)
				}
			}
		}
		return true
	})
	slices.Sort(declared)
	require.NotEmpty(t, adtyLocaleFiles)
	assert.Len(t, adtyLocaleFiles, 8)
	for _, file := range adtyLocaleFiles {
		data, err := localeFS.ReadFile(file)
		require.NoError(t, err)
		var messages map[string]string
		require.NoError(t, yaml.Unmarshal(data, &messages))
		keys := make([]string, 0, len(messages))
		for key := range messages {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		assert.Equal(t, declared, keys, file)
		lang := strings.TrimSuffix(strings.TrimPrefix(file, "locales/adty."), ".yaml")
		for _, key := range keys {
			text := Translate(lang, key, map[string]any{"Name": "n", "Count": 1, "Amount": "1", "Txn": "t", "Reason": "r"})
			assert.NotEqual(t, key, text, "%s %s", lang, key)
			assert.NotContains(t, text, "{{", "%s %s", lang, key)
		}
	}
}
