package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAdtyOwnerWallets(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, site.Id, false)
}

func GetAdtyOwnerWalletLedger(c *gin.Context) {
	site, ok := adtyOwnerSiteOr403(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, site.Id, true)
}

func GetAdtyAdminSiteWallets(c *gin.Context) {
	siteID, ok := siteIdParam(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, siteID, false)
}

func GetAdtyAdminSiteWalletLedger(c *gin.Context) {
	siteID, ok := siteIdParam(c)
	if !ok {
		return
	}
	adtySiteWalletResponse(c, siteID, true)
}

func adtySiteWalletResponse(c *gin.Context, siteID int, ledger bool) {
	var data any
	var err error
	if ledger {
		page := common.GetPageQuery(c)
		items, total, queryErr := service.ListAdtySiteWalletLedger(siteID, c.Query("kind"), page.GetStartIdx(), page.GetPageSize())
		data, err = gin.H{"total": total, "items": items, "page": page.GetPage(), "page_size": page.GetPageSize()}, queryErr
	} else {
		data, err = service.ListAdtySiteWallets(siteID)
	}
	switch {
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
