package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOfficialPostgresBillingRecoveryDSNRejectsNonPostgres 确认非 PostgreSQL
// 连接串不能作为 ADTY-23-10 的关闭条件；缺 DSN 必须标成 BLK-ENV-PG。
func TestOfficialPostgresBillingRecoveryDSNRejectsNonPostgres(t *testing.T) {
	t.Setenv("ADTY_TEST_PG_DSN", "root:pass@tcp(127.0.0.1:3306)/new-api")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err := model.OfficialPostgresAcceptanceDSN()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not PostgreSQL")

	t.Setenv("ADTY_TEST_PG_DSN", "")
	t.Setenv("TEST_POSTGRES_DSN", "")
	_, err = model.OfficialPostgresAcceptanceDSN()
	require.ErrorIs(t, err, model.ErrPostgresAcceptanceDSNUnset)
	assert.Contains(t, err.Error(), "BLK-ENV-PG")

	t.Setenv("TEST_POSTGRES_DSN", "file:memory?mode=memory&cache=shared")
	_, err = model.OfficialPostgresAcceptanceDSN()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not PostgreSQL")
}

func requireOfficialPostgresRecoveryDSN(t *testing.T) string {
	t.Helper()
	dsn, err := model.OfficialPostgresAcceptanceDSN()
	if errors.Is(err, model.ErrPostgresAcceptanceDSNUnset) {
		t.Skip("BLK-ENV-PG: ADTY_TEST_PG_DSN / TEST_POSTGRES_DSN is unset; this acceptance is not recorded as passed")
	}
	require.NoError(t, err)
	return dsn
}

// TestAdtyPostgreSQLBillingRecoveryAcceptance 是 ADTY-23-10 / A.21.5 在真实
// PostgreSQL 上的仓库内关闭条件。它不杀死或清空共享集群与共享 Redis，也不把
// FLOW-014、ADTY-23-04 或计划级故障演练记为已关闭。
func TestAdtyPostgreSQLBillingRecoveryAcceptance(t *testing.T) {
	dsn := requireOfficialPostgresRecoveryDSN(t)

	admin, err := model.OpenOfficialPostgresAcceptance(dsn)
	if err != nil {
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	if err := adminSQL.Ping(); err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	var current string
	if err := admin.Raw("SELECT current_database()").Scan(&current).Error; err != nil {
		_ = model.CloseOfficialGorm(admin)
		t.Fatalf("BLK-ENV-PG: PostgreSQL instance is unreachable: %v", err)
	}
	require.NotEmpty(t, current)
	require.False(t, model.OfficialThrowawayPostgresName(current), "admin DSN must not already point at a throwaway database")
	require.NotEqual(t, "", current)

	versionNum, version, err := model.OfficialPostgresServerVersion(admin)
	require.NoError(t, err)
	require.NotEmpty(t, versionNum, "live server_version_num must be recorded as evidence")
	t.Logf("ADTY-23-10 evidence only (not a pass/fail condition): server_version_num=%s server_version=%s driver=%s", versionNum, version, admin.Dialector.Name())
	t.Cleanup(func() {
		require.NoError(t, model.CloseOfficialGorm(admin))
	})

	name, err := model.NewOfficialThrowawayPostgresName(model.OfficialPostgresThrowawayPrefix2310)
	require.NoError(t, err)
	db, err := model.CreateOfficialThrowawayPostgresDatabase(admin, dsn, name)
	require.NoError(t, err, "create throwaway database %s", name)
	t.Logf("throwaway database: %s", name)

	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	restoreDB := model.SwapOfficialPackageDatabase(db, common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	dropped := false
	t.Cleanup(func() {
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		if dropped {
			return
		}
		dropped = true
		_ = model.CloseOfficialGorm(db)
		restoreDB()
		if dropErr := model.DropOfficialThrowawayPostgresDatabase(admin, name); dropErr != nil {
			t.Errorf("drop throwaway database %s: %v", name, dropErr)
		}
	})

	require.NoError(t, model.RunOfficialMigrateDB(), "official migrate must initialize the throwaway database")

	oldLease := billingProcessLeaseOwner
	t.Cleanup(func() {
		billingProcessLeaseOwner = oldLease
		clearActiveBillingReservations()
	})

	var historicalKey string
	var historicalSnapshot string
	var historicalWallet, historicalToken, historicalMovements int

	t.Run("interrupted_reserve_leaves_no_unilateral_effect", func(t *testing.T) {
		const userID, tokenID = 23101, 23101
		seedRecoveryUser(t, userID, 1000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23101", 1000)
		installBillingInterruptTrigger(t, "adty_fail_reserve_23101", "users", userID)

		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23101")
		info.RequestId = "recov-reserve-abort"
		apiErr := PreConsumeBilling(c, 100, info)
		require.NotNil(t, apiErr, "official reserve must abort when the funding transaction is interrupted")

		assert.Equal(t, 1000, getUserQuota(t, userID), "aborted reserve must not debit the wallet")
		assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID), "aborted reserve must not debit the token")
		assert.Zero(t, countBillingOperations(t, userID), "rolled-back reserve must not leave a durable operation")
		assert.Zero(t, sumBillingMovements(t, userID), "rolled-back reserve must not leave a ledger movement")
		assert.Equal(t, 0, getUserQuotaPending(t, userID))
	})

	t.Run("interrupted_settle_apply_then_restart_applies_once", func(t *testing.T) {
		const userID, tokenID = 23102, 23102
		seedRecoveryUser(t, userID, 1000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23102", 1000)
		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23102")
		info.RequestId = "recov-settle-abort"
		require.Nil(t, PreConsumeBilling(c, 100, info))
		require.Equal(t, 900, getUserQuota(t, userID))
		require.Equal(t, 900, getTokenRemainQuota(t, tokenID))
		key := billingSessionKey(t, info)
		beforeMovements := countOperationMovements(t, key)

		dropInterrupt := installBillingInterruptTrigger(t, "adty_fail_settle_23102", "tokens", tokenID)
		require.Error(t, info.Billing.Settle(80), "official settle apply must abort when the funding transaction is interrupted")
		assert.Equal(t, 900, getUserQuota(t, userID), "failed settle apply must leave only the already-reserved amount")
		assert.Equal(t, 900, getTokenRemainQuota(t, tokenID))
		assert.Equal(t, beforeMovements, countOperationMovements(t, key), "failed apply must not write a partial settle movement")
		op, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingSettlePending, op.State, "queued settle intent must remain durable after the apply rolls back")
		assert.Equal(t, 80, op.ActualQuota)

		// 事务中断只注入在本次 apply 中。新进程恢复时不再保留该故障注入。
		dropInterrupt()
		simulateBillingProcessRestart()
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		assert.Equal(t, 920, getUserQuota(t, userID), "pending settlement must apply once after restart")
		assert.Equal(t, 920, getTokenRemainQuota(t, tokenID))
		op, err = model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingSettled, op.State)
		assert.Equal(t, 80, op.AppliedQuota)
		assert.Equal(t, beforeMovements+1, countOperationMovements(t, key), "replay must not write a second apply movement")
		assert.Equal(t, -80, sumOperationWalletDelta(t, key))
	})

	t.Run("restart_refunds_abandoned_reservation_once", func(t *testing.T) {
		const userID, tokenID = 23103, 23103
		seedRecoveryUser(t, userID, 1000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23103", 1000)
		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23103")
		info.RequestId = "recov-abandoned"
		require.Nil(t, PreConsumeBilling(c, 100, info))
		key := billingSessionKey(t, info)
		require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("operation_key = ?", key).
			UpdateColumn("updated_at", time.Now().Add(-10*time.Minute).Unix()).Error)

		logsBefore := countUserLogs(t, userID)
		simulateBillingProcessRestart()
		require.False(t, reservationIsInProcessMemory(key), "recovery after process death must not depend on the dead process map")
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))

		assert.Equal(t, 1000, getUserQuota(t, userID), "an abandoned reservation must be refunded once from durable state")
		assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
		op, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingRefunded, op.State)
		assert.Zero(t, sumOperationWalletDelta(t, key))
		assert.Equal(t, logsBefore, countUserLogs(t, userID), "replay must not emit a second fund-result notification")

		historicalKey = key
		historicalSnapshot = op.PriceSnapshot
		historicalWallet = getUserQuota(t, userID)
		historicalToken = getTokenRemainQuota(t, tokenID)
		historicalMovements = countOperationMovements(t, key)
	})

	t.Run("restart_applies_refund_pending_once", func(t *testing.T) {
		const userID, tokenID = 23104, 23104
		seedRecoveryUser(t, userID, 1000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23104", 1000)
		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23104")
		info.RequestId = "recov-refund-pending"
		require.Nil(t, PreConsumeBilling(c, 100, info))
		key := billingSessionKey(t, info)
		require.NoError(t, model.QueueBillingSettlement(key, 0, true))
		op, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		require.Equal(t, model.BillingRefundPending, op.State)
		require.Equal(t, 900, getUserQuota(t, userID))

		logsBefore := countUserLogs(t, userID)
		simulateBillingProcessRestart()
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		assert.Equal(t, 1000, getUserQuota(t, userID))
		assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
		op, err = model.GetBillingOperation(key)
		require.NoError(t, err)
		assert.Equal(t, model.BillingRefunded, op.State)
		assert.Zero(t, sumOperationWalletDelta(t, key))
		assert.Equal(t, logsBefore, countUserLogs(t, userID), "replay must not emit a second fund-result notification")
	})

	t.Run("restart_finalizes_terminal_task_once", func(t *testing.T) {
		const userID, tokenID, channelID = 23105, 23105, 23105
		seedRecoveryUser(t, userID, 1000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23105", 1000)
		seedRecoveryChannel(t, channelID)
		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23105")
		info.RequestId = "recov-terminal-task"
		require.Nil(t, PreConsumeBilling(c, 100, info))
		task := &model.Task{
			TaskID:     "t23105",
			UserId:     userID,
			ChannelId:  channelID,
			Quota:      100,
			Status:     model.TaskStatus(model.TaskStatusInProgress),
			Group:      "default",
			Properties: model.Properties{OriginModelName: "recov-term-model"},
			PrivateData: model.TaskPrivateData{
				BillingSource:  BillingSourceWallet,
				TokenId:        tokenID,
				BillingContext: &model.TaskBillingContext{OriginModelName: "recov-term-model", GroupRatio: 1, ModelRatio: 1},
			},
		}
		AttachTaskBillingReservation(info, task)
		require.NoError(t, task.Insert())
		require.NoError(t, info.Billing.Settle(100))
		require.Equal(t, 900, getUserQuota(t, userID))

		require.NoError(t, model.DB.First(task, task.ID).Error)
		old := task.Status
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		task.FailReason = "upstream failed after restart"
		won, err := task.UpdateWithStatus(old)
		require.NoError(t, err)
		require.True(t, won)

		logsBefore := countUserLogs(t, userID)
		simulateBillingProcessRestart()
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		assert.Equal(t, 1000, getUserQuota(t, userID), "a terminal failure must refund the reservation once after restart")
		assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
		assert.Zero(t, getTaskQuota(t, task.ID))
		assert.Equal(t, logsBefore+1, countUserLogs(t, userID), "the fund-result log must be written once")
	})

	t.Run("flow012_original_price_snapshot_after_restart", func(t *testing.T) {
		const userID, tokenID, channelID = 23106, 23106, 23106
		seedRecoveryUser(t, userID, 100_000)
		seedRecoveryToken(t, tokenID, userID, "sk-r23106", 100_000)
		seedRecoveryChannel(t, channelID)

		savedRatios, savedGroups := ratio_setting.ModelRatio2JSONString(), ratio_setting.GroupRatio2JSONString()
		t.Cleanup(func() {
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
		})
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"recov-snap-model":2}`))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))

		c := adtyWalletRelayContext()
		info := adtyWalletRelayInfo(userID, tokenID, "sk-r23106")
		info.RequestId = "recov-snapshot"
		info.OriginModelName = "recov-snap-model"
		info.UsingGroup = "default"
		info.PriceData = hosttypes.PriceData{ModelRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}
		info.PriceData.GroupRatioInfo.SetBilledGroupRatio(decimal.NewFromInt(1))
		AdtyFreezeRequestPriceSnapshot(c, info)
		snap := AdtyFrozenPriceSnapshot(c)
		require.NotNil(t, snap, "official reserve path must store a price snapshot")
		assert.Equal(t, "2", snap.ModelRatio)
		assert.Equal(t, "1", snap.BilledGroupRatio)

		require.Nil(t, PreConsumeBilling(c, 4_000, info))
		key := billingSessionKey(t, info)
		reserved, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		stored := reserved.RequestPriceSnapshot()
		require.NotNil(t, stored, "official billing operation must persist the frozen snapshot")
		assert.Equal(t, snap.ModelRatio, stored.ModelRatio)
		assert.Equal(t, snap.FinalPrice, stored.FinalPrice)
		assert.Equal(t, snap.BilledGroupRatio, stored.BilledGroupRatio)
		frozenSnapshot := reserved.PriceSnapshot

		task := &model.Task{
			TaskID:     "t23106",
			UserId:     userID,
			ChannelId:  channelID,
			Quota:      4_000,
			Status:     model.TaskStatus(model.TaskStatusInProgress),
			Group:      "default",
			Properties: model.Properties{OriginModelName: "recov-snap-model"},
			PrivateData: model.TaskPrivateData{
				BillingSource: BillingSourceWallet,
				TokenId:       tokenID,
				BillingContext: &model.TaskBillingContext{
					OriginModelName: "recov-snap-model",
					ModelRatio:      2,
					GroupRatio:      1,
					PriceSnapshot:   snap,
				},
			},
		}
		AttachTaskBillingReservation(info, task)
		require.NoError(t, task.Insert())
		require.NoError(t, info.Billing.Settle(4_000))
		require.Equal(t, 96_000, getUserQuota(t, userID))

		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"recov-snap-model":9}`))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":3}`))

		require.NoError(t, model.DB.First(task, task.ID).Error)
		old := task.Status
		task.Status, task.Progress = model.TaskStatusSuccess, "100%"
		task.PrivateData.CompletionBilling = &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 1000}
		won, err := task.UpdateWithStatus(old)
		require.NoError(t, err)
		require.True(t, won)

		previous := GetTaskAdaptorFunc
		GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &mockAdaptor{adjustReturn: 0} }
		t.Cleanup(func() { GetTaskAdaptorFunc = previous })

		simulateBillingProcessRestart()
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))
		require.NoError(t, RecoverBillingOperations(context.Background(), 100))

		// 1000 tokens × frozen model 2 × frozen billed group 1 = 2000, not live 9×3.
		assert.Equal(t, 98_000, getUserQuota(t, userID), "terminal settlement after restart must use the original price snapshot")
		assert.Equal(t, 98_000, getTokenRemainQuota(t, tokenID))
		assert.Equal(t, 2_000, getTaskQuota(t, task.ID))

		finalOp, err := model.GetBillingOperation(key)
		require.NoError(t, err)
		afterSnap := finalOp.RequestPriceSnapshot()
		require.NotNil(t, afterSnap)
		assert.Equal(t, "2", afterSnap.ModelRatio)
		assert.Equal(t, "1", afterSnap.BilledGroupRatio)
		assert.Equal(t, snap.FinalPrice, afterSnap.FinalPrice)
		assert.NotEqual(t, "9", afterSnap.ModelRatio)
		// 结算可以补写已消耗额度，但不能改计价口径。
		assertOfficialPriceFieldsUnchanged(t, frozenSnapshot, finalOp.PriceSnapshot)

		require.NoError(t, model.DB.First(task, task.ID).Error)
		require.NotNil(t, task.PrivateData.BillingContext)
		require.NotNil(t, task.PrivateData.BillingContext.PriceSnapshot)
		assert.Equal(t, "2", task.PrivateData.BillingContext.PriceSnapshot.ModelRatio)
		assert.Equal(t, "1", task.PrivateData.BillingContext.PriceSnapshot.BilledGroupRatio)
	})

	t.Run("historical_financial_facts_are_preserved", func(t *testing.T) {
		require.NotEmpty(t, historicalKey, "earlier refunded reservation must still be addressable")
		op, err := model.GetBillingOperation(historicalKey)
		require.NoError(t, err)
		assert.Equal(t, model.BillingRefunded, op.State)
		assert.Equal(t, historicalSnapshot, op.PriceSnapshot)
		assert.Equal(t, historicalMovements, countOperationMovements(t, historicalKey))
		assert.Zero(t, sumOperationWalletDelta(t, historicalKey))
		assert.Equal(t, historicalWallet, getUserQuota(t, 23103))
		assert.Equal(t, historicalToken, getTokenRemainQuota(t, 23103))
		// 本验收只删除一次性库，不会在共享 postgres 库上删除财务行。
		var current string
		require.NoError(t, model.DB.Raw("SELECT current_database()").Scan(&current).Error)
		assert.True(t, model.OfficialThrowawayPostgresName(current))
		assert.NotEqual(t, "postgres", current)
	})
}

func seedRecoveryUser(t *testing.T, id, quota int) {
	t.Helper()
	user := &model.User{
		Id:          id,
		Username:    fmt.Sprintf("r%d", id),
		Password:    "password-for-test",
		Quota:       quota,
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
		Group:       "default",
		AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedRecoveryToken(t *testing.T, id, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "recovery-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedRecoveryChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: fmt.Sprintf("recovery-channel-%d", id), Key: fmt.Sprintf("sk-ch-%d", id), Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func billingSessionKey(t *testing.T, info *relaycommon.RelayInfo) string {
	t.Helper()
	session, ok := info.Billing.(*BillingSession)
	require.True(t, ok)
	require.NotEmpty(t, session.operationKey)
	return session.operationKey
}

func installBillingInterruptTrigger(t *testing.T, name, table string, rowID int) func() {
	t.Helper()
	fn := name + "_fn"
	require.Contains(t, []string{"users", "tokens"}, table)
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
BEGIN
	RAISE EXCEPTION 'forced billing transaction interrupt';
END;
$$ LANGUAGE plpgsql`, fn)).Error)
	require.NoError(t, model.DB.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW WHEN (OLD.id = %d) EXECUTE FUNCTION %s()`, name, table, rowID, fn)).Error)
	dropped := false
	drop := func() {
		if dropped {
			return
		}
		dropped = true
		_ = model.DB.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, name, table)).Error
		_ = model.DB.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, fn)).Error
	}
	t.Cleanup(drop)
	return drop
}

func simulateBillingProcessRestart() {
	clearActiveBillingReservations()
	billingProcessLeaseOwner = common.NewRequestId()
}

func clearActiveBillingReservations() {
	activeBillingReservations.Range(func(key, _ any) bool {
		activeBillingReservations.Delete(key)
		return true
	})
}

func reservationIsInProcessMemory(key string) bool {
	_, ok := activeBillingReservations.Load(key)
	return ok
}

func countBillingOperations(t *testing.T, userID int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

func countOperationMovements(t *testing.T, key string) int {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.BillingOperationMovement{}).Where("operation_key = ?", key).Count(&n).Error)
	return int(n)
}

func sumBillingMovements(t *testing.T, userID int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.BillingOperationMovement{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

func sumOperationWalletDelta(t *testing.T, key string) int {
	t.Helper()
	var sum *int
	require.NoError(t, model.DB.Model(&model.BillingOperationMovement{}).
		Select("COALESCE(SUM(wallet_delta),0)").Where("operation_key = ?", key).Scan(&sum).Error)
	if sum == nil {
		return 0
	}
	return *sum
}

func countUserLogs(t *testing.T, userID int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", userID).Count(&n).Error)
	return n
}

func getUserQuotaPending(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota_pending").Where("id = ?", id).First(&user).Error)
	return user.QuotaPending
}

func assertOfficialPriceFieldsUnchanged(t *testing.T, frozenEncoded, laterEncoded string) {
	t.Helper()
	frozen := model.DecodeRequestPriceSnapshot(frozenEncoded)
	later := model.DecodeRequestPriceSnapshot(laterEncoded)
	require.NotNil(t, frozen)
	require.NotNil(t, later)
	assert.Equal(t, frozen.ModelRatio, later.ModelRatio)
	assert.Equal(t, frozen.ModelBasePrice, later.ModelBasePrice)
	assert.Equal(t, frozen.GroupRatio, later.GroupRatio)
	assert.Equal(t, frozen.BilledGroupRatio, later.BilledGroupRatio)
	assert.Equal(t, frozen.FinalPrice, later.FinalPrice)
	assert.Equal(t, frozen.SiteMarkupRatio, later.SiteMarkupRatio)
	assert.Equal(t, frozen.SiteCostFactor, later.SiteCostFactor)
	assert.Equal(t, frozen.SellPrice, later.SellPrice)
}
