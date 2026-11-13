package service

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	adtySiteWithdrawMinKey    = "adty_site_withdraw.min_usd"
	adtySiteWithdrawFeeKey    = "adty_site_withdraw.fee_usd"
	adtySiteWithdrawAlipayKey = "adty_site_withdraw.alipay_cny_rate"
	adtySiteWithdrawUSDTKey   = "adty_site_withdraw.usdt_usd_rate"
)

var (
	ErrAdtySiteWithdrawUnset  = errors.New("site withdrawal rules are not configured")
	ErrAdtySiteWithdrawLocked = errors.New("withdrawal can no longer be cancelled")
)

// AdtySiteWithdrawRules is Root configuration. Empty or non-positive values are unset.
type AdtySiteWithdrawRules struct {
	MinUSD        string `json:"min_usd"`
	FeeUSD        string `json:"fee_usd"`
	AlipayCNYRate string `json:"alipay_cny_rate"`
	USDTUSDRate   string `json:"usdt_usd_rate"`
}

type AdtySiteWithdrawInput struct {
	SiteID         int
	OwnerID        int
	AmountUSD      decimal.Decimal
	Method         string
	Network        string
	AccountNo      string
	AccountName    string
	ExpectedRate   decimal.Decimal
	IdempotencyKey string
}

type adtySiteWithdrawConfig struct {
	Min           decimal.Decimal
	Fee           decimal.Decimal
	AlipayCNYRate decimal.Decimal
	AlipayCNYRaw  string
	USDTUSDRate   decimal.Decimal
	USDTUSDRaw    string
}

func SaveAdtySiteWithdrawRules(rules AdtySiteWithdrawRules, operatorID int) error {
	_, minRaw, err := requiredPositiveDecimal(rules.MinUSD)
	if err != nil {
		return ErrAdtySiteWithdrawUnset
	}
	_, feeRaw, err := requiredPositiveDecimal(rules.FeeUSD)
	if err != nil {
		return ErrAdtySiteWithdrawUnset
	}
	_, alipayRaw, err := requiredPositiveDecimal(rules.AlipayCNYRate)
	if err != nil {
		return ErrAdtySiteWithdrawUnset
	}
	_, usdtRaw, err := requiredPositiveDecimal(rules.USDTUSDRate)
	if err != nil {
		return ErrAdtySiteWithdrawUnset
	}
	values := map[string]string{
		adtySiteWithdrawMinKey:    minRaw,
		adtySiteWithdrawFeeKey:    feeRaw,
		adtySiteWithdrawAlipayKey: alipayRaw,
		adtySiteWithdrawUSDTKey:   usdtRaw,
	}
	_ = operatorID
	return model.UpdateOptionsBulk(values)
}

func requiredPositiveDecimal(raw string) (decimal.Decimal, string, error) {
	trimmed := strings.TrimSpace(raw)
	value, err := decimal.NewFromString(trimmed)
	if err != nil || !value.IsPositive() {
		return decimal.Zero, "", ErrAdtySiteWithdrawUnset
	}
	return value, trimmed, nil
}

func loadAdtySiteWithdrawConfigTx(tx *gorm.DB) (adtySiteWithdrawConfig, error) {
	var cfg adtySiteWithdrawConfig
	var err error
	if cfg.Min, _, err = readPositiveOptionTx(tx, adtySiteWithdrawMinKey); err != nil {
		return cfg, err
	}
	if cfg.Fee, _, err = readPositiveOptionTx(tx, adtySiteWithdrawFeeKey); err != nil {
		return cfg, err
	}
	if cfg.AlipayCNYRate, cfg.AlipayCNYRaw, err = readPositiveOptionTx(tx, adtySiteWithdrawAlipayKey); err != nil {
		return cfg, err
	}
	cfg.USDTUSDRate, cfg.USDTUSDRaw, err = readPositiveOptionTx(tx, adtySiteWithdrawUSDTKey)
	return cfg, err
}

func readPositiveOptionTx(tx *gorm.DB, key string) (decimal.Decimal, string, error) {
	raw, ok, err := model.ReadOptionForUpdate(tx, key)
	if err != nil {
		return decimal.Zero, "", err
	}
	if !ok {
		return decimal.Zero, "", ErrAdtySiteWithdrawUnset
	}
	return requiredPositiveDecimal(raw)
}

// AdtySiteWithdrawRulesView is what the owner page may show. Unset rules stay blank.
type AdtySiteWithdrawRulesView struct {
	Configured    bool     `json:"configured"`
	MinUSD        string   `json:"min_usd,omitempty"`
	FeeUSD        string   `json:"fee_usd,omitempty"`
	AlipayCNYRate string   `json:"alipay_cny_rate,omitempty"`
	USDTUSDRate   string   `json:"usdt_usd_rate,omitempty"`
	Methods       []string `json:"methods"`
}

func LoadAdtySiteWithdrawRules() (AdtySiteWithdrawRulesView, error) {
	view := AdtySiteWithdrawRulesView{Methods: []string{"alipay", "usdt"}}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		cfg, err := loadAdtySiteWithdrawConfigTx(tx)
		if errors.Is(err, ErrAdtySiteWithdrawUnset) {
			return nil
		}
		if err != nil {
			return err
		}
		view.Configured = true
		view.MinUSD = cfg.Min.String()
		view.FeeUSD = cfg.Fee.String()
		view.AlipayCNYRate = cfg.AlipayCNYRaw
		view.USDTUSDRate = cfg.USDTUSDRaw
		return nil
	})
	return view, err
}

func ListAdtySiteWithdrawals(siteID, ownerID int) ([]model.AdtySiteWithdrawal, error) {
	var rows []model.AdtySiteWithdrawal
	err := model.DB.Where("site_id = ? AND owner_user_id = ?", siteID, ownerID).Order("id desc").Limit(20).Find(&rows).Error
	return rows, err
}

func CreateAdtySiteWithdrawal(input AdtySiteWithdrawInput) (*model.AdtySiteWithdrawal, error) {
	method := strings.ToLower(strings.TrimSpace(input.Method))
	if method != "alipay" && method != "usdt" {
		return nil, common.NewMessage("Unsupported withdrawal method")
	}
	amount := input.AmountUSD
	if !amount.IsPositive() {
		return nil, common.NewMessage("Enter a withdrawal amount")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" || len(key) > 128 {
		return nil, common.NewMessage("Invalid idempotency key")
	}
	if method == "usdt" {
		network := strings.ToUpper(strings.TrimSpace(input.Network))
		if !ValidateUSDTAddress(network, input.AccountNo) {
			return nil, common.NewMessage("The address does not match the {{network}} network", map[string]any{"network": network})
		}
		input.Network = network
	} else if strings.TrimSpace(input.AccountName) == "" || strings.TrimSpace(input.AccountNo) == "" {
		return nil, common.NewMessage("Enter the payee name and account")
	}
	var created *model.AdtySiteWithdrawal
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		cfg, err := loadAdtySiteWithdrawConfigTx(tx)
		if err != nil {
			return err
		}
		if amount.LessThan(cfg.Min) || !amount.GreaterThan(cfg.Fee) {
			return common.NewMessage("The amount is below the configured withdrawal minimum")
		}
		var site model.AdtySite
		if err := model.AdtyLockForUpdate(tx).First(&site, input.SiteID).Error; err != nil {
			return err
		}
		if site.OwnerUserId != input.OwnerID || site.Status != model.AdtySiteStatusActive {
			return common.NewMessage("No permission to perform this action")
		}
		var existing []model.AdtySiteWithdrawal
		if err := tx.Where("idempotency_key = ?", key).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if existing[0].SiteId != input.SiteID || !existing[0].AmountUSD.Equal(amount) {
				return model.ErrAdtyIdempotencyConflict
			}
			created = &existing[0]
			return nil
		}
		rate := cfg.AlipayCNYRate
		rateRaw := cfg.AlipayCNYRaw
		currency := "CNY"
		if method == "usdt" {
			rate = cfg.USDTUSDRate
			rateRaw = cfg.USDTUSDRaw
			currency = "USDT"
		}
		if !rate.Equal(input.ExpectedRate) {
			return &RateChangedError{Change: ReferralRateChange{Rate: rate.String(), UpdatedAt: time.Now().Unix()}}
		}
		net := amount.Sub(cfg.Fee)
		payout := net.Mul(rate)
		if _, err := applyAdtySiteWalletChangeTx(tx, site.Id, model.AdtySiteWalletEarnings, amount.Neg(), AdtySiteWalletChange{
			Type: "earning_withdraw_freeze", IdempotencyKey: "site-withdraw:" + key, OperatorID: input.OwnerID, RefType: "site_withdraw", RefID: key,
		}); err != nil {
			return err
		}
		row := &model.AdtySiteWithdrawal{
			SiteId: site.Id, OwnerUserId: input.OwnerID, AmountUSD: amount, FeeUSD: cfg.Fee, NetUSD: net,
			Method: method, Network: input.Network, AccountNo: strings.TrimSpace(input.AccountNo), AccountName: strings.TrimSpace(input.AccountName),
			Rate: rateRaw, PayoutAmount: payout.String(), PayoutCurrency: currency,
			Status: model.AdtySiteWithdrawPending, IdempotencyKey: key,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	return created, err
}

func MarkAdtySiteWithdrawalPaying(id int64, reviewerID int) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
			return err
		}
		if row.Status != model.AdtySiteWithdrawPending {
			return ErrAdtySiteWithdrawLocked
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": model.AdtySiteWithdrawPaying, "reviewer_id": reviewerID, "reviewed_at": time.Now().Unix(),
		}).Error
	})
}

func MarkAdtySiteWithdrawalPaid(id int64, reviewerID int, txID, proof string) error {
	txID = strings.TrimSpace(txID)
	proof = strings.TrimSpace(proof)
	if txID == "" && proof == "" {
		return common.NewMessage("Enter the payout reference")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
			return err
		}
		if row.Status != model.AdtySiteWithdrawPaying {
			return ErrAdtySiteWithdrawLocked
		}
		if row.Method == "usdt" && txID == "" {
			return common.NewMessage("Enter the payout reference")
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": model.AdtySiteWithdrawPaid, "reviewer_id": reviewerID, "tx_id": txID, "proof_note": proof, "paid_at": time.Now().Unix(),
		}).Error
	})
}

func CancelAdtySiteWithdrawal(siteID, ownerID int, id int64) error {
	return releaseAdtySiteWithdrawal(id, siteID, ownerID, model.AdtySiteWithdrawCancelled, true)
}

func RejectAdtySiteWithdrawal(id int64, reviewerID int, reason string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
			return err
		}
		if row.Status != model.AdtySiteWithdrawPending && row.Status != model.AdtySiteWithdrawPaying {
			return ErrAdtySiteWithdrawLocked
		}
		if _, err := applyAdtySiteWalletChangeTx(tx, row.SiteId, model.AdtySiteWalletEarnings, row.AmountUSD, AdtySiteWalletChange{
			Type: "earning_withdraw_release", IdempotencyKey: "site-withdraw-release:" + row.IdempotencyKey, OperatorID: reviewerID, Reason: reason, RefType: "site_withdraw", RefID: row.IdempotencyKey,
		}); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{
			"status": model.AdtySiteWithdrawRejected, "reviewer_id": reviewerID, "reviewed_at": time.Now().Unix(), "proof_note": reason,
		}).Error
	})
}

func releaseAdtySiteWithdrawal(id int64, siteID, ownerID int, status string, ownerOnly bool) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtySiteWithdrawal
		if err := model.AdtyLockForUpdate(tx).First(&row, id).Error; err != nil {
			return err
		}
		if row.SiteId != siteID || (ownerOnly && row.OwnerUserId != ownerID) {
			return common.NewMessage("No permission to perform this action")
		}
		if row.Status != model.AdtySiteWithdrawPending {
			return ErrAdtySiteWithdrawLocked
		}
		if _, err := applyAdtySiteWalletChangeTx(tx, row.SiteId, model.AdtySiteWalletEarnings, row.AmountUSD, AdtySiteWalletChange{
			Type: "earning_withdraw_release", IdempotencyKey: "site-withdraw-release:" + row.IdempotencyKey, OperatorID: ownerID, RefType: "site_withdraw", RefID: row.IdempotencyKey,
		}); err != nil {
			return err
		}
		return tx.Model(&row).Update("status", status).Error
	})
}
