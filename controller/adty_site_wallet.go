package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PostAdtyEarningToMargin(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
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
	if err != nil || !amount.IsPositive() {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var tier model.AdtySiteTier
	if err := model.DB.Where("code = ? AND enabled = ?", site.TierCode, true).Limit(1).Find(&tier).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if !tier.DepositUSD.IsPositive() {
		common.ApiErrorT(c, "Margin tier is not configured")
		return
	}
	if err := service.TransferAdtyEarningToMargin(site.Id, c.GetInt("id"), amount, tier.DepositUSD, "earn-margin:"+key); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"transferred": true})
}

func GetAdtyOwnerWallets(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, site.Id, false, true)
}

func GetAdtyOwnerWalletLedger(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, site.Id, true, true)
}

func GetAdtyFundAnomalies(c *gin.Context) {
	siteID, err := strconv.Atoi(c.Query("site_id"))
	if err != nil || siteID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	kinds, err := service.CollectAdtyFundAnomalies(siteID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"kinds": kinds})
}

func GetAdtyAdminSiteWallets(c *gin.Context) {
	siteID, ok := siteIdParam(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, siteID, false, false)
}

func GetAdtyAdminSiteWalletLedger(c *gin.Context) {
	siteID, ok := siteIdParam(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, siteID, true, false)
}

func adtySiteWalletResponse(c *gin.Context, siteID int, ledger bool, hideOperating bool) {
	var data any
	var err error
	if ledger {
		page := common.GetPageQuery(c)
		var items []model.AdtySiteWalletLedger
		var total int64
		var queryErr error
		if hideOperating {
			items, total, queryErr = service.ListAdtyOwnerSiteWalletLedger(siteID, c.Query("kind"), page.GetStartIdx(), page.GetPageSize())
		} else {
			items, total, queryErr = service.ListAdtySiteWalletLedger(siteID, c.Query("kind"), page.GetStartIdx(), page.GetPageSize())
		}
		data, err = gin.H{"total": total, "items": items, "page": page.GetPage(), "page_size": page.GetPageSize()}, queryErr
	} else {
		wallets, queryErr := service.ListAdtySiteWallets(siteID)
		if queryErr == nil && hideOperating {
			visible := make([]model.AdtySiteWallet, 0, len(wallets))
			for _, wallet := range wallets {
				if wallet.Kind != model.AdtySiteWalletOperating {
					visible = append(visible, wallet)
				}
			}
			wallets = visible
		}
		data, err = wallets, queryErr
	}
	switch {
	case errors.Is(err, service.ErrAdtyOperatingWalletHidden):
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Operating wallet is not available to the site owner"))
		return
	case errors.Is(err, service.ErrAdtySiteNotFound):
		common.ApiErrorStatus(c, http.StatusNotFound, err)
	case errors.Is(err, service.ErrAdtySiteWalletInvalidKind), errors.Is(err, service.ErrAdtySiteWalletInvalidOperation):
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
	case err != nil:
		common.ApiError(c, err)
	default:
		common.ApiSuccess(c, data)
	}
}
