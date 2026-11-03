package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 易支付多通道（任务 5.1）。

const (
	AdtyEpayStatusEnabled     = "enabled"
	AdtyEpayStatusMaintenance = "maintenance"
	AdtyEpayStatusDisabled    = "disabled"

	AdtyEpayReasonManual = "manual"
	AdtyEpayReasonUnpaid = "unpaid"
	AdtyEpayReasonProbe  = "probe"

	AdtyEpayOrderModeAPI    = "api"
	AdtyEpayOrderModeSubmit = "submit"
)

var ErrAdtyEpayChannelNotFound = errors.New("adty epay channel not found")

// AdtyEpayChannel 是一个易支付商户。商户密钥存在 merchant_key 列，不序列化给前端。
type AdtyEpayChannel struct {
	Id              int                     `json:"id"`
	Name            string                  `json:"name" gorm:"type:varchar(128)"`
	GatewayURL      string                  `json:"gateway_url" gorm:"column:gateway_url;type:varchar(512)"`
	MerchantId      string                  `json:"merchant_id" gorm:"type:varchar(128)"`
	MerchantKey     string                  `json:"-" gorm:"column:merchant_key;type:varchar(512)"`
	Priority        int                     `json:"priority"`
	Weight          int                     `json:"weight"`
	OrderMode       string                  `json:"order_mode" gorm:"type:varchar(16)"`
	ProbeMode       string                  `json:"probe_mode" gorm:"type:varchar(16)"`
	Status          string                  `json:"status" gorm:"type:varchar(16);index"`
	StatusReason    string                  `json:"status_reason" gorm:"type:varchar(16)"`
	UnpaidStreak    int                     `json:"unpaid_streak"`
	UnpaidThreshold int                     `json:"unpaid_threshold"`
	ProbeFailCount  int                     `json:"probe_fail_count"`
	ProbeOkCount    int                     `json:"probe_ok_count"`
	LastProbeAt     int64                   `json:"last_probe_at"`
	CreatedAt       int64                   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64                   `json:"updated_at" gorm:"autoUpdateTime"`
	Methods         []AdtyEpayChannelMethod `json:"methods" gorm:"foreignKey:ChannelId"`
}

func (AdtyEpayChannel) TableName() string {
	return "adty_epay_channels"
}

// AdtyEpayChannelMethod 是一个通道上的一种支付方式和它的限额。限额 0 表示不限。
type AdtyEpayChannelMethod struct {
	Id            int             `json:"id"`
	ChannelId     int             `json:"channel_id" gorm:"index"`
	Method        string          `json:"method" gorm:"type:varchar(16)"`
	TypeCode      string          `json:"type_code" gorm:"type:varchar(32)"`
	MinCNY        decimal.Decimal `json:"min_cny" gorm:"column:min_cny;type:decimal(20,2);not null;default:0"`
	MaxCNY        decimal.Decimal `json:"max_cny" gorm:"column:max_cny;type:decimal(20,2);not null;default:0"`
	DailyLimitCNY decimal.Decimal `json:"daily_limit_cny" gorm:"column:daily_limit_cny;type:decimal(20,2);not null;default:0"`
	ReturnType    string          `json:"return_type" gorm:"type:varchar(16)"`
	Enabled       bool            `json:"enabled"`
}

func (AdtyEpayChannelMethod) TableName() string {
	return "adty_epay_channel_methods"
}

// MethodFor 返回这个通道上已启用的某种方式。
func (channel *AdtyEpayChannel) MethodFor(method string) *AdtyEpayChannelMethod {
	for i := range channel.Methods {
		if channel.Methods[i].Method == method && channel.Methods[i].Enabled {
			return &channel.Methods[i]
		}
	}
	return nil
}

// MethodByTypeCode 按回调里的 type 找方式（个别易支付程序的代码和方式名不同）。
func (channel *AdtyEpayChannel) MethodByTypeCode(typeCode string) *AdtyEpayChannelMethod {
	for i := range channel.Methods {
		code := channel.Methods[i].TypeCode
		if code == "" {
			code = channel.Methods[i].Method
		}
		if code == typeCode {
			return &channel.Methods[i]
		}
	}
	return nil
}

// ListAdtyEpayChannels 列出全部通道和它们的方式。
func ListAdtyEpayChannels() ([]AdtyEpayChannel, error) {
	var channels []AdtyEpayChannel
	err := DB.Preload("Methods").Order("priority DESC, id").Find(&channels).Error
	return channels, err
}

// GetAdtyEpayChannel 取一个通道和它的方式。
func GetAdtyEpayChannel(id int) (*AdtyEpayChannel, error) {
	var channels []AdtyEpayChannel
	if err := DB.Preload("Methods").Where("id = ?", id).Limit(1).Find(&channels).Error; err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, ErrAdtyEpayChannelNotFound
	}
	return &channels[0], nil
}

// SaveAdtyEpayChannel 新建或改一个通道，方式整组替换。keepKey 为真时不改商户密钥。
func SaveAdtyEpayChannel(channel *AdtyEpayChannel, keepKey bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		methods := channel.Methods
		channel.Methods = nil
		if channel.Id == 0 {
			if err := tx.Create(channel).Error; err != nil {
				return err
			}
		} else {
			omit := []string{"created_at", "unpaid_streak", "probe_fail_count", "probe_ok_count", "last_probe_at"}
			if keepKey {
				omit = append(omit, "merchant_key")
			}
			result := tx.Model(&AdtyEpayChannel{}).Where("id = ?", channel.Id).Omit(omit...).Select("*").Updates(channel)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrAdtyEpayChannelNotFound
			}
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&AdtyEpayChannelMethod{}).Error; err != nil {
			return err
		}
		for i := range methods {
			methods[i].Id = 0
			methods[i].ChannelId = channel.Id
			if methods[i].TypeCode == "" {
				methods[i].TypeCode = methods[i].Method
			}
		}
		if len(methods) > 0 {
			if err := tx.Create(&methods).Error; err != nil {
				return err
			}
		}
		channel.Methods = methods
		return nil
	})
}

// DeleteAdtyEpayChannel 删除通道。旧订单的回调仍按 channelId 路由，所以有订单的通道只能停用。
func DeleteAdtyEpayChannel(id int) error {
	var count int64
	if err := DB.Model(&AdtyTopupOrder{}).Where("channel_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return common.NewMessage("This channel has orders. Disable it instead of deleting it")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&AdtyEpayChannelMethod{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&AdtyEpayChannel{}).Error
	})
}

// SetAdtyEpayChannelStatus 改通道状态和原因；启用时清零连续未付和探活计数。
func SetAdtyEpayChannelStatus(id int, status, reason string) error {
	updates := map[string]any{"status": status, "status_reason": reason}
	if status == AdtyEpayStatusEnabled {
		updates["status_reason"] = ""
		updates["unpaid_streak"] = 0
		updates["probe_fail_count"] = 0
		updates["probe_ok_count"] = 0
	}
	result := DB.Model(&AdtyEpayChannel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAdtyEpayChannelNotFound
	}
	return nil
}

// UpdateAdtyEpayMethodReturnType 记下这种方式实际返回的是二维码、链接还是 scheme。
func UpdateAdtyEpayMethodReturnType(methodID int, returnType string) error {
	return DB.Model(&AdtyEpayChannelMethod{}).Where("id = ?", methodID).Update("return_type", returnType).Error
}

// SetAdtyEpayMethodEnabled 打开或关闭一种方式（测试下单发现没开通时关掉）。
func SetAdtyEpayMethodEnabled(methodID int, enabled bool) error {
	return DB.Model(&AdtyEpayChannelMethod{}).Where("id = ?", methodID).Update("enabled", enabled).Error
}

// SumAdtyEpayPaidToday 统计一个通道某种方式当天已到账的人民币（每日限额用）。
func SumAdtyEpayPaidToday(channelId int, method string, dayStart int64) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := DB.Table("adty_topup_orders AS a").
		Joins("JOIN top_ups t ON t.trade_no = a.trade_no").
		Where("a.channel_id = ? AND a.method = ? AND t.status = ? AND t.complete_time >= ?", channelId, method, common.TopUpStatusSuccess, dayStart).
		Select("COALESCE(SUM(a.pay_amount), 0)").Scan(&total).Error
	return total, err
}
