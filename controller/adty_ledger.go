package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetAdtyUserLedger 分页列出一个用户的余额流水（任务 7.2，用户·查看）：退款、人工调额、佣金转余额。
func GetAdtyUserLedger(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.ListAdtyLedgers(userID, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}
