package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 只有落在这个包里的测试才用得上 SetSysLogSink 本身；跨包用 common/logsinktest。
func TestSysLogSinkRestoresPrevious(t *testing.T) {
	var captured []string
	restore := SetSysLogSink(func(level, message string) {
		captured = append(captured, level+" "+message)
	})
	t.Cleanup(restore)

	SysLog("a log line")
	SysError("an error line")
	assert.Contains(t, captured, "LOG a log line")
	assert.Contains(t, captured, "ERROR an error line", "级别要能区分，否则告警和日志没法分开验")

	restore()
	SysLog("while detached")
	assert.Len(t, captured, 2, "restore 之后不该再写进原来的 sink")
}
