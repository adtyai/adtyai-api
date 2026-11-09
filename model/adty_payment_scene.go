package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scene identifiers are configured by Root; business names identify implemented
// accounting flows. The policy contains no merchant credentials.
type AdtyPaymentScene struct {
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Business   string          `json:"business"`
	Enabled    bool            `json:"enabled"`
	Providers  []string        `json:"providers"`
	Methods    []string        `json:"methods"`
	ChannelIDs []int           `json:"channel_ids"`
	Currencies []string        `json:"currencies"`
	SiteScope  string          `json:"site_scope"`
	SiteIDs    []int           `json:"site_ids"`
	UserTypes  []string        `json:"user_types"`
	MinUSD     decimal.Decimal `json:"min_usd"`
	MaxUSD     decimal.Decimal `json:"max_usd"`
}

type AdtyPaymentSceneConfig struct {
	ID        int    `gorm:"primaryKey;autoIncrement:false"`
	Version   int64  `gorm:"not null"`
	Payload   string `gorm:"type:text;not null"`
	UpdatedBy int
	UpdatedAt int64
}

var ErrAdtyPaymentSceneConflict = errors.New("payment scene configuration version conflict")

func seedAdtyPaymentScenes() error {
	scenes := []AdtyPaymentScene{{
		Key: "wallet_topup", Name: "Wallet top-up", Business: "wallet_topup", Enabled: true,
		Providers: []string{PaymentProviderAdtyBEpusdt, PaymentProviderAdtyEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake},
		Methods:   []string{}, ChannelIDs: []int{}, Currencies: []string{}, SiteScope: "all", SiteIDs: []int{}, UserTypes: []string{},
	}}
	payload, err := common.Marshal(scenes)
	if err != nil {
		return err
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&AdtyPaymentSceneConfig{ID: 1, Version: 1, Payload: string(payload)}).Error
}

func GetAdtyPaymentSceneConfig(db *gorm.DB, lock bool) (*AdtyPaymentSceneConfig, error) {
	if lock {
		db = lockForUpdate(db)
	}
	var config AdtyPaymentSceneConfig
	err := db.Where("id = ?", 1).First(&config).Error
	return &config, err
}
