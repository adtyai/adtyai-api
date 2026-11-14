package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AdtyAfterTopUpRefundedTx 在退款事务里调用（P6 按比例作废或扣回佣金，P11 还站长加赠）。
// 调用时 ext.RefundQuota 已经是这次退的本金额度 P，比例用 ext.RefundRatio()。
var AdtyAfterTopUpRefundedTx func(tx *gorm.DB, topUp *TopUp, ext *AdtyTopupOrder) error

// AdtyAfterTopUpRefundCommitted runs after a refund transaction commits.
var AdtyAfterTopUpRefundCommitted func(topUp *TopUp, ext *AdtyTopupOrder)

// AdtyTopUpRefundResult 是一次退款的结果：扣回额度 R、赠金额度 G、退的本金额度 P、应退金额（支付币种）。
type AdtyTopUpRefundResult struct {
	Ledger                 *AdtyBalanceLedger               `json:"ledger"`
	DeductedQuota          int64                            `json:"deducted_quota"`
	BonusQuota             int64                            `json:"bonus_quota"`
	RefundQuota            int64                            `json:"refund_quota"`
	RefundAmount           decimal.Decimal                  `json:"refund_amount"`
	Currency               string                           `json:"currency"`
	Facts                  *AdtyRefundPageFacts             `json:"facts,omitempty"`
	Commission             *AdtySiteCommissionRefundPreview `json:"commission,omitempty"`
	CommissionIssuedUSD    string                           `json:"commission_issued_usd,omitempty"`
	CommissionWithdrawnUSD string                           `json:"commission_withdrawn_usd,omitempty"`
	CommissionClawbackUSD  string                           `json:"commission_clawback_usd,omitempty"`
}

// FaceQuota 是订单本金 F：面额 × QuotaPerUnit 向下取整（和入账算 CreditQuota 一样），不超过 CreditQuota。
func (ext *AdtyTopupOrder) FaceQuota() int64 {
	face, err := common.WalletQuotaFromDecimalStrict(ext.FaceUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil || face < 0 {
		return 0
	}
	return min(int64(face), ext.CreditQuota)
}

// RefundRatio 是退掉的本金占本金的比例 P ÷ F（0 到 1）。本金为 0 时按全退算 1。
func (ext *AdtyTopupOrder) RefundRatio() decimal.Decimal {
	face := ext.FaceQuota()
	if face <= 0 {
		return decimal.NewFromInt(1)
	}
	ratio := decimal.NewFromInt(ext.RefundQuota).Div(decimal.NewFromInt(face))
	return decimal.Min(decimal.Max(ratio, decimal.Zero), decimal.NewFromInt(1))
}

// refundCurrency 是应退金额的币种：到账币种，没有时用下单币种。
func (ext *AdtyTopupOrder) refundCurrency() string {
	if ext.PaidCurrency != "" {
		return ext.PaidCurrency
	}
	return ext.PayCurrency
}

// adtyRefundAmount 算应退金额 = 实付 × P ÷ F，向下取到支付币种的最小单位：人民币 2 位；
// USDT 按网关给的实付小数位（数据库是 decimal(20,8)，先去掉末尾的 0 再数），至少 2 位。
func adtyRefundAmount(ext *AdtyTopupOrder, refundQuota, faceQuota int64) decimal.Decimal {
	if refundQuota <= 0 || faceQuota <= 0 || !ext.PaidAmount.IsPositive() {
		return decimal.Zero
	}
	places := int32(2)
	if !strings.EqualFold(ext.refundCurrency(), "CNY") {
		trimmed := decimal.RequireFromString(ext.PaidAmount.String())
		places = max(-trimmed.Exponent(), 2)
	}
	// 先乘后除，商按最小单位做整数除法，避免中间的除法舍入。
	numerator := ext.PaidAmount.Mul(decimal.NewFromInt(refundQuota)).Shift(places)
	units, _ := numerator.QuoRem(decimal.NewFromInt(faceQuota), 0)
	return units.Shift(-places)
}

// RefundAdtyTopUp 退款：只退没花掉的本金，赠金不退（2026-11-01 用户定的规则）。
// 余额以数据库行与持久化未刷差额之和为准，和消费、到账、刷盘共用用户行锁。
// 锁住用户行读余额 B 和还在钱包里的签到赠送 X。可扣余额是 B−X（先花本金，签到不算本金）。
// 扣回 R = min(max(B−X, 0), C)，余额最少到 X；应退本金额度 P = max(0, R − G)，
// 应退金额 = 实付 × P ÷ F（向下取整）。P = 0 时也改成 refunded。真正把钱退给用户在线下做。
// adtyRefundFigures 按当前余额算退款：扣回 R、赠金 G、退的本金额度 P、应退金额。预览和退款必须用同一套。
func adtyRefundFigures(ext *AdtyTopupOrder, balance, checkinQuota int64) (deducted, bonus, refundQuota int64, amount decimal.Decimal) {
	face := ext.FaceQuota()
	bonus = ext.CreditQuota - face
	if bonus < 0 {
		bonus = 0
	}
	// A.8 待履约 gift was not credited to the spendable wallet, so a refund
	// of the paid principal must not treat that promise as already-granted gift.
	if ext.GiftFulfillmentStatus == AdtyGiftFulfillmentPending {
		bonus = 0
	}
	spendable := balance - checkinQuota
	if spendable < 0 {
		spendable = 0
	}
	deducted = min(max(spendable, 0), ext.CreditQuota)
	refundQuota = max(deducted-bonus, 0)
	return deducted, bonus, refundQuota, adtyRefundAmount(ext, refundQuota, face)
}

// PreviewAdtyTopUpRefund 只算不写。退款请求必须带上这里的 refund_quota，事务里再算一遍对不上就拒绝。
func PreviewAdtyTopUpRefund(tradeNo string) (*AdtyTopUpRefundResult, error) {
	var topUp *TopUp
	var ext *AdtyTopupOrder
	var balance int64
	var checkin int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		topUp, ext, err = lockAdtyTopUpTx(tx, tradeNo)
		if err != nil {
			return err
		}
		balance, checkin, err = lockUserQuotaForRefundTx(tx, topUp.UserId)
		return err
	})
	if err != nil {
		return nil, err
	}
	if topUp.Status != common.TopUpStatusSuccess {
		return nil, common.NewMessage("Only credited orders can be refunded")
	}
	deducted, bonus, refundQuota, amount := adtyRefundFigures(ext, balance, checkin)
	result := &AdtyTopUpRefundResult{
		DeductedQuota: deducted, BonusQuota: bonus, RefundQuota: refundQuota,
		RefundAmount: amount, Currency: ext.refundCurrency(),
	}
	preview, err := LoadSiteInviteCommissionRefundPreview(DB, tradeNo, refundQuota, ext.FaceQuota())
	if err != nil {
		return nil, err
	}
	attachSiteInviteCommissionRefundPreview(result, preview)
	facts, err := LoadAdtyRefundPageFacts(tradeNo, decimal.Zero)
	if err != nil {
		return nil, err
	}
	if facts != nil && preview != nil {
		facts.Commission = preview
	}
	result.Facts = facts
	return result, nil
}

func attachSiteInviteCommissionRefundPreview(result *AdtyTopUpRefundResult, preview *AdtySiteCommissionRefundPreview) {
	if result == nil || preview == nil {
		return
	}
	result.Commission = preview
	result.CommissionIssuedUSD = preview.IssuedUSD
	result.CommissionWithdrawnUSD = preview.WithdrawnUSD
	result.CommissionClawbackUSD = preview.ExpectedClawbackUSD
}

// RefundQuotaToUSD converts a requested principal quota into the USD amount
// used by the manual refund application.
func RefundQuotaToUSD(quota int64) (decimal.Decimal, error) {
	if quota <= 0 {
		return decimal.Zero, common.NewMessage("Refund amount must be positive")
	}
	return decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal()), nil
}

// RefundAdtyTopUp files the same manual refund application as a refund_usd
// request. expectedQuota is converted to USD and occupies H under S+H+U+r ≤ L.
// It does not debit the wallet, write completed S, trigger after-sales hooks,
// or pretend the refund is finished. Replay of the same refund id returns the
// original application.
func RefundAdtyTopUp(tradeNo, reason string, operatorID int, idemKey string, expectedQuota int64) (*AdtyManualRefundResult, error) {
	requestUSD, err := RefundQuotaToUSD(expectedQuota)
	if err != nil {
		return nil, err
	}
	return ApplyAdtyTopUpManualRefund(tradeNo, reason, operatorID, idemKey, requestUSD)
}
