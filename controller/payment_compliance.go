package controller

import (
	"github.com/gin-gonic/gin"
)

// requirePaymentCompliance 总裁令：支付/兑换码/订阅/邀请奖励不再拦合规确认。
// 保留函数签名，避免各支付入口大面积改调用点。
func requirePaymentCompliance(c *gin.Context) bool {
	_ = c
	return true
}
