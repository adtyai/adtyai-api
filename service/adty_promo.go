package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// 优惠码（任务 12.4）的下单校验。平台出资的码谁出钱都不记站长收益；站长出资的码只给本站客户用，
// 优惠加赠和站点加赠一样从站长收益里扣（service/adty_site_earning.go 的 adtyBonusUSDForSite）。

var (
	ErrAdtyPromoInvalid      = common.NewMessage("This promo code is not valid")
	ErrAdtyPromoUsed         = common.NewMessage("You have already used this promo code")
	ErrAdtyPromoNewUsersOnly = common.NewMessage("This promo code is only for your first top-up")
	ErrAdtyPromoDisabled     = common.NewMessage("Promo codes are turned off")
	ErrAdtyPromoRetired      = common.NewMessage("Promo codes are not available")
	// ErrAdtySitePromoOverCap：站长出资的码加上站点该档加赠，超过「主站同档 + max_site_bonus_extra」。
	ErrAdtySitePromoOverCap = common.NewMessage("This promo code adds more bonus than this site may give")
)

// AdtyPromoMinFaceKey 是面额不够时的提示（带 amount 参数）。
const AdtyPromoMinFaceKey = "This promo code needs a top-up of at least ${{amount}}"

// AdtyPromoInput 是报价时带的优惠码和用户。Code 为空表示不用码。
type AdtyPromoInput struct {
	UserId int
	SiteId int
	Code   string
}

// AdtyUserSiteID 取用户所属站点（0 是主站），出错时当主站。
func AdtyUserSiteID(userId int) int {
	var siteID int
	if err := model.DB.Model(&model.User{}).Select("site_id").Where("id = ?", userId).Scan(&siteID).Error; err != nil {
		return 0
	}
	return siteID
}

// adtyPromoBonusFor refuses every new promo code. Historical redemption rows stay.
func adtyPromoBonusFor(in AdtyPromoInput, _ decimal.Decimal) (*model.AdtyPromoCode, decimal.Decimal, error) {
	if strings.TrimSpace(in.Code) == "" {
		return nil, decimal.Zero, nil
	}
	return nil, decimal.Zero, ErrAdtyPromoRetired
}
