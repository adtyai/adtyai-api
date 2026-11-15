package service

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这条是**守卫**，不是覆盖。
//
// `BillingSession.Settle` 里那段 legacy 结算路径（`funding.Settle` →
// `ChargeUserQuotaUpTo`）运行期不可达，唯一原因是生产只有一个构造点、
// 而它总把 `operationKey` 设成非空。**那个不变量没有任何强制。**
//
// 所以这条测试守的是「只有一个构造点」这件事本身：
// **谁新增第二个 `BillingSession{}` 构造点、并且可能不设 operationKey，
// 这里就红。** 红了不代表新代码一定错，代表「先去读 ISS-R4-0046 再决定
// 那段 legacy 分支要不要保留」。
func TestBillingSessionAlwaysCarriesOperationKey(t *testing.T) {
	raw, err := os.ReadFile("billing_session.go")
	require.NoError(t, err)
	// **先去掉整行注释再数**：构造点本身是代码，而注释里提到 `&BillingSession{}`
	// 不构成构造点。我第一版没做这一步，于是自己的注释把这条守卫判红了——
	// 一个「守卫把守卫者的注释当成违例」的假阳性。
	source := stripLineComments(string(raw))

	// 统计 `&BillingSession{` 的出现次数。构造点每多一处，这条就红。
	const marker = "&BillingSession{"
	count := strings.Count(source, marker)
	assert.Equal(t, 1, count,
		"BillingSession 的构造点从 1 变成 %d 处：新的构造点若可能不设 operationKey，Settle 的 legacy 分支会静默复活（见 ISS-R4-0046）", count)

	// 唯一那个构造点里必须给 operationKey 赋值。用「构造块内是否出现
	// operationKey:」判，而不是全文件搜——全文件搜会被 `s.operationKey` 干扰。
	start := strings.Index(source, marker)
	require.Positive(t, start)
	block := source[start:]
	if end := strings.Index(block, "}"); end > 0 {
		block = block[:end]
	}
	assert.Contains(t, block, "operationKey:",
		"构造 BillingSession 时必须设 operationKey，否则 Settle 会走 legacy 分支")
}

// 这条钉住「有且只有一处生产构造点」的另一个侧面：包外不能再有
// `BillingSession{`（比如在别的文件里直接拼一个）。
func TestNoOtherBillingSessionLiteralsInService(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, readErr := os.ReadFile(name)
		require.NoError(t, readErr)
		source := stripLineComments(string(raw))
		if name == "billing_session.go" {
			continue
		}
		assert.NotContains(t, source, "&BillingSession{",
			"%s 里新增了 BillingSession 构造点：settle 的 legacy 分支会静默复活（见 ISS-R4-0046）", name)
	}
}

// stripLineComments 丢掉整行注释。只处理 `//` 开头的行——本仓库的注释都是整行
// 行注释，不需要处理行尾注释或块注释。
func stripLineComments(source string) string {
	lines := strings.Split(source, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
