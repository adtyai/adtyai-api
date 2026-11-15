package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// 系统翻译的出口实现（ISS-R4-0012）。Root 在后台配好协议、地址和密钥后，
// 这里按配置真的发一次 HTTP 请求；没配好就老实报错，由调用方保留中文并标记英文未就绪。
// 约束：不把翻译请求发回本站中转，不用客户 Key，不扣客户、团队或站长余额。
const (
	adtyTranslationProtocolOpenAI    = "openai"
	adtyTranslationProtocolAnthropic = "anthropic"

	adtyTranslationDefaultTimeout  = 30 * time.Second
	adtyTranslationAnthropicVer    = "2023-06-01"
	adtyTranslationMaxBodyBytes    = 1 << 20
	adtyTranslationMaxOutputTokens = 2048
	adtyTranslationSystemPrompt    = "Translate the user's Simplified Chinese product text into English. " +
		"Keep every number, amount, currency code, order number, placeholder and URL exactly as written, " +
		"and keep the original negation. Reply with the English translation only."
)

var errAdtyTranslationNoReply = common.NewMessage("The translation service did not return English")

// adtyTranslationHTTPClient 造一次翻译用的客户端：复用进程的出口连接池，只覆盖超时，
// 免得慢翻译一直占着调用方的 goroutine。测试替换这里。
var adtyTranslationHTTPClient = func(timeout time.Duration) *http.Client {
	base := GetHttpClient()
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.Timeout = timeout
	return &client
}

// adtyTranslateViaProvider 是生产用的 AdtyTranslateFunc 实现：每次调用读一次 Root 配置，
// 所以后台改完配置立刻生效，不用重启。
func adtyTranslateViaProvider(modelName, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrAdtyTranslationInvalid
	}
	cfg, err := loadAdtyTranslationConfig()
	if err != nil {
		return "", err
	}
	protocol, err := adtyTranslationProtocolName(cfg.Protocol)
	if err != nil {
		return "", err
	}
	endpoint, err := adtyTranslationEndpoint(cfg.BaseURL, protocol)
	if err != nil {
		return "", err
	}
	token, err := model.OpenAdtyPurposeSecret("translation", cfg.TokenCipher)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = cfg.ModelName
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", ErrAdtyTranslationInvalid
	}

	payload, err := adtyTranslationPayload(protocol, modelName, text)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if protocol == adtyTranslationProtocolAnthropic {
		request.Header.Set("anthropic-version", adtyTranslationAnthropicVer)
		if token != "" {
			request.Header.Set("x-api-key", token)
		}
	} else if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := adtyTranslationHTTPClient(adtyTranslationTimeout(cfg.TimeoutSec)).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, adtyTranslationMaxBodyBytes))
	if err != nil {
		return "", err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("translator returned HTTP %d", response.StatusCode)
	}
	return adtyTranslationReply(protocol, raw)
}

// CheckAdtyTranslationProvider 在启动时确认「Root 打开了系统翻译」这条链真的能用。
// 没配或没启用时保持安静；启用但协议、地址不完整时返回原因，由 StartAdty 记一条告警。
func CheckAdtyTranslationProvider() error {
	cfg, err := loadAdtyTranslationConfig()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	protocol, err := adtyTranslationProtocolName(cfg.Protocol)
	if err != nil {
		return err
	}
	_, err = adtyTranslationEndpoint(cfg.BaseURL, protocol)
	return err
}

func adtyTranslationProtocolName(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "openai", "openai-compatible", "chat-completions":
		return adtyTranslationProtocolOpenAI, nil
	case "anthropic", "claude":
		return adtyTranslationProtocolAnthropic, nil
	}
	return "", ErrAdtyTranslationInvalid
}

// adtyTranslationEndpoint 把 Root 填的接口地址补成完整的调用地址。地址允许写成根地址、
// 带 /v1 的地址，或已经带上 /chat/completions 的完整地址。和保存时一样只收 https。
func adtyTranslationEndpoint(rawBaseURL, protocol string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return "", ErrAdtyTranslationInvalid
	}
	if adtyTranslationTargetIsSelf(parsed.Hostname()) {
		return "", ErrAdtyTranslationSelf
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if protocol == adtyTranslationProtocolAnthropic {
		if !strings.HasSuffix(path, "/messages") {
			if path == "" {
				path = "/v1/messages"
			} else {
				path += "/messages"
			}
		}
	} else if !strings.HasSuffix(path, "/chat/completions") {
		if path == "" {
			path = "/v1/chat/completions"
		} else {
			path += "/chat/completions"
		}
	}
	parsed.Path = path
	parsed.RawQuery = ""
	return parsed.String(), nil
}

// adtyTranslationTargetIsSelf 防止翻译请求回到本站：本站品牌域名、主站域名和分站根域名都算自己。
func adtyTranslationTargetIsSelf(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	own := []string{strings.TrimPrefix(strings.ToLower(common.AdtyBrandURL), "https://")}
	if main := adtyMainHost(); main != "" && main != "localhost" {
		own = append(own, adtyRegistrableDomain(main))
	}
	own = append(own, resellerSetting().RootDomains...)
	for _, domain := range own {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" && adtyHostUnder(host, domain) {
			return true
		}
	}
	return false
}

func adtyTranslationTimeout(seconds int) time.Duration {
	if seconds <= 0 || seconds > maxTimeoutSeconds {
		return adtyTranslationDefaultTimeout
	}
	return time.Duration(seconds) * time.Second
}

func adtyTranslationPayload(protocol, modelName, text string) (any, error) {
	switch protocol {
	case adtyTranslationProtocolAnthropic:
		return map[string]any{
			"model":      modelName,
			"max_tokens": adtyTranslationMaxOutputTokens,
			"system":     adtyTranslationSystemPrompt,
			"messages":   []map[string]string{{"role": "user", "content": text}},
		}, nil
	case adtyTranslationProtocolOpenAI:
		return map[string]any{
			"model":  modelName,
			"stream": false,
			"messages": []map[string]string{
				{"role": "system", "content": adtyTranslationSystemPrompt},
				{"role": "user", "content": text},
			},
		}, nil
	}
	return nil, ErrAdtyTranslationInvalid
}

func adtyTranslationReply(protocol string, raw []byte) (string, error) {
	var english string
	if protocol == adtyTranslationProtocolAnthropic {
		var decoded struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", errAdtyTranslationNoReply
		}
		for _, part := range decoded.Content {
			if part.Type == "text" {
				english = part.Text
				break
			}
		}
	} else {
		var decoded struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", errAdtyTranslationNoReply
		}
		if len(decoded.Choices) > 0 {
			english = decoded.Choices[0].Message.Content
		}
	}
	english = strings.TrimSpace(english)
	if english == "" {
		return "", errAdtyTranslationNoReply
	}
	return english, nil
}
