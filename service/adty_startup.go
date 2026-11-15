package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// adtyMasterTasks 是只在主节点启动的二开后台任务，后面各阶段往里加。
var adtyMasterTasks []func()

// CheckAdtyStartupSchema 在启动时点名「迁移没跑全」的必备表，返回缺表错误（若有）。
//
// 为什么要在启动时查：退款路径遇到缺表只能放行——报错会把退款一起回滚，钱就退不
// 回去（见 `model.ReleaseAdtyFundAllocationsTx`）。所以缺表不会自己冒出来，它只会让
// **整段部署窗口**里的每一次退款都「没有分摊可释放」，而库里零痕迹。把这件事从
// 运行期提前到启动期，运维才知道「从这次启动起，退款不可对账」，也才有窗口可查。
//
// 只告警、不拒绝启动：退款放行本身是对的，把启动拦下来会让已经上线的实例一起停摆，
// 那是比缺痕更坏的后果。告警必须带影响范围，否则它只会被当成噪声。
func CheckAdtyStartupSchema(db *gorm.DB) error {
	if err := model.AdtyFundAllocationTableReady(db); err != nil {
		common.SysError(common.LogText(
			"adty schema incomplete: %s — every refund from this start on runs without releasing fund allocations, so this whole window's refunds leave no allocation trail to reconcile",
			err.Error()))
		return err
	}
	return nil
}

// StartAdty 在设置载入之后调用：主节点写分组种子数据、写入官方术语表、清掉上游的占位键，再启动后台任务。
func StartAdty() {
	// 必备表检查与主从无关：每个节点都会跑退款路径。
	_ = CheckAdtyStartupSchema(model.DB)
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
	for _, task := range adtyMasterTasks {
		task()
	}
}
