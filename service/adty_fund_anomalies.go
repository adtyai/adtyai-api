package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const (
	AdtyAnomalyBalanceLedgerGap      = "balance_ledger_gap"
	AdtyAnomalyOpenShortfallHold     = "open_shortfall_hold"
	AdtyAnomalyMargin                = "margin_anomaly"
	AdtyAnomalyDuplicateCredit       = "duplicate_credit"
	AdtyAnomalyDuplicateRefund       = "duplicate_refund"
	AdtyAnomalyOrderSuccessUnbooked  = "order_success_unbooked"
	AdtyAnomalyGiftGuaranteeShort    = "gift_guarantee_short"
	AdtyAnomalyEarningSupplyMismatch = "earning_supply_mismatch"
	AdtyAnomalyRefundAmountMismatch  = "refund_amount_mismatch"
)

type adtyFundAnomalyItem struct {
	Kind, Ref, OrderRef, Difference, Evidence string
	SiteID, AccountID                         int
}

func detectAdtyFundAnomalies(siteID int) ([]adtyFundAnomalyItem, error) {
	if siteID <= 0 {
		return nil, ErrAdtySiteWalletInvalidOperation
	}
	items, err := detectExistingAdtyFundAnomalies(siteID)
	if err != nil {
		return nil, err
	}
	unbooked, err := detectUnbookedSuccessfulOrders(siteID)
	if err != nil {
		return nil, err
	}
	items = append(items, unbooked...)
	gifts, err := detectInsufficientGiftGuarantees(siteID)
	if err != nil {
		return nil, err
	}
	items = append(items, gifts...)
	earnings, err := detectEarningSupplyMismatches(siteID)
	if err != nil {
		return nil, err
	}
	items = append(items, earnings...)
	refunds, err := detectRefundAmountMismatches(siteID)
	if err != nil {
		return nil, err
	}
	items = append(items, refunds...)
	return items, nil
}

func detectExistingAdtyFundAnomalies(siteID int) ([]adtyFundAnomalyItem, error) {
	var wallets []model.AdtySiteWallet
	if err := model.DB.Where("site_id = ?", siteID).Find(&wallets).Error; err != nil {
		return nil, err
	}
	balanceGap := int64(0)
	marginGap := decimal.Zero
	var operating decimal.Decimal
	var walletMismatches []map[string]any
	for _, wallet := range wallets {
		var deltas []decimal.Decimal
		if err := model.DB.Model(&model.AdtySiteWalletLedger{}).Where("wallet_id = ?", wallet.Id).Pluck("delta", &deltas).Error; err != nil {
			return nil, err
		}
		sum := decimal.Zero
		for _, delta := range deltas {
			sum = sum.Add(delta)
		}
		if !sum.Equal(wallet.Balance) {
			balanceGap = 1
			walletMismatches = append(walletMismatches, map[string]any{
				"wallet_id": wallet.Id, "kind": wallet.Kind,
				"balance": wallet.Balance.String(), "ledger_sum": sum.String(),
				"difference": wallet.Balance.Sub(sum).String(),
			})
		}
		if wallet.Kind == model.AdtySiteWalletOperating {
			operating = wallet.Balance
		}
		if wallet.Kind == model.AdtySiteWalletMargin && wallet.CycleTierUSD.IsPositive() {
			over := wallet.CycleUsedUSD.Sub(AdtyMarginCycleLimit(wallet.CycleTierUSD))
			if over.IsPositive() {
				marginGap = over
			}
		}
	}
	var quota int64
	if err := model.DB.Model(&model.User{}).Where("site_id = ?", siteID).Select("COALESCE(SUM(quota), 0)").Scan(&quota).Error; err != nil {
		return nil, err
	}
	principal := decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal())
	operatingGap := ReconcileOperatingWallet(principal, operating)
	if !operatingGap.IsZero() {
		balanceGap = 1
	}
	var userIDs []int
	if err := model.DB.Model(&model.User{}).Where("site_id = ?", siteID).Pluck("id", &userIDs).Error; err != nil {
		return nil, err
	}
	var holds int64
	if len(userIDs) > 0 {
		if err := model.DB.Model(&model.AdtyBillingHold{}).Where("user_id IN ? AND status = ?", userIDs, model.AdtyBillingHoldOpen).Count(&holds).Error; err != nil {
			return nil, err
		}
	}
	kinds := AdtyFundAnomalies(balanceGap, int(holds), marginGap)
	dupCredit, err := adtyDuplicateEventTradeNos(siteID, "credited", "")
	if err != nil {
		return nil, err
	}
	dupRefund, err := adtyDuplicateEventTradeNos(siteID, "", "refund.succeeded")
	if err != nil {
		return nil, err
	}
	if len(dupCredit) > 0 {
		kinds = append(kinds, AdtyAnomalyDuplicateCredit)
	}
	if len(dupRefund) > 0 {
		kinds = append(kinds, AdtyAnomalyDuplicateRefund)
	}

	ref := strconv.Itoa(siteID)
	var items []adtyFundAnomalyItem
	for _, kind := range kinds {
		item := adtyFundAnomalyItem{Kind: kind, Ref: ref, SiteID: siteID}
		switch kind {
		case AdtyAnomalyBalanceLedgerGap:
			item.Difference = operatingGap.String()
			item.Evidence = mustAnomalyEvidence(map[string]any{
				"operating_wallet":   operating.String(),
				"customer_principal": principal.String(),
				"operating_gap":      operatingGap.String(),
				"wallet_mismatches":  walletMismatches,
			})
		case AdtyAnomalyOpenShortfallHold:
			item.Difference = strconv.FormatInt(holds, 10)
			item.Evidence = mustAnomalyEvidence(map[string]any{"open_holds": holds})
		case AdtyAnomalyMargin:
			item.Difference = marginGap.String()
			item.Evidence = mustAnomalyEvidence(map[string]any{"cycle_overage_usd": marginGap.String()})
		case AdtyAnomalyDuplicateCredit:
			item.Difference = strconv.Itoa(len(dupCredit))
			item.Evidence = mustAnomalyEvidence(map[string]any{"trade_nos": dupCredit, "reason": "repeated credited payment events"})
		case AdtyAnomalyDuplicateRefund:
			item.Difference = strconv.Itoa(len(dupRefund))
			item.Evidence = mustAnomalyEvidence(map[string]any{"trade_nos": dupRefund, "reason": "repeated refund.succeeded payment events"})
		}
		items = append(items, item)
	}
	return items, nil
}

func adtyDuplicateEventTradeNos(siteID int, outcome, eventType string) ([]string, error) {
	query := model.DB.Model(&model.AdtyOrderPaymentEvent{}).Where("site_id = ?", siteID)
	if outcome != "" {
		query = query.Where("outcome = ?", outcome)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	var trades []string
	if err := query.Group("trade_no").Having("COUNT(*) > 1").Pluck("trade_no", &trades).Error; err != nil {
		return nil, err
	}
	return trades, nil
}

func adtyDuplicateEventTrades(siteID int, outcome, eventType string) (int64, error) {
	trades, err := adtyDuplicateEventTradeNos(siteID, outcome, eventType)
	if err != nil {
		return 0, err
	}
	return int64(len(trades)), nil
}

type adtySuccessOrderRow struct {
	TradeNo     string
	UserId      int
	SiteId      int
	Status      string
	CreditQuota int64
	FaceUSD     decimal.Decimal
	HasExt      bool
}

func detectUnbookedSuccessfulOrders(siteID int) ([]adtyFundAnomalyItem, error) {
	var rows []adtySuccessOrderRow
	err := model.DB.Table("top_ups AS t").
		Select(`t.trade_no AS trade_no, t.user_id AS user_id, t.status AS status,
			COALESCE(a.site_id, u.site_id) AS site_id,
			COALESCE(a.credit_quota, 0) AS credit_quota,
			COALESCE(a.face_usd, 0) AS face_usd,
			(a.trade_no IS NOT NULL) AS has_ext`).
		Joins("JOIN users u ON u.id = t.user_id").
		Joins("LEFT JOIN adty_topup_orders a ON a.trade_no = t.trade_no").
		Where("t.status IN ? AND (a.site_id = ? OR (a.trade_no IS NULL AND u.site_id = ?))",
			[]string{common.TopUpStatusSuccess, model.TopUpStatusRefunded}, siteID, siteID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	var items []adtyFundAnomalyItem
	for _, row := range rows {
		expected := row.CreditQuota
		if expected <= 0 && row.FaceUSD.IsPositive() {
			expected = row.FaceUSD.Mul(common.QuotaPerUnitDecimal()).IntPart()
		}
		if expected <= 0 {
			continue
		}
		var booked int64
		if err := model.DB.Model(&model.AdtyBalanceLedger{}).
			Where("user_id = ? AND type = ? AND ref_type = ? AND ref_id = ?", row.UserId, model.AdtyLedgerTypeTopUp, "topup", row.TradeNo).
			Select("COALESCE(SUM(delta_quota), 0)").Scan(&booked).Error; err != nil {
			return nil, err
		}
		if booked >= expected {
			continue
		}
		gap := expected - booked
		items = append(items, adtyFundAnomalyItem{
			Kind:       AdtyAnomalyOrderSuccessUnbooked,
			Ref:        row.TradeNo,
			OrderRef:   row.TradeNo,
			SiteID:     siteID,
			AccountID:  row.UserId,
			Difference: decimal.NewFromInt(gap).Div(common.QuotaPerUnitDecimal()).String(),
			Evidence: mustAnomalyEvidence(map[string]any{
				"status": row.Status, "has_ext": row.HasExt,
				"expected_credit_quota": expected, "booked_quota": booked,
				"face_usd": row.FaceUSD.String(),
				"reason":   "successful top-up principal or official credit was not booked",
			}),
		})
	}
	return items, nil
}

func detectInsufficientGiftGuarantees(siteID int) ([]adtyFundAnomalyItem, error) {
	var orders []model.AdtyTopupOrder
	if err := model.DB.Where("site_id = ? AND (bonus_usd > 0 OR (promo_funder = ? AND promo_bonus_usd > 0))",
		siteID, model.AdtyPromoFunderSiteOwner).Find(&orders).Error; err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, nil
	}
	tradeNos := make([]string, 0, len(orders))
	for _, ext := range orders {
		tradeNos = append(tradeNos, ext.TradeNo)
	}
	var statusByTrade map[string]string
	var err error
	statusByTrade, err = loadTopUpStatuses(tradeNos)
	if err != nil {
		return nil, err
	}
	holdByTrade, ledgerCountByTrade, err := loadGiftHoldByTrade(siteID, tradeNos)
	if err != nil {
		return nil, err
	}
	var items []adtyFundAnomalyItem
	for _, ext := range orders {
		required := adtyBonusUSDForSite(&ext)
		if !required.IsPositive() {
			continue
		}
		status := strings.TrimSpace(statusByTrade[ext.TradeNo])
		if !giftGuaranteeShouldBeHeld(status) {
			continue
		}
		snapshot := ext.GiftReserveEarning.Add(ext.GiftReserveMargin)
		held := snapshot
		if ledgerCountByTrade[ext.TradeNo] > 0 {
			held = holdByTrade[ext.TradeNo]
		}
		if !held.LessThan(required) {
			continue
		}
		items = append(items, adtyFundAnomalyItem{
			Kind:       AdtyAnomalyGiftGuaranteeShort,
			Ref:        ext.TradeNo,
			OrderRef:   ext.TradeNo,
			SiteID:     siteID,
			AccountID:  ext.UserId,
			Difference: required.Sub(held).String(),
			Evidence: mustAnomalyEvidence(map[string]any{
				"status": status, "required_usd": required.String(),
				"reserved_earning": ext.GiftReserveEarning.String(),
				"reserved_margin":  ext.GiftReserveMargin.String(),
				"held_usd":         held.String(),
				"bonus_usd":        ext.BonusUSD.String(),
				"promo_bonus_usd":  ext.PromoBonusUSD.String(),
				"promo_funder":     ext.PromoFunder,
				"reason":           "site-funded gift guarantee is less than the official reserve",
			}),
		})
	}
	return items, nil
}

func giftGuaranteeShouldBeHeld(status string) bool {
	switch status {
	case common.TopUpStatusPending, common.TopUpStatusSuccess, model.TopUpStatusRefunded, "":
		// Pending payable orders must keep the reserve. Credited orders keep
		// the guarantee hold until a later official release or consumption.
		// An empty status means the extension row was found without a top_ups
		// row; still inspect the snapshot rather than hide a short reserve.
		return true
	default:
		return false
	}
}

func loadTopUpStatuses(tradeNos []string) (map[string]string, error) {
	out := map[string]string{}
	if len(tradeNos) == 0 {
		return out, nil
	}
	var rows []model.TopUp
	if err := model.DB.Select("trade_no", "status").Where("trade_no IN ?", tradeNos).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TradeNo] = row.Status
	}
	return out, nil
}

func loadGiftHoldByTrade(siteID int, tradeNos []string) (map[string]decimal.Decimal, map[string]int64, error) {
	holds := map[string]decimal.Decimal{}
	counts := map[string]int64{}
	if len(tradeNos) == 0 {
		return holds, counts, nil
	}
	type holdRow struct {
		RefId   string
		Held    decimal.Decimal
		Ledgers int64
	}
	var rows []holdRow
	err := model.DB.Model(&model.AdtySiteWalletLedger{}).
		Select("ref_id, COALESCE(SUM(hold_delta), 0) AS held, COUNT(*) AS ledgers").
		Where("site_id = ? AND ref_type = ? AND ref_id IN ? AND type IN ?", siteID, "topup", tradeNos, []string{"gift_reserve", "gift_release"}).
		Group("ref_id").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		holds[row.RefId] = row.Held
		counts[row.RefId] = row.Ledgers
	}
	return holds, counts, nil
}

func detectEarningSupplyMismatches(siteID int) ([]adtyFundAnomalyItem, error) {
	var events []model.AdtySiteEarningEvent
	if err := model.DB.Where("site_id = ? AND projected = ?", siteID, false).Find(&events).Error; err != nil {
		return nil, err
	}
	var items []adtyFundAnomalyItem
	seen := map[string]struct{}{}
	for _, event := range events {
		k := decimal.NewFromFloat(event.Markup)
		d := decimal.NewFromFloat(event.CostFactor)
		if event.Quota == 0 || !k.IsPositive() {
			continue
		}
		officialRevenue, officialPlatform, officialEarning := OfficialAdtySupplySplit(int64(event.Quota), k)
		bookedRevenue, bookedPlatform, bookedEarning := AdtySiteEarningSplit(int64(event.Quota), k, d)
		if officialPlatform.Equal(bookedPlatform) && officialEarning.Equal(bookedEarning) {
			continue
		}
		ref := event.OperationKey
		if strings.TrimSpace(ref) == "" {
			ref = "earning:" + strconv.Itoa(event.UserId) + ":" + event.Day
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		items = append(items, adtyFundAnomalyItem{
			Kind:       AdtyAnomalyEarningSupplyMismatch,
			Ref:        ref,
			OrderRef:   ref,
			SiteID:     siteID,
			AccountID:  event.UserId,
			Difference: officialEarning.Sub(bookedEarning).Abs().String(),
			Evidence: mustAnomalyEvidence(map[string]any{
				"operation_key": event.OperationKey, "quota": event.Quota,
				"markup": event.Markup, "cost_factor": event.CostFactor,
				"official_revenue": officialRevenue.String(), "official_platform": officialPlatform.String(),
				"official_earning": officialEarning.String(), "booked_revenue": bookedRevenue.String(),
				"booked_platform": bookedPlatform.String(), "booked_earning": bookedEarning.String(),
				"reason": "site earning and supply-cost books do not match A.10 (q = P×d×k, supply = P×d)",
			}),
		})
	}

	var dailies []model.AdtySiteEarningDaily
	if err := model.DB.Where("site_id = ? AND kind IN ?", siteID,
		[]string{model.AdtySiteEarningUsage, model.AdtySiteEarningRefund, model.AdtySiteEarningSkipped}).
		Find(&dailies).Error; err != nil {
		return nil, err
	}
	type eventSum struct {
		Quota                                                            int64
		OfficialPlatform, OfficialEarning, BookedPlatform, BookedEarning decimal.Decimal
	}
	sums := map[string]eventSum{}
	for _, event := range events {
		k := decimal.NewFromFloat(event.Markup)
		if event.Quota == 0 || !k.IsPositive() {
			continue
		}
		d := decimal.NewFromFloat(event.CostFactor)
		_, officialPlatform, officialEarning := OfficialAdtySupplySplit(int64(event.Quota), k)
		_, bookedPlatform, bookedEarning := AdtySiteEarningSplit(int64(event.Quota), k, d)
		key := strconv.Itoa(event.UserId) + "|" + event.Day
		row := sums[key]
		row.Quota += int64(event.Quota)
		row.OfficialPlatform = row.OfficialPlatform.Add(officialPlatform)
		row.OfficialEarning = row.OfficialEarning.Add(officialEarning)
		row.BookedPlatform = row.BookedPlatform.Add(bookedPlatform)
		row.BookedEarning = row.BookedEarning.Add(bookedEarning)
		sums[key] = row
	}
	for _, daily := range dailies {
		if daily.Quota == 0 && daily.Platform.IsZero() && daily.Earning.IsZero() {
			continue
		}
		key := strconv.Itoa(daily.UserId) + "|" + daily.Day
		sum, ok := sums[key]
		if !ok {
			continue
		}
		if daily.Platform.Equal(sum.OfficialPlatform) && daily.Earning.Equal(sum.OfficialEarning) {
			continue
		}
		if daily.Platform.Equal(sum.BookedPlatform) && daily.Earning.Equal(sum.BookedEarning) && sum.OfficialPlatform.Equal(sum.BookedPlatform) {
			continue
		}
		if !daily.Platform.Equal(sum.OfficialPlatform) || !daily.Earning.Equal(sum.OfficialEarning) {
			ref := "earning-daily:" + daily.Day + ":" + daily.Kind + ":" + strconv.Itoa(daily.UserId)
			if _, exists := seen[ref]; exists {
				continue
			}
			// If an event-level item already exists for this user/day, skip a
			// second daily row unless the stored daily also disagrees with both.
			already := false
			for _, item := range items {
				if item.AccountID == daily.UserId {
					already = true
					break
				}
			}
			if already && (daily.Platform.Equal(sum.BookedPlatform) && daily.Earning.Equal(sum.BookedEarning)) {
				continue
			}
			seen[ref] = struct{}{}
			items = append(items, adtyFundAnomalyItem{
				Kind:       AdtyAnomalyEarningSupplyMismatch,
				Ref:        ref,
				OrderRef:   ref,
				SiteID:     siteID,
				AccountID:  daily.UserId,
				Difference: daily.Earning.Sub(sum.OfficialEarning).Abs().String(),
				Evidence: mustAnomalyEvidence(map[string]any{
					"day": daily.Day, "kind": daily.Kind, "quota": daily.Quota,
					"booked_platform": daily.Platform.String(), "booked_earning": daily.Earning.String(),
					"official_platform": sum.OfficialPlatform.String(), "official_earning": sum.OfficialEarning.String(),
					"reason": "daily earning/supply totals do not match official A.10 books",
				}),
			})
		}
	}
	return items, nil
}

func detectRefundAmountMismatches(siteID int) ([]adtyFundAnomalyItem, error) {
	var refunds []model.AdtyManualRefund
	if err := model.DB.Where("site_id = ?", siteID).Order("id").Find(&refunds).Error; err != nil {
		return nil, err
	}
	var orders []model.AdtyTopupOrder
	if err := model.DB.Where("site_id = ? AND refund_quota > 0", siteID).Find(&orders).Error; err != nil {
		return nil, err
	}
	refundsByTrade := map[string][]model.AdtyManualRefund{}
	tradeSet := map[string]struct{}{}
	for _, row := range refunds {
		refundsByTrade[row.TradeNo] = append(refundsByTrade[row.TradeNo], row)
		tradeSet[row.TradeNo] = struct{}{}
	}
	ordersByTrade := map[string]model.AdtyTopupOrder{}
	for _, ext := range orders {
		ordersByTrade[ext.TradeNo] = ext
		tradeSet[ext.TradeNo] = struct{}{}
	}
	missing := make([]string, 0)
	for trade := range tradeSet {
		if _, ok := ordersByTrade[trade]; !ok {
			missing = append(missing, trade)
		}
	}
	if len(missing) > 0 {
		var extra []model.AdtyTopupOrder
		if err := model.DB.Where("site_id = ? AND trade_no IN ?", siteID, missing).Find(&extra).Error; err != nil {
			return nil, err
		}
		for _, ext := range extra {
			ordersByTrade[ext.TradeNo] = ext
		}
	}
	var items []adtyFundAnomalyItem
	for trade := range tradeSet {
		ext, ok := ordersByTrade[trade]
		if !ok {
			continue
		}
		rows := refundsByTrade[trade]
		if item, ok := refundOccupancyMismatch(siteID, &ext, rows); ok {
			items = append(items, item)
			continue
		}
		if item, ok := refundRowAmountMismatch(siteID, &ext, rows); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func refundOccupancyMismatch(siteID int, ext *model.AdtyTopupOrder, rows []model.AdtyManualRefund) (adtyFundAnomalyItem, bool) {
	completed := ext.RefundQuota
	if completed < 0 {
		completed = 0
	}
	var held, unknown int64
	for _, row := range rows {
		switch row.Status {
		case model.AdtyManualRefundStatusCompleted, model.AdtyManualRefundStatusTerminated:
			continue
		default:
			if row.PayoutStatus == model.AdtyManualRefundPayoutUnknown {
				unknown += row.RequestQuota
			} else {
				held += row.RequestQuota
			}
		}
	}
	occupied := completed + held + unknown
	face := ext.FaceQuota()
	if occupied <= face {
		return adtyFundAnomalyItem{}, false
	}
	gap := occupied - face
	return adtyFundAnomalyItem{
		Kind:       AdtyAnomalyRefundAmountMismatch,
		Ref:        ext.TradeNo,
		OrderRef:   ext.TradeNo,
		SiteID:     siteID,
		AccountID:  ext.UserId,
		Difference: decimal.NewFromInt(gap).Div(common.QuotaPerUnitDecimal()).String(),
		Evidence: mustAnomalyEvidence(map[string]any{
			"face_quota": face, "completed_s": completed, "in_progress_h": held,
			"unknown_u": unknown, "occupied": occupied,
			"reason": "refunded, occupied, or unknown amounts exceed the authorized remaining principal",
		}),
	}, true
}

func refundRowAmountMismatch(siteID int, ext *model.AdtyTopupOrder, rows []model.AdtyManualRefund) (adtyFundAnomalyItem, bool) {
	for _, row := range rows {
		if row.ReclaimedAt > 0 {
			sum := row.DeductedPrincipalQuota + row.ShortfallQuota
			if sum != row.RequestQuota {
				gap := row.RequestQuota - sum
				if gap < 0 {
					gap = -gap
				}
				return adtyFundAnomalyItem{
					Kind:       AdtyAnomalyRefundAmountMismatch,
					Ref:        ext.TradeNo,
					OrderRef:   ext.TradeNo,
					SiteID:     siteID,
					AccountID:  ext.UserId,
					Difference: decimal.NewFromInt(gap).Div(common.QuotaPerUnitDecimal()).String(),
					Evidence: mustAnomalyEvidence(map[string]any{
						"refund_id": row.RefundId, "request_quota": row.RequestQuota,
						"deducted_principal_quota": row.DeductedPrincipalQuota,
						"shortfall_quota":          row.ShortfallQuota,
						"reason":                   "deducted plus shortfall does not equal the authorized refund quota",
					}),
				}, true
			}
		}
		if row.PayoutStatus == model.AdtyManualRefundPayoutSuccess {
			expected := row.ForeignReference
			if !expected.IsPositive() {
				expected = row.RequestUSD
			}
			if expected.IsPositive() && !row.PayoutAmount.Equal(expected) {
				return adtyFundAnomalyItem{
					Kind:       AdtyAnomalyRefundAmountMismatch,
					Ref:        ext.TradeNo,
					OrderRef:   ext.TradeNo,
					SiteID:     siteID,
					AccountID:  ext.UserId,
					Difference: row.PayoutAmount.Sub(expected).Abs().String(),
					Evidence: mustAnomalyEvidence(map[string]any{
						"refund_id": row.RefundId, "payout_amount": row.PayoutAmount.String(),
						"foreign_reference": row.ForeignReference.String(),
						"request_usd":       row.RequestUSD.String(),
						"payout_status":     row.PayoutStatus,
						"reason":            "paid-out amount does not match the authorized refund",
					}),
				}, true
			}
		}
	}
	return adtyFundAnomalyItem{}, false
}

func upsertOpenAdtyAnomalyDisposition(item adtyFundAnomalyItem) error {
	kind, ref := strings.TrimSpace(item.Kind), strings.TrimSpace(item.Ref)
	if kind == "" || ref == "" || item.SiteID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	var existing []model.AdtyAnomalyDisposition
	if err := model.DB.Where("kind = ? AND ref = ? AND status = ?", kind, ref, "open").Limit(1).Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) == 0 {
		return model.DB.Create(&model.AdtyAnomalyDisposition{
			Kind: kind, Ref: ref, Status: "open", Note: "detected",
			SiteId: item.SiteID, AccountId: item.AccountID, OrderRef: strings.TrimSpace(item.OrderRef),
			Difference: item.Difference, Evidence: item.Evidence,
		}).Error
	}
	updates := map[string]any{
		"site_id": item.SiteID, "account_id": item.AccountID,
		"order_ref":  strings.TrimSpace(item.OrderRef),
		"difference": item.Difference, "evidence": item.Evidence,
	}
	return model.DB.Model(&existing[0]).Updates(updates).Error
}

func mustAnomalyEvidence(fields map[string]any) string {
	raw, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(raw)
}
