package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// 分站客户的风控（任务 11.5）：客户在分站注册时，复用 6.5 的指纹（adty_user_fingerprints）判断
//   - 疑似自用：和站长本人 fingerprint_window_days 天内的 IP 或设备哈希相同
//   - 疑似主站迁移：和主站某个充过值的用户的设备哈希相同
// 有标记并且没复核过的客户照常扣费，收益记进 skipped（不结算）。判断依据只写进 RiskReason，只给后台看。

// adtyCheckSiteCustomerRisk 在 AdtyRecordSiteCustomer 里调用（11.2 的注册钩子）。
func adtyCheckSiteCustomerRisk(siteId, userId int, ipHash, deviceHash string) (flag, reason string) {
	cfg := resellerSetting()
	if ipHash == "" && deviceHash == "" {
		return "", ""
	}
	site := AdtySiteById(siteId)
	if site == nil {
		return "", ""
	}
	since := time.Now().Unix() - int64(max(cfg.FingerprintWindowDays, 1))*86400
	if cfg.SelfUseCheckEnabled {
		query := model.DB.Model(&model.AdtyUserFingerprint{}).Where("user_id = ? AND created_at >= ?", site.OwnerUserId, since)
		switch {
		case ipHash != "" && deviceHash != "":
			query = query.Where("(ip_hash = ? OR device_hash = ?)", ipHash, deviceHash)
		case ipHash != "":
			query = query.Where("ip_hash = ?", ipHash)
		default:
			query = query.Where("device_hash = ?", deviceHash)
		}
		var matches int64
		if err := query.Count(&matches).Error; err != nil {
			common.SysError(common.LogText("site self-use check failed user=%d: %s", userId, err.Error()))
		} else if matches > 0 {
			return model.AdtySiteRiskSelfUse, "shares an IP or device with the site owner"
		}
	}
	if cfg.MigrationCheckEnabled && deviceHash != "" {
		// 主站用户：site_id = 0，并且有成功的充值。
		paid := model.DB.Model(&model.TopUp{}).Select("user_id").Where("status = ?", common.TopUpStatusSuccess)
		mainUsers := model.DB.Model(&model.User{}).Select("id").Where("site_id = 0 AND id <> ? AND id IN (?)", userId, paid)
		var matches int64
		if err := model.DB.Model(&model.AdtyUserFingerprint{}).
			Where("device_hash = ? AND created_at >= ? AND user_id IN (?)", deviceHash, since, mainUsers).
			Count(&matches).Error; err != nil {
			common.SysError(common.LogText("site migration check failed user=%d: %s", userId, err.Error()))
		} else if matches > 0 {
			return model.AdtySiteRiskMigration, "shares a device with a paying main-site customer"
		}
	}
	return "", ""
}

func init() {
	AdtyCheckSiteCustomerRisk = adtyCheckSiteCustomerRisk
}
