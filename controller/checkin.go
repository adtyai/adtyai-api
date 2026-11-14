package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetCheckinStatus does not advertise a live award range. Check-in is retired;
// HTTP GET /api/user/checkin stays a permanent 403 via AdtyClosedFeature.
func GetCheckinStatus(c *gin.Context) {
	common.ApiError(c, model.ErrCheckinRetired)
}

// DoCheckin is not a callable credit API. Re-pointing the 403 route here
// still cannot award quota.
func DoCheckin(c *gin.Context) {
	common.ApiError(c, model.ErrCheckinRetired)
}
