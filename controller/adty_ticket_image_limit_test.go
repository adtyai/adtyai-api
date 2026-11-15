package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// countingReader 记下**实际被读走的字节数**。这是这条用例的核心测量：
// 「请求体有没有在解码之前被截断」只能靠读了多少来证明。
type countingReader struct {
	reader io.Reader
	read   int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}

// 工单图片路由必须在解码之前限制请求体。
//
// 不限制时：`common.DecodeJson` 把整个 body 读进内存，base64 解码再拷一份，
// **2MiB 的检查在两跳之后**——100MiB 的 body 峰值约 175MiB。`/api` 组没有
// `MaxBytesReader`（全仓只有 4 处逐接口加过），relay 那个解压中间件也不在
// 这条路由上，所以这是唯一的口子。
//
// 断言的是「读走多少字节」，不是「返回什么错误」：后者会被解码失败、base64
// 失败、service 拒绝三种原因共用，区分不出来。我第一版就是断言返回值——
// **去掉上限之后它是因为 model.DB 为 nil 而 panic 才红的，不是因为尺寸**，
// 那种反向验证等于没验。
func TestTicketImageRouteStopsReadingAtTheLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const bodySize = 5 << 20 // 5MiB，超过 4MiB 上限
	body := `{"data":"` + strings.Repeat("A", bodySize) + `"}`
	counter := &countingReader{reader: strings.NewReader(body)}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/adty/tickets/1/images", counter)
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = gin.Params{{Key: "id", Value: "1"}}

	// 先调用、再断言，而且**测量那条放在断言 panic 之前**。
	// 反过来的话，去掉上限之后这条用例会因为 handler 走到 service、撞上 nil DB
	// 而 panic 先红——红的原因不是尺寸，反向验证就白做了。
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		PostAdtyTicketImage(context)
	}()

	// 上限 4MiB；MaxBytesReader 会多读一点才报错，留 1KiB 余量。
	assert.LessOrEqual(t, counter.read, int64(4<<20)+1024,
		"请求体必须在解码之前被截断：实际读走了 %d 字节（body 共 %d）", counter.read, len(body))
	assert.False(t, panicked, "截断之后应当走正常的参数错误分支，不该继续往下执行")
	// 只断言「不是成功响应」：`ApiError` 写的是 JSON 体，HTTP 状态由全局
	// 错误处理决定，在 handler 单测里它还是 recorder 的默认值，拿它当判据没有意义。
	assert.NotContains(t, recorder.Body.String(), `"success":true`,
		"超限请求不该被当成成功处理")
}

// 上限本身要钉住：改大到 100MiB 等价于没有上限。
func TestTicketImageBodyLimitIsBounded(t *testing.T) {
	source := stripGoComments(readControllerFile(t, "adty_workspace.go"))
	assert.Contains(t, source, "http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)",
		"上限必须是解码之前那道 MaxBytesReader；改这个数字之前先想清楚峰值内存")
}

func readControllerFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := readFileInPackage(name)
	assert.NoError(t, err)
	return raw
}

// stripGoComments 去掉整行注释——`TestBillingSessionAlwaysCarriesOperationKey`
// 那次我踩过：守卫把自己的说明文字判成了违例。
func stripGoComments(source string) string {
	lines := strings.Split(source, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func readFileInPackage(name string) (string, error) {
	raw, err := os.ReadFile(name)
	return string(raw), err
}
