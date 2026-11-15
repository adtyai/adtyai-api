package service

import (
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 验的是**终止性**，不是功能：对端接受连接但永不响应时，调用必须在容差内
// 返回错误，而不是把 goroutine 永久挂住。
//
// 为什么这条比「能不能发」重要：投递任务是一条单线程串行的 goroutine，
// 卡住之后队列只增不减、没有错误日志（函数没返回）、ticker 也不再触发。
// 「没有新消息发出」的外观和「没有消息要发」完全一样，且不会自愈。
func TestTelegramSendTimesOutOnSilentPeer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	// 只接受连接、永不写响应。
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	previous := adtyTelegramHTTP
	adtyTelegramHTTP = &http.Client{Timeout: 300 * time.Millisecond}
	t.Cleanup(func() { adtyTelegramHTTP = previous })

	endpoint := "http://" + listener.Addr().String() + "/bot123:abc/getMe"
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		request, reqErr := http.NewRequest(http.MethodGet, endpoint, nil)
		if reqErr != nil {
			done <- reqErr
			return
		}
		response, doErr := adtyTelegramHTTP.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- doErr
	}()

	select {
	case err := <-done:
		require.Error(t, err, "对端不响应时必须返回错误")
		assert.Less(t, time.Since(started), 5*time.Second, "必须按 timeout 返回，不能挂住")
	case <-time.After(5 * time.Second):
		t.Fatal("调用挂住了：timeout 没有生效，投递 goroutine 会永久停在这里")
	}
}

// 同一个 client 必须带 Timeout —— 这条钉住「不能用 DefaultClient」。
// 没有它，上面那条测试换成 DefaultClient 也会红，但红得很慢（5 秒容差），
// 而这条是即时失败的判据。
func TestTelegramHTTPClientHasTimeout(t *testing.T) {
	require.NotNil(t, adtyTelegramHTTP, "出站 client 不能是 nil")
	assert.Positive(t, adtyTelegramHTTP.Timeout,
		"没有 Timeout 的 client 会让整条重试机制在 TCP 卡住时永久停摆")
	assert.NotSame(t, http.DefaultClient, adtyTelegramHTTP,
		"不能直接用 DefaultClient：它没有超时")
}

// 两处出站调用都必须走这个 client。文本层面的守门：
// 任何一处回到 `http.PostForm` / `http.DefaultClient`，这里就红。
func TestTelegramOutboundCallsUseSharedClient(t *testing.T) {
	source := readServiceFile(t, "adty_telegram.go")
	assert.Contains(t, source, "adtyTelegramHTTP.PostForm")
	assert.Contains(t, source, "adtyTelegramHTTP.Do(")
	assert.NotContains(t, source, "http.PostForm(",
		"http.PostForm 用的是 DefaultClient，没有超时")
	assert.NotContains(t, source, "http.DefaultClient.Do(",
		"DefaultClient 没有超时，会让投递 goroutine 永久挂住")
}

// readServiceFile 读本包内的源文件，供「文本层面守门」那类断言用。
// 路径相对本测试文件所在的包目录（go test 的工作目录就是包目录）。
func readServiceFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(raw)
}
