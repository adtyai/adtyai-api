package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtySiteReferralCap(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	view, err := service.LoadAdtySiteReferralCap()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtySiteReferralCap(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		MaxRate string `json:"max_rate"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteReferralCap(body.MaxRate); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func GetAdtySiteInviteBatchPolicy(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	view, err := service.LoadAdtySiteInviteBatchPolicy()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtySiteInviteBatchPolicy(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		Threshold   string `json:"threshold"`
		WindowHours string `json:"window_hours"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteInviteBatchPolicy(body.Threshold, body.WindowHours); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func GetAdtySiteCommissionPolicy(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	view, err := service.LoadAdtySiteCommissionPolicy()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtySiteCommissionPolicy(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		PendingCoverageCap string `json:"pending_coverage_cap"`
		ObservationDays    string `json:"observation_days"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteCommissionPolicy(body.PendingCoverageCap, body.ObservationDays); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func GetAdtySiteCommissionMargin(c *gin.Context) {
	siteID, ok := adtyCommissionMarginSite(c)
	if !ok {
		return
	}
	allowed, err := service.SiteCommissionMarginAllowed(siteID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteID, "allowed": allowed})
}

func PutAdtySiteCommissionMargin(c *gin.Context) {
	siteID, ok := adtyCommissionMarginSite(c)
	if !ok {
		return
	}
	var body struct {
		Allowed *bool `json:"allowed"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Allowed == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.AuthorizeAdtySiteCommissionMargin(siteID, *body.Allowed); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"site_id": siteID, "allowed": *body.Allowed})
}

func adtyCommissionMarginSite(c *gin.Context) (int, bool) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return 0, false
	}
	siteID, err := strconv.Atoi(c.Param("id"))
	if err != nil || siteID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	var count int64
	if err := model.DB.Model(&model.AdtySite{}).Where("id = ?", siteID).Count(&count).Error; err != nil {
		common.ApiError(c, err)
		return 0, false
	}
	if count != 1 {
		common.ApiErrorT(c, "Invalid parameters")
		return 0, false
	}
	return siteID, true
}

func GetAdtyOwnerSiteReferral(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	view, err := service.LoadAdtyOwnerSiteReferral(site.Id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PutAdtyOwnerSiteReferral(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		Rate string `json:"rate"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtyOwnerSiteReferralRate(site.Id, c.GetInt("id"), body.Rate); err != nil {
		if errors.Is(err, service.ErrAdtySiteReferralUnset) {
			common.ApiErrorT(c, "The referral cap is not configured")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func GetAdtySiteCommissions(c *gin.Context) {
	rows, err := service.ListAdtySiteInviteCommissions(c.GetInt("id"), time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtySiteCommissionConvert(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		AmountUSD string `json:"amount_usd"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(body.AmountUSD))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ConvertAdtySiteInviteCommission(c.GetInt("id"), id, amount, strings.TrimSpace(c.GetHeader("Idempotency-Key"))); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"converted": true})
}

func GetAdtySiteInviteWithdrawRules(c *gin.Context) {
	view, err := service.LoadAdtySiteInviteWithdrawRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func PostAdtySiteInviteWithdrawal(c *gin.Context) {
	var body struct {
		AmountUSD       string `json:"amount_usd"`
		Method          string `json:"method"`
		Network         string `json:"network"`
		AccountNo       string `json:"account_no"`
		AccountName     string `json:"account_name"`
		QRImageURL      string `json:"qr_image_url"`
		ExpectedCNYRate string `json:"expected_cny_rate"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(body.AmountUSD))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	expected := decimal.Zero
	if strings.TrimSpace(body.ExpectedCNYRate) != "" {
		expected, err = decimal.NewFromString(strings.TrimSpace(body.ExpectedCNYRate))
		if err != nil {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
	}
	var user model.User
	if err := model.DB.Select("site_id").First(&user, c.GetInt("id")).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	row, err := service.CreateAdtySiteInviteWithdrawal(service.AdtySiteInviteWithdrawalInput{
		UserID: c.GetInt("id"), SiteID: user.SiteId, Amount: amount, Method: body.Method,
		Network: body.Network, AccountNo: body.AccountNo, AccountName: body.AccountName,
		QRImageURL: body.QRImageURL, ExpectedCNYRate: expected,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	})
	if err != nil {
		var changed *service.RateChangedError
		if errors.As(err, &changed) {
			common.ApiErrorStatus(c, http.StatusOK, common.NewMessage("The exchange rate has changed. Confirm the new amount"), gin.H{"code": "rate_changed", "data": changed.Change})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func GetAdtySiteInviteWithdrawals(c *gin.Context) {
	rows, err := service.ListAdtySiteInviteWithdrawals(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": presentAdtySiteInviteWithdrawals(rows, nil)})
}

func PostAdtySiteInviteWithdrawalCancel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.CancelAdtySiteInviteWithdrawal(c.GetInt("id"), id, time.Now().Unix()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"canceled": true})
}

func GetAdtyOwnerSiteInviteWithdrawals(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	rows, err := service.ListAdtyOwnerSiteInviteWithdrawals(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ids := make([]int, 0, len(rows))
	seen := map[int]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.UserId]; ok {
			continue
		}
		seen[row.UserId] = struct{}{}
		ids = append(ids, row.UserId)
	}
	names, err := adtyUsernamesByID(ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": presentAdtySiteInviteWithdrawals(rows, names)})
}

func PostAdtyOwnerSiteInviteWithdrawalReview(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.ReviewAdtySiteInviteWithdrawal(c.GetInt("id"), id, body.Approve, body.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"reviewed": true})
}

func PostAdtyOwnerSiteInviteWithdrawalPaid(c *gin.Context) {
	if _, ok := adtyOwnerSiteOr403(c); !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		TxID  string `json:"tx_id"`
		Proof string `json:"proof"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.MarkAdtySiteInviteWithdrawalPaid(c.GetInt("id"), id, body.TxID, body.Proof); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"paid": true})
}

func GetAdtyAdminSiteInviteWithdrawRules(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	GetAdtySiteInviteWithdrawRules(c)
}

func PutAdtySiteInviteWithdrawRules(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body struct {
		MinUSD  string `json:"min_usd"`
		FeeUSD  string `json:"fee_usd"`
		CNYRate string `json:"cny_rate"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteInviteWithdrawRules(body.MinUSD, body.FeeUSD, body.CNYRate); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

type adtySiteInviteWithdrawalItem struct {
	model.AdtySiteInviteWithdrawal
	Username string `json:"username,omitempty"`
}

func presentAdtySiteInviteWithdrawals(rows []model.AdtySiteInviteWithdrawal, names map[int]string) []adtySiteInviteWithdrawalItem {
	now := time.Now()
	out := make([]adtySiteInviteWithdrawalItem, len(rows))
	for i, row := range rows {
		row.QRImageURL = service.ResignReferralAssetURL(row.QRImageURL, now)
		row.ProofNote = service.ResignReferralAssetURL(row.ProofNote, now)
		item := adtySiteInviteWithdrawalItem{AdtySiteInviteWithdrawal: row}
		if names != nil {
			item.Username = names[row.UserId]
		}
		out[i] = item
	}
	return out
}

func adtyUsernamesByID(ids []int) (map[int]string, error) {
	names := map[int]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var users []model.User
	if err := model.DB.Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		names[user.Id] = user.Username
	}
	return names, nil
}

func PutAdtyOwnerSiteReferralEnabled(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.Enabled == nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SetAdtyOwnerSiteReferralEnabled(site.Id, c.GetInt("id"), *body.Enabled); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"enabled": *body.Enabled})
}

func PostAdtyOwnerSiteCoverageResume(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	if err := service.ResumeAdtySiteInviteCoverage(site.Id, time.Now()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"resumed": true})
}

func GetAdtyOwnerSiteTeams(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	rows, err := service.ListAdtyOwnerSiteTeams(site.Id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PutAdtyOwnerSiteTeamDiscount(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtyOwnerSiteTeamDiscount(site.Id, c.GetInt("id"), id, body.Discount); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func GetAdtyOwnerSiteInvites(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	rows, err := service.ListAdtySiteInvites(site.Id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyOwnerSiteInvite(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		InviterID int `json:"inviter_id"`
		InviteeID int `json:"invitee_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.BindAdtyOwnerSiteInviteFromIP(site.Id, c.GetInt("id"), body.InviterID, body.InviteeID, service.HashReferralRiskValue(c.ClientIP())); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"bound": true})
}
