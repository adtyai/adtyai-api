package model

// SidebarBadges counts for admin sidebar hints (总裁·第4).
type SidebarBadges struct {
	RiskPending     int64 `json:"risk_pending"`
	WithdrawPending int64 `json:"withdraw_pending"`
	TopupPending    int64 `json:"topup_pending"`
}

// BuildSidebarBadges returns pending counts:
//   - risk_pending: risk_reviews status=pending
//   - withdraw_pending: aff_withdrawals status=pending（仅待审）
//   - topup_pending: topup_orders status in (pending, need_manual)
func BuildSidebarBadges() (*SidebarBadges, error) {
	var risk, withdraw, topup int64
	if err := DB.Model(&RiskReview{}).
		Where("status = ?", RiskReviewStatusPending).
		Count(&risk).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&AffWithdrawal{}).
		Where("status = ?", AffWithdrawStatusPending).
		Count(&withdraw).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&TopupOrder{}).
		Where("status IN ?", []string{TopupOrderStatusPending, TopupOrderStatusNeedManual}).
		Count(&topup).Error; err != nil {
		return nil, err
	}
	return &SidebarBadges{
		RiskPending:     risk,
		WithdrawPending: withdraw,
		TopupPending:    topup,
	}, nil
}
