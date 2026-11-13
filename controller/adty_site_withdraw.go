package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PostAdtySiteWithdrawal(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	var body struct {
		AmountUSD    string `json:"amount_usd"`
		Method       string `json:"method"`
		Network      string `json:"network"`
		AccountNo    string `json:"account_no"`
		AccountName  string `json:"account_name"`
		ExpectedRate string `json:"expected_rate"`
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
	expected, err := decimal.NewFromString(strings.TrimSpace(body.ExpectedRate))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	row, err := service.CreateAdtySiteWithdrawal(service.AdtySiteWithdrawInput{
		SiteID: site.Id, OwnerID: c.GetInt("id"), AmountUSD: amount, Method: body.Method, Network: body.Network,
		AccountNo: body.AccountNo, AccountName: body.AccountName, ExpectedRate: expected,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func PostAdtySiteWithdrawalCancel(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	if err := service.CancelAdtySiteWithdrawal(site.Id, c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"cancelled": true})
}

func PutAdtySiteWithdrawRules(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	var body service.AdtySiteWithdrawRules
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := service.SaveAdtySiteWithdrawRules(body, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"saved": true})
}

func PostAdtySiteWithdrawalPaying(c *gin.Context) {
	reviewAdtySiteWithdrawal(c, "paying")
}

func PostAdtySiteWithdrawalReject(c *gin.Context) {
	reviewAdtySiteWithdrawal(c, "reject")
}

func PostAdtySiteWithdrawalPaid(c *gin.Context) {
	reviewAdtySiteWithdrawal(c, "paid")
}

func reviewAdtySiteWithdrawal(c *gin.Context, action string) {
	if c.GetInt("role") < common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
		return
	}
	id, ok := adtyInt64Param(c, "id")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
		TxID   string `json:"tx_id"`
		Proof  string `json:"proof"`
	}
	_ = common.DecodeJson(c.Request.Body, &body)
	var err error
	switch action {
	case "paying":
		err = service.MarkAdtySiteWithdrawalPaying(id, c.GetInt("id"))
	case "reject":
		err = service.RejectAdtySiteWithdrawal(id, c.GetInt("id"), body.Reason)
	default:
		err = service.MarkAdtySiteWithdrawalPaid(id, c.GetInt("id"), body.TxID, body.Proof)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}
