package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 优惠码（任务 12.4）：只能在充值时用，作用是多送加赠，必须先付钱；和 P1 删掉的兑换码无关。
// 下单时校验并快照进订单扩展表（PromoCodeId、PromoBonusUSD）；次数在入账事务里用条件更新扣减，
// 订单过期或取消不占次数。退款按订单的入账额度扣回，优惠加赠一起扣回。

const (
	AdtyPromoKindPercent = "percent" // 面额的百分数，10 表示 10%
	AdtyPromoKindFixed   = "fixed"   // 固定美元

	AdtyPromoFunderPlatform  = "platform"
	AdtyPromoFunderSiteOwner = "site_owner" // 站长出资：优惠加赠记进站长收益（负数）
)

// AdtyPromoCode 是一个优惠码。MaxUses、PerUserLimit 为 0 表示不限；时间为 0 表示不限。
// SiteId 是适用站点：0 是主站，其他是分站（用户的 site_id 必须相同）。
type AdtyPromoCode struct {
	Id           int             `json:"id"`
	Code         string          `json:"code" gorm:"type:varchar(32);uniqueIndex;not null"`
	Kind         string          `json:"kind" gorm:"type:varchar(16);not null"`
	Value        decimal.Decimal `json:"value" gorm:"type:decimal(20,8);not null;default:0"`
	MinFaceUSD   decimal.Decimal `json:"min_face_usd" gorm:"column:min_face_usd;type:decimal(20,8);not null;default:0"`
	MaxUses      int             `json:"max_uses" gorm:"not null"`
	PerUserLimit int             `json:"per_user_limit" gorm:"not null"`
	UsedCount    int             `json:"used_count" gorm:"not null"`
	StartsAt     int64           `json:"starts_at" gorm:"not null"`
	ExpiresAt    int64           `json:"expires_at" gorm:"not null"`
	NewUsersOnly bool            `json:"new_users_only" gorm:"not null"`
	SiteId       int             `json:"site_id" gorm:"index;not null"`
	Funder       string          `json:"funder" gorm:"type:varchar(16);not null"`
	Enabled      bool            `json:"enabled" gorm:"not null"`
	Note         string          `json:"note" gorm:"type:varchar(255)"`
	CreatedBy    int             `json:"created_by" gorm:"not null"`
	CreatedAt    int64           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64           `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyPromoCode) TableName() string { return "adty_promo_codes" }

// AdtyPromoRedemption 是一次用掉的优惠码（入账时写）。一笔订单只用一次。
type AdtyPromoRedemption struct {
	Id        int64           `json:"id"`
	CodeId    int             `json:"code_id" gorm:"index:idx_adty_promo_redemption_user,priority:1;not null"`
	UserId    int             `json:"user_id" gorm:"index:idx_adty_promo_redemption_user,priority:2;not null"`
	TradeNo   string          `json:"trade_no" gorm:"type:varchar(64);uniqueIndex;not null"`
	BonusUSD  decimal.Decimal `json:"bonus_usd" gorm:"column:bonus_usd;type:decimal(20,8);not null;default:0"`
	CreatedAt int64           `json:"created_at" gorm:"not null"`
}

func (AdtyPromoRedemption) TableName() string { return "adty_promo_redemptions" }

// NormalizeAdtyPromoCode 去掉空格、转大写。
func NormalizeAdtyPromoCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// GetAdtyPromoCodeByCode 按码取（不分大小写），没有时返回 nil。
func GetAdtyPromoCodeByCode(code string) (*AdtyPromoCode, error) {
	var rows []AdtyPromoCode
	if err := DB.Where("code = ?", NormalizeAdtyPromoCode(code)).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// CountAdtyPromoRedemptions 数一个人用过这个码几次。
func CountAdtyPromoRedemptions(tx *gorm.DB, codeId, userId int) (int64, error) {
	var count int64
	err := tx.Model(&AdtyPromoRedemption{}).Where("code_id = ? AND user_id = ?", codeId, userId).Count(&count).Error
	return count, err
}

// AdtyPromoUserIsNew 判断这个人能不能用只限新用户的码：自己没有成功充值，
// 同 IP 哈希或同设备哈希的其他账号也没有成功充值。空哈希不参与比较。
func AdtyPromoUserIsNew(tx *gorm.DB, userId int) (bool, error) {
	paid, err := AdtyUserHasPaidTopUp(tx, userId)
	if err != nil || paid {
		return false, err
	}
	var prints []AdtyUserFingerprint
	if err := tx.Select("ip_hash", "device_hash").Where("user_id = ?", userId).Find(&prints).Error; err != nil {
		return false, err
	}
	ips := make([]string, 0, len(prints))
	devices := make([]string, 0, len(prints))
	seenIP := map[string]struct{}{}
	seenDevice := map[string]struct{}{}
	for _, row := range prints {
		if row.IpHash != "" {
			if _, ok := seenIP[row.IpHash]; !ok {
				seenIP[row.IpHash] = struct{}{}
				ips = append(ips, row.IpHash)
			}
		}
		if row.DeviceHash != "" {
			if _, ok := seenDevice[row.DeviceHash]; !ok {
				seenDevice[row.DeviceHash] = struct{}{}
				devices = append(devices, row.DeviceHash)
			}
		}
	}
	if len(ips) == 0 && len(devices) == 0 {
		return true, nil
	}
	query := tx.Model(&AdtyUserFingerprint{}).Where("user_id <> ?", userId)
	switch {
	case len(ips) > 0 && len(devices) > 0:
		query = query.Where("ip_hash IN ? OR device_hash IN ?", ips, devices)
	case len(ips) > 0:
		query = query.Where("ip_hash IN ?", ips)
	default:
		query = query.Where("device_hash IN ?", devices)
	}
	var others []int
	if err := query.Distinct("user_id").Pluck("user_id", &others).Error; err != nil {
		return false, err
	}
	if len(others) == 0 {
		return true, nil
	}
	var paidRows []TopUp
	err = lockForUpdate(tx).Select("id").Where("user_id IN ? AND status = ?", others, common.TopUpStatusSuccess).Limit(1).Find(&paidRows).Error
	return len(paidRows) == 0, err
}

// LockAdtyPromoEligibilityTx serializes first-paid eligibility across codes and
// accounts sharing a fingerprint. Every successful top-up, including one with
// no promo, takes this fence before publishing its paid status. Lock users in a
// common order before locking a promo code; the predicate is rechecked below.
func LockAdtyPromoEligibilityTx(tx *gorm.DB, userId int) error {
	var prints []AdtyUserFingerprint
	if err := tx.Where("user_id = ?", userId).Find(&prints).Error; err != nil {
		return err
	}
	ips, devices := []string{}, []string{}
	for _, row := range prints {
		if row.IpHash != "" {
			ips = append(ips, row.IpHash)
		}
		if row.DeviceHash != "" {
			devices = append(devices, row.DeviceHash)
		}
	}
	ids := []int{userId}
	if len(ips)+len(devices) > 0 {
		var related []int
		if err := tx.Model(&AdtyUserFingerprint{}).Where("ip_hash IN ? OR device_hash IN ?", ips, devices).
			Distinct("user_id").Pluck("user_id", &related).Error; err != nil {
			return err
		}
		ids = append(ids, related...)
	}
	var users []User
	return lockForUpdate(tx).Select("id").Where("id IN ?", ids).Order("id").Find(&users).Error
}

// AdtyUserHasPaidTopUp：用户有没有成功的充值（「只限新用户」用）。
func AdtyUserHasPaidTopUp(tx *gorm.DB, userId int) (bool, error) {
	// A locking read observes the latest committed payment on MySQL as well as
	// PostgreSQL, even if an earlier fingerprint read established a snapshot.
	var rows []TopUp
	err := lockForUpdate(tx).Select("id").Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).Limit(1).Find(&rows).Error
	return len(rows) > 0, err
}

// ErrAdtyPromoUnavailable：入账时码已经不能用了（次数用完、每人次数到了、停用）。
var ErrAdtyPromoUnavailable = errors.New("promo code is no longer available")

// RedeemAdtyPromoTx 在入账事务里扣一次优惠码：锁码行，只限新用户的再核一次指纹和充值，
// 每人次数用条件更新加 used_count，写一行使用记录。不能用时返回 ErrAdtyPromoUnavailable，调用方按没有优惠码入账。
// 有效期和最低面额按下单快照，入账时不再查。
func RedeemAdtyPromoTx(tx *gorm.DB, ext *AdtyTopupOrder, now int64) error {
	var codes []AdtyPromoCode
	if err := lockForUpdate(tx).Where("id = ?", ext.PromoCodeId).Limit(1).Find(&codes).Error; err != nil {
		return err
	}
	if len(codes) == 0 || !codes[0].Enabled {
		return ErrAdtyPromoUnavailable
	}
	code := codes[0]
	limit := code.PerUserLimit
	if code.NewUsersOnly {
		limit = 1
		fresh, err := AdtyPromoUserIsNew(tx, ext.UserId)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrAdtyPromoUnavailable
		}
	}
	if limit > 0 {
		used, err := CountAdtyPromoRedemptions(tx, code.Id, ext.UserId)
		if err != nil {
			return err
		}
		if used >= int64(limit) {
			return ErrAdtyPromoUnavailable
		}
	}
	result := tx.Model(&AdtyPromoCode{}).Where("id = ? AND (max_uses = 0 OR used_count < max_uses)", code.Id).
		Update("used_count", gorm.Expr("used_count + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAdtyPromoUnavailable
	}
	return tx.Create(&AdtyPromoRedemption{CodeId: code.Id, UserId: ext.UserId, TradeNo: ext.TradeNo, BonusUSD: ext.PromoBonusUSD, CreatedAt: now}).Error
}

// adtyRedeemPromoOrDropTx 在入账时扣优惠码。码已经不能用（次数用完等）时照样入账面额和普通加赠，
// 把优惠加赠从扩展行里去掉（钱已经收到，不能因为优惠码拒绝入账）。用 SAVEPOINT 隔开，
// 扣减失败不会让 PostgreSQL 把整个入账事务作废。
func adtyRedeemPromoOrDropTx(tx *gorm.DB, ext *AdtyTopupOrder, now int64) error {
	err := tx.Transaction(func(inner *gorm.DB) error {
		return RedeemAdtyPromoTx(inner, ext, now)
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrAdtyPromoUnavailable) {
		return err
	}
	dropQuota := ext.PromoBonusUSD.Mul(common.QuotaPerUnitDecimal()).Floor().IntPart()
	common.SysLog(common.LogText("promo code %d no longer available for trade_no=%s, credited without the promo bonus", ext.PromoCodeId, ext.TradeNo))
	ext.CreditQuota -= dropQuota
	ext.PromoCodeId, ext.PromoBonusUSD, ext.PromoFunder = 0, decimal.Zero, ""
	return tx.Model(&AdtyTopupOrder{}).Where("id = ?", ext.Id).Updates(map[string]any{
		"credit_quota": ext.CreditQuota, "promo_code_id": 0, "promo_bonus_usd": decimal.Zero, "promo_funder": "",
	}).Error
}
