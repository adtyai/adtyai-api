package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AdminGetSidebarBadges GET /api/adty/admin/sidebar_badges
// 侧栏角标：风控待审 / 提现待审 / 充值待处理（pending+need_manual）。
func AdminGetSidebarBadges(c *gin.Context) {
	data, err := model.BuildSidebarBadges()
	if err != nil {
		adtyFail(c, http.StatusInternalServerError, "internal_error", "failed to load sidebar badges", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
