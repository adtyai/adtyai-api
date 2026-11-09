package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// AdtyOrderPaymentEvent records safe outcomes, never raw payloads, credentials,
// provider errors or manual proof. Credited/replayed amounts are order facts;
// failed attempts deliberately contain no unverified payment amount.
type AdtyOrderPaymentEvent struct {
	Id           int64           `json:"id"`
	TradeNo      string          `json:"trade_no" gorm:"type:varchar(64);not null;index:idx_adty_payment_event_order,priority:1"`
	SiteId       int             `json:"site_id"`
	UserId       int             `json:"user_id"`
	Provider     string          `json:"provider" gorm:"type:varchar(32);not null"`
	Source       string          `json:"source" gorm:"type:varchar(16);not null"`
	Outcome      string          `json:"outcome" gorm:"type:varchar(24);not null"`
	EventType    string          `json:"event_type" gorm:"type:varchar(128)"`
	PaidAmount   decimal.Decimal `json:"paid_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidCurrency string          `json:"paid_currency" gorm:"type:varchar(16)"`
	OperatorId   int             `json:"operator_id"`
	CreatedAt    int64           `json:"created_at" gorm:"autoCreateTime;index:idx_adty_payment_event_order,priority:2"`
}

func (AdtyOrderPaymentEvent) TableName() string { return "adty_order_payment_events" }

var ErrAdtyPaymentEventImmutable = errors.New("payment events are immutable")

func (AdtyOrderPaymentEvent) BeforeUpdate(*gorm.DB) error { return ErrAdtyPaymentEventImmutable }
func (AdtyOrderPaymentEvent) BeforeDelete(*gorm.DB) error { return ErrAdtyPaymentEventImmutable }

func appendAdtyPaymentEventTx(tx *gorm.DB, ext *AdtyTopupOrder, source, outcome string, operatorID int) error {
	event := AdtyOrderPaymentEvent{TradeNo: ext.TradeNo, SiteId: ext.SiteId, UserId: ext.UserId,
		Provider: ext.Provider, Source: source, Outcome: outcome, OperatorId: operatorID}
	if outcome == "credited" || outcome == "replayed" {
		event.PaidAmount, event.PaidCurrency = ext.PaidAmount, ext.PaidCurrency
		if source == AdtyPaymentSourceManual {
			event.PaidAmount, event.PaidCurrency = ext.PayAmount, ext.PayCurrency
		}
	}
	return tx.Create(&event).Error
}

// RecordAdtyPaymentAttempt resolves ownership from SQL, rather than trusting a
// caller's stale site/user snapshot. It does not create orders or move money.
func RecordAdtyPaymentAttempt(tradeNo, provider, source, outcome string, operatorID int) error {
	switch outcome {
	case "failed", "unpaid", "rejected":
	default:
		return errors.New("invalid payment attempt outcome")
	}
	var rows []AdtyTopupOrder
	err := DB.Table("adty_topup_orders AS a").Select("a.*").
		Joins("JOIN top_ups t ON t.trade_no = a.trade_no AND t.user_id = a.user_id AND t.payment_provider = a.provider").
		Where("a.trade_no = ? AND a.provider = ?", tradeNo, provider).Limit(1).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return err
	}
	return appendAdtyPaymentEventTx(DB, &rows[0], source, outcome, operatorID)
}

// RecordAdtyVerifiedCallback is called only after signature verification. A
// receipt is not payment proof: it carries neither an amount nor a wallet change.
func RecordAdtyVerifiedCallback(tradeNo, provider, eventType string) error {
	if tradeNo == "" {
		return nil
	}
	var rows []AdtyTopupOrder
	err := DB.Table("adty_topup_orders AS a").Select("a.*").
		Joins("JOIN top_ups t ON t.trade_no = a.trade_no AND t.user_id = a.user_id AND t.payment_provider = a.provider").
		Where("a.trade_no = ? AND a.provider = ?", tradeNo, provider).Limit(1).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return err
	}
	ext := rows[0]
	return DB.Create(&AdtyOrderPaymentEvent{TradeNo: ext.TradeNo, SiteId: ext.SiteId, UserId: ext.UserId,
		Provider: provider, Source: AdtyPaymentSourceCallback, Outcome: "received", EventType: eventType}).Error
}

func recordAdtyRejectedPayment(tradeNo string, facts AdtyPaymentFacts) {
	if err := RecordAdtyPaymentAttempt(tradeNo, facts.Provider, facts.Source, "rejected", facts.OperatorId); err != nil {
		common.SysError("payment attempt history could not be recorded")
	}
}
