package service

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

var ErrAdtyEpayNoChannel = errors.New("no epay channel can take this payment")

// adtyEpayRand 是挑通道用的随机源，测试可以换成固定种子。
var adtyEpayRand = func() *rand.Rand { return rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)) }

// adtyEpaySumToday 统计当天已到账金额，测试可以替换。
var adtyEpaySumToday = model.SumAdtyEpayPaidToday

// adtyDayStart 是 now 所在自然日的 0 点（服务器时区）。
func adtyDayStart(now int64) int64 {
	t := time.Unix(now, 0)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
}

// PickAdtyEpayChannel 按规则挑通道：只看启用的、支持这种方式的、金额在单笔范围内、
// 当天加上这笔不超过每日限额的通道 → 取优先级最高的一组 → 电脑上如果这组里有能返回二维码的，
// 就只在它们里面挑 → 按权重随机。
func PickAdtyEpayChannel(method string, payCNY decimal.Decimal, device string, exclude map[int]bool, now int64) (*model.AdtyEpayChannel, *model.AdtyEpayChannelMethod, error) {
	channels, err := model.ListAdtyEpayChannels()
	if err != nil {
		return nil, nil, err
	}
	type candidate struct {
		channel *model.AdtyEpayChannel
		method  *model.AdtyEpayChannelMethod
	}
	var eligible []candidate
	dayStart := adtyDayStart(now)
	for i := range channels {
		channel := &channels[i]
		if channel.Status != model.AdtyEpayStatusEnabled || exclude[channel.Id] {
			continue
		}
		m := channel.MethodFor(method)
		if m == nil {
			continue
		}
		if m.MinCNY.IsPositive() && payCNY.LessThan(m.MinCNY) {
			continue
		}
		if m.MaxCNY.IsPositive() && payCNY.GreaterThan(m.MaxCNY) {
			continue
		}
		if m.DailyLimitCNY.IsPositive() {
			paid, err := adtyEpaySumToday(channel.Id, method, dayStart)
			if err != nil {
				return nil, nil, err
			}
			if paid.Add(payCNY).GreaterThan(m.DailyLimitCNY) {
				continue
			}
		}
		eligible = append(eligible, candidate{channel: channel, method: m})
	}
	if len(eligible) == 0 {
		return nil, nil, ErrAdtyEpayNoChannel
	}
	top := eligible[0].channel.Priority
	for _, c := range eligible {
		top = max(top, c.channel.Priority)
	}
	var group []candidate
	for _, c := range eligible {
		if c.channel.Priority == top {
			group = append(group, c)
		}
	}
	if device != "mobile" {
		var qr []candidate
		for _, c := range group {
			if c.method.ReturnType == "qrcode" {
				qr = append(qr, c)
			}
		}
		if len(qr) > 0 {
			group = qr
		}
	}
	total := 0
	for _, c := range group {
		total += max(c.channel.Weight, 1)
	}
	roll := adtyEpayRand().IntN(total)
	for _, c := range group {
		roll -= max(c.channel.Weight, 1)
		if roll < 0 {
			return c.channel, c.method, nil
		}
	}
	last := group[len(group)-1]
	return last.channel, last.method, nil
}
