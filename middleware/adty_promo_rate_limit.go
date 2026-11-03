package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// PromoCodeUserRateLimit 只对带优惠码的请求按用户限速。没带码的报价不计数。
// 读过的正文会放回请求，后面的处理函数还能再读。
func PromoCodeUserRateLimit(scope string) gin.HandlerFunc {
	limit := UserCriticalRateLimit(scope)
	return func(c *gin.Context) {
		hasCode, err := peekPromoCode(c)
		if err != nil || !hasCode {
			c.Next()
			return
		}
		limit(c)
	}
}

func peekPromoCode(c *gin.Context) (bool, error) {
	if c.Request == nil || c.Request.Body == nil {
		return false, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	var probe struct {
		PromoCode string `json:"promo_code"`
	}
	if err := common.Unmarshal(body, &probe); err != nil {
		return false, nil
	}
	return strings.TrimSpace(probe.PromoCode) != "", nil
}
