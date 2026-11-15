package pgtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pinnedVersionAssertion 匹配「把 PostgreSQL 补丁号当通过条件」的写法：
//
//	require.Equal(t, 170012, version, ...)
//	require.EqualValues(t, 170012, ...)
//
// 只要数字是五到六位的 `17xxxx`，就是同一个错——**它把「我跑的是 17.12」
// 变成了测试通过的条件**，而测试要验的（扣费、退款、迁移、可见性）与补丁号无关。
// 升到 17.13 就红、改个数字就绿，这个约束于是变成一个没人看的常量。
var pinnedVersionAssertion = regexp.MustCompile(`require\.(Equal|EqualValues)\(t, 17\d{4},`)

// 这条是**门禁**：它不验产品行为，只防「有人又把版本号钉回去」。
// 判据是可执行的，不是一句「注意不要」。新增文件同样被扫到——这是它比
// code review 强的地方。
func TestNoPinnedPostgresPatchLevelAssertions(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "web" || name == "docs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// **先剥注释再匹配。** 注释里引用这种写法是正常的（这条门禁自己的
		// 说明文字就引用了它），不剥的话门禁会把自己的说明判成违例——
		// 今天这是第三次同一个坑（`TestBillingSessionAlwaysCarriesOperationKey`
		// 那次是自己的注释里写了构造点字面量）。
		if pinnedVersionAssertion.MatchString(stripLineComments(string(raw))) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk repo: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("这些文件把 PostgreSQL 补丁号钉成了通过条件，改成 pgtest.RecordServerVersion(t, db)：\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// helper 自己也要能记录证据（否则 17 处替换等于换了个名字）。
func TestRecordServerVersionNeedsLiveConnection(t *testing.T) {
	// 不连库，只验「没有连接时它会 Fatal 而不是静默通过」这件事无法在进程内
	// 断言（Fatal 会终止测试），所以这里只钉住「它必须读写 server_version_num」。
	raw, err := os.ReadFile("pgtest.go")
	if err != nil {
		t.Fatalf("read helper: %v", err)
	}
	source := string(raw)
	for _, want := range []string{"SHOW server_version_num", "SHOW server_version", "evidence only"} {
		if !strings.Contains(source, want) {
			t.Fatalf("helper 里缺少 %q：它必须真的读到版本并把它记成证据", want)
		}
	}
}

// stripLineComments 丢掉整行注释。本仓库的注释都是整行行注释。
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
