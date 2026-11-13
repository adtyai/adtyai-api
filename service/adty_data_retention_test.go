package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDataRetentionTestDB(t *testing.T) {
	t.Helper()
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	require.NoError(t, opened.DB.AutoMigrate(
		&model.AdtyDataRetentionPolicy{},
		&model.AdtyDataRetentionRun{},
		&model.AdtyDataRetentionRunItem{},
		&model.Log{},
		&model.QuotaData{},
		&model.AuditLog{},
		&model.AdtyNotificationPublication{},
		&model.UserSession{},
		&model.UserAccessToken{},
		&model.User{},
	))
}

func daysAgo(days int) int64 {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

func TestDataRetentionListsEveryCategoryAndRejectsNotificationPeriod(t *testing.T) {
	setupDataRetentionTestDB(t)

	view, err := ListDataRetentionSettings()
	require.NoError(t, err)
	categories, ok := view["categories"].([]DataRetentionCategoryView)
	require.True(t, ok)
	require.Len(t, categories, 9)

	seen := map[string]DataRetentionCategoryView{}
	for _, category := range categories {
		seen[category.Category] = category
		assert.False(t, category.Configured)
		assert.Nil(t, category.RetentionDays)
		if category.Category == model.DataRetentionCategoryNotificationHistory {
			assert.False(t, category.AutoCleanable)
			assert.Equal(t, model.DataRetentionActionExclude, category.Action)
		} else {
			assert.Equal(t, "no_automatic_deletion", category.DefaultBehavior)
		}
	}
	for _, category := range model.DataRetentionCategoryOrder() {
		_, ok := seen[category]
		assert.True(t, ok, category)
	}

	err = SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryNotificationHistory,
		RetentionDays: intPtr(30),
		Action:        model.DataRetentionActionDelete,
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Notification history cannot be given an automatic cleanup period")

	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryAPIUsageLogs,
		RetentionDays: intPtr(180),
		Action:        model.DataRetentionActionDelete,
	}}))
	view, err = ListDataRetentionSettings()
	require.NoError(t, err)
	categories = view["categories"].([]DataRetentionCategoryView)
	for _, category := range categories {
		if category.Category == model.DataRetentionCategoryAPIUsageLogs {
			assert.True(t, category.Configured)
			require.NotNil(t, category.RetentionDays)
			assert.Equal(t, 180, *category.RetentionDays)
			require.NotNil(t, category.MinRetentionDays)
			assert.Equal(t, 90, *category.MinRetentionDays)
		}
	}
}

func TestDataRetentionUnconfiguredCategoriesDoNotDelete(t *testing.T) {
	setupDataRetentionTestDB(t)
	now := time.Now()
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 9, CreatedAt: daysAgo(200), Type: model.LogTypeConsume, Content: "old-usage"}).Error)
	require.NoError(t, model.DB.Create(&model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: 1, PublisherName: "root", Audience: "all", TargetSiteName: "main",
		Kind: "notice", Title: "keep me", Content: "history", Business: "none", Status: "published",
		Revision: 1, ReadKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PublishedAt: daysAgo(400), UpdatedAt: daysAgo(400), UpdatedBy: 1,
	}).Error)

	run, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", now)
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Len(t, items, 9)

	byCategory := map[string]model.AdtyDataRetentionRunItem{}
	for _, item := range items {
		byCategory[item.Category] = item
	}
	assert.Equal(t, "unconfigured", byCategory[model.DataRetentionCategoryAPIUsageLogs].Rule)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryAPIUsageLogs].DeleteCount)
	assert.Contains(t, byCategory[model.DataRetentionCategoryNotificationHistory].SkipReason, "must not delete notification history")
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryNotificationHistory].DeleteCount)

	var usage, notices int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "old-usage").Count(&usage).Error)
	require.NoError(t, model.DB.Model(&model.AdtyNotificationPublication{}).Where("title = ?", "keep me").Count(&notices).Error)
	assert.EqualValues(t, 1, usage)
	assert.EqualValues(t, 1, notices)
}

func TestDataRetentionCleansConfiguredUsageLogsAndKeepsFloorAndNotifications(t *testing.T) {
	setupDataRetentionTestDB(t)
	now := time.Now()
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 9, CreatedAt: daysAgo(200), Type: model.LogTypeConsume, Content: "very-old-usage"}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 9, CreatedAt: daysAgo(60), Type: model.LogTypeConsume, Content: "sixty-day-usage"}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 9, CreatedAt: daysAgo(200), Type: model.LogTypeTopup, Content: "old-topup"}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 9, CreatedAt: daysAgo(200), Type: model.LogTypeLogin, Content: "old-login"}).Error)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 9, Username: "u", ModelName: "m", CreatedAt: daysAgo(200), UseGroup: "default"}).Error)
	require.NoError(t, model.DB.Create(&model.AdtyNotificationPublication{
		IssuerSiteId: 0, PublisherId: 1, PublisherName: "root", Audience: "all", TargetSiteName: "main",
		Kind: "notice", Title: "still here", Content: "history", Business: "none", Status: "published",
		Revision: 1, ReadKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", PublishedAt: daysAgo(400), UpdatedAt: daysAgo(400), UpdatedBy: 1,
	}).Error)

	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryAPIUsageLogs,
		RetentionDays: intPtr(30),
		Action:        model.DataRetentionActionDelete,
	}}))

	var stillThere int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "very-old-usage").Count(&stillThere).Error)
	assert.EqualValues(t, 1, stillThere, "changing a period must not immediately wipe data")

	run, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "task-1", now)
	require.NoError(t, err)
	require.NotNil(t, run)
	byCategory := map[string]model.AdtyDataRetentionRunItem{}
	for _, item := range items {
		byCategory[item.Category] = item
	}
	assert.Contains(t, byCategory[model.DataRetentionCategoryAPIUsageLogs].Rule, "safety floor 90 days")
	assert.GreaterOrEqual(t, byCategory[model.DataRetentionCategoryAPIUsageLogs].DeleteCount, int64(2))
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryNotificationHistory].DeleteCount)
	assert.Contains(t, byCategory[model.DataRetentionCategoryNotificationHistory].SkipReason, "must not delete notification history")

	var veryOld, sixty, topup, login, quota, notices int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "very-old-usage").Count(&veryOld).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "sixty-day-usage").Count(&sixty).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "old-topup").Count(&topup).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "old-login").Count(&login).Error)
	require.NoError(t, model.DB.Model(&model.QuotaData{}).Where("username = ?", "u").Count(&quota).Error)
	require.NoError(t, model.DB.Model(&model.AdtyNotificationPublication{}).Where("title = ?", "still here").Count(&notices).Error)
	assert.EqualValues(t, 0, veryOld)
	assert.EqualValues(t, 1, sixty, "90-day floor must keep logs newer than 90 days")
	assert.EqualValues(t, 1, topup)
	assert.EqualValues(t, 1, login, "login logs belong to login/security, not usage cleanup")
	assert.EqualValues(t, 0, quota)
	assert.EqualValues(t, 1, notices)
}

func TestDataRetentionConfigChangeIsAuditedAndDoesNotWipeUntilLaterRun(t *testing.T) {
	setupDataRetentionTestDB(t)
	now := time.Now()
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.AuditLog{}))
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 3, CreatedAt: daysAgo(200), Type: model.LogTypeConsume, Content: "held-until-run"}).Error)
	require.NoError(t, SaveDataRetentionPolicies(nil, 7, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryAPIUsageLogs,
		RetentionDays: intPtr(180),
		Action:        model.DataRetentionActionDelete,
	}}))

	var usage, audits int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "held-until-run").Count(&usage).Error)
	require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("action = ?", "data_retention.update").Count(&audits).Error)
	assert.EqualValues(t, 1, usage)
	assert.EqualValues(t, 1, audits)

	_, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", now)
	require.NoError(t, err)
	var cleaned int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("content = ?", "held-until-run").Count(&cleaned).Error)
	assert.EqualValues(t, 0, cleaned)
	for _, item := range items {
		if item.Category == model.DataRetentionCategoryAPIUsageLogs {
			assert.GreaterOrEqual(t, item.DeleteCount, int64(1))
		}
	}
}

func TestStartLogCleanupHonorsConfiguredPeriodAndFloor(t *testing.T) {
	setupDataRetentionTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	_, err := StartLogCleanupTask(time.Now().Add(-30 * 24 * time.Hour).Unix())
	require.ErrorIs(t, err, ErrLogCleanupRetention)

	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryAPIUsageLogs,
		RetentionDays: intPtr(180),
		Action:        model.DataRetentionActionDelete,
	}}))
	_, err = StartLogCleanupTask(time.Now().Add(-100 * 24 * time.Hour).Unix())
	require.ErrorIs(t, err, ErrLogCleanupRetention)
	task, err := StartLogCleanupTask(time.Now().Add(-200 * 24 * time.Hour).Unix())
	require.NoError(t, err)
	require.NotNil(t, task)
}

func TestDataRetentionSkipsUnsafeConfiguredCategories(t *testing.T) {
	setupDataRetentionTestDB(t)
	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{
		{Category: model.DataRetentionCategoryOrdersPayments, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryFinancialLedgers, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategorySupportTickets, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryTaskArtifacts, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryUserAccountProfile, RetentionDays: intPtr(30), Action: model.DataRetentionActionDeidentify},
	}))
	_, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", time.Now())
	require.NoError(t, err)
	for _, item := range items {
		switch item.Category {
		case model.DataRetentionCategoryOrdersPayments, model.DataRetentionCategoryFinancialLedgers, model.DataRetentionCategorySupportTickets, model.DataRetentionCategoryTaskArtifacts, model.DataRetentionCategoryUserAccountProfile:
			assert.Equal(t, int64(0), item.DeleteCount, item.Category)
			assert.NotEmpty(t, item.SkipReason, item.Category)
		}
	}
}

func intPtr(v int) *int { return &v }
