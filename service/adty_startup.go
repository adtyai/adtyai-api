package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// adtyMasterTasks 是只在主节点启动的二开后台任务，后面各阶段往里加。
var adtyMasterTasks []func()

// StartAdty 在设置载入之后调用：主节点写分组种子数据、写入官方术语表、清掉上游的占位键，再启动后台任务。
func StartAdty() {
	if !common.IsMasterNode {
		return
	}
	if err := model.SeedAdtyGroups(); err != nil {
		common.SysError(common.LogText("seed adty groups: %s", err.Error()))
	}
	if err := SeedAdtyOfficialGlossary(); err != nil {
		common.SysError(common.LogText("seed official glossary: %s", err.Error()))
	}
	if err := model.ClearAdtyPlaceholderGroupRatio(); err != nil {
		common.SysError(common.LogText("clear placeholder group ratio: %s", err.Error()))
	}
	// 系统翻译（ISS-R4-0012）：Root 打开了翻译但协议或地址填不全时，启动日志里说清楚，
	// 否则后台看起来一切正常，只是每一条动态译文都停在「英文未就绪」。
	if err := CheckAdtyTranslationProvider(); err != nil {
		common.SysError(common.LogText("adty translation enabled but unusable: %s", err.Error()))
	}
	for _, task := range adtyMasterTasks {
		task()
	}
}
