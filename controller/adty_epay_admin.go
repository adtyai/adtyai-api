package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// 支付通道后台（任务 5.4）。查看和启停按权限；新增、编辑（含密钥）和删除只有超级管理员。

type adtyEpayChannelView struct {
	model.AdtyEpayChannel
	HasKey   bool              `json:"has_key"`
	TodayCNY map[string]string `json:"today_cny"`
}

// GetAdtyEpayChannels 列出通道，带今天每种方式已到账的人民币。密钥不返回。
func GetAdtyEpayChannels(c *gin.Context) {
	channels, err := model.ListAdtyEpayChannels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now().Unix()
	dayStart := time.Unix(now, 0)
	start := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(), 0, 0, 0, 0, dayStart.Location()).Unix()
	out := make([]adtyEpayChannelView, 0, len(channels))
	for _, channel := range channels {
		today := map[string]string{}
		for _, m := range channel.Methods {
			sum, err := model.SumAdtyEpayPaidToday(channel.Id, m.Method, start)
			if err == nil {
				today[m.Method] = sum.StringFixed(2)
			}
		}
		out = append(out, adtyEpayChannelView{AdtyEpayChannel: channel, HasKey: channel.MerchantKey != "", TodayCNY: today})
	}
	common.ApiSuccess(c, out)
}

type adtyEpayChannelRequest struct {
	Name            string `json:"name"`
	GatewayURL      string `json:"gateway_url"`
	MerchantId      string `json:"merchant_id"`
	MerchantKey     string `json:"merchant_key"`
	Priority        int    `json:"priority"`
	Weight          int    `json:"weight"`
	OrderMode       string `json:"order_mode"`
	ProbeMode       string `json:"probe_mode"`
	UnpaidThreshold int    `json:"unpaid_threshold"`
	Methods         []struct {
		Method        string          `json:"method"`
		TypeCode      string          `json:"type_code"`
		MinCNY        decimal.Decimal `json:"min_cny"`
		MaxCNY        decimal.Decimal `json:"max_cny"`
		DailyLimitCNY decimal.Decimal `json:"daily_limit_cny"`
		Enabled       bool            `json:"enabled"`
		ReturnType    string          `json:"return_type"`
	} `json:"methods"`
}

func (req *adtyEpayChannelRequest) toChannel() (*model.AdtyEpayChannel, error) {
	req.GatewayURL = strings.TrimSpace(req.GatewayURL)
	if strings.TrimSpace(req.Name) == "" || req.GatewayURL == "" || strings.TrimSpace(req.MerchantId) == "" {
		return nil, common.NewMessage("Name, gateway address and merchant ID are required")
	}
	if !strings.HasPrefix(req.GatewayURL, "https://") && !strings.HasPrefix(req.GatewayURL, "http://") {
		return nil, common.NewMessage("The gateway address must start with http:// or https://")
	}
	if req.OrderMode != model.AdtyEpayOrderModeSubmit {
		req.OrderMode = model.AdtyEpayOrderModeAPI
	}
	switch req.ProbeMode {
	case "query", "balance", "order":
	default:
		req.ProbeMode = "off"
	}
	channel := &model.AdtyEpayChannel{
		Name:            strings.TrimSpace(req.Name),
		GatewayURL:      req.GatewayURL,
		MerchantId:      strings.TrimSpace(req.MerchantId),
		MerchantKey:     strings.TrimSpace(req.MerchantKey),
		Priority:        req.Priority,
		Weight:          max(req.Weight, 1),
		OrderMode:       req.OrderMode,
		ProbeMode:       req.ProbeMode,
		UnpaidThreshold: max(req.UnpaidThreshold, 0),
	}
	enabled := 0
	for _, m := range req.Methods {
		if !service.IsAdtyEpayMethod(m.Method) {
			return nil, common.NewMessage("Unsupported payment method")
		}
		if m.Enabled {
			enabled++
		}
		channel.Methods = append(channel.Methods, model.AdtyEpayChannelMethod{
			Method: m.Method, TypeCode: strings.TrimSpace(m.TypeCode),
			MinCNY: m.MinCNY.Round(2), MaxCNY: m.MaxCNY.Round(2), DailyLimitCNY: m.DailyLimitCNY.Round(2),
			Enabled: m.Enabled, ReturnType: strings.TrimSpace(m.ReturnType),
		})
	}
	if enabled == 0 {
		return nil, common.NewMessage("Enable at least one payment method")
	}
	return channel, nil
}

// CreateAdtyEpayChannel 新建通道（只有超级管理员）。新通道默认停用，测试通过后再启用。
func CreateAdtyEpayChannel(c *gin.Context) {
	var req adtyEpayChannelRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := req.toChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.MerchantKey == "" {
		common.ApiErrorT(c, "The merchant key is required")
		return
	}
	channel.Status = model.AdtyEpayStatusDisabled
	channel.StatusReason = model.AdtyEpayReasonManual
	if err := model.SaveAdtyEpayChannel(channel, false); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "adty.epay_channel.create", map[string]any{"id": channel.Id, "name": channel.Name})
	common.ApiSuccess(c, gin.H{"id": channel.Id})
}

// UpdateAdtyEpayChannel 改通道（只有超级管理员）。密钥留空表示不改。
func UpdateAdtyEpayChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	existing, err := model.GetAdtyEpayChannel(id)
	if err != nil {
		writeAdtyEpayAdminError(c, err)
		return
	}
	var req adtyEpayChannelRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := req.toChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel.Id = id
	channel.Status = existing.Status
	channel.StatusReason = existing.StatusReason
	keepKey := channel.MerchantKey == ""
	if err := model.SaveAdtyEpayChannel(channel, keepKey); err != nil {
		writeAdtyEpayAdminError(c, err)
		return
	}
	recordManageAudit(c, "adty.epay_channel.update", map[string]any{"id": id, "name": channel.Name, "key_changed": !keepKey})
	common.ApiSuccess(c, nil)
}

// DeleteAdtyEpayChannel 删除没有订单的通道（只有超级管理员）。
func DeleteAdtyEpayChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.DeleteAdtyEpayChannel(id); err != nil {
		writeAdtyEpayAdminError(c, err)
		return
	}
	recordManageAudit(c, "adty.epay_channel.delete", map[string]any{"id": id})
	common.ApiSuccess(c, nil)
}

// SetAdtyEpayChannelStatus 启用、停用或手动转维护（支付通道·启停）。
func SetAdtyEpayChannelStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	switch body.Status {
	case model.AdtyEpayStatusEnabled, model.AdtyEpayStatusDisabled, model.AdtyEpayStatusMaintenance:
	default:
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.SetAdtyEpayChannelStatus(id, body.Status, model.AdtyEpayReasonManual); err != nil {
		writeAdtyEpayAdminError(c, err)
		return
	}
	recordManageAudit(c, "adty.epay_channel.status", map[string]any{"id": id, "status": body.Status})
	common.ApiSuccess(c, nil)
}

type adtyEpayTestResult struct {
	Method     string `json:"method"`
	OK         bool   `json:"ok"`
	ReturnType string `json:"return_type"`
	Disabled   bool   `json:"disabled"`
	Message    string `json:"message"`
}

// TestAdtyEpayChannel 每种勾选的方式按最小金额下一笔测试单（订单号 TEST…，不写 top_ups，不计连续未付）。
// 通道回复没开通的方式，自动取消勾选。只对 API 下单的通道有意义。
func TestAdtyEpayChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := model.GetAdtyEpayChannel(id)
	if err != nil {
		writeAdtyEpayAdminError(c, err)
		return
	}
	if channel.OrderMode != model.AdtyEpayOrderModeAPI {
		common.ApiErrorT(c, "Only channels that order through the API can be tested")
		return
	}
	callback := strings.TrimRight(service.GetCallbackAddress(), "/")
	results := make([]adtyEpayTestResult, 0, len(channel.Methods))
	for i := range channel.Methods {
		method := &channel.Methods[i]
		if !method.Enabled {
			continue
		}
		money := method.MinCNY
		if !money.IsPositive() {
			money = decimal.NewFromInt(1)
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		result, err := service.CreateAdtyEpayOrder(ctx, channel, method, service.AdtyEpayCreateRequest{
			TradeNo:   fmt.Sprintf("TEST%d%d", channel.Id, time.Now().UnixNano()%1_000_000_000),
			Name:      "Channel test",
			ClientIP:  c.ClientIP(),
			Device:    "pc",
			MoneyCNY:  money,
			NotifyURL: callback + "/api/user/epay/notify/" + strconv.Itoa(channel.Id),
			ReturnURL: callback + "/wallet",
		})
		cancel()
		item := adtyEpayTestResult{Method: method.Method}
		if err != nil {
			item.Message = err.Error()
			if adtyEpayMethodNotOpen(err) {
				_ = model.SetAdtyEpayMethodEnabled(method.Id, false)
				item.Disabled = true
			}
		} else {
			item.OK = true
			item.ReturnType = result.Mode
			_ = model.UpdateAdtyEpayMethodReturnType(method.Id, result.Mode)
		}
		results = append(results, item)
	}
	recordManageAudit(c, "adty.epay_channel.test", map[string]any{"id": id})
	common.ApiSuccess(c, results)
}

// adtyEpayMethodNotOpen 判断网关的报错是不是「这种方式没开通」。各家措辞不同，按常见关键词匹配。
func adtyEpayMethodNotOpen(err error) bool {
	text := strings.ToLower(err.Error())
	for _, word := range []string{"未开通", "未开启", "不支持", "not open", "not enabled", "not supported", "支付方式"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func writeAdtyEpayAdminError(c *gin.Context, err error) {
	var message *common.Message
	switch {
	case errors.As(err, &message):
		common.ApiError(c, err)
	case errors.Is(err, model.ErrAdtyEpayChannelNotFound):
		common.ApiErrorT(c, "Payment channel not found")
	default:
		common.ApiError(c, err)
	}
}

func init() {
	adtyEpayChannelNames = func() map[int]string {
		channels, err := model.ListAdtyEpayChannels()
		names := map[int]string{}
		if err != nil {
			return names
		}
		for _, channel := range channels {
			names[channel.Id] = channel.Name
		}
		return names
	}
}
