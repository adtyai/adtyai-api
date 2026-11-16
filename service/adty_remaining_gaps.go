package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Exported functions in this file are landed production helpers with
// non-test callers. Unexported helpers are remaining-gap sketches or
// same-package internals; they are not landed HTTP or product APIs.

// AnnouncementDetail returns one published notice by key.
func AnnouncementDetail(items []map[string]any, key string) (map[string]any, bool) {
	key = strings.TrimSpace(key)
	for _, item := range items {
		if item["read_key"] == key || item["id"] == key {
			return item, true
		}
	}
	return nil, false
}

// ListStaffBalanceRequests returns adjustment requests for one site.
func ListStaffBalanceRequests(siteID int) ([]model.AdtyStaffBalanceRequest, error) {
	var rows []model.AdtyStaffBalanceRequest
	err := model.DB.Where("site_id = ?", siteID).Order("id").Find(&rows).Error
	return rows, err
}

// WalletUSDParts splits a wallet into principal and permanent bonus for display.
type WalletUSDParts struct {
	Principal decimal.Decimal
	Bonus     decimal.Decimal
	Total     decimal.Decimal
}

func WalletUSDPartsFromQuota(principalQuota, bonusQuota int64) WalletUSDParts {
	principal := QuotaToUSD(principalQuota)
	bonus := QuotaToUSD(bonusQuota)
	return WalletUSDParts{Principal: principal, Bonus: bonus, Total: principal.Add(bonus)}
}

// PersonalOrder is one row a customer can search.
type PersonalOrder struct {
	TradeNo string
	Kind    string
	Status  string
}

// HealthSampleStatus is no_sample, stale, or ok.
func HealthSampleStatus(samples int, updatedAt, now, staleSeconds int64) string {
	if samples <= 0 || updatedAt <= 0 {
		return "no_sample"
	}
	if staleSeconds > 0 && now-updatedAt > staleSeconds {
		return "stale"
	}
	return "ok"
}

// SiteTierRule is one assessment band.
type SiteTierRule struct {
	Code     string
	MinUsers int
	MinTopup decimal.Decimal
}

// UpdateAdtyTeamProfile lets the leader rename the team. Discount changes stay on the create path.
func UpdateAdtyTeamProfile(leaderId int, teamId int64, name string) error {
	name = strings.TrimSpace(name)
	if teamId <= 0 || name == "" || len(name) > 64 {
		return ErrAdtyTeamInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if team.LeaderUserId != leaderId {
			return ErrAdtyTeamForbidden
		}
		return tx.Model(team).Update("name", name).Error
	})
}

// SubmitAdtyStaffBalanceRequest records an employee request. It does not change the customer balance.
// The idempotency key is required so a retried submit cannot create a second booking.
func SubmitAdtyStaffBalanceRequest(staffUserID, customerUserID int, direction string, amount decimal.Decimal, idempotencyKey string) (*model.AdtyStaffBalanceRequest, error) {
	if staffUserID <= 0 || customerUserID <= 0 || !amount.IsPositive() {
		return nil, common.NewMessage("Invalid parameters")
	}
	if direction != model.AdtyStaffBalanceIncrease && direction != model.AdtyStaffBalanceDecrease {
		return nil, common.NewMessage("Invalid parameters")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 64 {
		return nil, common.NewMessage("Idempotency key is required")
	}
	var staff model.AdtySiteStaff
	if err := model.DB.Where("user_id = ? AND status = ?", staffUserID, "enabled").First(&staff).Error; err != nil {
		return nil, common.NewMessage("No permission to perform this action")
	}
	var customer model.User
	if err := model.DB.First(&customer, customerUserID).Error; err != nil {
		return nil, err
	}
	if customer.SiteId != staff.SiteId || customer.StaffSiteId != 0 {
		return nil, common.NewMessage("No permission to perform this action")
	}
	if existing, err := findAdtyStaffBalanceRequestByIdemKey(model.DB, idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		if !sameAdtyStaffBalanceRequest(existing, staff.SiteId, staffUserID, customerUserID, direction, amount) {
			return nil, model.ErrAdtyIdempotencyConflict
		}
		return existing, nil
	}
	row := &model.AdtyStaffBalanceRequest{
		SiteId: staff.SiteId, StaffUserId: staffUserID, CustomerUserId: customerUserID,
		Direction: direction, AmountUSD: amount, Status: model.AdtyStaffBalancePending,
		IdempotencyKey: &idempotencyKey,
	}
	if err := model.DB.Create(row).Error; err != nil {
		existing, lookupErr := findAdtyStaffBalanceRequestByIdemKey(model.DB, idempotencyKey)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if existing == nil {
			return nil, err
		}
		if !sameAdtyStaffBalanceRequest(existing, staff.SiteId, staffUserID, customerUserID, direction, amount) {
			return nil, model.ErrAdtyIdempotencyConflict
		}
		return existing, nil
	}
	return row, nil
}

func findAdtyStaffBalanceRequestByIdemKey(db *gorm.DB, key string) (*model.AdtyStaffBalanceRequest, error) {
	var rows []model.AdtyStaffBalanceRequest
	if err := db.Where("idempotency_key = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func sameAdtyStaffBalanceRequest(row *model.AdtyStaffBalanceRequest, siteID, staffUserID, customerUserID int, direction string, amount decimal.Decimal) bool {
	return row != nil && row.SiteId == siteID && row.StaffUserId == staffUserID &&
		row.CustomerUserId == customerUserID && row.Direction == direction && row.AmountUSD.Equal(amount)
}

func adtyStaffBalanceLedgerKey(row model.AdtyStaffBalanceRequest) string {
	if row.IdempotencyKey != nil {
		if key := strings.TrimSpace(*row.IdempotencyKey); key != "" {
			return "staff-balance:" + common.Sha1([]byte(key))
		}
	}
	return "staff-balance:" + strconv.FormatInt(row.Id, 10)
}

func sameStaffBalanceLedger(entry *model.AdtyBalanceLedger, row model.AdtyStaffBalanceRequest, delta int64) bool {
	if entry == nil {
		return false
	}
	expected := delta
	if row.Direction == model.AdtyStaffBalanceDecrease {
		expected = -delta
	}
	return entry.Type == model.AdtyLedgerTypeManualAdjust && entry.RefType == "staff_balance" &&
		entry.RefId == strconv.FormatInt(row.Id, 10) && entry.UserId == row.CustomerUserId &&
		entry.DeltaQuota == expected
}

// applyApprovedStaffBalanceTx credits or debits through the official wallet
// helpers so the same transaction writes the ledger and principal fund batches.
// A decrease is refused unless available balance (quota minus in-flight holds)
// covers the full amount.
func applyApprovedStaffBalanceTx(tx *gorm.DB, row model.AdtyStaffBalanceRequest, reviewerID int) error {
	quota, err := common.WalletQuotaFromDecimalStrict(row.AmountUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil {
		return err
	}
	delta := int64(quota)
	if delta <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	var user model.User
	if err := model.AdtyLockForUpdate(tx).First(&user, row.CustomerUserId).Error; err != nil {
		return err
	}
	key := adtyStaffBalanceLedgerKey(row)
	existing, err := model.FindAdtyLedgerByIdemKeyTx(tx, key)
	if err != nil {
		return err
	}
	if existing != nil {
		if !sameStaffBalanceLedger(existing, row, delta) {
			return model.ErrAdtyIdempotencyConflict
		}
		return nil
	}
	var before, applied int64
	if row.Direction == model.AdtyStaffBalanceDecrease {
		var debited int64
		before, debited, err = model.AdtyDebitUserQuotaTx(tx, row.CustomerUserId, delta)
		if err != nil {
			return err
		}
		if debited != delta {
			return common.NewMessage("Customer balance is not enough")
		}
		applied = -debited
		// 传 `before`，不传 `user.Quota`。
		//
		// `AdtyDebitUserQuotaTx` 已经把**逻辑余额**（quota + pending）当 before 返回了，
		// 而准入判据用的也是逻辑余额。所以扣批次必须用同一把尺子——用 raw quota 会让
		// 「钱主要在 quota_pending 里」的账户被判定为批次不够，整笔回滚，报
		// `fund batches cannot cover the debit`。管理员没有绕法。
		//
		// 同一个函数、相隔几行，正确值和错误值同时在手上。
		if err := model.ConsumeAdtyFundBatchesQuietTx(tx, row.CustomerUserId, debited, before); err != nil {
			return err
		}
	} else {
		// **加额度必须先有出资**（`ISS-R4-0075`）。
		//
		// 修前这个分支只做两件事：加客户额度 + 建一条「无资金来源」的批次。
		// 全函数没有任何出资调用，于是一个员工（或站长）可以无限次让平台凭空
		// 给客户加钱——批次表还会把这笔钱记成「有背书的本金」，看起来完全正常。
		//
		// 现在按 operating → earnings → margin 的顺序从站点钱包出资，扣不动就
		// 整笔拒绝。出资和加额度在**同一个事务**里，所以失败时客户余额一分不动。
		if err := adtyFundStaffBalanceIncreaseTx(tx, row, delta); err != nil {
			return err
		}
		before, err = model.AdtyCreditUserQuotaTx(tx, row.CustomerUserId, delta)
		if err != nil {
			return err
		}
		applied = delta
		// 员工调额批次记在申请单所在站（row.SiteId），与该单的出资钱包同站。
		if err := model.CreateAdtyFundBatchesWithSiteTx(tx, row.CustomerUserId, model.AdtyFundBatchSiteID(row.SiteId), adtyStaffBalanceLedgerKey(row), delta, 0); err != nil {
			return err
		}
	}
	return model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
		UserId: row.CustomerUserId, DeltaQuota: applied,
		BalanceBefore: before, BalanceAfter: before + applied,
		Type: model.AdtyLedgerTypeManualAdjust, RefType: "staff_balance",
		RefId: strconv.FormatInt(row.Id, 10), Reason: "staff balance request",
		OperatorId: reviewerID, IdempotencyKey: &key,
	})
}

// ReviewAdtyStaffBalanceRequest approves or rejects a pending request.
// Site owners review their own site. Root reviews any site. Approval changes the customer quota once
// through the official credit or debit path, writing the ledger and fund batches.
// adtyStaffBalanceFundingOrder 是「员工加款从哪儿出钱」的顺序。
//
// 运营钱包先出（那是站点的经营结余），其次已结算收益，最后保证金。
// 顺序有意义：保证金是履约担保，能不动就不动。
var adtyStaffBalanceFundingOrder = []string{
	model.AdtySiteWalletOperating,
	model.AdtySiteWalletEarnings,
	model.AdtySiteWalletMargin,
}

// adtyFundStaffBalanceIncreaseTx 为一次「员工给客户加款」找到出资并扣掉。
//
// 一个钱包扣不动就试下一个；三个都不够就返回 ErrAdtySiteWalletInsufficient，
// 由调用方整笔回滚——**不允许凭空加额度**（`ISS-R4-0075`）。
//
// 金额用 `delta / QuotaPerUnit` 而不是 `row.AmountUSD`：`delta` 是真正要加给客户
// 的额度，用它反算 USD 才能保证「站点扣了多少、客户得到多少」是同一笔。
// QuotaPerUnit 固定 500000，这个除法是精确的（最多 6 位小数）。
func adtyFundStaffBalanceIncreaseTx(tx *gorm.DB, row model.AdtyStaffBalanceRequest, delta int64) error {
	if delta <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	if row.SiteId <= 0 {
		return common.NewMessage("This request has no site to fund it")
	}
	var customer model.User
	if err := tx.Select("id", "site_id").First(&customer, row.CustomerUserId).Error; err != nil {
		return err
	}
	// 客户必须还在这个站点下：站点换了之后，旧站点不该继续为他的加款出钱。
	if customer.SiteId != row.SiteId {
		return common.NewMessage("No permission to perform this action")
	}
	amountUSD := decimal.NewFromInt(delta).Div(common.QuotaPerUnitDecimal())
	key := adtyStaffBalanceLedgerKey(row)
	for _, kind := range adtyStaffBalanceFundingOrder {
		_, err := applyAdtySiteWalletChangeTx(tx, row.SiteId, kind, amountUSD.Neg(), AdtySiteWalletChange{
			Type: "staff_balance", RefType: "staff_balance", RefID: strconv.FormatInt(row.Id, 10),
			Reason:         "staff balance increase for customer " + strconv.Itoa(row.CustomerUserId),
			IdempotencyKey: fmt.Sprintf("%s:fund:%s", key, kind),
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrAdtySiteWalletInsufficient) {
			continue
		}
		return err
	}
	return ErrAdtySiteWalletInsufficient
}

func ReviewAdtyStaffBalanceRequest(reviewerID int, reviewerRole string, requestID int64, approve bool) error {
	var changedUserID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyStaffBalanceRequest
		if err := model.AdtyLockForUpdate(tx).First(&row, requestID).Error; err != nil {
			return err
		}
		if row.Status != model.AdtyStaffBalancePending {
			return common.NewMessage("This request is already reviewed")
		}
		if reviewerRole == "owner" {
			var site model.AdtySite
			if err := tx.First(&site, row.SiteId).Error; err != nil {
				return err
			}
			if site.OwnerUserId != reviewerID {
				return common.NewMessage("No permission to perform this action")
			}
		} else if reviewerRole != "root" {
			return common.NewMessage("No permission to perform this action")
		}
		status := model.AdtyStaffBalanceRejected
		if approve {
			status = model.AdtyStaffBalanceApproved
			if err := applyApprovedStaffBalanceTx(tx, row, reviewerID); err != nil {
				return err
			}
			changedUserID = row.CustomerUserId
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": status, "reviewer_id": reviewerID, "reviewer_role": reviewerRole, "reviewed_at": common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return err
	}
	if changedUserID > 0 {
		model.AdtySyncUserQuotaCache(changedUserID, "staff-balance")
	}
	return nil
}

// ReconcileOperatingWallet returns operating balance minus customer principal.
// A non-zero result is a fund gap and must not be hidden.
func ReconcileOperatingWallet(customerPrincipal, operatingBalance decimal.Decimal) decimal.Decimal {
	return operatingBalance.Sub(customerPrincipal)
}

// AdtyFundAnomalies lists open fund exceptions. Empty means this check found none.
func AdtyFundAnomalies(balanceGap int64, openHolds int, marginGap decimal.Decimal) []string {
	var out []string
	if balanceGap != 0 {
		out = append(out, "balance_ledger_gap")
	}
	if openHolds > 0 {
		out = append(out, "open_shortfall_hold")
	}
	if !marginGap.IsZero() {
		out = append(out, "margin_anomaly")
	}
	return out
}

// AdtyHealthWindowStarts returns the 96 fifteen-minute window starts ending at now.
func AdtyHealthWindowStarts(now time.Time) []int64 {
	end := now.UTC().Truncate(15 * time.Minute)
	out := make([]int64, 96)
	for i := 0; i < 96; i++ {
		out[i] = end.Add(-time.Duration(95-i) * 15 * time.Minute).Unix()
	}
	return out
}

// SaveAdtyGlossaryTerm stores one source/target pair for a locale.
func SaveAdtyGlossaryTerm(source, locale, target string) error {
	source, locale, target = strings.TrimSpace(source), strings.TrimSpace(locale), strings.TrimSpace(target)
	if source == "" || locale == "" || target == "" {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyGlossaryTerm{Source: source, Locale: locale, Target: target}
	return model.DB.Where(model.AdtyGlossaryTerm{Source: source, Locale: locale}).Assign(row).FirstOrCreate(&row).Error
}

// CompleteAdtyTodo marks one severe inbox item done for that user.
func CompleteAdtyTodo(userID int, key string) error {
	return HideAdtyNotificationForUser(userID, "todo:"+strings.TrimSpace(key))
}

// QuotaToUSD converts internal quota to USD at the fixed integer scale.
func QuotaToUSD(quota int64) decimal.Decimal {
	return decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal())
}

// BindSiteInviteFromIP records one direct site invite and keeps the client IP hash.
// A saved Root batch threshold can refuse another same-IP bind and open a review.
// An unset threshold does not guess a count.
func BindSiteInviteFromIP(siteID, inviterID, inviteeID int, ipHash string) error {
	if siteID <= 0 || inviterID <= 0 || inviteeID <= 0 || inviterID == inviteeID {
		return common.NewMessage("Invalid parameters")
	}
	var rate model.AdtySiteReferralRate
	if err := model.DB.Where("site_id = ?", siteID).First(&rate).Error; err != nil {
		return common.NewMessage("Site invite rate is not configured")
	}
	var inviter, invitee model.User
	if err := model.DB.First(&inviter, inviterID).Error; err != nil {
		return err
	}
	if err := model.DB.First(&invitee, inviteeID).Error; err != nil {
		return err
	}
	if inviter.SiteId != siteID || invitee.SiteId != siteID {
		return common.NewMessage("No permission to perform this action")
	}
	var paid int64
	if err := model.DB.Model(&model.TopUp{}).Where("user_id = ? AND status = ?", inviteeID, common.TopUpStatusSuccess).Count(&paid).Error; err != nil {
		return err
	}
	if paid > 0 {
		return common.NewMessage("This customer already has a paid order")
	}
	var existing model.AdtySiteInvite
	lookup := model.DB.Where("invitee_id = ?", inviteeID).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected > 0 {
		if existing.InviterId != inviterID || existing.SiteId != siteID {
			return common.NewMessage("Invitee already has an inviter")
		}
		return nil
	}
	if err := refuseSiteInviteBatch(siteID, inviterID, ipHash); err != nil {
		return err
	}
	return model.DB.Create(&model.AdtySiteInvite{
		SiteId: siteID, InviterId: inviterID, InviteeId: inviteeID, Rate: rate.Rate, IpHash: strings.TrimSpace(ipHash),
	}).Error
}

func refuseSiteInviteBatch(siteID, inviterID int, ipHash string) error {
	ipHash = strings.TrimSpace(ipHash)
	if ipHash == "" {
		return nil
	}
	thresholdRaw, thresholdOK, err := model.ReadOptionFromDB(adtySiteBatchThresholdKey)
	if err != nil || !thresholdOK {
		return err
	}
	windowRaw, windowOK, err := model.ReadOptionFromDB(adtySiteBatchWindowKey)
	if err != nil || !windowOK {
		return err
	}
	threshold, err := strconv.ParseInt(strings.TrimSpace(thresholdRaw), 10, 64)
	if err != nil || threshold <= 0 {
		return nil
	}
	hours, err := strconv.ParseInt(strings.TrimSpace(windowRaw), 10, 64)
	if err != nil || hours <= 0 {
		return nil
	}
	since := time.Now().Unix() - hours*3600
	var count int64
	if err := model.DB.Model(&model.AdtySiteInvite{}).
		Where("inviter_id = ? AND ip_hash = ? AND created_at >= ?", inviterID, ipHash, since).
		Count(&count).Error; err != nil {
		return err
	}
	if count+1 < threshold {
		return nil
	}
	ref := strconv.Itoa(siteID)
	note := "inviter " + strconv.Itoa(inviterID)
	var existing int64
	if err := model.DB.Model(&model.AdtyAnomalyDisposition{}).
		Where("kind = ? AND ref = ? AND status = ? AND note = ?", "batch_register", ref, "open", note).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		if err := recordAdtyAnomalyDisposition("batch_register", ref, "open", note, 0); err != nil {
			return err
		}
	}
	return common.NewMessage("This invite needs review")
}

// SetSiteReferralRate stores the site's one-level invite rate. Rate is a fraction in [0, 1].
func SetSiteReferralRate(siteID int, rate decimal.Decimal) error {
	if siteID <= 0 || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(1)) {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtySiteReferralRate{SiteId: siteID, Rate: rate}
	return model.DB.Where(model.AdtySiteReferralRate{SiteId: siteID}).Assign(map[string]any{"rate": rate}).FirstOrCreate(&row).Error
}

// SiteInviteCommissionUSD is face times the site rate. It does not add a payment-method gift.
func SiteInviteCommissionUSD(face, rate decimal.Decimal) decimal.Decimal {
	if !face.IsPositive() || !rate.IsPositive() {
		return decimal.Zero
	}
	return face.Mul(rate).Round(8)
}

// recordAdtyAnomalyDisposition stores a review note for one detected anomaly.
func recordAdtyAnomalyDisposition(kind, ref, status, note string, operatorID int) error {
	kind, ref, status = strings.TrimSpace(kind), strings.TrimSpace(ref), strings.TrimSpace(status)
	if kind == "" || ref == "" || status == "" {
		return common.NewMessage("Invalid parameters")
	}
	return model.DB.Create(&model.AdtyAnomalyDisposition{Kind: kind, Ref: ref, Status: status, Note: note, OperatorId: operatorID}).Error
}

// RequestPriceSnapshot freezes the accepted model and group price.
type RequestPriceSnapshot struct {
	Model    string
	Group    string
	UnitUSD  decimal.Decimal
	FrozenAt int64
}

// HideAdtyNotificationForUser removes one inbox item for that user only.
func HideAdtyNotificationForUser(userID int, key string) error {
	return hideAdtyNotificationForUser(nil, userID, key)
}

// HideAdtyNotificationForUserFromRequest is the request-scoped hide path so
// the official audit can use the site already resolved for this request.
func HideAdtyNotificationForUserFromRequest(c *gin.Context, userID int, key string) error {
	return hideAdtyNotificationForUser(c, userID, key)
}

func hideAdtyNotificationForUser(c *gin.Context, userID int, key string) error {
	key = strings.TrimSpace(key)
	if userID <= 0 || key == "" {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyNotificationHide{UserId: userID, Key: key}
	result := model.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	writeAdtyNotificationHideAudit(c, userID, key)
	return nil
}

func writeAdtyNotificationHideAudit(c *gin.Context, userID int, key string) {
	rec := model.OfficialAuditWrite{
		UserId:     userID,
		SiteID:     model.OfficialAuditSiteIDPointer(0),
		ObjectType: "notification",
		ObjectID:   key,
		Action:     "adty.notification.hide",
		Category:   model.AuditCategoryOperation,
		Content:    "adty.notification.hide",
		Result:     model.AuditResultSuccess,
		Success:    true,
	}
	var actor *model.User
	if userID > 0 {
		if user, err := model.GetUserById(userID, false); err == nil && user != nil {
			actor = user
			rec.Username = user.Username
			rec.ActorRole = user.Role
		}
	}
	rec.SiteID = model.OfficialAuditSiteIDPointer(officialNotificationHideSiteID(c, actor))
	if err := model.WriteOfficialAuditLog(c, rec); err != nil {
		common.SysLog(common.LogText("official notification hide audit write failed: %s", err.Error()))
	}
}

func officialNotificationHideSiteID(c *gin.Context, user *model.User) int {
	if site := AdtySiteForRequest(c); site != nil {
		return site.Id
	}
	if user != nil {
		return user.SiteId
	}
	return 0
}
