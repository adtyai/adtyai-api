package service

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 站长分站的开站、审核、调整和关站（任务 11.1）。

var (
	ErrAdtySiteDisabled      = common.NewMessage("Reseller sites are not available")
	ErrAdtySiteNotFound      = common.NewMessage("Site not found")
	ErrAdtySiteCustomer      = common.NewMessage("Customers of a reseller site cannot open a site")
	ErrAdtySiteAlreadyOwner  = common.NewMessage("You already have a site")
	ErrAdtySiteSubdomainBad  = common.NewMessage("The subdomain may only contain lowercase letters, digits and hyphens, and cannot start or end with a hyphen")
	ErrAdtySiteSubdomainUsed = common.NewMessage("This subdomain is not available")
	ErrAdtySiteAgreement     = common.NewMessage("Accept the current site agreement first")
	ErrAdtySiteInviteInvalid = common.NewMessage("The invite code is invalid or used up")
	ErrAdtySiteInviteOff     = common.NewMessage("Invite codes are not enabled")
	ErrAdtySiteStatus        = common.NewMessage("The site cannot change to this status")
	ErrAdtySiteTransfer      = common.NewMessage("Closed-site customers can only be inherited by the main site")
	ErrAdtySitePauseRetired  = common.NewMessage("Independent site pause and resume is not supported")
)

// AdtySitePreset 是开站时预设的单站 d 和 k 上限（管理员直接开或邀请码带的），空就用全局。
type AdtySitePreset struct {
	CostFactor    *decimal.Decimal
	MaxMarkup     *decimal.Decimal
	ContactInfo   string
	AgreementVer  string
	InviteCodeId  int
	OperatorId    int
	ExplicitRoot  string // 管理员开站可以指定根域名，空就用 root_domains 的第一个
	InitialStatus string // 空则按开站方式决定
}

func resellerSetting() *adty_setting.ResellerSetting { return adty_setting.GetResellerSetting() }

// AdtyNormalizeSubdomain 去空白转小写后按 DNS 标签规则、保留名校验。
// xn-- 是 punycode，不能当站名。CNAME 目标和回源主机名在根域名下的那一段也自动保留。
func AdtyNormalizeSubdomain(raw string) (string, error) {
	label := strings.ToLower(strings.TrimSpace(raw))
	if !adty_setting.ResellerLabelPattern.MatchString(label) || strings.HasPrefix(label, "xn--") {
		return "", ErrAdtySiteSubdomainBad
	}
	cfg := resellerSetting()
	if slices.Contains(cfg.ReservedSubdomains, label) || slices.Contains(adtyInfrastructureLabels(cfg), label) {
		return "", ErrAdtySiteSubdomainUsed
	}
	return label, nil
}

// adtyInfrastructureLabels 取出 cname_target、fallback_origin 紧挨根域名的那一段标签。
func adtyInfrastructureLabels(cfg *adty_setting.ResellerSetting) []string {
	if cfg == nil {
		return nil
	}
	labels := make([]string, 0, 2)
	for _, host := range []string{cfg.CnameTarget, cfg.FallbackOrigin} {
		if label := adtyLabelUnderRoot(host, cfg.RootDomains); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func adtyLabelUnderRoot(host string, roots []string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return ""
	}
	for _, root := range roots {
		root = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(root)), ".")
		if root == "" || !strings.HasSuffix(host, "."+root) {
			continue
		}
		prefix := strings.TrimSuffix(host, "."+root)
		parts := strings.Split(prefix, ".")
		label := parts[len(parts)-1]
		if adty_setting.ResellerLabelPattern.MatchString(label) {
			return label
		}
	}
	return ""
}

// AdtyCurrentSiteAgreementVersion 是开站协议当前的版本号（字符串存进站点）。
func AdtyCurrentSiteAgreementVersion() string {
	return strconv.Itoa(adty_setting.GetLegalSetting().SiteAgreementVersion)
}

// AdtyCurrentSiteCustomerTermsVersion 是分站客户条款当前的版本号。0 表示还没写。
func AdtyCurrentSiteCustomerTermsVersion() string {
	return strconv.Itoa(adty_setting.GetLegalSetting().SiteCustomerTermsVersion)
}

// AdtySiteTermsStatus 说明登录后要不要重新勾选。站长看开站协议，分站客户看客户条款。
func AdtySiteTermsStatus(userId int) (required bool, version string) {
	if site, err := AdtyOwnerSite(userId); err == nil && site != nil {
		version = AdtyCurrentSiteAgreementVersion()
		return site.AgreementVer != version, version
	}
	version = AdtyCurrentSiteCustomerTermsVersion()
	if adty_setting.GetLegalSetting().SiteCustomerTermsVersion <= 0 {
		return false, version
	}
	var siteID int
	if err := model.DB.Model(&model.User{}).Select("site_id").Where("id = ?", userId).Scan(&siteID).Error; err != nil || siteID == 0 {
		return false, version
	}
	var rows []model.AdtySiteCustomer
	if err := model.DB.Select("terms_version").Where("user_id = ? AND site_id = ?", userId, siteID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return false, version
	}
	return rows[0].TermsVersion != version, version
}

// AcceptAdtySiteTerms 接受当前条款。站长写开站协议，分站客户写 TermsVersion 和 TermsAt。
// 版本对不上不写；已经是当前版本则直接成功。
func AcceptAdtySiteTerms(userId int, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return ErrAdtySiteAgreement
	}
	if _, err := AdtyOwnerSite(userId); err == nil {
		return acceptOwnerSiteAgreement(userId, version)
	}
	return acceptCustomerSiteTerms(userId, version)
}

func acceptOwnerSiteAgreement(userId int, version string) error {
	current := AdtyCurrentSiteAgreementVersion()
	if version != current {
		return ErrAdtySiteAgreement
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var sites []model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("owner_user_id = ? AND status IN ?", userId,
			[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}).Limit(1).Find(&sites).Error; err != nil {
			return err
		}
		if len(sites) == 0 {
			return ErrAdtySiteNotOwner
		}
		site := sites[0]
		if site.AgreementVer == current {
			return nil
		}
		now := time.Now().Unix()
		if err := tx.Model(&site).Updates(map[string]any{"agreement_ver": current, "agreement_at": now}).Error; err != nil {
			return err
		}
		return recordSiteAuditTx(tx, site.Id, "terms_accept", userId,
			map[string]any{"agreement_ver": site.AgreementVer}, map[string]any{"agreement_ver": current}, "")
	})
}

func acceptCustomerSiteTerms(userId int, version string) error {
	if adty_setting.GetLegalSetting().SiteCustomerTermsVersion <= 0 {
		return ErrAdtySiteAgreement
	}
	current := AdtyCurrentSiteCustomerTermsVersion()
	if version != current {
		return ErrAdtySiteAgreement
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var users []model.User
		if err := tx.Select("id", "site_id").Where("id = ?", userId).Limit(1).Find(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 || users[0].SiteId == 0 {
			return ErrAdtySiteNotOwner
		}
		var rows []model.AdtySiteCustomer
		if err := model.AdtyLockForUpdate(tx).Where("user_id = ? AND site_id = ?", userId, users[0].SiteId).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrAdtySiteNotOwner
		}
		if rows[0].TermsVersion == current {
			return nil
		}
		now := time.Now().Unix()
		return tx.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", userId).Updates(map[string]any{
			"terms_version": current, "terms_at": now,
		}).Error
	})
}

// validateSitePreset 检查预设的供货倍率和存储 k 上限：供货倍率必须为正且不低于下限，k 上限必须为正且不超过全局上限。
func validateSitePreset(costFactor, maxMarkup *decimal.Decimal) error {
	cfg := resellerSetting()
	if costFactor != nil {
		if !costFactor.IsPositive() {
			return common.NewMessage("The cost factor must be greater than 0")
		}
		if cfg.MinCostFactor != nil && costFactor.LessThan(decimal.NewFromFloat(*cfg.MinCostFactor)) {
			return common.NewMessage("The cost factor cannot be below the minimum cost factor")
		}
	}
	if maxMarkup != nil {
		if !maxMarkup.IsPositive() || (cfg.MaxMarkup != nil && maxMarkup.GreaterThan(decimal.NewFromFloat(*cfg.MaxMarkup))) {
			return common.NewMessage("The markup limit must be greater than 0 and at most the global limit")
		}
	}
	return nil
}

func recordSiteAuditTx(tx *gorm.DB, siteId int, action string, operatorId int, oldValue, newValue any, reason string) error {
	reason = truncateText(reason, 1000)
	if err := tx.Create(&model.AdtySiteAudit{
		SiteId: siteId, Action: action, OperatorId: operatorId,
		OldValue: common.GetJsonString(oldValue), NewValue: common.GetJsonString(newValue), Reason: reason,
	}).Error; err != nil {
		return err
	}
	username, role := "", 0
	if operatorId > 0 {
		var users []model.User
		if err := tx.Select("id", "username", "role").Where("id = ?", operatorId).Limit(1).Find(&users).Error; err != nil {
			return err
		}
		if len(users) > 0 {
			username, role = users[0].Username, users[0].Role
		}
	}
	officialAction := action
	if !strings.Contains(action, ".") {
		officialAction = "adty.site." + action
	}
	siteID := siteId
	if err := model.WriteOfficialAuditLog(nil, model.OfficialAuditWrite{
		UserId: operatorId, Username: username, ActorRole: role,
		SiteID: &siteID, ObjectType: "site", ObjectID: strconv.Itoa(siteId),
		Action: officialAction, Category: model.AuditCategoryOperation, Content: officialAction,
		Before: oldValue, After: newValue, Reason: reason, Result: model.AuditResultSuccess, Success: true,
	}); err != nil {
		// Compatibility AdtySiteAudit is already in this transaction. A log-store
		// failure must not roll back the business change.
		common.SysLog(common.LogText("official site audit write failed: %s", err.Error()))
	}
	return nil
}

// OpenAdtySite 开站：三种方式最后都调它。在一个事务里锁用户行、查重、建站点、写审计；
// 开成 active 时同时写 ReferralAffiliate.SiteOwnerSince（邀请返佣从此停止，执行层决定 27）。
// 申请开站（apply）建 pending，审核通过时再写 SiteOwnerSince。
func OpenAdtySite(userId int, subdomain, openedBy string, preset AdtySitePreset) (*model.AdtySite, error) {
	cfg := resellerSetting()
	if !cfg.Ready() {
		return nil, ErrAdtySiteDisabled
	}
	label, err := AdtyNormalizeSubdomain(subdomain)
	if err != nil {
		return nil, err
	}
	if err := validateSitePreset(preset.CostFactor, preset.MaxMarkup); err != nil {
		return nil, err
	}
	currentVer := AdtyCurrentSiteAgreementVersion()
	if openedBy != model.AdtySiteOpenedByAdmin && (adty_setting.GetLegalSetting().SiteAgreementVersion <= 0 || strings.TrimSpace(preset.AgreementVer) != currentVer) {
		return nil, ErrAdtySiteAgreement
	}
	root := preset.ExplicitRoot
	if root == "" {
		root = cfg.RootDomains[0]
	} else if !slices.Contains(cfg.RootDomains, root) {
		return nil, common.NewMessage("Invalid root domain {{domain}}", map[string]any{"domain": root})
	}
	status := preset.InitialStatus
	if status == "" {
		status = model.AdtySiteStatusActive
		if openedBy == model.AdtySiteOpenedByApply {
			status = model.AdtySiteStatusPending
		}
	}
	now := time.Now().Unix()
	site := &model.AdtySite{
		OwnerUserId: userId, Subdomain: label, RootDomain: root, Status: status,
		CostFactor: preset.CostFactor, MaxMarkup: preset.MaxMarkup, ContactInfo: truncateText(preset.ContactInfo, 500),
		OpenedBy: openedBy, AgreementVer: strings.TrimSpace(preset.AgreementVer),
	}
	if site.AgreementVer != "" {
		site.AgreementAt = now
	}
	var txOptions []*sql.TxOptions
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// Marker initialization can read before the user lock. Qualifications
		// checked after waiting must include another opening's committed site.
		txOptions = append(txOptions, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// Commission transfers lock the affiliate before the user wallet. Keep
		// the same order; invalid openings roll this uncommitted marker back.
		if status == model.AdtySiteStatusActive {
			if err := markAffiliateSiteOwnerTx(tx, userId, now); err != nil {
				return err
			}
		}
		var users []model.User
		if err := model.AdtyLockForUpdate(tx).Select("id", "site_id", "status").Where("id = ?", userId).Limit(1).Find(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 || users[0].Status != common.UserStatusEnabled {
			return common.NewMessage("User not found")
		}
		if users[0].SiteId > 0 {
			return ErrAdtySiteCustomer
		}
		var owned int64
		if err := tx.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status <> ?", userId, model.AdtySiteStatusClosed).Count(&owned).Error; err != nil {
			return err
		}
		if owned > 0 {
			return ErrAdtySiteAlreadyOwner
		}
		var taken int64
		if err := tx.Model(&model.AdtySite{}).Where("subdomain = ?", label).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return ErrAdtySiteSubdomainUsed
		}
		if preset.InviteCodeId > 0 {
			// 条件更新防并发超用：max_uses 为 0 不限。
			result := tx.Model(&model.AdtySiteInviteCode{}).
				Where("id = ? AND (max_uses = 0 OR used_count < max_uses) AND (expire_at = 0 OR expire_at > ?)", preset.InviteCodeId, now).
				Update("used_count", gorm.Expr("used_count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrAdtySiteInviteInvalid
			}
		}
		if err := tx.Create(site).Error; err != nil {
			return err
		}
		if err := ensureAdtySiteWalletsTx(tx, site.Id); err != nil {
			return err
		}
		return recordSiteAuditTx(tx, site.Id, "open", preset.OperatorId, nil, map[string]any{
			"opened_by": openedBy, "status": status, "subdomain": label, "cost_factor": site.CostFactor, "max_markup": site.MaxMarkup,
			"invite_code_id": preset.InviteCodeId,
		}, "")
	}, txOptions...)
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	return site, nil
}

// markAffiliateSiteOwnerTx 写 SiteOwnerSince（只在还是 0 时写，关站后重开不覆盖第一次的时间）。
// 还没有推广员行时补建，避免之后打开推广页建成 SiteOwnerSince 为 0 的行再拿邀请返佣。
func markAffiliateSiteOwnerTx(tx *gorm.DB, userId int, now int64) error {
	// Match existing owners too: a no-op conditional UPDATE can release their
	// lock under MySQL READ COMMITTED and allow referral restoration mid-open.
	result := tx.Model(&model.ReferralAffiliate{}).Where("user_id = ?", userId).
		Update("site_owner_since", gorm.Expr("CASE WHEN site_owner_since = 0 THEN ? ELSE site_owner_since END", now))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var affiliates []model.ReferralAffiliate
	if err := model.AdtyLockForUpdate(tx).Where("user_id = ?", userId).Limit(1).Find(&affiliates).Error; err != nil {
		return err
	}
	if len(affiliates) > 0 {
		return tx.Model(&model.ReferralAffiliate{}).Where("id = ? AND site_owner_since = 0", affiliates[0].Id).Update("site_owner_since", now).Error
	}
	code, err := newInviteCode()
	if err != nil {
		return err
	}
	affiliate := model.ReferralAffiliate{
		UserId: userId, InviteCode: code, Status: model.ReferralAffiliateStatusApproved, SourceType: "site",
		SettlementEnabled: true, WithdrawalEnabled: true, ApprovedAt: now, SiteOwnerSince: now,
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&affiliate).Error; err != nil {
		return err
	}
	// A referral-page request can create the row between our lookup and insert.
	// Lock the actual row before touching its account, preserving Affiliate→Account.
	if err := model.AdtyLockForUpdate(tx).Where("user_id = ?", userId).First(&affiliate).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.ReferralAffiliate{}).Where("id = ? AND site_owner_since = 0", affiliate.Id).Update("site_owner_since", now).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ReferralCommissionAccount{
		AffiliateId: affiliate.Id, UserId: userId, SettlementCurrency: "USD",
	}).Error
}

// ApplyAdtySite 是站长申请开站：带有效的邀请码就直接开成 active 并用码上的预设，否则建 pending 等审核。
func ApplyAdtySite(userId int, subdomain, contactInfo, agreeVersion, inviteCode string) (*model.AdtySite, error) {
	preset := AdtySitePreset{ContactInfo: contactInfo, AgreementVer: agreeVersion, OperatorId: userId}
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if inviteCode == "" {
		return OpenAdtySite(userId, subdomain, model.AdtySiteOpenedByApply, preset)
	}
	if !resellerSetting().InviteCodeEnabled {
		return nil, ErrAdtySiteInviteOff
	}
	var codes []model.AdtySiteInviteCode
	if err := model.DB.Where("code = ?", inviteCode).Limit(1).Find(&codes).Error; err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return nil, ErrAdtySiteInviteInvalid
	}
	code := codes[0]
	preset.InviteCodeId, preset.CostFactor, preset.MaxMarkup = code.Id, code.CostFactor, code.MaxMarkup
	return OpenAdtySite(userId, subdomain, model.AdtySiteOpenedByInvite, preset)
}

// ReviewAdtySiteApplication 通过或驳回开站申请。通过时写 SiteOwnerSince，可以同时设单站 d 和 k 上限。
func ReviewAdtySiteApplication(siteId, adminId int, approve bool, costFactor, maxMarkup *decimal.Decimal, note string) (*model.AdtySite, error) {
	if approve && !resellerSetting().Ready() {
		return nil, ErrAdtySiteDisabled
	}
	if approve {
		if err := validateSitePreset(costFactor, maxMarkup); err != nil {
			return nil, err
		}
	}
	var site model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		if site.Status != model.AdtySiteStatusPending {
			return ErrAdtySiteStatus
		}
		updates := map[string]any{"review_note": truncateText(note, 1000)}
		action := "reject"
		if approve {
			action = "approve"
			updates["status"] = model.AdtySiteStatusActive
			if costFactor != nil {
				updates["cost_factor"] = *costFactor
			}
			if maxMarkup != nil {
				updates["max_markup"] = *maxMarkup
			}
			if err := markAffiliateSiteOwnerTx(tx, site.OwnerUserId, time.Now().Unix()); err != nil {
				return err
			}
			// Keep the affiliate-before-user order used by commission transfers.
			// The marker stays uncommitted until this identity lock allows activation
			// to commit, so an earlier order retains its pre-opening snapshot.
			var owner model.User
			if err := model.AdtyLockForUpdate(tx).Select("id").Where("id = ?", site.OwnerUserId).First(&owner).Error; err != nil {
				return err
			}
		} else {
			// 驳回的申请直接关闭，子域名留着存档。站长可以换个子域名重新申请。
			updates["status"] = model.AdtySiteStatusClosed
			updates["closed_at"] = time.Now().Unix()
			updates["close_reason"] = "application rejected"
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(updates).Error; err != nil {
			return err
		}
		return recordSiteAuditTx(tx, site.Id, action, adminId, map[string]any{"status": site.Status}, updates, note)
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	return GetAdtySite(siteId)
}

// GetAdtySite 按 id 取站点。
func GetAdtySite(siteId int) (*model.AdtySite, error) {
	var site model.AdtySite
	if err := model.DB.Where("id = ?", siteId).First(&site).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdtySiteNotFound
		}
		return nil, err
	}
	return &site, nil
}

// AdtySiteAdjust 是管理员对单站的调整；字段为 nil 表示不改，Clear* 改回「用全局」。
type AdtySiteAdjust struct {
	CostFactor         *decimal.Decimal
	ClearCostFactor    bool
	MaxMarkup          *decimal.Decimal
	ClearMaxMarkup     bool
	Visibility         *string
	AllowedGroups      *string
	ClearAllowedGroups bool
	Reason             string
}

// AdjustAdtySite 改单站 d、k 上限、可见范围。改 d 的通知站长由 11.9 的通知接上（AdtyOnSiteCostFactorChanged）。
func AdjustAdtySite(siteId, adminId int, adjust AdtySiteAdjust) (*model.AdtySite, error) {
	if err := validateSitePreset(adjust.CostFactor, adjust.MaxMarkup); err != nil {
		return nil, err
	}
	if adjust.Visibility != nil && strings.TrimSpace(*adjust.Visibility) != "" {
		values := map[string]string{}
		if err := common.UnmarshalJsonStr(*adjust.Visibility, &values); err != nil {
			return nil, common.NewMessage("Invalid parameters")
		}
		probe := adty_setting.ResellerSetting{Visibility: values, UnknownHostMode: adty_setting.ResellerUnknownHostMain, FingerprintWindowDays: 1}
		if err := adty_setting.ValidateResellerSetting(&probe, true); err != nil {
			return nil, err
		}
	}
	var site model.AdtySite
	var costChanged bool
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		if site.Status == model.AdtySiteStatusClosed {
			return ErrAdtySiteStatus
		}
		old := map[string]any{"cost_factor": site.CostFactor, "max_markup": site.MaxMarkup, "visibility": site.Visibility}
		updates := map[string]any{}
		var pendingNotice *decimal.Decimal
		var pendingAt int64
		switch {
		case adjust.ClearCostFactor:
			updates["cost_factor"] = nil
			updates["pending_cost_factor"] = gorm.Expr("NULL")
			updates["pending_cost_factor_at"] = 0
		case adjust.CostFactor != nil:
			current, hasCurrent := AdtySiteCostFactor(&site)
			if hasCurrent && adjust.CostFactor.GreaterThan(current) {
				delay := resellerSetting().MarkupIncreaseDelayHours
				if delay < 1 {
					delay = 24
				}
				pendingAt = time.Now().Unix() + int64(delay)*3600
				updates["pending_cost_factor"] = *adjust.CostFactor
				updates["pending_cost_factor_at"] = pendingAt
				pendingNotice = adjust.CostFactor
			} else {
				updates["cost_factor"] = *adjust.CostFactor
				updates["pending_cost_factor"] = gorm.Expr("NULL")
				updates["pending_cost_factor_at"] = 0
			}
		}
		switch {
		case adjust.ClearMaxMarkup:
			updates["max_markup"] = nil
		case adjust.MaxMarkup != nil:
			updates["max_markup"] = *adjust.MaxMarkup
		}
		if adjust.Visibility != nil {
			updates["visibility"] = strings.TrimSpace(*adjust.Visibility)
		}
		switch {
		case adjust.ClearAllowedGroups:
			updates["allowed_groups"] = ""
		case adjust.AllowedGroups != nil:
			encoded, err := ParseAdtyAllowedGroups(*adjust.AllowedGroups)
			if err != nil {
				return err
			}
			updates["allowed_groups"] = encoded
		}
		if len(updates) == 0 {
			return nil
		}
		_, costChanged = updates["cost_factor"]
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(updates).Error; err != nil {
			return err
		}
		if pendingNotice != nil {
			site.PendingCostFactor = pendingNotice
			site.PendingCostFactorAt = pendingAt
		}
		return recordSiteAuditTx(tx, site.Id, "adjust", adminId, old, updates, adjust.Reason)
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	updated, err := GetAdtySite(siteId)
	if err == nil && costChanged && AdtyOnSiteCostFactorChanged != nil {
		AdtyOnSiteCostFactorChanged(updated)
	}
	if err == nil && updated != nil && updated.PendingCostFactor != nil && updated.PendingCostFactorAt > 0 {
		adtyNotifySiteCostFactorPending(updated)
	}
	return updated, err
}

// ApplyDueAdtySiteCostFactors writes pending higher cost factors whose time has
// arrived. A later decrease clears the pending columns, so a due row is still
// an increase that was announced in advance.
func ApplyDueAdtySiteCostFactors(now int64) (int64, error) {
	var due []model.AdtySite
	if err := model.DB.Where("pending_cost_factor_at > 0 AND pending_cost_factor_at <= ?", now).Find(&due).Error; err != nil {
		return 0, err
	}
	var applied int64
	for _, site := range due {
		if site.PendingCostFactor == nil {
			continue
		}
		result := model.DB.Model(&model.AdtySite{}).
			Where("id = ? AND pending_cost_factor_at > 0 AND pending_cost_factor_at <= ?", site.Id, now).
			Updates(map[string]any{
				"cost_factor": site.PendingCostFactor, "pending_cost_factor": gorm.Expr("NULL"), "pending_cost_factor_at": 0,
			})
		if result.Error != nil {
			return applied, result.Error
		}
		applied += result.RowsAffected
	}
	if applied > 0 {
		InvalidateAdtySiteCache()
	}
	return applied, nil
}

// ApplyDueAdtySiteTierCostFactors writes a scheduled higher performance-tier d.
func ApplyDueAdtySiteTierCostFactors(now int64) (int64, error) {
	var due []model.AdtySite
	if err := model.DB.Where("pending_tier_cost_factor_at > 0 AND pending_tier_cost_factor_at <= ?", now).
		Where("(tier_mode = ? OR tier_mode IS NULL) AND (tier_code = ? OR tier_code IS NULL)", "", "").Find(&due).Error; err != nil {
		return 0, err
	}
	var applied int64
	for _, site := range due {
		if site.PendingTierCostFactor == nil {
			continue
		}
		result := model.DB.Model(&model.AdtySite{}).
			Where("id = ? AND pending_tier_cost_factor_at > 0 AND pending_tier_cost_factor_at <= ?", site.Id, now).
			Where("(tier_mode = ? OR tier_mode IS NULL) AND (tier_code = ? OR tier_code IS NULL)", "", "").
			Updates(map[string]any{
				"tier_cost_factor": site.PendingTierCostFactor, "pending_tier_cost_factor": gorm.Expr("NULL"), "pending_tier_cost_factor_at": 0,
			})
		if result.Error != nil {
			return applied, result.Error
		}
		applied += result.RowsAffected
	}
	if applied > 0 {
		InvalidateAdtySiteCache()
	}
	return applied, nil
}

// ApplyDueMinCostFactor writes a scheduled higher floor once it is due.
func ApplyDueMinCostFactor(now int64) error {
	cfg := adty_setting.GetResellerSetting()
	if cfg.PendingMinCostFactor == nil || cfg.PendingMinCostFactorAt <= 0 || cfg.PendingMinCostFactorAt > now {
		return nil
	}
	factor := *cfg.PendingMinCostFactor
	cfg.MinCostFactor = &factor
	cfg.PendingMinCostFactor = nil
	cfg.PendingMinCostFactorAt = 0
	mapped, err := config.ConfigToMap(cfg)
	if err != nil {
		return err
	}
	options := map[string]string{}
	for _, key := range []string{"min_cost_factor", "pending_min_cost_factor", "pending_min_cost_factor_at"} {
		options[adty_setting.ResellerSettingModule+"."+key] = mapped[key]
	}
	if err := model.UpdateOptionsBulk(options); err != nil {
		return err
	}
	InvalidateAdtySiteCache()
	return nil
}

// ApplyDueGlobalCostFactor writes a scheduled higher global d once it is due.
func ApplyDueGlobalCostFactor(now int64) error {
	cfg := adty_setting.GetResellerSetting()
	if cfg.PendingCostFactor == nil || cfg.PendingCostFactorAt <= 0 || cfg.PendingCostFactorAt > now {
		return nil
	}
	factor := *cfg.PendingCostFactor
	cfg.CostFactor = &factor
	cfg.PendingCostFactor = nil
	cfg.PendingCostFactorAt = 0
	mapped, err := config.ConfigToMap(cfg)
	if err != nil {
		return err
	}
	options := map[string]string{}
	for _, key := range []string{"cost_factor", "pending_cost_factor", "pending_cost_factor_at"} {
		options[adty_setting.ResellerSettingModule+"."+key] = mapped[key]
	}
	if err := model.UpdateOptionsBulk(options); err != nil {
		return err
	}
	InvalidateAdtySiteCache()
	return nil
}

// AdtyOnSiteCostFactorChanged 在管理员改了单站 d 之后调用（11.9 的站内信和邮件通知挂在这里）。
var AdtyOnSiteCostFactorChanged func(site *model.AdtySite)

// SetAdtySiteStatus 不再暂停或恢复分站。独立暂停/恢复已退役（DEC-019 / SITE-008）；
// 关站走 ScheduleAdtySiteClose。任何把站点改成或改出 paused 的请求都返回明确错误，不会静默成功。
func SetAdtySiteStatus(siteId, adminId int, status, reason string) (*model.AdtySite, error) {
	return nil, ErrAdtySitePauseRetired
}

// RequestAdtySiteClose 是站长申请关站：只记下申请时间。关闭仍由 Root 在主站后台确认，客户一律由主站继承。
func RequestAdtySiteClose(ownerUserId int, reason string) (*model.AdtySite, error) {
	var site model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.AdtyLockForUpdate(tx).Where("owner_user_id = ? AND status IN ?", ownerUserId,
			[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusPending}).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		if site.CloseRequestedAt > 0 {
			return nil
		}
		now := time.Now().Unix()
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
			"close_requested_at": now, "close_reason": truncateText(reason, 1000),
		}).Error; err != nil {
			return err
		}
		site.CloseRequestedAt = now
		return recordSiteAuditTx(tx, site.Id, "close_request", ownerUserId, nil, map[string]any{"close_requested_at": now}, reason)
	})
	if err != nil {
		return nil, err
	}
	return &site, nil
}

// AdtySiteClosePlan 是管理员定下的关站方式。
type AdtySiteClosePlan struct {
	TransferTo int  // 只能是 0（主站）；非 0 会被拒绝，不会改写后继续关站
	ClawBack   bool // 兼容字段：G.7 / SITE-010 不允许因关站作废或扣回收益，传入也会被忽略
	Immediate  bool // 不等 close_notice_days，立即执行（例如违规）
	Reason     string
}

// ScheduleAdtySiteClose 把站点改成 closing，close_transfer_to 只记 0（由主站继承），
// close_notice_days 天后由主节点执行；Immediate 时立即执行。站点 pending（还没开）时直接关闭，没有客户。
func ScheduleAdtySiteClose(siteId, adminId int, plan AdtySiteClosePlan) (*model.AdtySite, error) {
	now := time.Now().Unix()
	immediate := plan.Immediate
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		if site.Status == model.AdtySiteStatusClosed {
			return ErrAdtySiteStatus
		}
		if site.Status == model.AdtySiteStatusPending {
			immediate = true
		}
		if plan.TransferTo != 0 {
			return ErrAdtySiteTransfer
		}
		executeAt := now + int64(max(resellerSetting().CloseNoticeDays, 0))*86400
		if immediate {
			executeAt = now
		}
		updates := map[string]any{
			"status": model.AdtySiteStatusClosing, "close_execute_at": executeAt, "close_transfer_to": 0,
			"close_claw_back": false, "close_reason": truncateText(plan.Reason, 1000),
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(updates).Error; err != nil {
			return err
		}
		return recordSiteAuditTx(tx, site.Id, "close_schedule", adminId, map[string]any{"status": site.Status}, updates, plan.Reason)
	})
	if err != nil {
		return nil, err
	}
	InvalidateAdtySiteCache()
	if AdtyOnSiteClosing != nil && !immediate {
		if site, err := GetAdtySite(siteId); err == nil {
			AdtyOnSiteClosing(site)
		}
	}
	if immediate {
		if err := ExecuteAdtySiteClose(siteId, now); err != nil {
			return nil, err
		}
	}
	return GetAdtySite(siteId)
}

// AdtyOnSiteClosing 在站点进入 closing 后调用，11.3 用它给客户发关站通知邮件（按站点品牌）。
var AdtyOnSiteClosing func(site *model.AdtySite)

// AdtyOnSiteClosed 在关站事务提交后调用，11.3、11.8 用它删除发件域名和自定义域名。
var AdtyOnSiteClosed func(site *model.AdtySite)

// AdtySiteClawBackTx 曾由 11.5 注册，强制关站时作废 pending 收益并扣回已结算佣金。
// G.7 / SITE-010 要求关站后收益与佣金按原来源承接，此钩子不再被调用；即使被调用也不会改动资金。
var AdtySiteClawBackTx func(tx *gorm.DB, site *model.AdtySite, operatorId int) error

// ExecuteAdtySiteClose 执行到期的关站：客户、团队一律由主站继承，站点改 closed。可重复调用。
// 已排期但 close_transfer_to 仍指向其他站的 in-flight 关站同样只继承到主站，不会再分流。
// 关站不会作废未结收益，也不会扣回已结算佣金；这些金额继续按原来源持有，后续结算、转余额、提现或退款走原规则。
func ExecuteAdtySiteClose(siteId int, now int64) error {
	var moved []int
	var closed model.AdtySite
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			return err
		}
		if site.Status != model.AdtySiteStatusClosing || site.CloseExecuteAt > now {
			return nil
		}
		const target = 0
		var err error
		moved, err = model.AdtySetUsersSiteTx(tx, site.Id, target)
		if err != nil {
			return err
		}
		if err := InheritAdtyVouchersOnCloseTx(tx, site.Id, moved); err != nil {
			return err
		}
		if err := InheritAdtyTeamsOnCloseTx(tx, site.Id); err != nil {
			return err
		}
		// 主站没有分站客户行；继承后删掉，避免残留归属。
		if len(moved) > 0 {
			if err := tx.Where("user_id IN ?", moved).Delete(&model.AdtySiteCustomer{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Updates(map[string]any{
			"status": model.AdtySiteStatusClosed, "closed_at": now, "close_transfer_to": target,
		}).Error; err != nil {
			return err
		}
		site.Status, site.ClosedAt, site.CloseTransferTo = model.AdtySiteStatusClosed, now, target
		closed = site
		return recordSiteAuditTx(tx, site.Id, "close", 0, nil, map[string]any{"transfer_to": target, "customers": len(moved), "claw_back": false}, "")
	})
	if err != nil {
		return err
	}
	model.AdtyInvalidateUserCaches(moved)
	InvalidateAdtySiteCache()
	if closed.Id > 0 && AdtyOnSiteClosed != nil {
		AdtyOnSiteClosed(&closed)
	}
	return nil
}

// carryCustomerPricesTx：转给另一个站时，新站对这批客户的 k 不能高于原来的。
// 对每个客户、每个出现过的分组（含「所有分组」）取 min(原站 k, 新站 k)；只要有一项比新站低，
// 就把这个客户所有分组都写成单独定价。客户级的「所有分组」一行会盖过新站的分组价，所以不能只写低的那几项。
// 两边都没单设的分组走客户的「所有分组」那一行，它等于两边全站默认的较小值，同样不高于原价。
func carryCustomerPricesTx(tx *gorm.DB, fromSite, toSite int) error {
	var userIds []int
	if err := tx.Model(&model.User{}).Where("site_id = ?", fromSite).Pluck("id", &userIds).Error; err != nil {
		return err
	}
	if len(userIds) == 0 {
		return nil
	}
	var fromPrices, toPrices []model.AdtySitePrice
	if err := tx.Where("site_id = ?", fromSite).Find(&fromPrices).Error; err != nil {
		return err
	}
	if err := tx.Where("site_id = ?", toSite).Find(&toPrices).Error; err != nil {
		return err
	}
	groups := map[string]bool{"": true}
	for _, price := range fromPrices {
		groups[price.Group] = true
	}
	for _, price := range toPrices {
		groups[price.Group] = true
	}
	now := time.Now().Unix()
	for _, userId := range userIds {
		targets := map[string]decimal.Decimal{}
		lower := false
		for group := range groups {
			oldK := ResolveAdtySiteMarkup(fromPrices, userId, group)
			newK := ResolveAdtySiteMarkup(toPrices, userId, group)
			targets[group] = decimal.Min(oldK, newK)
			if oldK.LessThan(newK) {
				lower = true
			}
		}
		if !lower {
			continue
		}
		for group, markup := range targets {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "site_id"}, {Name: "user_id"}, {Name: "group"}},
				DoUpdates: clause.Assignments(map[string]any{"markup": markup, "pending_markup": decimal.Zero, "pending_at": 0, "updated_at": now}),
			}).Create(&model.AdtySitePrice{SiteId: toSite, UserId: userId, Group: group, Markup: markup, UpdatedAt: now, Version: 1}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// ResolveAdtySiteMarkup 在一个站的定价里按「客户 + 分组 → 客户 → 全站 + 分组 → 全站 → 1」取存储 k（11.4）。正数 k（含小于 1）都会采用。
func ResolveAdtySiteMarkup(prices []model.AdtySitePrice, userId int, group string) decimal.Decimal {
	best, bestRank := decimal.NewFromInt(1), 0
	for _, price := range prices {
		rank := 0
		switch {
		case price.UserId == userId && userId != 0 && price.Group == group && group != "":
			rank = 4
		case price.UserId == userId && userId != 0 && price.Group == "":
			rank = 3
		case price.UserId == 0 && price.Group == group && group != "":
			rank = 2
		case price.UserId == 0 && price.Group == "":
			rank = 1
		}
		if rank > bestRank && price.Markup.IsPositive() {
			best, bestRank = price.Markup, rank
		}
	}
	return best
}

// RunAdtySiteCloseOnce 执行所有到期的关站，由主节点任务调用。
func RunAdtySiteCloseOnce(now int64) error {
	var ids []int
	if err := model.DB.Model(&model.AdtySite{}).Where("status = ? AND close_execute_at <= ?", model.AdtySiteStatusClosing, now).Pluck("id", &ids).Error; err != nil {
		return err
	}
	var first error
	for _, id := range ids {
		if err := ExecuteAdtySiteClose(id, now); err != nil {
			common.SysError(common.LogText("execute site close failed site=%d: %s", id, err.Error()))
			if first == nil {
				first = err
			}
		}
	}
	return first
}

const adtySiteInviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewAdtySiteInviteCode 生成一个 12 位的开站邀请码（去掉容易看错的 0、O、1、I）。
func NewAdtySiteInviteCode() (string, error) {
	var b strings.Builder
	for range 12 {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(adtySiteInviteAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(adtySiteInviteAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// SaveAdtySiteInviteCode 新建（Id 为 0）或修改一个开站邀请码。新建时 Code 为空就自动生成。
func SaveAdtySiteInviteCode(code *model.AdtySiteInviteCode) error {
	if code.MaxUses < 0 || code.ExpireAt < 0 {
		return common.NewMessage("Invalid parameters")
	}
	if err := validateSitePreset(code.CostFactor, code.MaxMarkup); err != nil {
		return err
	}
	code.Code = strings.ToUpper(strings.TrimSpace(code.Code))
	code.Note = truncateText(code.Note, 500)
	if code.Id == 0 {
		if code.Code == "" {
			generated, err := NewAdtySiteInviteCode()
			if err != nil {
				return err
			}
			code.Code = generated
		}
		if len(code.Code) > 32 || !adty_setting.ResellerLabelPattern.MatchString(strings.ToLower(code.Code)) {
			return common.NewMessage("Invalid parameters")
		}
		var exists int64
		if err := model.DB.Model(&model.AdtySiteInviteCode{}).Where("code = ?", code.Code).Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			return common.NewMessage("This invite code already exists")
		}
		return model.DB.Create(code).Error
	}
	return model.DB.Model(&model.AdtySiteInviteCode{}).Where("id = ?", code.Id).Updates(map[string]any{
		"max_uses": code.MaxUses, "expire_at": code.ExpireAt, "cost_factor": code.CostFactor, "max_markup": code.MaxMarkup, "note": code.Note,
	}).Error
}

// AdtySiteDisplayHost 是站点的平台子域名主机名。
func AdtySiteDisplayHost(site *model.AdtySite) string {
	if site == nil {
		return ""
	}
	return fmt.Sprintf("%s.%s", site.Subdomain, site.RootDomain)
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, startAdtySiteCloseTask)
}

// startAdtySiteCloseTask 在主节点每 10 分钟执行一次到期的关站。
func startAdtySiteCloseTask() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := RunAdtySiteCloseOnce(time.Now().Unix()); err != nil {
				common.SysError(common.LogText("site close task: %s", err.Error()))
			}
		}
	}()
}

// ClearAdtySiteNotice 是管理员一键下线分站公告（分站·调整，任务 11.3、11.9）：清空 Brand.notice，其余品牌字段不动，写审计。
// 返回下线前的公告（审计里也有）。没有公告时什么都不改。
func ClearAdtySiteNotice(siteId, adminId int, reason string) (string, error) {
	var old string
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).Where("id = ?", siteId).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdtySiteNotFound
			}
			return err
		}
		brand := map[string]any{}
		if strings.TrimSpace(site.Brand) != "" {
			if err := common.UnmarshalJsonStr(site.Brand, &brand); err != nil {
				return err
			}
		}
		old, _ = brand["notice"].(string)
		if old == "" {
			return nil
		}
		brand["notice"] = ""
		encoded, err := common.Marshal(brand)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("brand", string(encoded)).Error; err != nil {
			return err
		}
		return recordSiteAuditTx(tx, site.Id, "notice_takedown", adminId, map[string]any{"notice": old}, map[string]any{"notice": ""}, reason)
	})
	if err != nil {
		return "", err
	}
	InvalidateAdtySiteCache()
	return old, nil
}
