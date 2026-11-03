package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
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
		out = append(out, AdtyMethodInfo{Id: method, Kind: "usdt", Available: configured})
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

func init() {
	RegisterAdtyPaymentProvider(adtyBEpusdtProvider{})
}
