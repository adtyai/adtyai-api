package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/adty_setting"
)

// 两个辅助函数放在 _test.go 里：它们接受 *testing.T，只服务测试。
// 留在生产文件里会让 service 包在构建产物中依赖 testing，
// 那是只有测试才需要的标准库，不该进二进制。
func SetAdtyMethodBonusForTest(t *testing.T, method string, enabled bool, rate string) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := cloneMethodBonus(setting.MethodBonus)
	if setting.MethodBonus == nil {
		setting.MethodBonus = map[string]adty_setting.MethodBonusRule{}
	}
	setting.MethodBonus[AdtyBonusMethod(method)] = adty_setting.MethodBonusRule{Enabled: enabled, Rate: rate}
	t.Cleanup(func() { setting.MethodBonus = old })
}

func ClearAdtyMethodBonusForTest(t *testing.T) {
	t.Helper()
	setting := adty_setting.GetTopupSetting()
	old := cloneMethodBonus(setting.MethodBonus)
	setting.MethodBonus = map[string]adty_setting.MethodBonusRule{}
	t.Cleanup(func() { setting.MethodBonus = old })
}
