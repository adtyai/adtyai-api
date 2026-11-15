package service

import (
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 验的是**终止性**，而且走的是真实调用点（`adtyTelegramSendMessageHTTP` /
// `adtyTelegramGetMeHTTP`），不是自己另调一个 client。
//
// 只证明「http.Client 会超时」没有意义——那验的是标准库。要证明的是
// **这两处出站调用真的走那个带 Timeout 的 client**，所以 base URL 必须是变量
// （`adtyTelegramAPIBase`），否则测试只能拼一个自己的 URL。
//
// 两条断言缺一不可：
//  1. `err != nil` —— 证明它不是静默成功；
//  2. 耗时 < 2s   —— 证明超时真的生效。
//
// **只断言第一条不够**：挂死的调用最终也会因底层 TCP 超时返回错误，但那是
// 两分钟，不是我们设的那个值。
func TestTelegramOutboundCallsGiveUpOnSilentPeer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	previousBase, previousClient := adtyTelegramAPIBase, adtyTelegramHTTP
	previousInteractive := adtyTelegramInteractiveHTTP
	adtyTelegramAPIBase = "http://" + listener.Addr().String()
	adtyTelegramHTTP = &http.Client{Timeout: 300 * time.Millisecond}
	adtyTelegramInteractiveHTTP = &http.Client{Timeout: 300 * time.Millisecond}
	t.Cleanup(func() {
		adtyTelegramAPIBase, adtyTelegramHTTP = previousBase, previousClient
		adtyTelegramInteractiveHTTP = previousInteractive
	})

	cases := map[string]func() error{
		"sendMessage": func() error { return adtyTelegramSendMessageHTTP("123:abc", "@ops", "hello") },
		"getMe": func() error {
			_, _, getErr := adtyTelegramGetMeHTTP("123:abc")
			return getErr
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			done := make(chan error, 1)
			go func() { done <- call() }()

			select {
			case callErr := <-done:
				require.Error(t, callErr, "对端不响应时必须返回错误，不能静默成功")
				assert.Less(t, time.Since(started), 2*time.Second,
					"必须按 client 的 timeout 返回；等到 TCP 自己超时是两分钟，不是 15 秒")
			case <-time.After(2 * time.Second):
				t.Fatal("调用挂住了：这两处没有走那个带 Timeout 的 client")
			}
		})
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

// 两处出站调用都必须走**各自那个**带 Timeout 的 client，不能回到裸调用。
//
// 两个 client 分工不同：投递（后台 goroutine，没人等）用 15 秒的；
// getMe（用户点完保存同步等）用 5 秒的。所以这里分别钉住。
//
// **失败信息只报命中的那一行，不 dump 整个文件。** 上一版用 `assert.Contains`
// 断言整份源码，失败时会把 600 行源码灌进日志，真正要看的那一句被埋掉。
func TestTelegramOutboundCallsUseBoundedClients(t *testing.T) {
	source := readServiceFile(t, "adty_telegram.go")
	for name, want := range map[string]string{
		"投递":    "adtyTelegramHTTP.PostForm",
		"getMe": "adtyTelegramInteractiveHTTP.Do(",
	} {
		assert.Contains(t, source, want, "%s 必须走带 Timeout 的 client", name)
	}
	for _, banned := range []struct{ call, why string }{
		{"http.PostForm(", "http.PostForm 用的是 DefaultClient，没有超时"},
		{"http.DefaultClient.Do(", "DefaultClient 没有超时，会让投递 goroutine 永久挂住"},
		{"&http.Client{}", "新建的 client 没有 Timeout：即使它调得对，也绕过了这两个共用 client"},
	} {
		assert.NotContains(t, source, banned.call, "%s", banned.why)
	}
}

// readServiceFile 读本包内的源文件，供「文本层面守门」那类断言用。
// 路径相对本测试文件所在的包目录（go test 的工作目录就是包目录）。
func readServiceFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(raw)
}

// **配了 bot 但发不出去** → 启动必须喊一声。
//
// 不设 `ADTY_TELEGRAM_ALLOW_NETWORK=1` 时 `AdtyTelegramSender` 是 nil：队列照常
// 入队，但一条都发不出去，而且会一路走到 dead。**外部看到的和「没有消息要发」
// 完全一样**，所以这行日志是唯一能区分两者的东西。
func TestTelegramDeliveryTaskWarnsWhenBotConfiguredButSenderMissing(t *testing.T) {
	db := announcementTestDB(t)
	t.Setenv("CRYPTO_SECRET", "adty-telegram-warn-test-secret-32bytes-min")
	cipher, err := model.SealAdtyPurposeSecret("telegram:"+model.AdtyTelegramPurposeAdmin, "123456789:token")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.AdtyTelegramBot{
		Purpose: model.AdtyTelegramPurposeAdmin, BotId: 1, Username: "root_bot",
		TokenCipher: cipher, Enabled: true,
	}).Error)

	previous := AdtyTelegramSender
	AdtyTelegramSender = nil
	t.Cleanup(func() { AdtyTelegramSender = previous })

	logged := captureSysLog(t)
	startAdtyTelegramDeliveryTask()

	assert.Contains(t, logged.String(), "bot is configured",
		"配了 bot 却发不出去时必须留下痕迹，否则「开关没开」和「没消息」同形")
	assert.Contains(t, logged.String(), "ADTY_TELEGRAM_ALLOW_NETWORK",
		"提示要指名那个变量，否则读日志的人还得自己找")
}

// **本来就不用 Telegram 的部署不该喊。** 只看 sender 为 nil 的话，本地开发、
// 不带通知的单体部署、CI 每次启动都会打一条 ERROR——后果不是多一行日志，
// 是这条日志从此不被读（`ISS-R4-0065`）。
func TestTelegramDeliveryTaskStaysQuietWhenNoBotConfigured(t *testing.T) {
	db := announcementTestDB(t)
	require.NoError(t, db.Model(&model.AdtyTelegramBot{}).Where("1 = 1").Delete(&model.AdtyTelegramBot{}).Error)

	previous := AdtyTelegramSender
	AdtyTelegramSender = nil
	t.Cleanup(func() { AdtyTelegramSender = previous })

	logged := captureSysLog(t)
	startAdtyTelegramDeliveryTask()

	assert.NotContains(t, logged.String(), "bot is configured",
		"没配 bot 就是没打算发 Telegram：这不是异常，不该报警")
}

// 有发送实现时不该喊这一条——否则它每天都会在日志里出现，然后被无视。
func TestTelegramDeliveryTaskStaysQuietWhenSenderIsSet(t *testing.T) {
	previous := AdtyTelegramSender
	AdtyTelegramSender = func(token, chat, text string) error { return nil }
	t.Cleanup(func() { AdtyTelegramSender = previous })

	logged := captureSysLog(t)
	startAdtyTelegramDeliveryTask()

	assert.NotContains(t, logged.String(), "telegram delivery is disabled")
}

// 交互式调用（保存 bot token 时的 getMe）必须用**更短**的超时。
//
// 它和投递的 15 秒分开，是因为等待的人不同：投递是后台 goroutine，没人等；
// getMe 是用户点了「保存」之后同步等结果——15 秒会让人以为卡死，然后重复点。
func TestTelegramInteractiveClientIsShorterThanDelivery(t *testing.T) {
	require.NotNil(t, adtyTelegramInteractiveHTTP)
	assert.Positive(t, adtyTelegramInteractiveHTTP.Timeout, "交互式 client 也必须有超时")
	assert.NotSame(t, adtyTelegramHTTP, adtyTelegramInteractiveHTTP,
		"两者必须是不同的 client，否则「交互式更短」这件事无处安放")
	assert.Less(t, adtyTelegramInteractiveHTTP.Timeout, adtyTelegramHTTP.Timeout,
		"交互式等待必须短于后台投递：%v vs %v",
		adtyTelegramInteractiveHTTP.Timeout, adtyTelegramHTTP.Timeout)
}

// getMe 必须走交互式那个 client —— 文本守门。
func TestTelegramGetMeUsesInteractiveClient(t *testing.T) {
	source := readServiceFile(t, "adty_telegram.go")
	assert.Contains(t, source, "adtyTelegramInteractiveHTTP.Do(request)",
		"getMe 是同步交互调用，必须走 5 秒那个 client")
}
