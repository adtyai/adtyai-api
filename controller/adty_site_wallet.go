package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

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
