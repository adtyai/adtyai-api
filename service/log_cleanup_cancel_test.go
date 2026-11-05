/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCancelledLogCleanupDoesNotDeleteRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}, &model.Log{}))
	old := time.Now().Add(-200 * 24 * time.Hour).Unix()
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: old, Type: model.LogTypeConsume, Content: "old-usage"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: old, Type: model.LogTypeTopup, Content: "old-payment"}).Error)

	task, err := StartLogCleanupTask(old + 1)
	require.NoError(t, err)
	cancelled, err := model.RequestSystemTaskCancel(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusCancelled, cancelled.Status)

	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogCleanup, "runner", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, claimed)

	var usage, payment int64
	require.NoError(t, db.Model(&model.Log{}).Where("content = ?", "old-usage").Count(&usage).Error)
	require.NoError(t, db.Model(&model.Log{}).Where("content = ?", "old-payment").Count(&payment).Error)
	require.EqualValues(t, 1, usage)
	require.EqualValues(t, 1, payment)
	_ = context.Background()
}

func TestRunningLogCleanupStopsAfterCancel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}, &model.Log{}))
	old := time.Now().Add(-200 * 24 * time.Hour).Unix()
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: old, Type: model.LogTypeConsume, Content: "still-there"}).Error)

	task, err := StartLogCleanupTask(old + 1)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogCleanup, "runner", time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.True(t, ok)
	_, err = model.RequestSystemTaskCancel(task.TaskID)
	require.NoError(t, err)
	runLogCleanupTask(context.Background(), claimed, "runner")

	var usage int64
	require.NoError(t, db.Model(&model.Log{}).Where("content = ?", "still-there").Count(&usage).Error)
	require.EqualValues(t, 1, usage, "cancel must stop the running cleanup before it deletes usage logs")
	stored, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusCancelled, stored.Status)
}
