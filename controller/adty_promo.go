package controller

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 后台优惠码（任务 12.4）：管理员建平台出资的码。站长出资的码由站长在站长中心建（本文件后半）。

var adtyPromoCodePattern = regexp.MustCompile(`^[A-Z0-9_-]{8,32}$`)

type adtyPromoCodeInput struct {
	Code         string          `json:"code"`
	Kind         string          `json:"kind"`
	Value        decimal.Decimal `json:"value"`
	MinFaceUSD   decimal.Decimal `json:"min_face_usd"`
	MaxUses      int             `json:"max_uses"`
	PerUserLimit int             `json:"per_user_limit"`
	StartsAt     int64           `json:"starts_at"`
	ExpiresAt    int64           `json:"expires_at"`
	NewUsersOnly bool            `json:"new_users_only"`
	SiteId       int             `json:"site_id"`
	Enabled      bool            `json:"enabled"`
	Note         string          `json:"note"`
}

// apply 校验并写进 code。百分比 0 < 值 ≤ 100；固定金额 > 0；次数不能为负；有效期结束要晚于开始。
func (in *adtyPromoCodeInput) apply(code *model.AdtyPromoCode) bool {
	in.Code = model.NormalizeAdtyPromoCode(in.Code)
	switch {
	case !adtyPromoCodePattern.MatchString(in.Code),
		in.Kind != model.AdtyPromoKindPercent && in.Kind != model.AdtyPromoKindFixed,
		!in.Value.IsPositive(),
		in.Kind == model.AdtyPromoKindPercent && in.Value.GreaterThan(decimal.NewFromInt(100)),
		in.Kind == model.AdtyPromoKindFixed && in.Value.GreaterThan(decimal.NewFromInt(100000)),
		in.MinFaceUSD.IsNegative(), in.MaxUses < 0, in.PerUserLimit < 0, in.SiteId < 0,
		in.StartsAt < 0, in.ExpiresAt < 0, in.ExpiresAt > 0 && in.ExpiresAt <= in.StartsAt,
		len([]rune(in.Note)) > 255:
		return false
	}
	code.Code, code.Kind, code.Value, code.MinFaceUSD = in.Code, in.Kind, in.Value, in.MinFaceUSD
	code.MaxUses, code.PerUserLimit, code.StartsAt, code.ExpiresAt = in.MaxUses, in.PerUserLimit, in.StartsAt, in.ExpiresAt
	code.NewUsersOnly, code.SiteId, code.Enabled, code.Note = in.NewUsersOnly, in.SiteId, in.Enabled, strings.TrimSpace(in.Note)
	code.Funder = model.AdtyPromoFunderPlatform
	return true
}

// GetAdtyPromoCodes 列出优惠码（支付·查看）。
func GetAdtyPromoCodes(c *gin.Context) {
	query := model.DB.Model(&model.AdtyPromoCode{})
	if keyword := model.NormalizeAdtyPromoCode(c.Query("keyword")); keyword != "" {
		query = query.Where("code LIKE ?", "%"+keyword+"%")
	}
	var items []model.AdtyPromoCode
	referralPage(c, query, &items)
}

// PostAdtyPromoCode 新建平台出资的优惠码（Root）。
func PostAdtyPromoCode(c *gin.Context) {
	var in adtyPromoCodeInput
	code := model.AdtyPromoCode{CreatedBy: c.GetInt("id")}
	if err := common.DecodeJson(c.Request.Body, &in); err != nil || !in.apply(&code) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	existing, err := model.GetAdtyPromoCodeByCode(code.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing != nil {
		common.ApiErrorT(c, "This promo code already exists")
		return
	}
	if err := model.DB.Create(&code).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.promo_create", map[string]any{"id": code.Id, "code": code.Code})
	common.ApiSuccess(c, code)
}

// PutAdtyPromoCode 改优惠码（Root）。码本身和已用次数不改。
func PutAdtyPromoCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var code model.AdtyPromoCode
	if err := model.DB.First(&code, id).Error; err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var in adtyPromoCodeInput
	original := code.Code
	if err := common.DecodeJson(c.Request.Body, &in); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	in.Code = original
	if !in.apply(&code) {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.DB.Model(&model.AdtyPromoCode{}).Where("id = ?", code.Id).
		Select("kind", "value", "min_face_usd", "max_uses", "per_user_limit", "starts_at", "expires_at", "new_users_only", "site_id", "enabled", "note").
		Updates(&code).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.marketing.promo_update", map[string]any{"id": code.Id, "code": code.Code, "enabled": code.Enabled})
	common.ApiSuccess(c, code)
}

// 站长中心的优惠码（12.4 站长出资）。码固定为本站、出资方是站长，优惠加赠从站长收益里扣。
// 前端入口等站长中心（11.6）做好后再加。

// adtyPromoOwnerSite 取调用者是站长的 active 或 paused 站点，不是返回 403。
func adtyPromoOwnerSite(c *gin.Context) (*model.AdtySite, bool) {
	var sites []model.AdtySite
	if err := model.DB.Where("owner_user_id = ? AND status IN ?", c.GetInt("id"),
		[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused}).Limit(1).Find(&sites).Error; err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if len(sites) == 0 {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("You do not have an open site"))
		return nil, false
	}
	return &sites[0], true
}

// adtySitePromoByParam 取本站的一个码；别站的码当作不存在。
func adtySitePromoByParam(c *gin.Context, site *model.AdtySite) (*model.AdtyPromoCode, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, false
	}
	var codes []model.AdtyPromoCode
	if err := model.DB.Where("id = ? AND site_id = ? AND funder = ?", id, site.Id, model.AdtyPromoFunderSiteOwner).Limit(1).Find(&codes).Error; err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if len(codes) == 0 {
		common.ApiErrorT(c, "Promo code not found")
		return nil, false
	}
	return &codes[0], true
}

// applySiteOwner 把站长提交的码固定成本站、站长出资，并检查面值不超过站点能给的上限。
func (in *adtyPromoCodeInput) applySiteOwner(c *gin.Context, code *model.AdtyPromoCode, site *model.AdtySite) bool {
	if !in.apply(code) {
		common.ApiErrorT(c, "Invalid parameters")
		return false
	}
	code.SiteId, code.Funder = site.Id, model.AdtyPromoFunderSiteOwner
	if err := service.ValidateAdtySitePromoValue(code); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

// GetAdtySitePromoCodes 列出本站的优惠码（站长）。
func GetAdtySitePromoCodes(c *gin.Context) {
	site, ok := adtyPromoOwnerSite(c)
	if !ok {
		return
	}
	var items []model.AdtyPromoCode
	referralPage(c, model.DB.Model(&model.AdtyPromoCode{}).Where("site_id = ? AND funder = ?", site.Id, model.AdtyPromoFunderSiteOwner), &items)
}

// PostAdtySitePromoCode 站长新建本站的优惠码。
func PostAdtySitePromoCode(c *gin.Context) {
	site, ok := adtyPromoOwnerSite(c)
	if !ok {
		return
	}
	var in adtyPromoCodeInput
	code := model.AdtyPromoCode{CreatedBy: c.GetInt("id")}
	if err := common.DecodeJson(c.Request.Body, &in); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if !in.applySiteOwner(c, &code, site) {
		return
	}
	// 二开：二次验证放在校验参数之后、写库之前（任务 11.6）；新建时 target 为 0。
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionPromo, 0) {
		return
	}
	existing, err := model.GetAdtyPromoCodeByCode(code.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if existing != nil {
		common.ApiErrorT(c, "This promo code already exists")
		return
	}
	if err := model.DB.Create(&code).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.promo_create", map[string]any{"id": code.Id, "site_id": site.Id, "code": code.Code})
	common.ApiSuccess(c, code)
}

// PutAdtySitePromoCode 站长改本站的码；码本身、站点、出资方不改。
func PutAdtySitePromoCode(c *gin.Context) {
	site, ok := adtyPromoOwnerSite(c)
	if !ok {
		return
	}
	code, ok := adtySitePromoByParam(c, site)
	if !ok {
		return
	}
	var in adtyPromoCodeInput
	if err := common.DecodeJson(c.Request.Body, &in); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	in.Code = code.Code
	if !in.applySiteOwner(c, code, site) {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionPromo, code.Id) {
		return
	}
	if err := model.DB.Model(&model.AdtyPromoCode{}).Where("id = ? AND site_id = ?", code.Id, site.Id).
		Select("kind", "value", "min_face_usd", "max_uses", "per_user_limit", "starts_at", "expires_at", "new_users_only", "enabled", "note").
		Updates(code).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.promo_update", map[string]any{"id": code.Id, "site_id": site.Id, "enabled": code.Enabled})
	common.ApiSuccess(c, code)
}

// DeleteAdtySitePromoCode 站长删本站的码。未付订单仍可入账本金和档位加赠，核销时移除已删除码的优惠加赠。
func DeleteAdtySitePromoCode(c *gin.Context) {
	site, ok := adtyPromoOwnerSite(c)
	if !ok {
		return
	}
	code, ok := adtySitePromoByParam(c, site)
	if !ok {
		return
	}
	if !adtyRequireOwnerProof(c, service.AdtySiteOwnerActionPromo, code.Id) {
		return
	}
	if err := model.DB.Where("id = ? AND site_id = ?", code.Id, site.Id).Delete(&model.AdtyPromoCode{}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.site.promo_delete", map[string]any{"id": code.Id, "site_id": site.Id})
	common.ApiSuccess(c, nil)
}
