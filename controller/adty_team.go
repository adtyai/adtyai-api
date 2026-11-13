package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PutAdtyMainTeamAgreementPrice(c *gin.Context) {
	if c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to perform this action"))
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
	if err := service.SaveAdtyMainTeamAgreementPrice(id, body.Discount); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": true})
}

func PostAdtyTeamTopup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		AmountUSD string `json:"amount_usd"`
		Method    string `json:"method"`
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
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if method := strings.TrimSpace(body.Method); method == model.PaymentMethodStripe {
		if !isStripeTopUpEnabled() {
			common.ApiErrorT(c, "Stripe payment is not available")
			return
		}
		user, err := model.GetUserById(c.GetInt("id"), false)
		if err != nil || user == nil {
			common.ApiErrorT(c, "User does not exist")
			return
		}
		pay, ok := stripeQuotedPay(amount.IntPart(), user.Group)
		if !ok {
			common.ApiErrorT(c, "Top-up amount must be a whole number of quota units")
			return
		}
		order, err := service.StartAdtyTeamStripeTopup(c.GetInt("id"), id, amount, pay.Money, key, func(tradeNo string, money decimal.Decimal) (string, string, error) {
			session, err := genStripeCheckout(tradeNo, user.StripeCustomer, user.Email, money, "", "")
			if err != nil {
				return "", "", err
			}
			return session.URL, session.ID, nil
		})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, order)
		return
	}
	if method := strings.TrimSpace(body.Method); method != "" {
		order, err := service.StartAdtyTeamTopupPayment(c.GetInt("id"), id, amount, method, key, c.ClientIP())
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, order)
		return
	}
	order, err := service.CreateAdtyTeamTopupOrder(c.GetInt("id"), id, amount, key)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, order)
}

func GetAdtyTeamTopups(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	rows, err := service.ListAdtyTeamTopups(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows})
}

func PostAdtyTeam(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Discount string `json:"discount"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	discount, err := decimal.NewFromString(body.Discount)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	created, err := service.CreateAdtyTeamForCurrentUser(c.GetInt("id"), body.Name, discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, created)
}

func GetAdtyTeamMine(c *gin.Context) {
	mine, err := service.GetAdtyTeamMine(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"team": mine})
}

func PostAdtyTeamInvite(c *gin.Context) {
	id, ok := adtyPathId(c)
	if !ok {
		return
	}
	code, err := service.CreateAdtyTeamInvite(c.GetInt("id"), int64(id), 24*time.Hour)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"code": code})
}

func PostAdtyTeamAccept(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	member, err := service.AcceptAdtyTeamInvite(c.GetInt("id"), body.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, member)
}
