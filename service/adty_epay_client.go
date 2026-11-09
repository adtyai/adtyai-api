package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// 易支付的 API 下单、页面跳转、查单和探活（任务 5.2、5.3）。各家程序有差异，返回字段要容错。

// AdtyEpayCreateRequest 是一笔易支付下单。
type AdtyEpayCreateRequest struct {
	TradeNo, Name, ClientIP, Device string
	MoneyCNY                        decimal.Decimal
	NotifyURL, ReturnURL            string // ReturnURL 不带 query
}

// AdtyEpayCreateResult 是网关返回的支付信息。Mode 是 qrcode、payurl、urlscheme 或 submit。
type AdtyEpayCreateResult struct{ Mode, QRCode, PayURL, URLScheme, GatewayTradeNo string }

// AdtyEpayQueryResult 是查单结果。查单返回没有签名，只对 https 网关当真。
type AdtyEpayQueryResult struct {
	Paid           bool
	MoneyCNY       decimal.Decimal
	TypeCode       string
	GatewayTradeNo string
	OutTradeNo     string
	MerchantId     string
}

var adtyEpayHTTPClient = func() *http.Client {
	if client := GetHttpClient(); client != nil {
		return client
	}
	return http.DefaultClient
}

func adtyEpayEndpoint(channel *model.AdtyEpayChannel, path string) string {
	base := strings.TrimRight(strings.TrimSpace(channel.GatewayURL), "/")
	for _, suffix := range []string{"/submit.php", "/mapi.php", "/api.php"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base + path
}

// adtyEpayOrderParams 是 mapi.php 和 submit.php 共用的参数，签名用原值。
func adtyEpayOrderParams(channel *model.AdtyEpayChannel, method *model.AdtyEpayChannelMethod, req AdtyEpayCreateRequest) map[string]string {
	typeCode := method.TypeCode
	if typeCode == "" {
		typeCode = method.Method
	}
	params := map[string]string{
		"pid":          channel.MerchantId,
		"type":         typeCode,
		"out_trade_no": req.TradeNo,
		"notify_url":   req.NotifyURL,
		"name":         req.Name,
		"money":        req.MoneyCNY.StringFixed(2),
	}
	if strings.TrimSpace(req.ReturnURL) != "" {
		params["return_url"] = req.ReturnURL
	}
	return params
}

// CreateAdtyEpayOrder 下单。API 模式 POST mapi.php；页面跳转模式只生成签名后的 submit.php 链接。
func CreateAdtyEpayOrder(ctx context.Context, channel *model.AdtyEpayChannel, method *model.AdtyEpayChannelMethod, req AdtyEpayCreateRequest) (*AdtyEpayCreateResult, error) {
	params := adtyEpayOrderParams(channel, method, req)
	if channel.OrderMode == model.AdtyEpayOrderModeSubmit {
		sign, err := EpaySign(params, channel.MerchantKey)
		if err != nil {
			return nil, err
		}
		values := url.Values{}
		for key, value := range params {
			values.Set(key, value)
		}
		values.Set("sign", sign)
		values.Set("sign_type", "MD5")
		result := &AdtyEpayCreateResult{Mode: "submit", PayURL: adtyEpayEndpoint(channel, "/submit.php") + "?" + values.Encode()}
		if err := adtyRejectUnsafeEpayLink(result); err != nil {
			return nil, err
		}
		return result, nil
	}
	params["clientip"] = req.ClientIP
	params["device"] = req.Device
	sign, err := EpaySign(params, channel.MerchantKey)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	form.Set("sign", sign)
	form.Set("sign_type", "MD5")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, adtyEpayEndpoint(channel, "/mapi.php"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := adtyEpayHTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Code      json.RawMessage `json:"code"`
		Msg       string          `json:"msg"`
		TradeNo   string          `json:"trade_no"`
		PayURL    string          `json:"payurl"`
		QRCode    string          `json:"qrcode"`
		URLScheme string          `json:"urlscheme"`
	}
	if err := common.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("epay mapi returned non-JSON (http %d)", resp.StatusCode)
	}
	code := strings.Trim(strings.TrimSpace(string(parsed.Code)), `"`)
	if code != "1" {
		return nil, fmt.Errorf("epay mapi code %s: %s", code, parsed.Msg)
	}
	result := &AdtyEpayCreateResult{GatewayTradeNo: parsed.TradeNo, QRCode: parsed.QRCode, PayURL: parsed.PayURL, URLScheme: parsed.URLScheme}
	switch {
	case parsed.QRCode != "":
		result.Mode = "qrcode"
	case parsed.PayURL != "":
		result.Mode = "payurl"
	case parsed.URLScheme != "":
		result.Mode = "urlscheme"
	default:
		return nil, fmt.Errorf("epay mapi returned no payment link")
	}
	if err := adtyRejectUnsafeEpayLink(result); err != nil {
		return nil, err
	}
	return result, nil
}

// adtyDangerousEpaySchemes 不能进二维码，也不能被浏览器打开。
var adtyDangerousEpaySchemes = map[string]bool{
	"javascript": true,
	"data":       true,
	"vbscript":   true,
	"file":       true,
	"blob":       true,
}

func adtyEpayScheme(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "invalid"
	}
	return strings.ToLower(parsed.Scheme)
}

// adtyEpayOpenedLinkSafe 是会交给浏览器打开的链接：只允许 http(s) 和支付宝、微信的固定 scheme。
func adtyEpayOpenedLinkSafe(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "\\") || strings.HasPrefix(trimmed, "//") {
		return false
	}
	switch adtyEpayScheme(trimmed) {
	case "http", "https", "alipay", "alipays", "weixin":
		return true
	default:
		return false
	}
}

// adtyEpayQRSafe 是画进二维码的内容。没有 scheme 的原文可以画；危险 scheme 不行。
func adtyEpayQRSafe(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "\\") || strings.HasPrefix(trimmed, "//") {
		return false
	}
	scheme := adtyEpayScheme(trimmed)
	if scheme == "" {
		return !strings.Contains(trimmed, ":")
	}
	return scheme != "invalid" && !adtyDangerousEpaySchemes[scheme]
}

func adtyRejectUnsafeEpayLink(result *AdtyEpayCreateResult) error {
	safe := false
	switch result.Mode {
	case "qrcode":
		safe = adtyEpayQRSafe(result.QRCode)
	case "payurl", "submit":
		safe = adtyEpayOpenedLinkSafe(result.PayURL)
	case "urlscheme":
		safe = adtyEpayOpenedLinkSafe(result.URLScheme)
	}
	if !safe {
		return fmt.Errorf("epay returned an unsafe payment link")
	}
	return nil
}

// QueryAdtyEpayOrder 按我们的订单号查单（api.php?act=order）。status、code 兼容数字和字符串。
func QueryAdtyEpayOrder(ctx context.Context, channel *model.AdtyEpayChannel, outTradeNo string) (*AdtyEpayQueryResult, error) {
	values := url.Values{}
	values.Set("act", "order")
	values.Set("pid", channel.MerchantId)
	values.Set("key", channel.MerchantKey)
	values.Set("out_trade_no", outTradeNo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, adtyEpayEndpoint(channel, "/api.php")+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// Merchant credentials are in the query. Never forward them to a redirect,
	// including an HTTPS gateway redirecting its order lookup to plain HTTP.
	client := *adtyEpayHTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epay query http %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Code       json.RawMessage `json:"code"`
		Status     json.RawMessage `json:"status"`
		Money      json.RawMessage `json:"money"`
		Type       string          `json:"type"`
		TradeNo    string          `json:"trade_no"`
		OutTradeNo string          `json:"out_trade_no"`
		MerchantId json.RawMessage `json:"pid"`
	}
	if err := common.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("epay query returned non-JSON")
	}
	if strings.Trim(strings.TrimSpace(string(parsed.Code)), `"`) != "1" {
		return nil, fmt.Errorf("epay order query did not succeed")
	}
	result := &AdtyEpayQueryResult{TypeCode: parsed.Type, GatewayTradeNo: parsed.TradeNo, OutTradeNo: parsed.OutTradeNo,
		MerchantId: strings.Trim(strings.TrimSpace(string(parsed.MerchantId)), `"`)}
	result.Paid = strings.Trim(strings.TrimSpace(string(parsed.Status)), `"`) == "1"
	if money := strings.Trim(strings.TrimSpace(string(parsed.Money)), `"`); money != "" {
		if value, err := decimal.NewFromString(money); err == nil {
			result.MoneyCNY = value
		}
	}
	return result, nil
}

// ProbeAdtyEpayChannel 探活：按 ProbeMode 调 act=query、act=balance，或查一个不存在的订单。
// HTTP 200 并且返回 JSON 就算通（查不到订单的 code=-1 也算网关正常）。
func ProbeAdtyEpayChannel(ctx context.Context, channel *model.AdtyEpayChannel) error {
	values := url.Values{}
	values.Set("pid", channel.MerchantId)
	values.Set("key", channel.MerchantKey)
	switch channel.ProbeMode {
	case "query":
		values.Set("act", "query")
	case "balance":
		values.Set("act", "balance")
	default:
		values.Set("act", "order")
		values.Set("out_trade_no", "PROBE"+fmt.Sprint(time.Now().Unix()))
	}
	raw, err := adtyEpayGet(ctx, adtyEpayEndpoint(channel, "/api.php")+"?"+values.Encode())
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := common.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("epay probe returned non-JSON")
	}
	return nil
}

func adtyEpayGet(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := adtyEpayHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epay http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
