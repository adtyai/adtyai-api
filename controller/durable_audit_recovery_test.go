package controller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openDurableAuditDatabase(t *testing.T, dialect, dsn string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector = sqlite.Open(dsn)
	if dialect == "postgres" {
		driver = postgres.Open(dsn)
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		pool, err := db.DB()
		if err == nil {
			_ = pool.Close()
		}
	})
	return db
}

func TestDurableAuditRecoveryDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_POSTGRES_DSN")
			if dialect == "postgres" && dsn == "" {
				t.Skip("TEST_POSTGRES_DSN is not configured")
			}
			for _, failure := range []string{"log_outage_restart", "log_insert_acknowledgement", "receipt_write", "business_commit_acknowledgement", "event_identity_conflict", "runner_process_restart", "concurrent_replay", "nested_metadata_normalization"} {
				t.Run(failure, func(t *testing.T) {
					prepareDurableOption(t)
					main, mainDSN := newAuditTestDatabase(t, dialect, dsn)
					logs, logDSN := newAuditTestDatabase(t, dialect, dsn)
					oldMain, oldLogs := model.DB, model.LOG_DB
					oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
					model.DB, model.LOG_DB = main, logs
					dbType := common.DatabaseTypeSQLite
					if dialect == "postgres" {
						dbType = common.DatabaseTypePostgreSQL
						var version int
						require.NoError(t, main.Raw("SHOW server_version_num").Scan(&version).Error)
						require.Positive(t, version)
						t.Logf("server_version_num=%d", version)
					}
					common.SetDatabaseTypes(dbType, dbType)
					t.Cleanup(func() {
						model.DB, model.LOG_DB = oldMain, oldLogs
						common.SetDatabaseTypes(oldMainType, oldLogType)
					})
					// Upgrade an existing option store and repeat the forward migration.
					require.NoError(t, main.AutoMigrate(&model.Option{}))
					require.NoError(t, main.Create(&model.Option{Key: "SystemName", Value: "before-audit-migration"}).Error)
					for range 2 {
						require.NoError(t, main.AutoMigrate(&model.AuditIntent{}))
						require.NoError(t, logs.AutoMigrate(&model.AuditLog{}))
					}
					var original model.Option
					require.NoError(t, main.First(&original).Error)
					require.Equal(t, "before-audit-migration", original.Value)

					event := model.PrepareAuditLog(nil, model.AuditLog{
						UserId: 77, Username: "historical-operator", ActorRole: common.RoleRootUser,
						Category: model.AuditCategoryOperation, Action: "option.update", Success: true, Status: 200,
						RequestId: "durable-request", UserAgent: strings.Repeat("界", 700), Method: "PUT", Route: "/api/option/",
						Other: model.AuditOther{Op: &model.AuditOperation{Action: "option.update", Params: model.AuditFields{"key": "SystemName"}}},
					})
					if failure == "nested_metadata_normalization" {
						event.Other.Op.Params["patch"] = map[string]any{"adty_log": map[string]bool{"read": false}, "adty_user": map[string]bool{"read": true, "write": false}}
					}
					commitUnknown := false
					if failure == "business_commit_acknowledgement" {
						connection, err := main.DB()
						require.NoError(t, err)
						wrapped := main.Session(&gorm.Session{NewDB: true, Context: context.Background()})
						pool := &resetCommitErrorPool{DB: connection, committed: &commitUnknown}
						wrapped.ConnPool, wrapped.Statement.ConnPool = pool, pool
						model.DB = wrapped
					}
					err := model.UpdateOptionWithAudit("SystemName", "configuration-secret-must-not-be-audited", event)
					if failure == "business_commit_acknowledgement" {
						require.Error(t, err)
						require.True(t, commitUnknown, "the real SQL commit must finish before losing its acknowledgement")
						model.DB = main
					} else {
						require.NoError(t, err)
					}
					var intent model.AuditIntent
					require.NoError(t, main.Where("event_id = ?", event.EventId).Take(&intent).Error)
					assert.NotContains(t, intent.Payload, "configuration-secret-must-not-be-audited")
					assert.Equal(t, event.CreatedAt, intent.CreatedAt)
					assert.Zero(t, intent.DeliveredAt)
					frozen := intent.Payload
					require.Error(t, main.Transaction(func(tx *gorm.DB) error { return model.AppendAuditIntentTx(tx, event) }), "an existing intent identity cannot be overwritten")

					const callback = "test:durable-audit-fault"
					injected := false
					switch failure {
					case "log_outage_restart", "runner_process_restart":
						connection, err := logs.DB()
						require.NoError(t, err)
						require.NoError(t, connection.Close())
						require.Error(t, model.DeliverAuditIntent(context.Background(), event.EventId))
						model.LOG_DB = openDurableAuditDatabase(t, dialect, logDSN)
					case "log_insert_acknowledgement":
						require.NoError(t, logs.Callback().Create().After("gorm:commit_or_rollback_transaction").Register(callback, func(tx *gorm.DB) {
							if !injected && tx.Statement.Table == "audit_logs" && tx.Error == nil {
								injected = true
								tx.AddError(errors.New("sensitive-driver-error-must-not-be-retained"))
							}
						}))
						require.Error(t, model.DeliverAuditIntent(context.Background(), event.EventId))
						require.True(t, injected)
						require.NoError(t, logs.Callback().Create().Remove(callback))
						var count int64
						require.NoError(t, logs.Model(&model.AuditLog{}).Count(&count).Error)
						require.EqualValues(t, 1, count, "the log insert must really commit before its reply is lost")
					case "receipt_write":
						require.NoError(t, main.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
							if tx.Statement.Table == "audit_intents" {
								injected = true
								tx.AddError(assert.AnError)
							}
						}))
						require.Error(t, model.DeliverAuditIntent(context.Background(), event.EventId))
						require.True(t, injected)
						require.NoError(t, main.Callback().Update().Remove(callback))
					case "event_identity_conflict":
						conflicting := event
						conflicting.Username = "different-event"
						require.NoError(t, logs.Create(&conflicting).Error)
						require.Error(t, model.DeliverAuditIntent(context.Background(), event.EventId))
						require.NoError(t, main.Where("event_id = ?", event.EventId).Take(&intent).Error)
						assert.Zero(t, intent.DeliveredAt, "a collision must never acknowledge the wrong event")
						require.NoError(t, logs.Delete(&conflicting).Error)
					case "concurrent_replay":
						start, results := make(chan struct{}), make(chan error, 6)
						for range 6 {
							go func() {
								<-start
								results <- model.DeliverAuditIntent(context.Background(), event.EventId)
							}()
						}
						close(start)
						for range 6 {
							require.NoError(t, <-results)
						}
					case "nested_metadata_normalization":
						stored := event
						require.NoError(t, logs.Create(&stored).Error)
						// SQL JSON stores may reorder nested object keys and whitespace.
						normalized := `{"op":{"params":{"patch":{"adty_user":{"write":false,"read":true},"adty_log":{"read":false}},"key":"SystemName"},"action":"option.update"}}`
						require.NoError(t, logs.Model(&model.AuditLog{}).Where("event_id = ?", event.EventId).Update("other", normalized).Error)
					}

					// Reopen the real primary connection; no in-memory wakeup/queue is retained.
					connection, err := main.DB()
					require.NoError(t, err)
					require.NoError(t, connection.Close())
					main = openDurableAuditDatabase(t, dialect, mainDSN)
					model.DB = main
					require.NoError(t, main.Where("event_id = ?", event.EventId).Take(&intent).Error)
					assert.Equal(t, frozen, intent.Payload)
					assert.NotContains(t, intent.LastErrorCategory, "sensitive-driver-error")
					require.NoError(t, main.Model(&model.AuditIntent{}).Where("event_id = ?", event.EventId).Update("next_attempt_at", 0).Error)
					if failure == "runner_process_restart" {
						require.NoError(t, main.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
						for range 2 {
							task, err := model.CreateSystemTask("audit_delivery", nil, nil)
							require.NoError(t, err)
							cancelled, err := model.RequestSystemTaskCancel(task.TaskID)
							require.NoError(t, err)
							assert.Equal(t, model.SystemTaskStatusCancelled, cancelled.Status)
						}
						deleted, err := model.DeleteSystemTaskHistory(model.SystemTaskFilter{Type: "audit_delivery"})
						require.NoError(t, err)
						assert.EqualValues(t, 1, deleted)
						require.NoError(t, main.Where("event_id = ?", event.EventId).Take(&intent).Error)
						assert.Zero(t, intent.DeliveredAt, "cancelling/cleaning scheduler history must preserve pending evidence")
						// Make the preserved scheduler receipt due without sleeping through its interval.
						require.NoError(t, main.Model(&model.SystemTask{}).Where("type = ?", "audit_delivery").Update("updated_at", common.GetTimestamp()-20).Error)
						executable, err := os.Executable()
						require.NoError(t, err)
						ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
						defer cancel()
						command := exec.CommandContext(ctx, executable, "-test.run=^TestDurableAuditRunnerProcess$", "-test.timeout=15s")
						command.Env = append(os.Environ(), "ADTY_AUDIT_RUNNER_HELPER=1", "ADTY_AUDIT_RUNNER_DIALECT="+dialect,
							"ADTY_AUDIT_RUNNER_MAIN_DSN="+mainDSN, "ADTY_AUDIT_RUNNER_LOG_DSN="+logDSN)
						output, err := command.CombinedOutput()
						require.NoError(t, err, "%s", output)
						t.Log("fresh process recovered the intent through StartSystemTaskRunner")
					}
					count, err := model.DeliverPendingAuditIntents(context.Background(), 100)
					require.NoError(t, err)
					if failure == "runner_process_restart" || failure == "concurrent_replay" {
						assert.Zero(t, count)
					} else {
						assert.Equal(t, 1, count)
					}
					count, err = model.DeliverPendingAuditIntents(context.Background(), 100)
					require.NoError(t, err)
					assert.Zero(t, count)
					require.NoError(t, model.DeliverAuditIntent(context.Background(), event.EventId))
					var rows []model.AuditLog
					require.NoError(t, model.LOG_DB.Where("event_id = ?", event.EventId).Find(&rows).Error)
					require.Len(t, rows, 1)
					assert.Equal(t, "historical-operator", rows[0].Username)
					assert.Equal(t, common.RoleRootUser, rows[0].ActorRole)
					assert.Len(t, []rune(rows[0].UserAgent), 512)
					require.NoError(t, main.Where("event_id = ?", event.EventId).Take(&intent).Error)
					assert.Positive(t, intent.DeliveredAt)
					assert.Empty(t, intent.LastErrorCategory)
					assert.Equal(t, frozen, intent.Payload)
					var option model.Option
					require.NoError(t, main.Where("key = ?", "SystemName").Take(&option).Error)
					assert.Equal(t, "configuration-secret-must-not-be-audited", option.Value)
				})
			}
		})
	}
}

// This helper runs only in a fresh test-binary process. It uses the real
// scheduler, registry, lease and log database, with no retained caller memory.
func TestDurableAuditRunnerProcess(t *testing.T) {
	if os.Getenv("ADTY_AUDIT_RUNNER_HELPER") != "1" {
		return
	}
	dialect := os.Getenv("ADTY_AUDIT_RUNNER_DIALECT")
	model.DB = openDurableAuditDatabase(t, dialect, os.Getenv("ADTY_AUDIT_RUNNER_MAIN_DSN"))
	model.LOG_DB = openDurableAuditDatabase(t, dialect, os.Getenv("ADTY_AUDIT_RUNNER_LOG_DSN"))
	dbType := common.DatabaseTypeSQLite
	if dialect == "postgres" {
		dbType = common.DatabaseTypePostgreSQL
	}
	common.SetDatabaseTypes(dbType, dbType)
	common.IsMasterNode, common.RedisEnabled = true, false
	service.StartSystemTaskRunner()
	require.Eventually(t, func() bool {
		var pending, finished int64
		if model.DB.Model(&model.AuditIntent{}).Where("delivered_at = ?", 0).Count(&pending).Error != nil {
			return false
		}
		if model.DB.Model(&model.SystemTask{}).Where("type = ? AND status = ?", "audit_delivery", model.SystemTaskStatusSucceeded).Count(&finished).Error != nil {
			return false
		}
		return pending == 0 && finished == 1
	}, 10*time.Second, 10*time.Millisecond)
}
