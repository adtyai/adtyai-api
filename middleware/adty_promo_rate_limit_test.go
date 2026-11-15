package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 优惠码探测中间件只应读取有上界的正文：超限的请求必须在读完上界后停下并
// 被拒绝，正常请求的正文要原样还原给后面的处理函数。

type countingBody struct {
	reader io.Reader
	read   int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *countingBody) Close() error { return nil }

type promoProbeOutcome struct {
	status    int
	respBody  string
	reached   bool
	gotBody   []byte
	bodyErr   error
	wireBytes int64
}

func runPromoProbe(t *testing.T, payload []byte) promoProbeOutcome {
	t.Helper()
	gin.SetMode(gin.TestMode)

	out := promoProbeOutcome{}
	router := gin.New()
	router.POST("/quote",
		func(c *gin.Context) { c.Set("id", 7); c.Next() }, // 中间件挂在 UserAuth() 之后
		PromoCodeUserRateLimit("CT:promo-quote"),
		func(c *gin.Context) {
			out.reached = true
			out.gotBody, out.bodyErr = io.ReadAll(c.Request.Body)
			c.Status(http.StatusOK)
		},
	)

	body := &countingBody{reader: bytes.NewReader(payload)}
	req := httptest.NewRequest(http.MethodPost, "/quote", nil)
	req.Body = body
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	out.status = recorder.Code
	out.respBody = recorder.Body.String()
	out.wireBytes = body.read
	return out
}

func oversizedPayload(t *testing.T) []byte {
	t.Helper()
	payload := []byte(`{"promo_code":"","pad":"` + strings.Repeat("a", 1<<20) + `"}`)
	return payload
}

func TestPromoCodeProbeRejectsOversizedBody(t *testing.T) {
	payload := oversizedPayload(t)

	out := runPromoProbe(t, payload)

	assert.Equal(t, http.StatusBadRequest, out.status, "超限正文应被拒绝")
	assert.Contains(t, out.respBody, "REQUEST_BODY_TOO_LARGE")
	assert.False(t, out.reached, "超限请求不应进入后续处理函数")
	// 只读到上界+1 就停下：整个 1MB 正文没有被读进内存。
	assert.Equal(t, maxPromoProbeBodyBytes+1, out.wireBytes, "只应读取上限+1 字节")
	assert.Less(t, out.wireBytes, int64(len(payload)))
}

func TestPromoCodeProbeRestoresBodyForNormalRequest(t *testing.T) {
	payload := []byte(`{"promo_code":"  ","biz_no":"b-1","amount":1200}`)

	out := runPromoProbe(t, payload)

	require.Equal(t, http.StatusOK, out.status)
	require.True(t, out.reached, "正常请求应进入后续处理函数")
	require.NoError(t, out.bodyErr)
	assert.Equal(t, string(payload), string(out.gotBody), "还原后的正文应与原始正文完全一致")
}

func TestPromoCodeProbeBodyLimitBoundary(t *testing.T) {
	head := `{"promo_code":"","pad":"`
	tail := `"}`

	atLimit := []byte(head + strings.Repeat("a", int(maxPromoProbeBodyBytes)-len(head)-len(tail)) + tail)
	require.Equal(t, int(maxPromoProbeBodyBytes), len(atLimit))

	out := runPromoProbe(t, atLimit)
	assert.Equal(t, http.StatusOK, out.status, "恰好等于上限的正文应放行")
	assert.True(t, out.reached)
	assert.Equal(t, string(atLimit), string(out.gotBody))

	out = runPromoProbe(t, append(atLimit, ' '))
	assert.Equal(t, http.StatusBadRequest, out.status, "超过上限一个字节就应拒绝")
	assert.False(t, out.reached)
}
