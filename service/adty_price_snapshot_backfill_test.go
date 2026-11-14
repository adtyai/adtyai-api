package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriceSnapshotBackfillFillsFromStoredValuesWithoutRewritingAmounts(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.Task{}, &model.Channel{}, &model.BillingOperation{}, &model.BillingOperationMovement{}))
	root := referralUser(t, db, "snap-fill-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	_, site := openActiveSite(t, db, "snap-fill")
	customer := siteCustomer(t, db, site, "snap-fill-customer")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)

	task := &model.Task{TaskID: "snap-fill-task", UserId: customer.Id, Group: "default", Quota: 2400, Status: model.TaskStatusInProgress, SubmitTime: 1_700_000_000}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "snap-fill-model", ModelRatio: 2, GroupRatio: 1.2, SiteMarkup: 1.25, SiteCostFactor: 0.8, SiteId: site.Id}
	task.PrivateData.BillingOperationKey = "task:hist-fill"
	require.NoError(t, db.Create(task).Error)
	op := model.BillingOperation{
		OperationKey: "task:hist-fill", UserId: customer.Id, Kind: "task", ReferenceId: task.ID,
		ReservedQuota: 2400, ActualQuota: 0, State: model.BillingReserved, SiteId: site.Id, SiteMarkup: 1.25, SiteCostFactor: 0.8, CreatedAt: 1_700_000_000,
	}
	payload, err := common.Marshal(task.PrivateData.BillingContext)
	require.NoError(t, err)
	op.BillingContext = string(payload)
	require.NoError(t, db.Create(&op).Error)

	unsafe := &model.Task{TaskID: "snap-skip-task", UserId: customer.Id, Group: "default", Quota: 1000, Status: model.TaskStatusInProgress, SubmitTime: 1_700_000_001}
	unsafe.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "snap-skip-model", ModelRatio: 2}
	require.NoError(t, db.Create(unsafe).Error)

	requestOnly := model.BillingOperation{
		OperationKey: "request:no-context", UserId: customer.Id, Kind: "request", ReservedQuota: 500, State: model.BillingReserved,
		SiteMarkup: 1.25, SiteCostFactor: 0.8, CreatedAt: 1_700_000_002,
	}
	require.NoError(t, db.Create(&requestOnly).Error)

	admin := referralUser(t, db, "snap-fill-admin")
	require.NoError(t, db.Model(admin).Update("role", common.RoleAdminUser).Error)
	_, err = ApplyPriceSnapshotBackfill(admin.Id, 50)
	require.ErrorIs(t, err, ErrPriceSnapshotBackfillForbidden)

	preview, err := PreviewPriceSnapshotBackfill(root.Id, 50)
	require.NoError(t, err)
	require.True(t, preview.DryRun)
	assert.Equal(t, 1, preview.FilledTasks)
	require.NoError(t, db.First(task, task.ID).Error)
	require.Nil(t, task.PrivateData.BillingContext.PriceSnapshot, "preview must not write")
	var still model.BillingOperation
	require.NoError(t, db.Where("operation_key = ?", "task:hist-fill").First(&still).Error)
	assert.Empty(t, still.PriceSnapshot)

	report, err := ApplyPriceSnapshotBackfill(root.Id, 50)
	require.NoError(t, err)
	require.False(t, report.DryRun)
	assert.True(t, report.AmountsPreserved)
	assert.Equal(t, 1, report.FilledTasks)
	assert.GreaterOrEqual(t, report.FilledOperations, 1)
	assert.GreaterOrEqual(t, report.Skipped, 2)

	require.NoError(t, db.First(task, task.ID).Error)
	require.NotNil(t, task.PrivateData.BillingContext.PriceSnapshot)
	assert.Equal(t, 2400, task.Quota)
	assert.Equal(t, 2.0, task.PrivateData.BillingContext.ModelRatio)
	assert.Equal(t, 1.2, task.PrivateData.BillingContext.GroupRatio)
	assert.Equal(t, 1.25, task.PrivateData.BillingContext.SiteMarkup)
	assert.Equal(t, 0.8, task.PrivateData.BillingContext.SiteCostFactor)
	snap := task.PrivateData.BillingContext.PriceSnapshot
	require.True(t, snap.HasOfficialFields())
	assert.Equal(t, "2", snap.ModelRatio)
	assert.Equal(t, "1.2", snap.BilledGroupRatio)
	assert.Equal(t, "2.4", snap.FinalPrice)

	require.NoError(t, db.Where("operation_key = ?", "task:hist-fill").First(&still).Error)
	assert.Equal(t, 2400, still.ReservedQuota)
	assert.Zero(t, still.ActualQuota)
	assert.InDelta(t, 1.25, still.SiteMarkup, 1e-12)
	assert.InDelta(t, 0.8, still.SiteCostFactor, 1e-12)
	require.NotNil(t, still.RequestPriceSnapshot())

	require.NoError(t, db.First(unsafe, unsafe.ID).Error)
	require.Nil(t, unsafe.PrivateData.BillingContext.PriceSnapshot)
	assert.Equal(t, 1000, unsafe.Quota)
	assert.Equal(t, 2.0, unsafe.PrivateData.BillingContext.ModelRatio)

	var skippedOp model.BillingOperation
	require.NoError(t, db.Where("operation_key = ?", "request:no-context").First(&skippedOp).Error)
	assert.Empty(t, skippedOp.PriceSnapshot)
	assert.Equal(t, 500, skippedOp.ReservedQuota)

	// A second apply must not rewrite already-filled rows or leftover skipped rows.
	again, err := ApplyPriceSnapshotBackfill(root.Id, 50)
	require.NoError(t, err)
	assert.Equal(t, 0, again.FilledTasks)
}

func TestPriceSnapshotBackfillResumeUsesSnapshotNotLivePrices(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}, &model.Task{}, &model.Channel{}, &model.BillingOperation{}, &model.BillingOperationMovement{}))
	root := referralUser(t, db, "snap-resume-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	_, site := openActiveSite(t, db, "snap-resume")
	customer := siteCustomer(t, db, site, "snap-resume-customer")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", 10_000_000).Error)

	savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-resume-model":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

	task := &model.Task{TaskID: "snap-resume-task", UserId: customer.Id, Group: "default", Quota: 2000, Status: model.TaskStatusSuccess, SubmitTime: 1_700_000_010}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "snap-resume-model", ModelRatio: 2, GroupRatio: 1}
	require.NoError(t, db.Create(task).Error)
	_, err := ApplyPriceSnapshotBackfill(root.Id, 50)
	require.NoError(t, err)
	require.NoError(t, db.First(task, task.ID).Error)
	require.NotNil(t, task.PrivateData.BillingContext.PriceSnapshot)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"snap-resume-model":9}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":3}`))
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ?", site.Id).Update("markup", decimal.NewFromInt(3)).Error)
	InvalidateAdtySitePriceCache()

	require.True(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000))
	assert.Equal(t, 2000, task.Quota, "1000 tokens × stored model 2 × stored group 1, not live 9×3")
}

func TestPriceSnapshotBackfillDoesNotRunDuringMigrate(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.BillingOperation{}))
	task := &model.Task{TaskID: "snap-migrate-task", UserId: 1, Group: "default", Quota: 800, SubmitTime: 11}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "snap-migrate-model", ModelRatio: 2, GroupRatio: 1}
	require.NoError(t, db.Create(task).Error)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.BillingOperation{}, &model.User{}))
	require.NoError(t, db.First(task, task.ID).Error)
	require.Nil(t, task.PrivateData.BillingContext.PriceSnapshot)
	assert.Equal(t, 800, task.Quota)
	assert.Equal(t, 2.0, task.PrivateData.BillingContext.ModelRatio)
}
