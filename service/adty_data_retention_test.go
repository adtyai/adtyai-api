package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	require.NoError(t, model.DB.AutoMigrate(
		&model.AdtyTicket{}, &model.AdtyTicketMessage{}, &model.AdtyTicketImage{}, &model.AdtyTicketEvent{},
		&model.ExternalIdentityClaim{},
	))
	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{
		{Category: model.DataRetentionCategoryOrdersPayments, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryFinancialLedgers, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategorySupportTickets, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryTaskArtifacts, RetentionDays: intPtr(30), Action: model.DataRetentionActionDelete},
		{Category: model.DataRetentionCategoryUserAccountProfile, RetentionDays: intPtr(30), Action: model.DataRetentionActionDeidentify},
	}))
	_, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", time.Now())
	require.NoError(t, err)
	byCategory := map[string]model.AdtyDataRetentionRunItem{}
	for _, item := range items {
		byCategory[item.Category] = item
	}
	assert.Equal(t, dataRetentionSkipOrders, byCategory[model.DataRetentionCategoryOrdersPayments].SkipReason)
	assert.Equal(t, dataRetentionSkipFinancial, byCategory[model.DataRetentionCategoryFinancialLedgers].SkipReason)
	assert.Equal(t, dataRetentionSkipArtifacts, byCategory[model.DataRetentionCategoryTaskArtifacts].SkipReason)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryOrdersPayments].DeleteCount)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryFinancialLedgers].DeleteCount)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryTaskArtifacts].DeleteCount)
	assert.Empty(t, byCategory[model.DataRetentionCategorySupportTickets].SkipReason)
	assert.Contains(t, byCategory[model.DataRetentionCategorySupportTickets].Rule, "safety floor 90 days")
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategorySupportTickets].DeleteCount)
	assert.Empty(t, byCategory[model.DataRetentionCategoryUserAccountProfile].SkipReason)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryUserAccountProfile].DeleteCount)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryUserAccountProfile].DeidentifyCount)
}

func TestDataRetentionClearsLeftoverCancelledProfileWithoutDeletingFacts(t *testing.T) {
	setupDataRetentionTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.ExternalIdentityClaim{}, &model.TopUp{}, &model.AdtyTopupOrder{},
		&model.AdtyBalanceLedger{}, &model.Task{},
	))
	now := time.Now()
	legacy := &model.User{
		Username: "legacy-cancelled", Password: "hash", Email: "legacy-cancelled@example.com",
		DisplayName: "Legacy Person", GitHubId: "legacy-gh", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(legacy).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: legacy.Id, TradeNo: "legacy-order", Status: common.TopUpStatusSuccess, Amount: 10}).Error)
	require.NoError(t, model.DB.Create(&model.AdtyBalanceLedger{UserId: legacy.Id, DeltaQuota: 100, Type: model.AdtyLedgerTypeTopUp, RefId: "legacy-order"}).Error)
	require.NoError(t, model.DB.Create(&model.Task{UserId: legacy.Id, TaskID: "legacy-task", Status: model.TaskStatusSuccess, Action: "image"}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.AuditLog{EventId: "legacy-audit", UserId: legacy.Id, Username: "legacy-cancelled", Category: model.AuditCategoryOperation, Action: "user.note", Content: "keep", CreatedAt: now.Unix()}).Error)
	require.NoError(t, model.DB.Delete(legacy).Error)
	require.NoError(t, model.DB.Unscoped().Model(legacy).Update("deleted_at", now.Add(-200*24*time.Hour)).Error)

	recent := &model.User{
		Username: "recent-cancelled", Password: "hash", Email: "recent-cancelled@example.com",
		DisplayName: "Recent Person", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(recent).Error)
	require.NoError(t, model.DB.Delete(recent).Error)

	active := &model.User{
		Username: "still-active", Password: "hash", Email: "still-active@example.com",
		DisplayName: "Active Person", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(active).Error)

	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategoryUserAccountProfile,
		RetentionDays: intPtr(30),
		Action:        model.DataRetentionActionDeidentify,
	}}))

	_, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", now)
	require.NoError(t, err)
	byCategory := map[string]model.AdtyDataRetentionRunItem{}
	for _, item := range items {
		byCategory[item.Category] = item
	}
	assert.Empty(t, byCategory[model.DataRetentionCategoryUserAccountProfile].SkipReason)
	assert.Equal(t, int64(0), byCategory[model.DataRetentionCategoryUserAccountProfile].DeleteCount)
	assert.EqualValues(t, 1, byCategory[model.DataRetentionCategoryUserAccountProfile].DeidentifyCount)

	var storedLegacy, storedRecent, storedActive model.User
	require.NoError(t, model.DB.Unscoped().First(&storedLegacy, legacy.Id).Error)
	require.NoError(t, model.DB.Unscoped().First(&storedRecent, recent.Id).Error)
	require.NoError(t, model.DB.First(&storedActive, active.Id).Error)
	assert.True(t, storedLegacy.DeletedAt.Valid)
	assert.Empty(t, storedLegacy.Email)
	assert.Empty(t, storedLegacy.DisplayName)
	assert.Empty(t, storedLegacy.GitHubId)
	assert.NotEqual(t, "legacy-cancelled", storedLegacy.Username)
	assert.Equal(t, "recent-cancelled@example.com", storedRecent.Email)
	assert.Equal(t, "Recent Person", storedRecent.DisplayName)
	assert.Equal(t, "still-active@example.com", storedActive.Email)
	assert.Equal(t, "Active Person", storedActive.DisplayName)
	assert.False(t, storedActive.DeletedAt.Valid)

	var orders, ledgers, tasks, audits int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "legacy-order").Count(&orders).Error)
	require.NoError(t, model.DB.Model(&model.AdtyBalanceLedger{}).Where("ref_id = ?", "legacy-order").Count(&ledgers).Error)
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", "legacy-task").Count(&tasks).Error)
	require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("event_id = ?", "legacy-audit").Count(&audits).Error)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, ledgers)
	assert.EqualValues(t, 1, tasks)
	assert.EqualValues(t, 1, audits)
}

func TestDataRetentionDeletesClosedTicketsAfterFloorAndKeepsOpenOrFailedFiles(t *testing.T) {
	setupDataRetentionTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AdtyTicket{}, &model.AdtyTicketMessage{}, &model.AdtyTicketImage{}, &model.AdtyTicketEvent{}))
	now := time.Now()
	imageDir := t.TempDir()
	restore := SetAdtyTicketImageDirForTest(imageDir)
	t.Cleanup(restore)

	oldClosed := createRetentionTicket(t, model.AdtyTicketClosed, now.Add(-200*24*time.Hour))
	oldResolved := createRetentionTicket(t, model.AdtyTicketResolved, now.Add(-200*24*time.Hour))
	recentClosed := createRetentionTicket(t, model.AdtyTicketClosed, now.Add(-60*24*time.Hour))
	openTicket := createRetentionTicket(t, model.AdtyTicketInProgress, now.Add(-200*24*time.Hour))
	failedTicket := createRetentionTicket(t, model.AdtyTicketClosed, now.Add(-200*24*time.Hour))

	oldImage := writeRetentionTicketImage(t, imageDir, oldClosed.Id, "old-closed.png", []byte("old-closed"))
	resolvedImage := writeRetentionTicketImage(t, imageDir, oldResolved.Id, "old-resolved.png", []byte("old-resolved"))
	recentImage := writeRetentionTicketImage(t, imageDir, recentClosed.Id, "recent-closed.png", []byte("recent-closed"))
	openImage := writeRetentionTicketImage(t, imageDir, openTicket.Id, "open-ticket.png", []byte("open-ticket"))
	failedImage := writeRetentionTicketBlockedImage(t, imageDir, failedTicket.Id, "blocked-image")

	require.NoError(t, SaveDataRetentionPolicies(nil, 1, "root", []DataRetentionPolicyInput{{
		Category:      model.DataRetentionCategorySupportTickets,
		RetentionDays: intPtr(30),
		Action:        model.DataRetentionActionDelete,
	}}))

	_, items, err := RunDataRetentionCleanup(context.Background(), model.DataRetentionTriggerManual, "", now)
	require.NoError(t, err)
	byCategory := map[string]model.AdtyDataRetentionRunItem{}
	for _, item := range items {
		byCategory[item.Category] = item
	}
	assert.Contains(t, byCategory[model.DataRetentionCategorySupportTickets].Rule, "safety floor 90 days")
	assert.Empty(t, byCategory[model.DataRetentionCategorySupportTickets].SkipReason)
	assert.EqualValues(t, 2, byCategory[model.DataRetentionCategorySupportTickets].DeleteCount)
	assert.EqualValues(t, 1, byCategory[model.DataRetentionCategorySupportTickets].FailureCount)

	assert.NoFileExists(t, filepath.Join(imageDir, oldImage.Storage))
	assert.NoFileExists(t, filepath.Join(imageDir, resolvedImage.Storage))
	assert.FileExists(t, filepath.Join(imageDir, recentImage.Storage))
	assert.FileExists(t, filepath.Join(imageDir, openImage.Storage))

	var closedLeft, resolvedLeft, recentLeft, openLeft, failedLeft, failedImages int64
	require.NoError(t, model.DB.Model(&model.AdtyTicket{}).Where("id = ?", oldClosed.Id).Count(&closedLeft).Error)
	require.NoError(t, model.DB.Model(&model.AdtyTicket{}).Where("id = ?", oldResolved.Id).Count(&resolvedLeft).Error)
	require.NoError(t, model.DB.Model(&model.AdtyTicket{}).Where("id = ?", recentClosed.Id).Count(&recentLeft).Error)
	require.NoError(t, model.DB.Model(&model.AdtyTicket{}).Where("id = ?", openTicket.Id).Count(&openLeft).Error)
	require.NoError(t, model.DB.Model(&model.AdtyTicket{}).Where("id = ?", failedTicket.Id).Count(&failedLeft).Error)
	require.NoError(t, model.DB.Model(&model.AdtyTicketImage{}).Where("id = ?", failedImage.Id).Count(&failedImages).Error)
	assert.Zero(t, closedLeft)
	assert.Zero(t, resolvedLeft)
	assert.EqualValues(t, 1, recentLeft)
	assert.EqualValues(t, 1, openLeft)
	assert.EqualValues(t, 1, failedLeft)
	assert.EqualValues(t, 1, failedImages)
}

func createRetentionTicket(t *testing.T, status string, closedAt time.Time) model.AdtyTicket {
	t.Helper()
	ticket := model.AdtyTicket{UserId: 9, Title: "retention-" + status, Status: status, Priority: model.AdtyTicketPriorityNormal}
	require.NoError(t, model.DB.Create(&ticket).Error)
	stamp := closedAt.Unix()
	require.NoError(t, model.DB.Model(&ticket).Updates(map[string]any{"created_at": stamp, "updated_at": stamp}).Error)
	event := model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: 1, Action: "status", Detail: status}
	require.NoError(t, model.DB.Create(&event).Error)
	require.NoError(t, model.DB.Model(&event).Update("created_at", stamp).Error)
	message := model.AdtyTicketMessage{TicketId: ticket.Id, AuthorId: 9, Visibility: model.AdtyTicketVisibilityPublic, Body: "body-" + status, Dedupe: fmt.Sprintf("retention-msg-%d", ticket.Id)}
	require.NoError(t, model.DB.Create(&message).Error)
	return ticket
}

func writeRetentionTicketImage(t *testing.T, dir string, ticketID int64, name string, payload []byte) model.AdtyTicketImage {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), payload, 0o640))
	image := model.AdtyTicketImage{TicketId: ticketID, Mime: "image/png", Size: len(payload), Storage: name}
	require.NoError(t, model.DB.Create(&image).Error)
	return image
}

func writeRetentionTicketBlockedImage(t *testing.T, dir string, ticketID int64, name string) model.AdtyTicketImage {
	t.Helper()
	blocked := filepath.Join(dir, name)
	require.NoError(t, os.Mkdir(blocked, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "keep"), []byte("cannot-remove-dir"), 0o640))
	image := model.AdtyTicketImage{TicketId: ticketID, Mime: "image/png", Size: 4, Storage: name}
	require.NoError(t, model.DB.Create(&image).Error)
	return image
}

func intPtr(v int) *int { return &v }
