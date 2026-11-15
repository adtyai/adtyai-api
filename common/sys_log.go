package common

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogWriterMu protects concurrent access to gin.DefaultWriter/gin.DefaultErrorWriter
// during log file rotation. Acquire RLock when reading/writing through the writers,
// acquire Lock when swapping writers and closing old files.
var LogWriterMu sync.RWMutex

// sysLogSink 是 SysLog / SysError 的可替换出口，测试用 SetLogSinkForTest 装上。
//
// 为什么需要它：验「某条日志确实发出去了」的测试原来只能自己持 LogWriterMu
// 换掉 gin.DefaultWriter/DefaultErrorWriter。**那把锁是实现的细节**，让每个测试
// 复述它，等于要求每个写测试的人都记得同一件事——而「记得」是最不可靠的机制。
//
// 这里故意**不导出**这个变量：读写都走函数，测试通过 SetLogSinkForTest 绑 t，
// 生产代码没有理由去动它。
var sysLogSink func(level, message string)

func sysLogWrite(level, message string) {
	sysLogSinkMu.RLock()
	sink := sysLogSink
	sysLogSinkMu.RUnlock()
	if sink != nil {
		sink(level, message)
		return
	}
	t := time.Now()
	LogWriterMu.RLock()
	writer := gin.DefaultWriter
	if level == "ERROR" {
		writer = gin.DefaultErrorWriter
	}
	_, _ = fmt.Fprintf(writer, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), message)
	LogWriterMu.RUnlock()
}

// sysLogSinkMu 只保护那个函数变量本身，和 LogWriterMu（保护 gin 的 writer）是
// 两把不同的锁，不要合并——sink 被替换时不该阻塞正在写文件的调用。
var sysLogSinkMu sync.RWMutex

// SetSysLogSink 装上（或传 nil 卸下）日志出口，返回恢复函数。
//
// **这里故意不收 `*testing.T`。** 生产代码 import "testing" 本身就是一条已签的
// finding（`ISS-R4-0022`），而这个 hook 存在的理由恰恰是给测试用——用一个已被
// 判定为缺陷的形态去实现它，是拿同一个错解上一道题。
//
// 所以分工是：这里只提供「换 / 恢复」这个原语，**绑 `t.Cleanup` 的那层放在各包
// 的 `_test.go` 里**（`SetLogSinkForTest(t, sink)`）。调用方仍然不需要记得还原，
// 因为 `t` 那一侧把 restore 注册进 cleanup 了。
func SetSysLogSink(sink func(level, message string)) (restore func()) {
	sysLogSinkMu.Lock()
	previous := sysLogSink
	sysLogSink = sink
	sysLogSinkMu.Unlock()
	return func() {
		sysLogSinkMu.Lock()
		sysLogSink = previous
		sysLogSinkMu.Unlock()
	}
}

func SysLog(s string)   { sysLogWrite("LOG", s) }
func SysError(s string) { sysLogWrite("ERROR", s) }

func FatalLog(v ...any) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[FATAL] %v | %v \n", t.Format("2006/01/02 - 15:04:05"), v)
	LogWriterMu.RUnlock()
	os.Exit(1)
}

func LogStartupSuccess(startTime time.Time, port string) {
	duration := time.Since(startTime)
	durationMs := duration.Milliseconds()

	// Get network IPs
	networkIps := GetNetworkIps()

	LogWriterMu.RLock()
	defer LogWriterMu.RUnlock()

	if SessionCookieSecure == false {
		// Warn when the local HTTP compatibility mode disables cookie transport
		// security and refresh/logout Origin validation.
		fmt.Fprintf(gin.DefaultWriter, "\n")
		fmt.Fprintf(gin.DefaultWriter, "  \033[33m%s\033[0m\n", LogText("Warning: Refresh cookie is not secure and refresh/logout Origin validation is disabled. Please set SESSION_COOKIE_SECURE=true in production."))
		fmt.Fprintf(gin.DefaultWriter, "\n")
	}

	fmt.Fprintf(gin.DefaultWriter, "\n")
	fmt.Fprintf(gin.DefaultWriter, "  \033[32m%s %s\033[0m  %s\n", SystemName, Version, LogText("ready in %d ms", durationMs))
	fmt.Fprintf(gin.DefaultWriter, "\n")

	if !IsRunningInContainer() {
		fmt.Fprintf(gin.DefaultWriter, "  ➜  \033[1m%-8s\033[0m http://localhost:%s/\n", LogText("Local:"), port)
	}

	for _, ip := range networkIps {
		fmt.Fprintf(gin.DefaultWriter, "  ➜  \033[1m%-8s\033[0m http://%s:%s/\n", LogText("Network:"), ip, port)
	}

	fmt.Fprintf(gin.DefaultWriter, "\n")
}
