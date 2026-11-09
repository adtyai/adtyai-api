package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
)

// adtyBEpusdtProvider 把 BEpusdt 登记进钱包的支付商（任务 4.2）。
type adtyBEpusdtProvider struct{}

func (adtyBEpusdtProvider) Name() string { return AdtyProviderBEpusdt }

// Methods 列出全部 USDT 链；没启用或没配置的标为不可用。
func (adtyBEpusdtProvider) Methods() []AdtyMethodInfo {
	configured := setting.BEpusdtConfigured()
	out := make([]AdtyMethodInfo, 0, len(AdtyUSDTMethods))
	for _, method := range AdtyUSDTMethods {
		if !setting.BEpusdtChainEnabled(method) {
			continue
		}
		out = append(out, AdtyMethodInfo{Id: method, Kind: "usdt", Currency: "USDT", Available: configured})
	}
	return out
}

func (adtyBEpusdtProvider) AvailableFor(method string, quote *AdtyQuote) bool {
	return setting.BEpusdtConfigured() && setting.BEpusdtChainEnabled(method) && quote != nil && quote.PayAmount.IsPositive()
}

// Cancel 调网关取消；还没拿到 trade_id 的订单不用取消。
func (adtyBEpusdtProvider) Cancel(ext *model.AdtyTopupOrder) error {
	if ext == nil {
		return nil
	}
	return CancelBEpusdtOrder(ext.GatewayTradeId)
}

// Query uses BEpusdt's public checkout Info contract. Its response has no
// signature: trust only the original HTTPS origin and bind every payment fact
// to the frozen local order. Disabling new checkouts does not disable recovery.
func (adtyBEpusdtProvider) Query(ctx context.Context, ext *model.AdtyTopupOrder) (*model.AdtyPaymentFacts, error) {
	if ext == nil || ext.Provider != model.PaymentProviderAdtyBEpusdt || ext.TradeNo == "" ||
		ext.GatewayTradeId == "" || ext.PayAddress == "" || ext.PayURL == "" ||
		!strings.HasPrefix(ext.Method, "usdt.") || ext.PayCurrency != "USDT" || !ext.PayAmount.IsPositive() {
		return nil, model.ErrPaymentMethodMismatch
	}
	endpoint, err := url.Parse(strings.TrimSpace(setting.GetBEpusdtSetting().BaseURL))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("original HTTPS BEpusdt gateway is unavailable")
	}
	checkout, err := url.Parse(ext.PayURL)
	if err != nil || checkout.Scheme != "https" || checkout.User != nil || !strings.EqualFold(checkout.Host, endpoint.Host) {
		return nil, errors.New("original HTTPS BEpusdt gateway is unavailable")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1/pay/info"
	endpoint.RawPath = ""
	payload, err := common.Marshal(map[string]string{"trade_id": ext.GatewayTradeId})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("BEpusdt order query failed")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := *bepusdtHTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	transport, ok := baseTransport.(*http.Transport)
	if !ok || transport.DialTLS != nil || transport.DialTLSContext != nil {
		return nil, errors.New("verified BEpusdt HTTPS transport is unavailable")
	}
	// Relay's global TLS opt-out must not authenticate an unsigned payment
	// response. Keep configured trust roots but verify this gateway's hostname.
	transport = transport.Clone()
	tlsConfig := &tls.Config{}
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
	}
	tlsConfig.InsecureSkipVerify = false
	tlsConfig.ServerName = ""
	transport.TLSClientConfig = tlsConfig
	client.Transport = transport
	defer transport.CloseIdleConnections()
	if client.Timeout <= 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("BEpusdt order query failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("BEpusdt order query failed")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("BEpusdt order query returned an invalid response")
	}
	var result struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			TradeID      string          `json:"trade_id"`
			OrderID      string          `json:"order_id"`
			TradeType    string          `json:"trade_type"`
			Status       *int            `json:"status"`
			Money        json.RawMessage `json:"money"`
			ActualAmount json.RawMessage `json:"actual_amount"`
			Token        string          `json:"token"`
			Fiat         string          `json:"fiat"`
		} `json:"data"`
	}
	if common.Unmarshal(raw, &result) != nil || result.StatusCode != http.StatusOK || result.Data.Status == nil ||
		*result.Data.Status < 1 || *result.Data.Status > 6 {
		return nil, errors.New("BEpusdt order query returned an invalid response")
	}
	data := result.Data
	if data.TradeID != ext.GatewayTradeId || data.OrderID != ext.TradeNo || data.TradeType != ext.Method || data.Token != ext.PayAddress || data.Fiat != "USD" {
		return nil, model.ErrPaymentMethodMismatch
	}
	if *data.Status != 2 {
		return nil, nil
	}
	fiat, fiatErr := decimalFromJSON(data.Money)
	amount, amountErr := decimalFromJSON(data.ActualAmount)
	ratio := ext.TopupGroupRatio
	if !ratio.IsPositive() {
		ratio = decimal.NewFromInt(1)
	}
	if fiatErr != nil || amountErr != nil || !fiat.IsPositive() || !amount.IsPositive() ||
		!fiat.Equal(ext.FaceUSD.Mul(ratio).Round(2)) || !amount.Equal(ext.PayAmount) {
		return nil, model.ErrPaymentAmountMismatch
	}
	return &model.AdtyPaymentFacts{
		Provider: ext.Provider, Method: ext.Method, PaidAmount: amount, FiatAmount: fiat,
		PaidCurrency: "USDT", PayAddress: data.Token, GatewayTradeId: data.TradeID,
		Source: model.AdtyPaymentSourceQuery,
	}, nil
}

func init() {
	RegisterAdtyPaymentProvider(adtyBEpusdtProvider{})
}
