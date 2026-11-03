package service

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type adtyMaintenanceHook struct {
	name string
	fn   func(now int64)
}

var (
	adtyMaintenanceMu    sync.RWMutex
	adtyMaintenanceHooks []adtyMaintenanceHook
	adtyMaintenanceOnce  sync.Once
	// AdtyExpiredHook 在过期任务把订单改成 expired 之后调用（P5 按通道计连续未付）。
	AdtyExpiredHook func(expired []model.ExpiredAdtyTopUp)
)

// RegisterAdtyMaintenanceHook 登记每轮维护都要跑的钩子（P5 的补查、连续未付、探活）。
func RegisterAdtyMaintenanceHook(name string, fn func(now int64)) {
	adtyMaintenanceMu.Lock()
	defer adtyMaintenanceMu.Unlock()
	for i, hook := range adtyMaintenanceHooks {
		if hook.name == name {
			adtyMaintenanceHooks[i].fn = fn
			return
		}
	}
	adtyMaintenanceHooks = append(adtyMaintenanceHooks, adtyMaintenanceHook{name: name, fn: fn})
}

// RunAdtyTopUpMaintenanceOnce 跑一轮：先把过期的 pending 改成 expired，再跑各钩子。
func RunAdtyTopUpMaintenanceOnce(now int64) {
	for {
		expired, err := model.ExpireAdtyTopUps(now, 200)
		if err != nil {
			common.SysError(common.LogText("expire adty topups: %s", err.Error()))
			break
		}
		if len(expired) > 0 && AdtyExpiredHook != nil {
			AdtyExpiredHook(expired)
		}
		if len(expired) < 200 {
			break
		}
	}
	adtyMaintenanceMu.RLock()
	hooks := append([]adtyMaintenanceHook(nil), adtyMaintenanceHooks...)
	adtyMaintenanceMu.RUnlock()
	for _, hook := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysError(common.LogText("adty maintenance hook %s panic: %v", hook.name, r))
				}
			}()
			hook.fn(now)
		}()
	}
}

// StartAdtyTopUpMaintenance 只在主节点启动；间隔默认 60 秒（ADTY_TOPUP_MAINTENANCE_INTERVAL_SECONDS）。
func StartAdtyTopUpMaintenance() {
	if !common.IsMasterNode {
		return
	}
	adtyMaintenanceOnce.Do(func() {
		interval := common.GetEnvOrDefault("ADTY_TOPUP_MAINTENANCE_INTERVAL_SECONDS", 60)
		if interval < 5 {
			interval = 5
		}
		go func() {
			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				RunAdtyTopUpMaintenanceOnce(time.Now().Unix())
			}
		}()
	})
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, StartAdtyTopUpMaintenance)
}
