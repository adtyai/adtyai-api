package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const affPayoutQrUploadLimit = 1 << 20

// UploadAffPayoutQr POST /api/aff/payout-qr
func UploadAffPayoutQr(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, affPayoutQrUploadLimit)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		adtyFail(c, http.StatusBadRequest, "invalid_payout_qr", "invalid request", nil)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, modelPayoutQrReadLimit()))
	if err != nil || len(data) == 0 || int64(len(data)) > 512*1024 {
		adtyFail(c, http.StatusBadRequest, "invalid_payout_qr", "invalid request", nil)
		return
	}
	row, err := model.SaveAffPayoutQr(c.GetInt("id"), data)
	if err != nil {
		adtyFail(c, http.StatusBadRequest, "invalid_payout_qr", "invalid request", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": row.Id, "sha256": row.Sha256}})
}

func modelPayoutQrReadLimit() int64 { return 512*1024 + 1 }

func writeAffPayoutQr(c *gin.Context, row *model.AffPayoutQr, err error) {
	if err != nil {
		if errors.Is(err, model.ErrAffPayoutQrNotFound) {
			adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
			return
		}
		adtyFail(c, http.StatusOK, "bad_request", "invalid request", nil)
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, row.ContentType, row.Data)
}

// GetAffPayoutQrSelf GET /api/aff/payout-qr/:id
func GetAffPayoutQrSelf(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	row, err := model.GetAffPayoutQrForUser(id, c.GetInt("id"))
	writeAffPayoutQr(c, row, err)
}

// AdminGetAffPayoutQr GET /api/admin/aff/payout-qr/:id
func AdminGetAffPayoutQr(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	row, err := model.GetAffPayoutQrForAdmin(id)
	writeAffPayoutQr(c, row, err)
}

// ListAffInvitees GET /api/aff/invitees
func ListAffInvitees(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := model.ListAffInvitees(c.GetInt("id"), limit, offset, true)
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", "invalid request", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// AdminAffOverview GET /api/admin/aff/overview
func AdminAffOverview(c *gin.Context) {
	data, err := model.GetAffOverview()
	if err != nil {
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// AdminListAffInviters GET /api/admin/aff/inviters
func AdminListAffInviters(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	rows, total, err := model.ListAffAdminInviters(c.Query("keyword"), limit, offset)
	if err != nil {
		adtyInternalError(c, err)
		return
	}
	if rows == nil {
		rows = []model.AffInviterRow{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows, "total": total})
}

// AdminAffUserDetail GET /api/admin/aff/users/:id
func AdminAffUserDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		adtyFail(c, http.StatusBadRequest, "invalid_id", "invalid id", nil)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	inviteeOffset, _ := strconv.Atoi(c.DefaultQuery("invitee_offset", "0"))
	commissionOffset, _ := strconv.Atoi(c.DefaultQuery("commission_offset", "0"))
	withdrawalOffset, _ := strconv.Atoi(c.DefaultQuery("withdrawal_offset", "0"))
	transferOffset, _ := strconv.Atoi(c.DefaultQuery("transfer_offset", "0"))
	data, err := model.GetAffAdminUserDetailPage(id, model.AffAdminUserDetailQuery{
		Limit:            limit,
		InviteeOffset:    inviteeOffset,
		CommissionOffset: commissionOffset,
		WithdrawalOffset: withdrawalOffset,
		TransferOffset:   transferOffset,
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			adtyFail(c, http.StatusNotFound, "not_found", "not found", nil)
			return
		}
		adtyFail(c, http.StatusOK, "bad_request", affClientMessage(err), nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
