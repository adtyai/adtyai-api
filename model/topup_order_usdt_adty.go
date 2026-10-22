package model

import "strings"

const (
	TopupDisplayPaid    = "paid"
	TopupDisplayReview  = "review"
	TopupDisplayFailed  = "failed"
	TopupDisplayExpired = "expired"
	TopupDisplayPending = "pending"

	TopupNextContinue   = "continue_pay"
	TopupNextCreateNew  = "create_new"
	TopupNextContact    = "contact_support"
	TopupNextViewCredit = "view_credit"
)

// SetTopupOrderUsdtChain binds the network chosen at checkout. Payment is not started here.
func SetTopupOrderUsdtChain(order *TopupOrder, chain string) error {
	if order == nil {
		return ErrTopupOrderNotFound
	}
	chain = strings.ToLower(strings.TrimSpace(chain))
	if err := DB.Model(order).Update("usdt_chain", chain).Error; err != nil {
		return err
	}
	order.UsdtChain = chain
	return nil
}

// SaveUsdtCashierSnapshot stores the address the user must pay. Continue-pay reads this
// instead of creating another BEpusdt order.
func SaveUsdtCashierSnapshot(order *TopupOrder, address, paymentURL, amount string) error {
	if order == nil {
		return ErrTopupOrderNotFound
	}
	address = strings.TrimSpace(address)
	paymentURL = strings.TrimSpace(paymentURL)
	amount = strings.TrimSpace(amount)
	if address == "" || amount == "" {
		return ErrUsdtPaymentInvalid
	}
	updates := map[string]any{
		"usdt_pay_address":    address,
		"usdt_payment_url":    paymentURL,
		"usdt_payment_amount": amount,
	}
	if err := DB.Model(order).Updates(updates).Error; err != nil {
		return err
	}
	order.UsdtPayAddress = address
	order.UsdtPaymentURL = paymentURL
	order.UsdtPaymentAmount = amount
	return nil
}

// TopupOrderDisplayStatus is what the user should read. Pending USDT past usdt_expire_at
// is shown as expired. The row stays pending so a later verified status-2 callback can still credit.
func TopupOrderDisplayStatus(order *TopupOrder, now int64) string {
	if order == nil {
		return ""
	}
	switch order.Status {
	case TopupOrderStatusPaid:
		return TopupDisplayPaid
	case TopupOrderStatusNeedManual:
		return TopupDisplayReview
	case TopupOrderStatusFailed:
		return TopupDisplayFailed
	case TopupDisplayExpired:
		return TopupDisplayExpired
	}
	if order.Status == TopupOrderStatusPending && usdtCheckoutExpired(order, now) {
		return TopupDisplayExpired
	}
	if order.Status == TopupOrderStatusPending {
		return TopupDisplayPending
	}
	return order.Status
}

// TopupOrderCanContinue is true only while the same checkout can still be opened.
func TopupOrderCanContinue(order *TopupOrder, now int64) bool {
	if order == nil || TopupOrderDisplayStatus(order, now) != TopupDisplayPending {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(order.PaymentMethod)) {
	case "usdt":
		return strings.TrimSpace(order.UsdtPayAddress) != "" && strings.TrimSpace(order.UsdtPaymentAmount) != ""
	case "alipay", "wxpay":
		return strings.TrimSpace(order.EpayPayURL) != "" && order.PayCNY > 0 && order.EpayInstanceId > 0
	default:
		return false
	}
}

// TopupOrderNextAction tells the user what to do next. A closed window is not a review.
func TopupOrderNextAction(order *TopupOrder, now int64) (action, hint string) {
	switch TopupOrderDisplayStatus(order, now) {
	case TopupDisplayPaid:
		return TopupNextViewCredit, "paid_bonus_split"
	case TopupDisplayReview:
		return TopupNextContact, "exception_review"
	case TopupDisplayFailed:
		return TopupNextCreateNew, "failed_retry"
	case TopupDisplayExpired:
		return TopupNextCreateNew, "window_ended"
	case TopupDisplayPending:
		if TopupOrderCanContinue(order, now) {
			return TopupNextContinue, "inside_window"
		}
		return TopupNextCreateNew, "not_payable"
	default:
		return TopupNextCreateNew, "not_payable"
	}
}

func usdtCheckoutExpired(order *TopupOrder, now int64) bool {
	if order == nil || strings.ToLower(strings.TrimSpace(order.PaymentMethod)) != "usdt" {
		return false
	}
	return order.UsdtExpireAt > 0 && now >= order.UsdtExpireAt
}

// ListTopupOrdersByUser returns newest first. limit is clamped to 1–50.
func ListTopupOrdersByUser(userId, page, pageSize int) ([]TopupOrder, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	q := DB.Model(&TopupOrder{}).Where("user_id = ?", userId)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TopupOrder
	err := q.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error
	return rows, total, err
}
