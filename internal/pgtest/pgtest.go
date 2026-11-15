// Package pgtest 提供「记录 PostgreSQL 证据」的统一入口。
//
// **为什么要一个包**：绑 `t.Fatalf` / `t.Logf` 的 helper 必须收 `testing.TB`，
// 而 `model` 是生产包——生产代码 import "testing" 本身是一条已签的 finding
// （`ISS-R4-0022`）。所以这一层单独放，只有 `_test.go` 会 import 它，
// 它也就不进任何二进制。
package pgtest

import (
	"testing"

	"gorm.io/gorm"
)

// RecordServerVersion 把当前连接的 PostgreSQL 版本记成证据，并返回 version_num。
//
// **它不断言具体版本号**。原来的写法是
// `require.Equal(t, 170012, version, "证据必须来自 PostgreSQL 17.12")`——
// 把「我跑的是 17.12」变成了通过条件：升到 17.13、或者别人用 17.14 跑，
// 测试会红，**而它要验的东西与补丁号无关**。那种红是假失败，会被人用
// 「改一下断言里的数字」消掉，然后这个约束就变成一个没人看的常量。
//
// 版本号仍然出现一次，出现在日志里，可以贴进交付；但它不再是判据。
func RecordServerVersion(t testing.TB, db *gorm.DB) string {
	t.Helper()
	// 这里自己发这两条 SQL，而不是调 `model.OfficialPostgresServerVersion`：
	// `model` 包内的测试是 `package model`（内部测试），它们 import 本包时，
	// 本包再 import `model` 就是 import cycle。**所以本包不能依赖 model。**
	var versionNum, version string
	if err := db.Raw("SHOW server_version_num").Scan(&versionNum).Error; err != nil {
		t.Fatalf("read server_version_num as evidence: %v", err)
	}
	if err := db.Raw("SHOW server_version").Scan(&version).Error; err != nil {
		t.Fatalf("read server_version as evidence: %v", err)
	}
	if versionNum == "" {
		t.Fatal("live server_version_num must be recorded as evidence")
	}
	t.Logf("evidence only (not a pass/fail condition): server_version_num=%s server_version=%s", versionNum, version)
	return versionNum
}
