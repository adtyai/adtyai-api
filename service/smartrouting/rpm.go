package smartrouting

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func rpmKey(channelID int, minute time.Time) string {
	return fmt.Sprintf("sr:rpm:%d:%s", channelID, minute.UTC().Format("200601021504"))
}

func IncRPM(channelID int) {
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	if common.RedisEnabled && common.RDB != nil {
		key := rpmKey(channelID, minuteFloor(now))
		common.RDB.Incr(ctx, key)
		common.RDB.Expire(ctx, key, 2*time.Minute)
	}
}

func RPMExceeded(channelID int) bool {
	var cap model.ChannelCapability
	if err := model.DB.First(&cap, "channel_id = ?", channelID).Error; err != nil {
		return false
	}
	if cap.RPMHint <= 0 {
		return false
	}
	ctx := context.Background()
	now, _ := RedisTIME(ctx)
	if common.RedisEnabled && common.RDB != nil {
		n, err := common.RDB.Get(ctx, rpmKey(channelID, minuteFloor(now))).Int()
		if err != nil {
			return false
		}
		return n >= cap.RPMHint
	}
	return false
}

func IsCostGuardExcluded(channelID int) bool {
	day := time.Now().Format("2006-01-02")
	var g model.ChannelCostGuard
	if err := model.DB.Where("channel_id = ? AND day = ? AND excluded = ?", channelID, day, true).First(&g).Error; err != nil {
		return false
	}
	return true
}

// CapabilityReject returns exclusion reason if request needs a capability the channel lacks.
// unknown (nil) = allow.
func CapabilityReject(channelID int, needTools, needVision, needJSON bool, needContext int) string {
	var cap model.ChannelCapability
	if err := model.DB.First(&cap, "channel_id = ?", channelID).Error; err != nil {
		return ""
	}
	if needTools && cap.SupportsTools != nil && !*cap.SupportsTools {
		return "capability:tools"
	}
	if needVision && cap.SupportsVision != nil && !*cap.SupportsVision {
		return "capability:vision"
	}
	if needJSON && cap.SupportsJSONSchema != nil && !*cap.SupportsJSONSchema {
		return "capability:json_schema"
	}
	if needContext > 0 && cap.MaxContext > 0 && needContext > cap.MaxContext {
		return "capability:max_context"
	}
	return ""
}
