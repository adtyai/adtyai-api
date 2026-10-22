package service

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const (
	adtyBepusdtUserAgent = "Mozilla/5.0 (compatible; ADTYAI-BEpusdt/1.0)"
	adtyUsdtPayMethod    = "usdt"
)

// Injectable HTTP client for acceptance mocks.
var (
	adtyBepusdtHTTPMu     sync.RWMutex
	adtyBepusdtHTTPClient *http.Client
	adtyBepusdtProbeOK    *bool // nil = unset; acceptance can force
)

func SetAdtyBepusdtHTTPClient(c *http.Client) {
	adtyBepusdtHTTPMu.Lock()
	defer adtyBepusdtHTTPMu.Unlock()
	adtyBepusdtHTTPClient = c
}

func SetAdtyBepusdtProbeOverride(ok *bool) {
	adtyBepusdtHTTPMu.Lock()
	defer adtyBepusdtHTTPMu.Unlock()
	adtyBepusdtProbeOK = ok
}

func adtyBepusdtClient() *http.Client {
	adtyBepusdtHTTPMu.RLock()
	c := adtyBepusdtHTTPClient
	adtyBepusdtHTTPMu.RUnlock()
	if c != nil {
		return c
	}
	if client := GetHttpClient(); client != nil {
		return client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

type AdtyBepusdtCreateOrderRequest struct {
	OrderID     string
	Amount      float64 // fiat amount (USD tiers)
	Currency    string
	TradeType   string
	NotifyURL   string
	RedirectURL string
	Name        string
}

type AdtyBepusdtCreateOrderResponse struct {
	OrderID         string
	PaymentURL      string
	TransactionID   string
	PaymentAddress  string
	PaymentAmount   string // USDT to pay
	PaymentCurrency string
	ExpirationTime  int64 // remaining seconds from create-transaction, not a unix timestamp
	Raw             map[string]interface{}
}

type AdtyBepusdtGatewayError struct {
	StatusCode int
	Body       string
	Endpoint   string
}

func (err AdtyBepusdtGatewayError) Error() string {
	if strings.TrimSpace(err.Body) == "" {
		return fmt.Sprintf("bepusdt %s status %d", err.Endpoint, err.StatusCode)
	}
	return fmt.Sprintf("bepusdt %s status %d: %s", err.Endpoint, err.StatusCode, err.Body)
}

// AdtyBepusdtSign MD5(sorted key=value&... + secret), lowercase hex. Same as BEpusdt.
func AdtyBepusdtSign(values map[string]interface{}, secretKey string) string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if key == "signature" || key == "sign" {
			continue
		}
		if adtyBepusdtStringify(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, adtyBepusdtStringify(values[key])))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + secretKey))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func AdtyVerifyBepusdtSignature(values map[string]interface{}, secretKey string) bool {
	signature := strings.ToLower(adtyBepusdtFirstString(values, "signature", "sign"))
	if signature == "" || strings.TrimSpace(secretKey) == "" {
		return false
	}
	return signature == AdtyBepusdtSign(values, secretKey)
}

func AdtyVerifyBepusdtSignatureWithConfiguredToken(values map[string]interface{}) bool {
	return AdtyVerifyBepusdtSignature(values, setting.GetBepusdtApiToken())
}

// ProbeAdtyBepusdt: GET base URL; 2xx = ok. Override via SetAdtyBepusdtProbeOverride.
func ProbeAdtyBepusdt() (ok bool, reason string) {
	adtyBepusdtHTTPMu.RLock()
	override := adtyBepusdtProbeOK
	adtyBepusdtHTTPMu.RUnlock()
	if override != nil {
		if *override {
			return true, ""
		}
		return false, "探活失败"
	}
	if !setting.IsBepusdtConfigured() {
		return false, "USDT 未配置"
	}
	base := setting.GetBepusdtBaseURL()
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/", nil)
	if err != nil {
		return false, "探活失败"
	}
	req.Header.Set("User-Agent", adtyBepusdtUserAgent)
	resp, err := adtyBepusdtClient().Do(req)
	if err != nil {
		return false, "探活失败"
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		// 2xx–4xx means host reachable (create path may 404 on GET)
		return true, ""
	}
	return false, "探活失败"
}

// IsAdtyUsdtAvailable drives tiers pay_methods usdt.available.
func IsAdtyUsdtAvailable() (ok bool, reason string) {
	if !setting.IsBepusdtConfigured() {
		return false, "支付维护中"
	}
	if len(setting.EnabledUsdtChains()) == 0 {
		return false, "未开通收款网络"
	}
	return ProbeAdtyBepusdt()
}

func CreateAdtyBepusdtOrder(req AdtyBepusdtCreateOrderRequest) (*AdtyBepusdtCreateOrderResponse, error) {
	if !setting.IsBepusdtConfigured() {
		return nil, errors.New("usdt gateway is not configured")
	}
	if req.OrderID == "" || req.Amount <= 0 || req.NotifyURL == "" {
		return nil, errors.New("invalid bepusdt order")
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = setting.GetBepusdtCurrency()
	}
	tradeType := strings.TrimSpace(req.TradeType)
	if tradeType == "" {
		tradeType = setting.GetBepusdtTradeType()
	}
	bodyMap := map[string]interface{}{
		"order_id":     req.OrderID,
		"amount":       req.Amount,
		"fiat":         currency,
		"trade_type":   tradeType,
		"notify_url":   req.NotifyURL,
		"redirect_url": req.RedirectURL,
	}
	if strings.TrimSpace(req.Name) != "" {
		bodyMap["name"] = req.Name
	}
	if bodyMap["redirect_url"] == "" {
		delete(bodyMap, "redirect_url")
	}
	bodyMap["signature"] = AdtyBepusdtSign(bodyMap, setting.GetBepusdtApiToken())
	return createAdtyBepusdtOrderAtPath("/api/v1/order/create-transaction", bodyMap, req.OrderID)
}

func createAdtyBepusdtOrderAtPath(path string, bodyMap map[string]interface{}, fallbackOrderID string) (*AdtyBepusdtCreateOrderResponse, error) {
	body, err := common.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	endpoint, err := adtyBepusdtURL(path)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", adtyBepusdtUserAgent)
	resp, err := adtyBepusdtClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, AdtyBepusdtGatewayError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respBody)), Endpoint: path}
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(respBody, &payload); err != nil {
		return nil, err
	}
	if adtyBepusdtBusinessCodeFailed(payload) {
		return nil, AdtyBepusdtGatewayError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respBody)), Endpoint: path}
	}
	data := adtyExtractBepusdtData(payload)
	result := &AdtyBepusdtCreateOrderResponse{
		OrderID:         adtyBepusdtFirstString(data, "order_id", "orderId"),
		PaymentURL:      adtyBepusdtFirstString(data, "payment_url", "paymentUrl", "pay_url", "payUrl", "checkout_url", "url"),
		TransactionID:   adtyBepusdtFirstString(data, "trade_id", "tradeId", "transaction_id", "transactionId", "tx_id"),
		PaymentAddress:  adtyBepusdtFirstString(data, "token", "receive_address", "receiveAddress", "payment_address", "paymentAddress", "address"),
		PaymentAmount:   adtyBepusdtFirstString(data, "actual_amount", "payment_amount", "paymentAmount"),
		PaymentCurrency: adtyBepusdtFirstString(data, "payment_currency", "paymentCurrency", "token_name"),
		Raw:             payload,
	}
	// Prefer dedicated address fields over token name
	addr := adtyBepusdtFirstString(data, "receive_address", "receiveAddress", "payment_address", "paymentAddress", "address")
	if addr != "" {
		result.PaymentAddress = addr
	}
	if exp := adtyBepusdtFirstString(data, "expiration_time", "expirationTime", "expire_time", "expireTime"); exp != "" {
		if n, err := strconv.ParseInt(exp, 10, 64); err == nil {
			result.ExpirationTime = n
		}
	}
	if result.OrderID == "" {
		result.OrderID = fallbackOrderID
	}
	if result.PaymentAmount == "" {
		return nil, errors.New("bepusdt response missing payment amount")
	}
	if result.PaymentAddress == "" && result.PaymentURL == "" {
		return nil, errors.New("bepusdt response missing payment address/url")
	}
	return result, nil
}

func adtyBepusdtURL(path string) (string, error) {
	base := strings.TrimRight(setting.GetBepusdtBaseURL(), "/")
	if base == "" {
		return "", errors.New("bepusdt base url is empty")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}

func adtyBepusdtBusinessCodeFailed(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(adtyBepusdtFirstString(payload, "status_code", "code")))
	if code == "" || code == "0" || code == "1" || code == "200" || code == "success" {
		return false
	}
	return true
}

func adtyExtractBepusdtData(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		return data
	}
	return payload
}

func AdtyBepusdtCallbackOrderNo(values map[string]interface{}) string {
	return adtyBepusdtFirstString(values, "order_id", "orderId", "out_trade_no")
}

func AdtyBepusdtCallbackStatus(values map[string]interface{}) string {
	return strings.ToLower(adtyBepusdtFirstString(values, "status", "trade_status", "payment_status"))
}

func AdtyBepusdtCallbackTradeID(values map[string]interface{}) string {
	return adtyBepusdtFirstString(values, "trade_id", "tradeId", "transaction_id", "block_transaction_id", "tx_id")
}

// AdtyBepusdtCallbackPaidUSDT prefers actual_amount (USDT). amount/money is used only when fiat is explicitly USDT.
func AdtyBepusdtCallbackPaidUSDT(values map[string]interface{}) float64 {
	amountText := adtyBepusdtFirstString(values, "actual_amount", "payment_amount", "paid_usdt")
	if amountText == "" {
		if !adtyBepusdtFiatIsUSDT(values) {
			return -1
		}
		amountText = adtyBepusdtFirstString(values, "amount", "money", "paid_amount")
	}
	if amountText == "" {
		return -1
	}
	amount, err := strconv.ParseFloat(amountText, 64)
	if err != nil {
		return -1
	}
	return amount
}

func adtyBepusdtFiatIsUSDT(values map[string]interface{}) bool {
	fiat := strings.ToLower(strings.TrimSpace(adtyBepusdtFirstString(values, "fiat", "currency", "trade_currency", "fiat_currency")))
	return fiat == "usdt"
}

func AdtyIsBepusdtPaidStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "2", "paid", "success", "succeeded", "completed", "confirmed", "trade_success":
		return true
	default:
		return false
	}
}

func adtyBepusdtFirstString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := obj[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case json.Number:
			return v.String()
		}
	}
	return ""
}

func adtyBepusdtStringify(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

// AdtySensitiveOptionFingerprint returns "hm_" + first 8 hex of HMAC(ADTY_SECRET_KEY, plaintext).
func AdtySensitiveOptionFingerprint(plaintext string) (string, error) {
	fp, err := common.AdtySecretFingerprint(plaintext)
	if err != nil {
		return "", err
	}
	return "hm_" + fp, nil
}
