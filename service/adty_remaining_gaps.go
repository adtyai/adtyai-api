package service

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

// publicPageResponsive reports whether the public shell has a viewport and a small-screen breakpoint.
func publicPageResponsive(html, component string) bool {
	return strings.Contains(html, "width=device-width") && strings.Contains(component, "sm:")
}

// saveSiteTelegramBot stores a site management bot. The token is masked by the shared saver.
func saveSiteTelegramBot(siteID int, token string, enabled bool) (*AdtyTelegramSaved, error) {
	if siteID <= 0 {
		return nil, ErrAdtyTelegramInvalid
	}
	return SaveAdtyTelegramBot("site:"+strconv.Itoa(siteID), token, enabled)
}

// setModelShelf marks a model alias available or hidden.
func setModelShelf(alias string, enabled bool) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.AdtyModelAlias{}).Where("alias = ?", alias).Update("enabled", enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("Model not found")
	}
	return nil
}

// listGlobalOrders returns every top-up order, newest id first.
func listGlobalOrders(limit int) ([]model.AdtyTopupOrder, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []model.AdtyTopupOrder
	err := model.DB.Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// filterOrdersByStatus keeps personal orders with the requested status.
func filterOrdersByStatus(rows []PersonalOrder, status string) []PersonalOrder {
	status = strings.TrimSpace(status)
	if status == "" {
		return rows
	}
	var out []PersonalOrder
	for _, row := range rows {
		if row.Status == status {
			out = append(out, row)
		}
	}
	return out
}

// listUserLedger returns balance ledger rows for one user.
func listUserLedger(userID, limit int) ([]model.AdtyBalanceLedger, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []model.AdtyBalanceLedger
	err := model.DB.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// storeDynamicTranslation saves the English result for one Chinese source hash.
func storeDynamicTranslation(objectKey, sourceHash, english string) error {
	objectKey = strings.TrimSpace(objectKey)
	sourceHash = strings.TrimSpace(sourceHash)
	english = strings.TrimSpace(english)
	if objectKey == "" || sourceHash == "" || english == "" {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyTranslationHead{ObjectKey: objectKey, SourceHash: sourceHash, Version: 1}
	return model.DB.Where("object_key = ?", objectKey).Assign(model.AdtyTranslationHead{SourceHash: sourceHash, Version: 1}).FirstOrCreate(&row).Error
}

// changeOwnPassword rejects a short password and a mismatch with the current hash check result.
func changeOwnPassword(currentOK bool, next string) error {
	if !currentOK {
		return common.NewMessage("Current password is incorrect")
	}
	if len(strings.TrimSpace(next)) < 8 {
		return common.NewMessage("Password is too short")
	}
	return nil
}

// accountLoginAllowed is false when the account is disabled.
func accountLoginAllowed(status int) bool {
	return status == common.UserStatusEnabled
}

// serviceHealthy is true only when the database ping succeeds.
func serviceHealthy(dbErr error) bool {
	return dbErr == nil
}

// setKeyRateLimit stores how many requests a key may make each minute.
func setKeyRateLimit(tokenID, perMinute int) error {
	if tokenID <= 0 || perMinute <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyKeyLimit{TokenId: tokenID, PerMinute: perMinute}
	return model.DB.Where("token_id = ?", tokenID).Assign(row).FirstOrCreate(&row).Error
}

// saveModelAlias stores an enabled public name, upstream target, and context window.
func saveModelAlias(alias, target string, context int, enabled bool) error {
	alias, target = strings.TrimSpace(alias), strings.TrimSpace(target)
	if alias == "" || target == "" || context <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyModelAlias{Alias: alias, Target: target, Context: context, Enabled: enabled}
	return model.DB.Where("alias = ?", alias).Assign(row).FirstOrCreate(&row).Error
}

// saveGroupConfig stores a group's ratio and model list. Site 0 is the main site.
func saveGroupConfig(name string, siteID int, ratio decimal.Decimal, models string) error {
	name = strings.TrimSpace(name)
	if name == "" || !ratio.IsPositive() {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyGroupConfig{Name: name, SiteId: siteID, Ratio: ratio, Models: strings.TrimSpace(models)}
	return model.DB.Where("name = ? AND site_id = ?", name, siteID).Assign(row).FirstOrCreate(&row).Error
}

// requestModality classifies a payload as text, multimodal, or tools.
func requestModality(hasImage, hasTools bool) string {
	switch {
	case hasTools:
		return "tools"
	case hasImage:
		return "multimodal"
	default:
		return "text"
	}
}

// relayTerminal returns retry, failed, or success.
func relayTerminal(success bool, attempts, maxAttempts int) string {
	if success {
		return "success"
	}
	if attempts < maxAttempts {
		return "retry"
	}
	return "failed"
}

// listSiteOrders returns top-up orders for one site.
func listSiteOrders(siteID int) ([]model.AdtyTopupOrder, error) {
	var rows []model.AdtyTopupOrder
	err := model.DB.Where("site_id = ?", siteID).Order("id").Find(&rows).Error
	return rows, err
}

// orderFundSource names the payment source stored on an order.
func orderFundSource(provider, method string) string {
	provider, method = strings.TrimSpace(provider), strings.TrimSpace(method)
	if provider == "" && method == "" {
		return ""
	}
	if method == "" {
		return provider
	}
	if provider == "" {
		return method
	}
	return provider + ":" + method
}

// ListStaffBalanceRequests returns adjustment requests for one site.
func ListStaffBalanceRequests(siteID int) ([]model.AdtyStaffBalanceRequest, error) {
	var rows []model.AdtyStaffBalanceRequest
	err := model.DB.Where("site_id = ?", siteID).Order("id").Find(&rows).Error
	return rows, err
}

// setKeyGroup stores the group a key may use.
func setKeyGroup(userID, tokenID int, group string) error {
	group = strings.TrimSpace(group)
	if userID <= 0 || tokenID <= 0 || group == "" {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.Token{}).Where("id = ? AND user_id = ?", tokenID, userID).Update("group", group)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("No permission to perform this action")
	}
	return nil
}

// setKeyAllowIps stores a comma-separated IP allow list for a key.
func setKeyAllowIps(userID, tokenID int, ips string) error {
	ips = strings.ReplaceAll(strings.TrimSpace(ips), " ", "")
	if userID <= 0 || tokenID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.Token{}).Where("id = ? AND user_id = ?", tokenID, userID).Update("allow_ips", ips)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("No permission to perform this action")
	}
	return nil
}

// parseRequestTarget returns the model and group carried by a request.
func parseRequestTarget(modelName, group string) (string, string, error) {
	modelName = strings.TrimSpace(modelName)
	group = strings.TrimSpace(group)
	if modelName == "" || group == "" {
		return "", "", common.NewMessage("Invalid parameters")
	}
	return modelName, group, nil
}

// assignSiteOwner sets the owner only while the site has none.
func assignSiteOwner(siteID, ownerID int) error {
	if siteID <= 0 || ownerID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.AdtySite{}).Where("id = ? AND owner_user_id = 0", siteID).Update("owner_user_id", ownerID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("Site owner is already assigned")
	}
	return nil
}

// setSiteTier stores the assessed tier code.
func setSiteTier(siteID int, code string) error {
	code = strings.TrimSpace(code)
	if siteID <= 0 || code == "" {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.AdtySite{}).Where("id = ?", siteID).Update("tier_code", code)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("Site not found")
	}
	return nil
}

// listSiteCustomerIDs returns users registered on one site.
func listSiteCustomerIDs(siteID int) ([]int, error) {
	var ids []int
	err := model.DB.Model(&model.User{}).Where("site_id = ?", siteID).Order("id").Pluck("id", &ids).Error
	return ids, err
}

// classifyOrderKind maps a row to topup, consume, voucher, or refund.
func classifyOrderKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "topup", "consume", "voucher", "refund":
		return kind
	default:
		return ""
	}
}

// financeExportAllowed is true only for root and finance roles.
func financeExportAllowed(role string) bool {
	return role == "root" || role == "finance"
}

// reuseTranslation returns the stored English when the source hash matches.
func reuseTranslation(sourceHash, storedHash, english string) (string, bool) {
	if sourceHash == "" || sourceHash != storedHash || english == "" {
		return "", false
	}
	return english, true
}

// maskSecret keeps only the last four characters.
func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

// orderDetail finds one personal order and returns its status for display.
func orderDetail(rows []PersonalOrder, tradeNo string) (PersonalOrder, bool) {
	tradeNo = strings.TrimSpace(tradeNo)
	for _, row := range rows {
		if row.TradeNo == tradeNo {
			return row, true
		}
	}
	return PersonalOrder{}, false
}

// setKeyEnabled enables or disables an API key owned by the user.
func setKeyEnabled(userID, tokenID int, enabled bool) error {
	if userID <= 0 || tokenID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	status := 2
	if enabled {
		status = 1
	}
	result := model.DB.Model(&model.Token{}).Where("id = ? AND user_id = ?", tokenID, userID).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("No permission to perform this action")
	}
	return nil
}

// remainingSetSiteBrand stores the public site name.
func remainingSetSiteBrand(siteID int, name string) error {
	name = strings.TrimSpace(name)
	if siteID <= 0 || name == "" || len(name) > 64 {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&model.AdtySite{}).Where("id = ?", siteID).Update("brand", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("Site not found")
	}
	return nil
}

// setSiteEnabled no longer pauses or resumes a site. Independent pause/resume is retired.
func setSiteEnabled(siteID int, enabled bool) error {
	return ErrAdtySitePauseRetired
}

// testTranslationConnectivity rejects an empty or non-https endpoint before any dial.
func testTranslationConnectivity(baseURL string) error {
	baseURL = strings.TrimSpace(baseURL)
	if !strings.HasPrefix(baseURL, "https://") {
		return common.NewMessage("Translation endpoint must use https")
	}
	return nil
}

// saveBilingualTemplate stores a fixed notice in Chinese and English.
func saveBilingualTemplate(key, zh, en string) error {
	key, zh, en = strings.TrimSpace(key), strings.TrimSpace(zh), strings.TrimSpace(en)
	if key == "" || zh == "" || en == "" {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyNoticeTemplate{Key: key, Zh: zh, En: en}
	return model.DB.Where("notice_key = ?", key).Assign(row).FirstOrCreate(&row).Error
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

// filterPersonalOrders returns matching orders for one page. Page starts at 1.
func filterPersonalOrders(rows []PersonalOrder, tradeNo, kind string, page, pageSize int) []PersonalOrder {
	tradeNo = strings.TrimSpace(tradeNo)
	kind = strings.TrimSpace(kind)
	var matched []PersonalOrder
	for _, row := range rows {
		if tradeNo != "" && !strings.Contains(row.TradeNo, tradeNo) {
			continue
		}
		if kind != "" && row.Kind != kind {
			continue
		}
		matched = append(matched, row)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(matched) {
		return nil
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end]
}

// notificationVisible reports whether a viewer is inside the publication audience.
func notificationVisible(audience string, targetSite, targetTeam, targetUser, viewerSite, viewerTeam, viewerUser int) bool {
	switch audience {
	case "all":
		return true
	case "site":
		return targetSite > 0 && viewerSite == targetSite
	case "team":
		return targetTeam > 0 && viewerTeam == targetTeam
	case "user":
		return targetUser > 0 && viewerUser == targetUser
	default:
		return false
	}
}

// translationPlaceholdersMatch requires the translation to keep amounts, order numbers, and {{tokens}}.
func translationPlaceholdersMatch(source, translated string) bool {
	return extractPlaceholders(source) == extractPlaceholders(translated)
}

func extractPlaceholders(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '{' && i+1 < len(text) && text[i+1] == '{' {
			end := strings.Index(text[i:], "}}")
			if end >= 0 {
				b.WriteString(text[i : i+end+2])
				i += end + 1
			}
		}
	}
	return b.String()
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

// requestEmailChange stores a pending address change. It does not update the login email until confirmed.
func requestEmailChange(userID int, email string) error {
	email = strings.TrimSpace(email)
	if userID <= 0 || !strings.Contains(email, "@") {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyEmailChange{UserId: userID, NewEmail: email, Status: "pending"}
	return model.DB.Where("user_id = ?", userID).Assign(row).FirstOrCreate(&row).Error
}

// requestAccountClosure starts a closure that still needs email confirmation.
func requestAccountClosure(userID int) error {
	if userID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyAccountClosure{UserId: userID, Status: "pending_email"}
	return model.DB.Where("user_id = ?", userID).Assign(row).FirstOrCreate(&row).Error
}

// SiteTierRule is one assessment band.
type SiteTierRule struct {
	Code     string
	MinUsers int
	MinTopup decimal.Decimal
}

// evaluateSiteTier picks the highest band whose user and top-up floors are met.
func evaluateSiteTier(users int, topup decimal.Decimal, rules []SiteTierRule) string {
	best := ""
	bestUsers := -1
	for _, rule := range rules {
		if users >= rule.MinUsers && topup.GreaterThanOrEqual(rule.MinTopup) && rule.MinUsers >= bestUsers {
			best = rule.Code
			bestUsers = rule.MinUsers
		}
	}
	return best
}

// saveTranslationChoice stores the selected model and endpoint. The key is masked, not returned.
func saveTranslationChoice(modelName, baseURL string) error {
	modelName = strings.TrimSpace(modelName)
	baseURL = strings.TrimSpace(baseURL)
	if modelName == "" || baseURL == "" || !strings.HasPrefix(baseURL, "https://") {
		return common.NewMessage("Invalid parameters")
	}
	row := model.AdtyTranslationConfig{Id: 1, ModelName: modelName, BaseURL: baseURL, TokenMask: "****", Enabled: true, Mode: model.AdtyTranslationModeOwn}
	return model.DB.Where("id = ?", 1).Assign(row).FirstOrCreate(&row).Error
}

// translationRetryAllowed reports whether a failed job may run again.
func translationRetryAllowed(status string, attempts, limit int) bool {
	return status == model.AdtyTranslationFailed && attempts < limit
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

// creditAdtyTeamTopup adds principal quota to the team shared balance.
// Team top-ups do not receive a payment-method gift.
func creditAdtyTeamTopup(leaderId int, teamId int64, principalQuota int64, ref string) error {
	if principalQuota <= 0 {
		return ErrAdtyTeamInvalid
	}
	return CreditAdtyTeam(int64(leaderId), teamId, principalQuota, "team-topup:"+strings.TrimSpace(ref))
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
	rawBefore := int64(user.Quota)
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
		if err := model.ConsumeAdtyFundBatchesQuietTx(tx, row.CustomerUserId, debited, rawBefore); err != nil {
			return err
		}
	} else {
		before, err = model.AdtyCreditUserQuotaTx(tx, row.CustomerUserId, delta)
		if err != nil {
			return err
		}
		applied = delta
		if err := model.CreateAdtyFundBatchesTx(tx, row.CustomerUserId, adtyStaffBalanceLedgerKey(row), delta, 0); err != nil {
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

// recordAdtyPriceRevision appends one price change. History rows are not updated.
func recordAdtyPriceRevision(siteID, actorID int, kind, before, after string) error {
	kind = strings.TrimSpace(kind)
	if kind == "" || before == after {
		return common.NewMessage("Invalid parameters")
	}
	return model.DB.Create(&model.AdtyPriceRevision{SiteId: siteID, ActorId: actorID, Kind: kind, Before: before, After: after}).Error
}

// adtyBalanceLedgerGap is the difference between a wallet balance and the sum of its ledger deltas.
func adtyBalanceLedgerGap(balance int64, ledgerDeltas []int64) int64 {
	var sum int64
	for _, delta := range ledgerDeltas {
		sum += delta
	}
	return balance - sum
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

// ownerEarningView drops channel fields. Site owners see model and group, not the upstream channel.
func ownerEarningView(row map[string]any) map[string]any {
	delete(row, "channel")
	delete(row, "channel_id")
	delete(row, "channel_name")
	return row
}

// setMainTeamAgreementPrice sets the discount on a main-site team. Subsite teams are rejected.
func setMainTeamAgreementPrice(teamID int64, discount decimal.Decimal) error {
	if teamID <= 0 || discount.LessThan(decimal.RequireFromString("0.90")) || discount.GreaterThan(decimal.NewFromInt(1)) {
		return ErrAdtyTeamInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamID).Error; err != nil {
			return err
		}
		if team.SiteId != 0 || team.Status != model.AdtyTeamStatusActive {
			return ErrAdtyTeamForbidden
		}
		return tx.Model(&team).Update("discount", discount).Error
	})
}

// applyDailyEarningOnce credits one day's earning. A second call with the same key does not add again.
func applyDailyEarningOnce(siteID int, day string, amount decimal.Decimal) error {
	day = strings.TrimSpace(day)
	if siteID <= 0 || day == "" || !amount.IsPositive() {
		return ErrAdtySiteWalletInvalidOperation
	}
	_, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletEarnings, amount, AdtySiteWalletChange{
		Type: "daily_bill", RefType: "day", RefID: day, IdempotencyKey: "daily:" + day,
	})
	return err
}

// snapshotInviteCommission copies the invite rate onto the order. Later rate changes do not alter it.
func snapshotInviteCommission(order *model.AdtyTopupOrder, rate decimal.Decimal) {
	if order == nil {
		return
	}
	order.ReferralRate = rate
}

// recordCommissionLedger appends one commission detail. The same trade cannot be inserted twice.
func recordCommissionLedger(tradeNo string, affiliateID int, face, rate decimal.Decimal) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" || affiliateID <= 0 || !face.IsPositive() || rate.IsNegative() {
		return common.NewMessage("Invalid parameters")
	}
	amount, _ := face.Mul(rate).Div(decimal.NewFromInt(100)).Float64()
	faceAmount, _ := face.Float64()
	row := model.ReferralCommission{
		AffiliateId: affiliateID, SourceType: "topup", SourceTradeNo: tradeNo,
		BaseAmount: faceAmount, CommissionAmount: amount, Status: "pending",
	}
	return model.DB.Where("source_trade_no = ? AND source_type = ?", tradeNo, "topup").FirstOrCreate(&row).Error
}

// snapshotWithdrawFX freezes the CNY rate used for one withdrawal.
func snapshotWithdrawFX(amountUSD, rate decimal.Decimal, at int64) (decimal.Decimal, int64, error) {
	if !amountUSD.IsPositive() || !rate.IsPositive() {
		return decimal.Zero, 0, common.NewMessage("Exchange rate must be positive")
	}
	return amountUSD.Mul(rate).Round(2), at, nil
}

// assignAdtyTicket moves a ticket to another agent and records the official in-progress status.
func assignAdtyTicket(ticketID int64, assigneeID int) error {
	if ticketID <= 0 || assigneeID <= 0 {
		return common.NewMessage("Invalid parameters")
	}
	var ticket model.AdtyTicket
	if err := model.DB.First(&ticket, ticketID).Error; err != nil {
		return err
	}
	if !canAssignAdtyTicket(model.NormalizeAdtyTicketStatus(ticket.Status, ticket.AssigneeId)) {
		return common.NewMessage("Invalid parameters")
	}
	result := model.DB.Model(&ticket).Updates(map[string]any{
		"assignee_id": assigneeID, "status": model.AdtyTicketInProgress,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return common.NewMessage("Ticket not found")
	}
	return nil
}

// CompleteAdtyTodo marks one severe inbox item done for that user.
func CompleteAdtyTodo(userID int, key string) error {
	return HideAdtyNotificationForUser(userID, "todo:"+strings.TrimSpace(key))
}

// rejectCrossSite blocks a read or write when the actor and the resource are on different sites.
func rejectCrossSite(actorSiteID, resourceSiteID int) error {
	if actorSiteID == resourceSiteID {
		return nil
	}
	return common.NewMessage("No permission to perform this action")
}

// QuotaToUSD converts internal quota to USD at the fixed integer scale.
func QuotaToUSD(quota int64) decimal.Decimal {
	return decimal.NewFromInt(quota).Div(common.QuotaPerUnitDecimal())
}

// setMainReferralRate stores the main-site default commission percent. 10 means 10%.
func setMainReferralRate(percent decimal.Decimal) error {
	if percent.IsNegative() || percent.GreaterThan(decimal.NewFromInt(100)) {
		return common.NewMessage("Invalid parameters")
	}
	value, _ := percent.Float64()
	adty_setting.GetReferralSetting().DefaultRate = &value
	return nil
}

// bindSiteInvite records one direct site invite. An invitee can have only one inviter.
func bindSiteInvite(siteID, inviterID, inviteeID int) error {
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
	return BindSiteInviteFromIP(siteID, inviterID, inviteeID, "")
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

// sensitiveActionNeedsProof is false only for Root. Other roles must prove password or email code.
func sensitiveActionNeedsProof(role int) bool {
	return role != common.RoleRootUser
}

// sensitiveProofMethods lists the G.14 important-operation step-up methods on
// the live path: password first, then email_code. Completing either one is
// enough. Enrolled 2FA/Passkey may also be offered by securityVerificationPolicy,
// but they do not replace these two. Root has no challenge methods.
func sensitiveProofMethods(role int) []string {
	if !sensitiveActionNeedsProof(role) {
		return nil
	}
	return []string{VerificationMethodPassword, VerificationMethodEmailCode}
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

// teamDiscountAllowed rejects a subsite team price below the supply factor.
// The main site has no supply floor.
func teamDiscountAllowed(supply, discounted decimal.Decimal, mainSite bool) error {
	if mainSite {
		return nil
	}
	if discounted.LessThan(supply) {
		return common.NewMessage("Team price is below the site supply price")
	}
	return nil
}

// duplicateRefundTrades returns trade numbers that appear more than once.
func duplicateRefundTrades(tradeNos []string) []string {
	counts := map[string]int{}
	for _, trade := range tradeNos {
		counts[trade]++
	}
	var dups []string
	for trade, count := range counts {
		if count > 1 {
			dups = append(dups, trade)
		}
	}
	return dups
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

// freezeRequestPrice records the price used for one accepted request.
func freezeRequestPrice(modelName, group string, unitUSD decimal.Decimal, at int64) RequestPriceSnapshot {
	return RequestPriceSnapshot{Model: modelName, Group: group, UnitUSD: unitUSD, FrozenAt: at}
}

// estimateRequestUSD multiplies the frozen unit price by the estimated units.
func estimateRequestUSD(snapshot RequestPriceSnapshot, units decimal.Decimal) decimal.Decimal {
	if !snapshot.UnitUSD.IsPositive() || !units.IsPositive() {
		return decimal.Zero
	}
	return snapshot.UnitUSD.Mul(units).Round(8)
}

// unusedPrechargeRelease is the quota to return when actual usage is below the precharge.
func unusedPrechargeRelease(pre, actual int) int {
	if pre <= actual || pre <= 0 {
		return 0
	}
	return pre - actual
}

// failureFeeQuota is zero when the upstream call was not accepted. A failed call does not keep the precharge.
func failureFeeQuota(upstreamAccepted bool, pre int) int {
	if !upstreamAccepted || pre <= 0 {
		return 0
	}
	return pre
}

// setUSDToCNYRate sets the main-site USD to CNY payment rate. Zero and negative values are rejected.
func setUSDToCNYRate(rate decimal.Decimal) error {
	if !rate.IsPositive() {
		return common.NewMessage("Exchange rate must be positive")
	}
	value, _ := rate.Float64()
	operation_setting.Price = value
	return nil
}

// debitOperatingPrincipal records a customer principal spend against the internal operating wallet.
func debitOperatingPrincipal(siteID int, amount decimal.Decimal, ref string) error {
	if siteID <= 0 || !amount.IsPositive() || strings.TrimSpace(ref) == "" {
		return ErrAdtySiteWalletInvalidOperation
	}
	_, err := ApplyAdtySiteWalletChange(siteID, model.AdtySiteWalletOperating, amount.Neg(), AdtySiteWalletChange{
		Type: "principal_spend", RefType: "consume", RefID: ref, IdempotencyKey: "operating:" + ref, AllowOverdraft: true,
	})
	return err
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
