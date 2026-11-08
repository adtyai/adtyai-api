package model

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex

func init() {
	for range BatchUpdateTypeCount {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		batchUpdate()
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
}

// FlushBatchUpdates 在进程退出前把队列里的额度差额写进数据库。
func FlushBatchUpdates() error {
	return batchUpdate()
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	old, ok := batchUpdateStores[type_][id]
	if !ok {
		batchUpdateStores[type_][id] = value
		return
	}

	sum := old + value
	if (value > 0 && sum < old) || (value < 0 && sum > old) {
		common.SysError(common.LogText("batch update overflow: type=%d id=%d old=%d value=%d", type_, id, old, value))
		if value > 0 {
			sum = math.MaxInt
		} else {
			sum = math.MinInt
		}
	}
	batchUpdateStores[type_][id] = sum
}

func batchUpdate() error {
	var failures []error
	// check if there's any data to update
	hasData := false
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}
	// Durable deltas survive an empty local queue and process restarts. Page by
	// primary key: OFFSET would skip rows as concurrent flushes clear pending.
	var pendingUsers []int
	lastID := 0
	for {
		var page []User
		if err := DB.Unscoped().Select("id").Where("id > ? AND quota_pending <> 0", lastID).Order("id").Limit(256).Find(&page).Error; err != nil {
			common.SysError(common.LogText("failed to scan durable wallet deltas: %s", err.Error()))
			failures = append(failures, err)
			break
		}
		for _, user := range page {
			pendingUsers = append(pendingUsers, user.Id)
		}
		if len(page) < 256 {
			break
		}
		lastID = page[len(page)-1].Id
	}
	if len(pendingUsers) > 0 {
		hasData = true
	}
	var pendingTokens []int
	lastID = 0
	for {
		var page []Token
		if err := DB.Unscoped().Select("id").Where("id > ? AND quota_pending <> 0", lastID).Order("id").Limit(256).Find(&page).Error; err != nil {
			common.SysError(common.LogText("failed to scan durable token deltas: %s", err.Error()))
			failures = append(failures, err)
			break
		}
		for _, token := range page {
			pendingTokens = append(pendingTokens, token.Id)
		}
		if len(page) < 256 {
			break
		}
		lastID = page[len(page)-1].Id
	}
	if len(pendingTokens) > 0 {
		hasData = true
	}

	if !hasData {
		return errors.Join(failures...)
	}

	common.SysLog(common.LogText("batch update started"))
	stores := make([]map[int]int, BatchUpdateTypeCount)
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		stores[i] = batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
	}
	for _, id := range pendingTokens {
		stores[BatchUpdateTypeTokenQuota][id] = 0
	}

	for i, store := range stores {
		if i == BatchUpdateTypeUserQuota || i == BatchUpdateTypeUsedQuota || i == BatchUpdateTypeRequestCount {
			continue
		}
		for key, value := range store {
			switch i {
			case BatchUpdateTypeTokenQuota:
				err := flushTokenQuota(key)
				if err != nil {
					common.SysLog(common.LogText("failed to batch update token quota: %s", err.Error()))
					failures = append(failures, err)
					addNewRecord(BatchUpdateTypeTokenQuota, key, 0)
				}
			case BatchUpdateTypeChannelUsedQuota:
				if err := updateChannelUsedQuota(key, value); err != nil {
					failures = append(failures, err)
					addNewRecord(BatchUpdateTypeChannelUsedQuota, key, value)
				}
			}
		}
	}

	userQuotaStore := stores[BatchUpdateTypeUserQuota]
	usedQuotaStore := stores[BatchUpdateTypeUsedQuota]
	requestCountStore := stores[BatchUpdateTypeRequestCount]

	userIDs := make(map[int]struct{}, len(userQuotaStore)+len(usedQuotaStore)+len(requestCountStore))
	for key := range userQuotaStore {
		userIDs[key] = struct{}{}
	}
	for _, id := range pendingUsers {
		userIDs[id] = struct{}{}
	}
	for key := range usedQuotaStore {
		userIDs[key] = struct{}{}
	}
	for key := range requestCountStore {
		userIDs[key] = struct{}{}
	}
	for key := range userIDs {
		if err := updateUserQuotaUsedQuotaAndRequestCount(key, usedQuotaStore[key], requestCountStore[key]); err != nil {
			common.SysError(common.LogText("failed to flush durable wallet for user %d: %s", key, err.Error()))
			failures = append(failures, err)
			addNewRecord(BatchUpdateTypeUserQuota, key, 0)
			if value := usedQuotaStore[key]; value != 0 {
				addNewRecord(BatchUpdateTypeUsedQuota, key, value)
			}
			if value := requestCountStore[key]; value != 0 {
				addNewRecord(BatchUpdateTypeRequestCount, key, value)
			}
		}
	}
	common.SysLog(common.LogText("batch update finished"))
	return errors.Join(failures...)
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
