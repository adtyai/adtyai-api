package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 从旧项目移植并精简（任务 3.2）：只保留非 Stripe 的人工审核。补单入账改调 CompleteAdtyTopUpTx。

const (
	PaymentOrphanStatusPendingReview = "pending_review"
	PaymentOrphanStatusCredited      = "credited"
	PaymentOrphanStatusRefunded      = "refunded"
	PaymentOrphanStatusDismissed     = "dismissed"
)

var (
	ErrPaymentOrphanNotFound       = errors.New("payment orphan event not found")
	ErrPaymentOrphanNotCredit      = errors.New("payment orphan event cannot be credited")
	ErrPaymentOrphanPayloadInvalid = errors.New("payment orphan event payload is invalid")
)

// PaymentOrphanEvent 是对不上订单、或入账失败的支付回调，等管理员处理。
type PaymentOrphanEvent struct {
	ID             int64  `json:"id"`
	Provider       string `json:"provider" gorm:"type:varchar(32);not null;index:idx_adty_orphan_provider_reference,priority:1"`
	EventID        string `json:"event_id" gorm:"type:varchar(128);not null;uniqueIndex"`
	EventType      string `json:"event_type" gorm:"type:varchar(128);not null;index"`
	ReferenceID    string `json:"reference_id" gorm:"type:varchar(255);index:idx_adty_orphan_provider_reference,priority:2"`
	SessionID      string `json:"session_id" gorm:"type:varchar(128);index"`
	Status         string `json:"status" gorm:"type:varchar(32);not null;index"`
	Reason         string `json:"reason" gorm:"type:varchar(512)"`
	Error          string `json:"error" gorm:"type:text"`
	Payload        string `json:"-" gorm:"type:text"`
	CreateTime     int64  `json:"create_time" gorm:"index"`
	UpdateTime     int64  `json:"update_time"`
	ResolvedBy     int    `json:"resolved_by" gorm:"index"`
	ResolvedAt     int64  `json:"resolved_at" gorm:"index"`
	Resolution     string `json:"resolution" gorm:"type:varchar(32)"`
	ResolutionNote string `json:"resolution_note" gorm:"type:text"`
	CanCredit      bool   `json:"can_credit" gorm:"-"`
}

func (PaymentOrphanEvent) TableName() string {
	return "adty_payment_orphan_events"
}

// RecordPaymentOrphanEvent 记一条待审核事件；同一事件重复上报只更新内容。
func RecordPaymentOrphanEvent(event *PaymentOrphanEvent) error {
	if event == nil {
		return nil
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.EventType = strings.TrimSpace(event.EventType)
	event.ReferenceID = strings.TrimSpace(event.ReferenceID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.Status = strings.TrimSpace(event.Status)
	if event.Status == "" {
		event.Status = PaymentOrphanStatusPendingReview
	}
	event.EventID = paymentOrphanEventID(event)
	now := common.GetTimestamp()
	if event.CreateTime <= 0 {
		event.CreateTime = now
	}
	event.UpdateTime = now
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"provider", "event_type", "reference_id", "session_id", "reason", "error", "payload", "update_time",
		}),
	}).Create(event).Error
}

// ListPaymentOrphanEvents 分页列出事件，status 为空或 all 时不过滤。
func ListPaymentOrphanEvents(status string, pageInfo *common.PageInfo) (events []*PaymentOrphanEvent, total int64, err error) {
	query := DB.Model(&PaymentOrphanEvent{})
	status = strings.TrimSpace(status)
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&events).Error
	for _, event := range events {
		event.CanCredit = CanCreditPaymentOrphanEvent(event)
	}
	return events, total, err
}

// CanCreditPaymentOrphanEvent：只有待审核、订单还在、支付商相同、订单没入过账，并且错误不是金额或方式不符的事件，
// 才能在对账页直接补入账。金额不符的走手动补单（任务 3.5）。
func CanCreditPaymentOrphanEvent(orphan *PaymentOrphanEvent) bool {
	if orphan == nil || orphan.Status != PaymentOrphanStatusPendingReview || strings.TrimSpace(orphan.ReferenceID) == "" {
		return false
	}
	if !IsAdtyPaymentProvider(strings.TrimSpace(orphan.Provider)) {
		return false
	}
	errorText := strings.ToLower(orphan.Error)
	if strings.Contains(errorText, ErrPaymentMethodMismatch.Error()) || strings.Contains(errorText, ErrPaymentAmountMismatch.Error()) {
		return false
	}
	var topUps []TopUp
	if err := DB.Select("payment_provider", "status").Where("trade_no = ?", orphan.ReferenceID).Limit(1).Find(&topUps).Error; err != nil || len(topUps) == 0 {
		return false
	}
	topUp := topUps[0]
	if !strings.EqualFold(topUp.PaymentProvider, orphan.Provider) {
		return false
	}
	switch topUp.Status {
	case common.TopUpStatusPending, common.TopUpStatusFailed, common.TopUpStatusExpired, TopUpStatusCanceled:
		return true
	}
	return false
}

// LockPaymentOrphanTx 在事务里锁住一条事件。
func LockPaymentOrphanTx(tx *gorm.DB, id int64) (*PaymentOrphanEvent, error) {
	var rows []PaymentOrphanEvent
	if err := lockForUpdate(tx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrPaymentOrphanNotFound
	}
	return &rows[0], nil
}

// MarkPaymentOrphanCreditedTx 在补入账的同一个事务里把事件标成已入账。
func MarkPaymentOrphanCreditedTx(tx *gorm.DB, orphan *PaymentOrphanEvent, resolvedBy int, note string) error {
	now := common.GetTimestamp()
	return tx.Model(&PaymentOrphanEvent{}).Where("id = ?", orphan.ID).Updates(map[string]any{
		"status":          PaymentOrphanStatusCredited,
		"resolved_by":     resolvedBy,
		"resolved_at":     now,
		"resolution":      PaymentOrphanStatusCredited,
		"resolution_note": strings.TrimSpace(note),
		"update_time":     now,
	}).Error
}

// MarkPaymentOrphanResolved 把事件标成已退款或已忽略（线下处理完以后）。
func MarkPaymentOrphanResolved(id int64, status string, resolvedBy int, note string) error {
	if status != PaymentOrphanStatusRefunded && status != PaymentOrphanStatusDismissed {
		return ErrPaymentOrphanNotCredit
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		orphan, err := LockPaymentOrphanTx(tx, id)
		if err != nil {
			return err
		}
		if orphan.Status != PaymentOrphanStatusPendingReview {
			return nil
		}
		now := common.GetTimestamp()
		return tx.Model(&PaymentOrphanEvent{}).Where("id = ?", orphan.ID).Updates(map[string]any{
			"status":          status,
			"resolved_by":     resolvedBy,
			"resolved_at":     now,
			"resolution":      status,
			"resolution_note": strings.TrimSpace(note),
			"update_time":     now,
		}).Error
	})
}

// PaymentOrphanPayloadString 从回调参数里取一个值，数字按十进制原样转成字符串。
func PaymentOrphanPayloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func paymentOrphanEventID(event *PaymentOrphanEvent) string {
	if eventID := strings.TrimSpace(event.EventID); eventID != "" {
		return eventID
	}
	source := strings.Join([]string{
		strings.TrimSpace(event.Provider),
		strings.TrimSpace(event.EventType),
		strings.TrimSpace(event.ReferenceID),
		strings.TrimSpace(event.SessionID),
		strings.TrimSpace(event.Payload),
	}, "|")
	return "payment_orphan_" + common.Sha1([]byte(source))
}
