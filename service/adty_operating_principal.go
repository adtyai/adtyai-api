package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func init() {
	model.AdtyAfterPrincipalSpentTx = adtyDebitOperatingPrincipalTx
}

func adtyCreditOperatingPrincipalTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder) error {
	if tx == nil || ext == nil || ext.SiteId <= 0 || ext.TeamId > 0 {
		return nil
	}
	return adtyMoveOperatingPrincipalTx(tx, ext.SiteId, ext.FaceQuota(), "principal_credit", "topup", ext.TradeNo, "operating:topup:"+ext.TradeNo)
}

func adtyDebitOperatingPrincipalTx(tx *gorm.DB, userID int, principalQuota int64) error {
	if tx == nil || userID <= 0 || principalQuota <= 0 {
		return nil
	}
	var user model.User
	if err := tx.Select("id", "site_id").First(&user, userID).Error; err != nil {
		return err
	}
	if user.SiteId <= 0 {
		return nil
	}
	key := "operating:consume:" + common.GetUUID()
	return adtyMoveOperatingPrincipalTx(tx, user.SiteId, -principalQuota, "principal_consume", "consume", key, key)
}

func adtyMoveOperatingPrincipalTx(tx *gorm.DB, siteID int, quota int64, changeType, refType, refID, key string) error {
	if siteID <= 0 || quota == 0 {
		return nil
	}
	usd := decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal())
	if usd.IsZero() {
		return nil
	}
	_, err := applyAdtySiteWalletChangeTx(tx, siteID, model.AdtySiteWalletOperating, usd, AdtySiteWalletChange{
		Type: changeType, RefType: refType, RefID: refID, IdempotencyKey: key,
	})
	return err
}
