package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyTopUpMoneyColumn struct {
	Id              int `gorm:"primaryKey"`
	UserId          int `gorm:"index"`
	Amount          int64
	Money           float64
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `gorm:"type:varchar(50)"`
	PaymentProvider string `gorm:"type:varchar(50);default:''"`
	CreateTime      int64
	CompleteTime    int64
	Status          string
}

func (legacyTopUpMoneyColumn) TableName() string { return "top_ups" }

func TestTopUpMoneyExactColumnPreservesLegacyRowsAndExactFace(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&legacyTopUpMoneyColumn{}))
	require.False(t, db.Migrator().HasColumn(&TopUp{}, "money_exact"))

	legacy := TopUp{
		UserId: 7, Amount: 5_000_000, Money: 10.5,
		TradeNo: "legacy-float-money", PaymentMethod: PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe, CreateTime: 100, CompleteTime: 200,
		Status: common.TopUpStatusSuccess,
	}
	require.NoError(t, db.Create(&legacyTopUpMoneyColumn{
		UserId: legacy.UserId, Amount: legacy.Amount, Money: legacy.Money,
		TradeNo: legacy.TradeNo, PaymentMethod: legacy.PaymentMethod,
		PaymentProvider: legacy.PaymentProvider, CreateTime: legacy.CreateTime,
		CompleteTime: legacy.CompleteTime, Status: legacy.Status,
	}).Error)

	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.True(t, db.Migrator().HasColumn(&TopUp{}, "money_exact"))

	var moneyType string
	require.NoError(t, db.Raw(`SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'top_ups' AND column_name = 'money_exact'`).Scan(&moneyType).Error)
	assert.Equal(t, "numeric", moneyType)

	var preserved TopUp
	require.NoError(t, db.Where("trade_no = ?", legacy.TradeNo).First(&preserved).Error)
	assert.Equal(t, legacy.Money, preserved.Money)
	assert.True(t, preserved.MoneyExact.IsZero())
	assert.True(t, preserved.CompatFaceUSD().Equal(decimal.NewFromFloat(legacy.Money)))

	// This 8-decimal face cannot round-trip through float64, which is why
	// the compatibility projection cannot use Money as the exact amount.
	inexact := decimal.RequireFromString("1234567890.12345678")
	_, exact := inexact.Float64()
	require.False(t, exact, "test amount must be inexact in float64")
	require.False(t, decimal.NewFromFloat(inexact.InexactFloat64()).Equal(inexact))

	row := TopUp{
		UserId: 8, Amount: 9_995_000, Money: inexact.InexactFloat64(), MoneyExact: inexact,
		TradeNo: "exact-face-projection", PaymentMethod: "main_user",
		PaymentProvider: "voucher", CreateTime: time.Now().Unix(), CompleteTime: time.Now().Unix(),
		Status: common.TopUpStatusSuccess,
	}
	require.NoError(t, db.Create(&row).Error)

	var loaded TopUp
	require.NoError(t, db.Where("trade_no = ?", row.TradeNo).First(&loaded).Error)
	assert.True(t, loaded.MoneyExact.Equal(inexact))
	assert.True(t, loaded.CompatFaceUSD().Equal(inexact))
	assert.False(t, decimal.NewFromFloat(loaded.Money).Equal(inexact), "legacy Money must not be treated as the exact face")

	var stored string
	require.NoError(t, db.Raw(`SELECT money_exact::text FROM top_ups WHERE trade_no = ?`, row.TradeNo).Scan(&stored).Error)
	assert.True(t, decimal.RequireFromString(stored).Equal(inexact))

	listed, total, err := GetUserTopUps(row.UserId, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, listed, 1)
	assert.Equal(t, row.TradeNo, listed[0].TradeNo)
	assert.True(t, listed[0].MoneyExact.Equal(inexact))
	assert.True(t, listed[0].CompatFaceUSD().Equal(inexact))
	assert.NotZero(t, listed[0].Money, "unified list still exposes the legacy money field")

	payload, err := json.Marshal(listed[0])
	require.NoError(t, err)
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(payload, &asMap))
	assert.Contains(t, asMap, "money")
	assert.Equal(t, "1234567890.12345678", asMap["money_exact"])
}

func TestTopUpCompatFaceUSDFallsBackForOfficialPaymentRows(t *testing.T) {
	row := TopUp{Money: 12.5}
	assert.True(t, row.CompatFaceUSD().Equal(decimal.RequireFromString("12.5")))
	assert.True(t, (&TopUp{}).CompatFaceUSD().IsZero())
	assert.True(t, (*TopUp)(nil).CompatFaceUSD().IsZero())
}
