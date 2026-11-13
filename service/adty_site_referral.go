package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const adtySiteReferralCapKey = "adty_site_referral.max_rate"

var ErrAdtySiteReferralUnset = errors.New("site referral cap is not configured")

// AdtySiteReferralView is what the owner page may show. Unset values stay blank.
type AdtySiteReferralView struct {
	CapConfigured  bool   `json:"cap_configured"`
	MaxRate        string `json:"max_rate,omitempty"`
	RateConfigured bool   `json:"rate_configured"`
	Rate           string `json:"rate,omitempty"`
}

// SaveAdtySiteReferralCap stores Root's maximum one-level site invite rate.
// The value is a fraction. Empty, zero, or a value above 1 is refused. No default is written.
func SaveAdtySiteReferralCap(raw string) error {
	trimmed := strings.TrimSpace(raw)
	value, err := decimal.NewFromString(trimmed)
	if err != nil || !value.IsPositive() || value.GreaterThan(decimal.NewFromInt(1)) {
		return common.NewMessage("Invalid parameters")
	}
	return model.UpdateOptionsBulk(map[string]string{adtySiteReferralCapKey: trimmed})
}

// LoadAdtySiteReferralCap returns Root's saved cap. An unset cap stays blank.
func LoadAdtySiteReferralCap() (AdtySiteReferralView, error) {
	view := AdtySiteReferralView{}
	raw, _, err := loadAdtySiteReferralCap()
	if errors.Is(err, ErrAdtySiteReferralUnset) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	view.CapConfigured = true
	view.MaxRate = raw
	return view, nil
}

func loadAdtySiteReferralCap() (string, decimal.Decimal, error) {
	raw, ok, err := model.ReadOptionFromDB(adtySiteReferralCapKey)
	if err != nil {
		return "", decimal.Zero, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return "", decimal.Zero, ErrAdtySiteReferralUnset
	}
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() || value.GreaterThan(decimal.NewFromInt(1)) {
		return "", decimal.Zero, ErrAdtySiteReferralUnset
	}
	return strings.TrimSpace(raw), value, nil
}

func lockOwnerSite(siteID, ownerID int) (*model.AdtySite, error) {
	var site model.AdtySite
	if err := model.DB.First(&site, siteID).Error; err != nil {
		return nil, err
	}
	if site.OwnerUserId != ownerID {
		return nil, common.NewMessage("No permission to perform this action")
	}
	if site.Status != model.AdtySiteStatusActive && site.Status != model.AdtySiteStatusPaused {
		return nil, common.NewMessage("Site is not open for registration")
	}
	return &site, nil
}

// SaveAdtyOwnerSiteReferralRate stores the site rate only when it is within Root's cap.
func SaveAdtyOwnerSiteReferralRate(siteID, ownerID int, raw string) error {
	if _, err := lockOwnerSite(siteID, ownerID); err != nil {
		return err
	}
	_, cap, err := loadAdtySiteReferralCap()
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(raw)
	value, err := decimal.NewFromString(trimmed)
	if err != nil || value.IsNegative() || value.GreaterThan(cap) {
		return common.NewMessage("Invalid parameters")
	}
	return SetSiteReferralRate(siteID, value)
}

// LoadAdtyOwnerSiteReferral returns the saved cap and site rate. It does not invent either.
func LoadAdtyOwnerSiteReferral(siteID, ownerID int) (AdtySiteReferralView, error) {
	if _, err := lockOwnerSite(siteID, ownerID); err != nil {
		return AdtySiteReferralView{}, err
	}
	view := AdtySiteReferralView{}
	if raw, _, err := loadAdtySiteReferralCap(); err == nil {
		view.CapConfigured = true
		view.MaxRate = raw
	} else if !errors.Is(err, ErrAdtySiteReferralUnset) {
		return view, err
	}
	var row model.AdtySiteReferralRate
	err := model.DB.First(&row, siteID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	view.RateConfigured = true
	view.Rate = row.Rate.String()
	return view, nil
}

// BindAdtyOwnerSiteInvite records one direct invite on the owner's site.
// It does not pay commission. A customer who already has a successful top-up cannot be bound.
func BindAdtyOwnerSiteInvite(siteID, ownerID, inviterID, inviteeID int) error {
	if _, err := lockOwnerSite(siteID, ownerID); err != nil {
		return err
	}
	if inviterID == inviteeID {
		return common.NewMessage("Invalid parameters")
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
	return BindSiteInvite(siteID, inviterID, inviteeID)
}

// SaveAdtyOwnerSiteTeamDiscount updates one site team's discount.
// A price below the site supply factor is refused. Shared quota is not changed.
func SaveAdtyOwnerSiteTeamDiscount(siteID, ownerID int, teamID int64, raw string) error {
	site, err := lockOwnerSite(siteID, ownerID)
	if err != nil {
		return err
	}
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if teamID <= 0 || err != nil || !value.IsPositive() || value.GreaterThan(decimal.NewFromInt(1)) {
		return common.NewMessage("Invalid parameters")
	}
	if site.CostFactor != nil && value.LessThan(*site.CostFactor) {
		return common.NewMessage("Team price is below the site supply price")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamID).Error; err != nil {
			return err
		}
		if team.SiteId != siteID || team.Status != model.AdtyTeamStatusActive {
			return common.NewMessage("No permission to perform this action")
		}
		return tx.Model(&team).Update("discount", value).Error
	})
}

// ListAdtyOwnerSiteTeams returns active teams on the owner's site.
func ListAdtyOwnerSiteTeams(siteID, ownerID int) ([]model.AdtyTeam, error) {
	if _, err := lockOwnerSite(siteID, ownerID); err != nil {
		return nil, err
	}
	var rows []model.AdtyTeam
	err := model.DB.Where("site_id = ? AND status = ?", siteID, model.AdtyTeamStatusActive).Order("id desc").Limit(50).Find(&rows).Error
	return rows, err
}

// ListAdtySiteInvites returns the owner's direct site invites. It does not create any.
func ListAdtySiteInvites(siteID, ownerID int) ([]model.AdtySiteInvite, error) {
	if _, err := lockOwnerSite(siteID, ownerID); err != nil {
		return nil, err
	}
	var rows []model.AdtySiteInvite
	err := model.DB.Where("site_id = ?", siteID).Order("id desc").Limit(50).Find(&rows).Error
	return rows, err
}
