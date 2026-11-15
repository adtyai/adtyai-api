package logsinktest

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureCollectsBothLevels(t *testing.T) {
	buffer := Capture(t)
	common.SysLog("a log line")
	common.SysError("an error line")

	assert.Contains(t, buffer.String(), "LOG a log line")
	assert.Contains(t, buffer.String(), "ERROR an error line", "级别要能区分，否则告警和日志没法分开验")
}

// 还原必须绑在 t 上：嵌套安装按栈回退，调用方不需要记得。
func TestCaptureRestoresOnCleanup(t *testing.T) {
	outer := Capture(t)
	inner := &Buffer{}
	SetSink(t, inner.sink)

	common.SysLog("goes to inner")
	assert.Contains(t, inner.String(), "goes to inner")
	assert.NotContains(t, outer.String(), "goes to inner", "内层接管期间不该写到外层")
}

// 没有 sink 时不能崩——生产路径不能因为测试基础设施而变形。
func TestNoSinkDoesNotPanic(t *testing.T) {
	restore := common.SetSysLogSink(nil)
	t.Cleanup(restore)
	require.NotPanics(t, func() { common.SysLog("no sink installed") })
}
