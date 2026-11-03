package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// adtyMasterTasks 是只在主节点启动的二开后台任务，后面各阶段往里加。
var adtyMasterTasks []func()

// StartAdty 在设置载入之后调用：主节点写分组种子数据、清掉上游的占位键，再启动后台任务。
func StartAdty() {
	if !common.IsMasterNode {
		return
	}
	if err := model.SeedAdtyGroups(); err != nil {
		common.SysError(common.LogText("seed adty groups: %s", err.Error()))
	}
	if err := model.ClearAdtyPlaceholderGroupRatio(); err != nil {
		common.SysError(common.LogText("clear placeholder group ratio: %s", err.Error()))
	}
	for _, task := range adtyMasterTasks {
		task()
	}
}
