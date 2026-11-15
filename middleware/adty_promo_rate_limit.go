package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// maxPromoProbeBodyBytes 是探测优惠码时允许读取的请求体上限。
// /adty/topup/quote 的正文是一个小 JSON；超过这个量说明这不是本中间件要
// 判断的请求形态，直接拒绝，免得把任意大的正文整个读进内存。
const maxPromoProbeBodyBytes int64 = 4096

// errPromoProbeBodyTooLarge 在正文超过 maxPromoProbeBodyBytes 时返回，
// 上层据此回 400 而不是截断后继续解析。
var errPromoProbeBodyTooLarge = fmt.Errorf("%w: promo code probe body exceeds %d bytes", common.ErrRequestBodyTooLarge, maxPromoProbeBodyBytes)

// PromoCodeUserRateLimit 只对带优惠码的请求按用户限速。没带码的报价不计数。
// 读过的正文会放回请求，后面的处理函数还能再读；正文超过上界则直接 400。
func PromoCodeUserRateLimit(scope string) gin.HandlerFunc {
	limit := UserCriticalRateLimit(scope)
	return func(c *gin.Context) {
		hasCode, err := peekPromoCode(c)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				common.ApiErrorStatus(c, http.StatusBadRequest, err, gin.H{"code": "REQUEST_BODY_TOO_LARGE"})
				c.Abort()
				return
			}
			c.Next()
			return
		}
		if !hasCode {
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
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPromoProbeBodyBytes+1))
	_ = c.Request.Body.Close()
	if err != nil {
		return false, err
	}
	if int64(len(body)) > maxPromoProbeBodyBytes {
		return false, errPromoProbeBodyTooLarge
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	var probe struct {
		PromoCode string `json:"promo_code"`
	}
	if err := common.Unmarshal(body, &probe); err != nil {
		return false, nil
	}
	return strings.TrimSpace(probe.PromoCode) != "", nil
}
