// Package logsinktest 给其它包的测试提供「把日志接到内存」的入口。
//
// **为什么单独一个包**：绑 `t.Cleanup` 的版本必须收 `*testing.T`，而
// `common` 是生产包——生产代码 import "testing" 本身就是一条已签的 finding
// （`ISS-R4-0022`）。所以把那一层放在这里，只有 `_test.go` 会 import 它，
// 它也就不会进任何二进制。
//
// 用法：
//
//	buffer := logsinktest.Buffer(t)
//	doSomething()
//	require.Contains(t, buffer.String(), "expected")
package logsinktest

import (
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// Buffer 是线程安全的日志缓冲。
type Buffer struct {
	mu   sync.Mutex
	text strings.Builder
}

func (b *Buffer) sink(level, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.WriteString(level)
	b.text.WriteString(" ")
	b.text.WriteString(message)
	b.text.WriteString("\n")
}

// String 返回截至目前的全部输出。级别前缀是 `LOG` / `ERROR`。
func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

// SetSink 装上自定义 sink，并在 t 结束时还原。**调用方不需要记得还原。**
func SetSink(t *testing.T, sink func(level, message string)) {
	t.Helper()
	restore := common.SetSysLogSink(sink)
	t.Cleanup(restore)
}

// Capture 装上缓冲并返回它。
func Capture(t *testing.T) *Buffer {
	t.Helper()
	buffer := &Buffer{}
	SetSink(t, buffer.sink)
	return buffer
}
