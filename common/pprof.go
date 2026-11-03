package common

import (
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

var readCPUPercent = cpu.Percent
var monitorIdleSleep = 30 * time.Second

// Monitor 定时监控cpu使用率，超过阈值输出pprof文件。
// 采样失败只记日志，不能把进程打退出。
func Monitor() {
	for {
		monitorCPUOnce()
	}
}

func monitorCPUOnce() {
	percent, err := readCPUPercent(time.Second, false)
	if err != nil {
		SysLog(LogText("failed to read cpu usage: %s", err.Error()))
		time.Sleep(monitorIdleSleep)
		return
	}
	if len(percent) == 0 || percent[0] <= 80 {
		time.Sleep(monitorIdleSleep)
		return
	}
	fmt.Println(LogText("cpu usage too high"))
	if _, err := os.Stat("./pprof"); os.IsNotExist(err) {
		if err := os.Mkdir("./pprof", os.ModePerm); err != nil {
			SysLog(LogText("failed to create the pprof directory: %s", err.Error()))
			time.Sleep(monitorIdleSleep)
			return
		}
	}
	f, err := os.Create("./pprof/" + fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405")))
	if err != nil {
		SysLog(LogText("failed to create the pprof file: %s", err.Error()))
		time.Sleep(monitorIdleSleep)
		return
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		SysLog(LogText("failed to start pprof: %s", err.Error()))
		f.Close()
		time.Sleep(monitorIdleSleep)
		return
	}
	time.Sleep(10 * time.Second)
	pprof.StopCPUProfile()
	f.Close()
	time.Sleep(monitorIdleSleep)
}
