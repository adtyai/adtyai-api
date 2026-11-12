package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const AdtyTranslationNotReady = "英文译文未就绪"

var (
	ErrAdtyTranslationInvalid = common.NewMessage("Invalid parameters")
	ErrAdtyTranslationSelf    = common.NewMessage("The translator cannot call this relay")
	adtyTranslationFlight     sync.Mutex
	adtyTranslationInflight   int
	adtyAmountToken           = regexp.MustCompile(`\d+(?:\.\d+)?`)
	adtyURLToken              = regexp.MustCompile(`https?://[^\s]+`)
	adtyPlaceholderToken      = regexp.MustCompile(`\{\{[^}]+\}\}|\{[0-9]+\}|%[sd]`)
	adtyOrderToken            = regexp.MustCompile(`[A-Za-z]{0,6}\d{6,}`)
	adtyNegationSource        = regexp.MustCompile(`不|未|禁止|没有|勿`)
	adtyNegationTarget        = regexp.MustCompile(`(?i)\b(not|no|never|without|cannot|can't|do not)\b`)
)

// AdtyTranslateFunc performs one system translation. The default does not dial a provider or debit a customer.
var AdtyTranslateFunc = func(modelName, text string) (string, error) {
	return "", ErrAdtyTranslationInvalid
}

type AdtyTranslationView struct {
	Mode         string `json:"mode"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"base_url"`
	ModelName    string `json:"model_name"`
	BackupModel  string `json:"backup_model"`
	TokenMask    string `json:"token_mask"`
	TimeoutSec   int    `json:"timeout_sec"`
	Concurrency  int    `json:"concurrency"`
	MaxChars     int    `json:"max_chars"`
	DailyQuota   int    `json:"daily_quota"`
	RetryLimit   int    `json:"retry_limit"`
	Enabled      bool   `json:"enabled"`
	SavedAt      int64  `json:"saved_at"`
	TestedAt     int64  `json:"tested_at"`
	TranslatedAt int64  `json:"translated_at"`
}

type AdtyTranslationInput struct {
	Mode        string `json:"mode"`
	Protocol    string `json:"protocol"`
	BaseURL     string `json:"base_url"`
	ModelName   string `json:"model_name"`
	BackupModel string `json:"backup_model"`
	Token       string `json:"token"`
	TimeoutSec  int    `json:"timeout_sec"`
	Concurrency int    `json:"concurrency"`
	MaxChars    int    `json:"max_chars"`
	DailyQuota  int    `json:"daily_quota"`
	RetryLimit  int    `json:"retry_limit"`
	Enabled     bool   `json:"enabled"`
	RequestHost string `json:"-"`
}

type AdtyTranslationResult struct {
	Source  string `json:"source"`
	English string `json:"english"`
	Status  string `json:"status"`
	Notice  string `json:"notice"`
	Version int    `json:"version"`
	Reused  bool   `json:"reused"`
}

func GetAdtyTranslationConfig() (*AdtyTranslationView, error) {
	var row model.AdtyTranslationConfig
	if err := model.DB.First(&row, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &AdtyTranslationView{}, nil
		}
		return nil, err
	}
	return adtyTranslationView(row), nil
}

func SaveAdtyTranslationConfig(input AdtyTranslationInput) (*AdtyTranslationView, error) {
	input.Mode = strings.TrimSpace(input.Mode)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.ModelName = strings.TrimSpace(input.ModelName)
	if input.Mode != model.AdtyTranslationModeOwn && input.Mode != model.AdtyTranslationModeSet {
		return nil, ErrAdtyTranslationInvalid
	}
	if input.ModelName == "" || input.MaxChars < 0 || input.DailyQuota < 0 || input.RetryLimit < 0 || input.Concurrency < 0 {
		return nil, ErrAdtyTranslationInvalid
	}
	if input.Mode == model.AdtyTranslationModeOwn {
		if err := rejectAdtyTranslationSelf(input.BaseURL, input.RequestHost, input.Token); err != nil {
			return nil, err
		}
	}
	var saved model.AdtyTranslationConfig
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		lookup := model.AdtyLockForUpdate(tx).First(&saved, 1)
		missing := errors.Is(lookup.Error, gorm.ErrRecordNotFound)
		if lookup.Error != nil && !missing {
			return lookup.Error
		}
		saved.Id = 1
		saved.Mode = input.Mode
		saved.Protocol = input.Protocol
		saved.BaseURL = input.BaseURL
		saved.ModelName = input.ModelName
		saved.BackupModel = strings.TrimSpace(input.BackupModel)
		saved.TimeoutSec = input.TimeoutSec
		saved.Concurrency = input.Concurrency
		saved.MaxChars = input.MaxChars
		saved.DailyQuota = input.DailyQuota
		saved.RetryLimit = input.RetryLimit
		saved.Enabled = input.Enabled
		saved.SavedAt = time.Now().Unix()
		if strings.TrimSpace(input.Token) != "" {
			if err := rejectAdtyCustomerToken(input.Token); err != nil {
				return err
			}
			cipher, err := model.SealAdtyPurposeSecret("translation", input.Token)
			if err != nil {
				return err
			}
			saved.TokenCipher = cipher
			saved.TokenMask = adtyMaskTail(input.Token)
		}
		if missing {
			return tx.Create(&saved).Error
		}
		return tx.Save(&saved).Error
	})
	if err != nil {
		return nil, err
	}
	return adtyTranslationView(saved), nil
}

func TestAdtyTranslation(requestHost string) (*AdtyTranslationView, error) {
	cfg, err := loadAdtyTranslationConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Mode == model.AdtyTranslationModeOwn {
		token, _ := model.OpenAdtyPurposeSecret("translation", cfg.TokenCipher)
		if err := rejectAdtyTranslationSelf(cfg.BaseURL, requestHost, token); err != nil {
			return nil, err
		}
	}
	if _, err := callAdtyTranslator(cfg.ModelName, "连接测试"); err != nil {
		return nil, err
	}
	if err := model.DB.Model(cfg).Update("tested_at", time.Now().Unix()).Error; err != nil {
		return nil, err
	}
	cfg.TestedAt = time.Now().Unix()
	return adtyTranslationView(*cfg), nil
}

func TranslateAdtyDynamic(objectKey, source string, siteId int) (*AdtyTranslationResult, error) {
	source = strings.TrimSpace(source)
	objectKey = strings.TrimSpace(objectKey)
	if source == "" || objectKey == "" || adtyTextHasSecret(source) {
		return &AdtyTranslationResult{Source: source, Status: model.AdtyTranslationFailed, Notice: AdtyTranslationNotReady}, nil
	}
	sum := sha256.Sum256([]byte(source))
	hash := hex.EncodeToString(sum[:])
	var head model.AdtyTranslationHead
	lookup := model.DB.Where("object_key = ?", objectKey).Limit(1).Find(&head)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected > 0 && head.SourceHash == hash && head.Status == model.AdtyTranslationReady && head.English != "" {
		return &AdtyTranslationResult{Source: source, English: head.English, Status: head.Status, Version: head.Version, Reused: true}, nil
	}
	version := 1
	if lookup.RowsAffected > 0 {
		version = head.Version + 1
	}
	cfg, err := loadAdtyTranslationConfig()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fallbackAdtyTranslation(objectKey, source, siteId, version, hash, "")
		}
		return nil, err
	}
	if cfg.MaxChars > 0 && len([]rune(source)) > cfg.MaxChars {
		return fallbackAdtyTranslation(objectKey, source, siteId, version, hash, "")
	}
	if cfg.DailyQuota > 0 {
		var usage model.AdtyTranslationUsage
		_ = model.DB.Where("day = ?", time.Now().UTC().Format("2006-01-02")).Limit(1).Find(&usage).Error
		if usage.Calls >= cfg.DailyQuota {
			return fallbackAdtyTranslation(objectKey, source, siteId, version, hash, "")
		}
	}
	english, modelName, callErr := translateWithBackup(cfg, source)
	if callErr != nil || !adtyTranslationKeepsFacts(source, english) {
		result, err := fallbackAdtyTranslation(objectKey, source, siteId, version, hash, modelName)
		return result, err
	}
	var saved model.AdtyTranslationHead
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.AdtyTranslationHead
		found := model.AdtyLockForUpdate(tx).Where("object_key = ?", objectKey).Limit(1).Find(&current)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 && (current.HumanEdited || current.Version > version) {
			saved = current
			return nil
		}
		current.ObjectKey = objectKey
		current.SiteId = siteId
		current.Version = version
		current.SourceHash = hash
		current.Source = source
		current.English = english
		current.Status = model.AdtyTranslationReady
		current.ModelName = modelName
		current.Notice = ""
		if found.RowsAffected == 0 {
			if err := tx.Create(&current).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&current).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.AdtyTranslationJob{ObjectKey: objectKey, Version: version, SourceHash: hash, Status: model.AdtyTranslationReady, ModelName: modelName, FinishedAt: time.Now().Unix()}).Error; err != nil {
			return err
		}
		saved = current
		return tx.Model(&model.AdtyTranslationConfig{}).Where("id = ?", 1).Update("translated_at", time.Now().Unix()).Error
	})
	if err != nil {
		return nil, err
	}
	return &AdtyTranslationResult{Source: saved.Source, English: saved.English, Status: saved.Status, Notice: saved.Notice, Version: saved.Version}, nil
}

func CompleteAdtyTranslationJob(objectKey string, version int, english string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var head model.AdtyTranslationHead
		if err := model.AdtyLockForUpdate(tx).Where("object_key = ?", objectKey).First(&head).Error; err != nil {
			return err
		}
		if head.HumanEdited || head.Version != version {
			return ErrAdtyTranslationInvalid
		}
		if !adtyTranslationKeepsFacts(head.Source, english) {
			return ErrAdtyTranslationInvalid
		}
		return tx.Model(&head).Updates(map[string]any{"english": english, "status": model.AdtyTranslationReady, "notice": ""}).Error
	})
}

func EditAdtyTranslation(objectKey, english string) error {
	english = strings.TrimSpace(english)
	if english == "" {
		return ErrAdtyTranslationInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var head model.AdtyTranslationHead
		if err := model.AdtyLockForUpdate(tx).Where("object_key = ?", objectKey).First(&head).Error; err != nil {
			return err
		}
		return tx.Model(&head).Updates(map[string]any{"english": english, "human_edited": true, "status": model.AdtyTranslationReady, "notice": ""}).Error
	})
}

func AdtyTranslationUsageToday() (*model.AdtyTranslationUsage, error) {
	var row model.AdtyTranslationUsage
	err := model.DB.Where("day = ?", time.Now().UTC().Format("2006-01-02")).Limit(1).Find(&row).Error
	return &row, err
}

func adtyTranslationView(row model.AdtyTranslationConfig) *AdtyTranslationView {
	return &AdtyTranslationView{
		Mode: row.Mode, Protocol: row.Protocol, BaseURL: row.BaseURL, ModelName: row.ModelName, BackupModel: row.BackupModel,
		TokenMask: row.TokenMask, TimeoutSec: row.TimeoutSec, Concurrency: row.Concurrency, MaxChars: row.MaxChars,
		DailyQuota: row.DailyQuota, RetryLimit: row.RetryLimit, Enabled: row.Enabled, SavedAt: row.SavedAt, TestedAt: row.TestedAt, TranslatedAt: row.TranslatedAt,
	}
}

func loadAdtyTranslationConfig() (*model.AdtyTranslationConfig, error) {
	var row model.AdtyTranslationConfig
	if err := model.DB.First(&row, 1).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func rejectAdtyTranslationSelf(baseURL, requestHost, token string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return ErrAdtyTranslationSelf
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, requestHost) {
		return ErrAdtyTranslationSelf
	}
	if strings.Contains(strings.ToLower(parsed.Path), "/v1/") && strings.EqualFold(host, requestHost) {
		return ErrAdtyTranslationSelf
	}
	return rejectAdtyCustomerToken(token)
}

func rejectAdtyCustomerToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	var count int64
	if err := model.DB.Model(&model.Token{}).Where("key = ?", token).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrAdtyTranslationSelf
	}
	return nil
}

func translateWithBackup(cfg *model.AdtyTranslationConfig, source string) (string, string, error) {
	english, err := callAdtyTranslator(cfg.ModelName, source)
	if err == nil {
		return english, cfg.ModelName, nil
	}
	if cfg.RetryLimit > 0 && cfg.BackupModel != "" && cfg.BackupModel != cfg.ModelName {
		english, err = callAdtyTranslator(cfg.BackupModel, source)
		if err == nil {
			return english, cfg.BackupModel, nil
		}
	}
	return "", cfg.ModelName, err
}

func callAdtyTranslator(modelName, text string) (string, error) {
	adtyTranslationFlight.Lock()
	cfgConcurrency := 0
	var cfg model.AdtyTranslationConfig
	_ = model.DB.Select("concurrency").First(&cfg, 1).Error
	cfgConcurrency = cfg.Concurrency
	if cfgConcurrency > 0 && adtyTranslationInflight >= cfgConcurrency {
		adtyTranslationFlight.Unlock()
		return "", ErrAdtyTranslationInvalid
	}
	adtyTranslationInflight++
	adtyTranslationFlight.Unlock()
	started := time.Now()
	english, err := AdtyTranslateFunc(modelName, text)
	adtyTranslationFlight.Lock()
	adtyTranslationInflight--
	adtyTranslationFlight.Unlock()
	_ = noteAdtyTranslationUsage(err != nil, time.Since(started).Milliseconds())
	return english, err
}

func noteAdtyTranslationUsage(failed bool, latency int64) error {
	day := time.Now().UTC().Format("2006-01-02")
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyTranslationUsage
		lookup := tx.Where("day = ?", day).Limit(1).Find(&row)
		if lookup.Error != nil {
			return lookup.Error
		}
		row.Day = day
		row.Calls++
		row.LatencyMs += latency
		if failed {
			row.Failures++
		}
		if lookup.RowsAffected == 0 {
			return tx.Create(&row).Error
		}
		return tx.Save(&row).Error
	})
}

func fallbackAdtyTranslation(objectKey, source string, siteId, version int, hash, modelName string) (*AdtyTranslationResult, error) {
	head := model.AdtyTranslationHead{
		ObjectKey: objectKey, SiteId: siteId, Version: version, SourceHash: hash, Source: source,
		Status: model.AdtyTranslationFailed, Notice: AdtyTranslationNotReady, ModelName: modelName,
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var current model.AdtyTranslationHead
		found := model.AdtyLockForUpdate(tx).Where("object_key = ?", objectKey).Limit(1).Find(&current)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 && (current.HumanEdited || current.Version > version) {
			head = current
			return nil
		}
		head.Id = current.Id
		if found.RowsAffected == 0 {
			return tx.Create(&head).Error
		}
		return tx.Model(&current).Updates(map[string]any{
			"version": version, "source_hash": hash, "source": source, "english": "", "status": model.AdtyTranslationFailed, "notice": AdtyTranslationNotReady, "model_name": modelName,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &AdtyTranslationResult{Source: source, Status: model.AdtyTranslationFailed, Notice: AdtyTranslationNotReady, Version: version}, nil
}

func adtyTranslationKeepsFacts(source, english string) bool {
	if strings.TrimSpace(english) == "" {
		return false
	}
	for _, pattern := range []*regexp.Regexp{adtyAmountToken, adtyURLToken, adtyPlaceholderToken, adtyOrderToken} {
		for _, token := range pattern.FindAllString(source, -1) {
			if !strings.Contains(english, token) {
				return false
			}
		}
	}
	if adtyNegationSource.MatchString(source) && !adtyNegationTarget.MatchString(english) {
		return false
	}
	return true
}

func AdtyTranslationCallsDoNotDebit(userId int) (int, error) {
	var user model.User
	if err := model.DB.Select("quota").First(&user, userId).Error; err != nil {
		return 0, err
	}
	return user.Quota, nil
}
