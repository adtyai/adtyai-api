package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
)

func pendingBatchDelta(kind int, id int) int {
	if kind < 0 || kind >= len(batchUpdateStores) || id == 0 {
		return 0
	}
	batchUpdateLocks[kind].Lock()
	defer batchUpdateLocks[kind].Unlock()
	return batchUpdateStores[kind][id]
}

func saturatingAddInt(base int, delta int) int {
	sum := int64(base) + int64(delta)
	if sum > math.MaxInt {
		return math.MaxInt
	}
	if sum < math.MinInt {
		return math.MinInt
	}
	return int(sum)
}

// TokenQuotaForBilling 返回令牌当前剩余和已用。批量更新还没刷进库的差额也算进去。
func TokenQuotaForBilling(token *Token) (remain int, used int) {
	if token == nil {
		return 0, 0
	}
	delta := pendingBatchDelta(BatchUpdateTypeTokenQuota, token.Id)
	return saturatingAddInt(token.RemainQuota, delta), saturatingAddInt(token.UsedQuota, -delta)
}

// UserQuotaForBilling 返回用户当前剩余和已用。批量更新打开时以数据库加队列差额为准，避免和 Redis 增量加两次。
func UserQuotaForBilling(id int) (remain int, used int, err error) {
	if common.BatchUpdateEnabled {
		remain, err = GetUserQuota(id, true)
	} else {
		remain, err = GetUserQuota(id, false)
	}
	if err != nil {
		return 0, 0, err
	}
	used, err = GetUserUsedQuota(id)
	if err != nil {
		return 0, 0, err
	}
	if common.BatchUpdateEnabled {
		remain = saturatingAddInt(remain, pendingBatchDelta(BatchUpdateTypeUserQuota, id))
		used = saturatingAddInt(used, pendingBatchDelta(BatchUpdateTypeUsedQuota, id))
	}
	return remain, used, nil
}
