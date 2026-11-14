package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 站长定价接口（任务 11.4）。站长中心的其余接口在 11.6；要求重新验证身份的二次确认也在 11.6 接上。

// adtyOwnedSite 取当前用户是站长的、没关的站；不是站长返回 403。
func adtyOwnedSite(c *gin.Context) (*model.AdtySite, bool) {
	var sites []model.AdtySite
	if err := model.DB.Where("owner_user_id = ? AND status IN ?", c.GetInt("id"),
		[]string{model.AdtySiteStatusActive, model.AdtySiteStatusPaused, model.AdtySiteStatusClosing}).Limit(1).Find(&sites).Error; err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if len(sites) == 0 {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("You do not have an open site"))
		return nil, false
	}
	return &sites[0], true
}

type adtySitePricingBody struct {
	DefaultMarkup   *string           `json:"default_markup"`
	GroupMarkups    map[string]string `json:"group_markups"` // 值为 "0" 表示删掉这个分组的单独价
	ModelMarkups    map[string]string `json:"model_markups"` // 值为 "0" 表示删掉这个模型的独立加价
	ExpectedVersion int64             `json:"expected_version"`
}

func saveAdtySitePricing(c *gin.Context, site *model.AdtySite, userId int) {
	var body adtySitePricingBody
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	input := service.AdtySitePricingInput{SiteId: site.Id, UserId: userId, ExpectedVersion: body.ExpectedVersion, GroupMarkups: map[string]decimal.Decimal{}, ModelMarkups: map[string]decimal.Decimal{}}
	if body.DefaultMarkup != nil && strings.TrimSpace(*body.DefaultMarkup) != "" {
		value, err := decimal.NewFromString(strings.TrimSpace(*body.DefaultMarkup))
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		input.DefaultMarkup = &value
	}
	for group, raw := range body.GroupMarkups {
		value, err := decimal.NewFromString(strings.TrimSpace(raw))
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		input.GroupMarkups[group] = value
	}
	for modelName, raw := range body.ModelMarkups {
		value, err := decimal.NewFromString(strings.TrimSpace(raw))
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		input.ModelMarkups[modelName] = value
	}
	result, err := service.SaveAdtySitePricing(input)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// PutAdtySitePricing 保存全站定价：PUT /api/adty/site/pricing {default_markup, group_markups, model_markups, expected_version}。
// 只接受加价配置；供货倍率和主站标准价格不能从这条路径改（SPRC-014）。
func PutAdtySitePricing(c *gin.Context) {
	site, ok := adtyOwnedSite(c)
	if !ok {
		return
	}
	saveAdtySitePricing(c, site, 0)
}

// PutAdtySiteCustomerPricing 保存一个客户的单独定价：PUT /api/adty/site/customers/:id/pricing。
// 别的站的客户、主站用户一律当作不存在。
func PutAdtySiteCustomerPricing(c *gin.Context) {
	site, ok := adtyOwnedSite(c)
	if !ok {
		return
	}
	customerId, err := strconv.Atoi(c.Param("id"))
	if err != nil || customerId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var count int64
	if err := model.DB.Model(&model.User{}).Where("id = ? AND site_id = ?", customerId, site.Id).Count(&count).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if count == 0 {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Customer not found"))
		return
	}
	saveAdtySitePricing(c, site, customerId)
}
