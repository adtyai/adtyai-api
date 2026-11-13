package service

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type AdtyPaymentScenes struct {
	Version         int64                    `json:"version"`
	Scenes          []model.AdtyPaymentScene `json:"scenes"`
	ProviderOptions []AdtyPaymentCapability  `json:"provider_options,omitempty"`
	MethodOptions   []AdtyMethodInfo         `json:"method_options,omitempty"`
}

type AdtyPaymentSceneView struct {
	Key     string           `json:"key"`
	Name    string           `json:"name"`
	MinUSD  decimal.Decimal  `json:"min_usd"`
	MaxUSD  decimal.Decimal  `json:"max_usd"`
	Methods []AdtyMethodInfo `json:"methods"`
}

var adtySceneKey = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var adtySceneCurrency = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,15}$`)

func GetAdtyPaymentScenes() (*AdtyPaymentScenes, error) {
	config, err := model.GetAdtyPaymentSceneConfig(model.DB, false)
	if err != nil {
		return nil, err
	}
	view := &AdtyPaymentScenes{Version: config.Version}
	err = common.UnmarshalJsonStr(config.Payload, &view.Scenes)
	if err == nil {
		view.ProviderOptions = AdtyPaymentCapabilities()
		for _, provider := range AdtyPaymentProviders() {
			view.MethodOptions = append(view.MethodOptions, provider.Methods()...)
		}
	}
	return view, err
}

// SaveAdtyPaymentScenes uses one CAS transaction for configuration and durable
// audit evidence. A log-store outage cannot erase a committed policy change.
func SaveAdtyPaymentScenes(actorID int, expected int64, scenes []model.AdtyPaymentScene, audit model.AuditLog) (*AdtyPaymentScenes, error) {
	if expected <= 0 || len(scenes) > 100 {
		return nil, common.NewMessage("Invalid parameters")
	}
	seen := map[string]bool{}
	for i := range scenes {
		scene := &scenes[i]
		// Keep optional restrictions as arrays in the editor's JSON contract.
		scene.Methods = append([]string{}, scene.Methods...)
		scene.ChannelIDs = append([]int{}, scene.ChannelIDs...)
		scene.Currencies = append([]string{}, scene.Currencies...)
		scene.SiteIDs = append([]int{}, scene.SiteIDs...)
		scene.UserTypes = append([]string{}, scene.UserTypes...)
		scene.Name = strings.TrimSpace(scene.Name)
		if !adtySceneKey.MatchString(scene.Key) || seen[scene.Key] || scene.Name == "" || utf8.RuneCountInString(scene.Name) > 120 ||
			scene.Business != "wallet_topup" || len(scene.Providers) == 0 || len(scene.Providers) > 6 ||
			len(scene.Methods) > 100 || len(scene.ChannelIDs) > 1000 || len(scene.SiteIDs) > 1000 || len(scene.Currencies) > 32 || len(scene.UserTypes) > 6 ||
			!slices.Contains([]string{"all", "main", "all_sites", "selected"}, scene.SiteScope) ||
			(scene.SiteScope == "selected" && len(scene.SiteIDs) == 0) || (scene.SiteScope != "selected" && len(scene.SiteIDs) != 0) ||
			scene.MinUSD.IsNegative() || scene.MaxUSD.IsNegative() || (scene.MaxUSD.IsPositive() && scene.MinUSD.GreaterThan(scene.MaxUSD)) {
			return nil, common.NewMessage("Invalid parameters")
		}
		seen[scene.Key] = true
		for _, amount := range []decimal.Decimal{scene.MinUSD, scene.MaxUSD} {
			if amount.IsPositive() {
				if _, err := common.WalletQuotaFromDecimalStrict(amount.Mul(common.QuotaPerUnitDecimal())); err != nil {
					return nil, common.NewMessage("Invalid parameters")
				}
			}
		}
		for _, provider := range scene.Providers {
			if !model.IsAdtyPaymentProvider(provider) {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
		for _, method := range scene.Methods {
			if len(method) == 0 || len(method) > 255 || strings.TrimSpace(method) != method {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
		for _, currency := range scene.Currencies {
			if !adtySceneCurrency.MatchString(currency) {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
		for _, kind := range scene.UserTypes {
			if !slices.Contains([]string{"main_customer", "site_customer", "site_owner", "affiliate", "admin", "root"}, kind) {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
		for _, id := range scene.SiteIDs {
			var count int64
			if id <= 0 {
				return nil, common.NewMessage("Invalid parameters")
			}
			if err := model.DB.Model(&model.AdtySite{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return nil, err
			}
			if count != 1 {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
		for _, id := range scene.ChannelIDs {
			var count int64
			if id <= 0 || !slices.Contains(scene.Providers, model.PaymentProviderAdtyEpay) {
				return nil, common.NewMessage("Invalid parameters")
			}
			if err := model.DB.Model(&model.AdtyEpayChannel{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return nil, err
			}
			if count != 1 {
				return nil, common.NewMessage("Invalid parameters")
			}
		}
	}
	if scenes == nil {
		scenes = []model.AdtyPaymentScene{}
	}
	payload, err := common.Marshal(scenes)
	if err != nil {
		return nil, err
	}
	if len(payload) > 256<<10 {
		return nil, common.NewMessage("Invalid parameters")
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		config, err := model.GetAdtyPaymentSceneConfig(tx, true)
		if err != nil {
			return err
		}
		if config.Version != expected {
			return model.ErrAdtyPaymentSceneConflict
		}
		var actor model.User
		if err := tx.First(&actor, actorID).Error; err != nil {
			return err
		}
		if actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled || actor.StaffSiteId != 0 {
			return common.NewMessage("Forbidden")
		}
		result := tx.Model(config).Where("version = ?", expected).Updates(map[string]any{"version": expected + 1, "payload": string(payload), "updated_by": actorID, "updated_at": time.Now().Unix()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrAdtyPaymentSceneConflict
		}
		audit.UserId, audit.Username, audit.ActorRole = actor.Id, actor.Username, actor.Role
		audit.Category, audit.Action, audit.Success = model.AuditCategoryOperation, "adty.payment_scenes.update", true
		audit.Other = model.AuditOther{RootInfo: model.AuditFields{"previous_version": expected, "version": expected + 1, "scene_count": len(scenes)}}
		audit = model.PrepareAuditLog(nil, audit)
		return model.AppendAuditIntentTx(tx, audit)
	})
	if err != nil {
		return nil, err
	}
	deliveryContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = model.DeliverAuditIntent(deliveryContext, audit.EventId)
	return &AdtyPaymentScenes{Version: expected + 1, Scenes: scenes}, nil
}

// SQL identities, never client-supplied roles or cached site assignments, decide
// audience eligibility. Employee accounts cannot pay even with a matching rule.
func adtyPaymentIdentity(db *gorm.DB, userID int, lock bool) (*model.User, string, error) {
	if userID <= 0 {
		return nil, "", common.NewMessage("This payment method is not available")
	}
	var user model.User
	query := db
	if lock {
		query = model.AdtyLockForUpdate(query)
	}
	if err := query.First(&user, userID).Error; err != nil {
		return nil, "", err
	}
	if user.Status != common.UserStatusEnabled || user.StaffSiteId != 0 {
		return nil, "", common.NewMessage("This payment method is not available")
	}
	kind := "main_customer"
	switch {
	case user.Role == common.RoleRootUser:
		kind = "root"
	case user.Role == common.RoleAdminUser:
		kind = "admin"
	case user.SiteId > 0:
		kind = "site_customer"
	default:
		var affiliates []model.ReferralAffiliate
		if err := db.Where("user_id = ?", userID).Limit(1).Find(&affiliates).Error; err != nil {
			return nil, "", err
		}
		if len(affiliates) > 0 && affiliates[0].SiteOwnerSince > 0 {
			kind = "site_owner"
		} else if len(affiliates) > 0 && affiliates[0].Status == model.ReferralAffiliateStatusApproved {
			kind = "affiliate"
		}
	}
	return &user, kind, nil
}

func adtySceneAudience(scene model.AdtyPaymentScene, siteID int, kind string) bool {
	if !scene.Enabled || scene.Business != "wallet_topup" || (len(scene.UserTypes) > 0 && !slices.Contains(scene.UserTypes, kind)) {
		return false
	}
	switch scene.SiteScope {
	case "all":
		return true
	case "main":
		return siteID == 0
	case "all_sites":
		return siteID > 0
	case "selected":
		return slices.Contains(scene.SiteIDs, siteID)
	}
	return false
}

func adtySceneMethod(scene model.AdtyPaymentScene, provider, method, currency string) bool {
	return slices.Contains(scene.Providers, provider) && (len(scene.Methods) == 0 || slices.Contains(scene.Methods, method)) &&
		(len(scene.Currencies) == 0 || slices.Contains(scene.Currencies, strings.ToUpper(currency)))
}

func AdtyPaymentScenesForUser(userID int) ([]AdtyPaymentSceneView, error) {
	config, err := GetAdtyPaymentScenes()
	if err != nil {
		return nil, err
	}
	user, kind, err := adtyPaymentIdentity(model.DB, userID, false)
	if err != nil {
		return nil, err
	}
	views := []AdtyPaymentSceneView{}
	for _, scene := range config.Scenes {
		if !adtySceneAudience(scene, user.SiteId, kind) {
			continue
		}
		view := AdtyPaymentSceneView{Key: scene.Key, Name: scene.Name, MinUSD: scene.MinUSD, MaxUSD: scene.MaxUSD, Methods: []AdtyMethodInfo{}}
		for _, provider := range AdtyPaymentProviders() {
			for _, method := range provider.Methods() {
				if !method.Available || !adtySceneMethod(scene, provider.Name(), method.Id, method.Currency) {
					continue
				}
				if method.Kind == "epay" && len(scene.ChannelIDs) > 0 {
					channels, err := model.ListAdtyEpayChannels()
					if err != nil {
						return nil, err
					}
					allowed := false
					for _, channel := range channels {
						if slices.Contains(scene.ChannelIDs, channel.Id) && channel.Status == model.AdtyEpayStatusEnabled && channel.MethodFor(method.Id) != nil {
							allowed = true
							break
						}
					}
					if !allowed {
						continue
					}
				}
				if method.FaceUSD != "" {
					face, err := decimal.NewFromString(method.FaceUSD)
					if err != nil || face.LessThan(scene.MinUSD) || (scene.MaxUSD.IsPositive() && face.GreaterThan(scene.MaxUSD)) {
						continue
					}
				}
				view.Methods = append(view.Methods, method)
			}
		}
		if len(view.Methods) > 0 {
			views = append(views, view)
		}
	}
	return views, nil
}

func authorizeAdtyPaymentScene(db *gorm.DB, config *AdtyPaymentScenes, userID int, key, provider, method string, quote *AdtyQuote, lockUser bool) (*model.AdtyPaymentScene, *model.User, error) {
	if key == "" {
		key = "wallet_topup"
	}
	user, kind, err := adtyPaymentIdentity(db, userID, lockUser)
	if err != nil {
		return nil, nil, err
	}
	for _, scene := range config.Scenes {
		if scene.Key != key {
			continue
		}
		if !adtySceneAudience(scene, user.SiteId, kind) || quote == nil || !quote.FaceUSD.IsPositive() ||
			quote.FaceUSD.LessThan(scene.MinUSD) || (scene.MaxUSD.IsPositive() && quote.FaceUSD.GreaterThan(scene.MaxUSD)) ||
			!adtySceneMethod(scene, provider, method, quote.PayCurrency) {
			break
		}
		return &scene, user, nil
	}
	return nil, nil, common.NewMessage("This payment method is not available")
}

func AuthorizeAdtyPaymentScene(userID int, key, provider, method string, quote *AdtyQuote) (*model.AdtyPaymentScene, error) {
	config, err := GetAdtyPaymentScenes()
	if err != nil {
		return nil, err
	}
	scene, _, err := authorizeAdtyPaymentScene(model.DB, config, userID, key, provider, method, quote, false)
	return scene, err
}

// All production payment entry points converge here. Read/quote authorization
// cannot replace this transaction's check of the latest policy and SQL identity.
func CreateAdtyPaymentOrder(topUp *model.TopUp, ext *model.AdtyTopupOrder, key string) error {
	if topUp == nil || ext == nil || topUp.UserId <= 0 || topUp.UserId != ext.UserId {
		return common.NewMessage("Invalid parameters")
	}
	method := ext.Method
	if ext.Provider == model.PaymentProviderCreem {
		method = "creem:" + ext.ProviderProductId
	}
	quote := &AdtyQuote{FaceUSD: ext.FaceUSD, PayAmount: ext.PayAmount, PayCurrency: ext.PayCurrency}
	config := &AdtyPaymentScenes{}
	prepare := func(tx *gorm.DB) error {
		row, err := model.GetAdtyPaymentSceneConfig(tx, true)
		if err != nil {
			return err
		}
		config.Version = row.Version
		if err := common.UnmarshalJsonStr(row.Payload, &config.Scenes); err != nil {
			return err
		}
		if ext.Provider == model.PaymentProviderAdtyEpay {
			var channel model.AdtyEpayChannel
			if ext.ChannelId <= 0 {
				return common.NewMessage("This payment method is not available")
			}
			if err := model.AdtyLockForUpdate(tx).First(&channel, ext.ChannelId).Error; err != nil {
				return err
			}
			if channel.Status != model.AdtyEpayStatusEnabled {
				return common.NewMessage("This payment method is not available")
			}
		}
		return nil
	}
	validate := func(tx *gorm.DB) error {
		scene, user, err := authorizeAdtyPaymentScene(tx, config, topUp.UserId, key, ext.Provider, method, quote, true)
		if err != nil {
			return err
		}
		if user.SiteId != ext.SiteId {
			return common.NewMessage("This payment method is not available")
		}
		// Epay capability is checked on this transaction's locked rows. Its
		// registry probe uses the global pool and must not run inside it.
		if ext.Provider != model.PaymentProviderAdtyEpay {
			provider := AdtyPaymentProviderByName(ext.Provider)
			if provider == nil || !provider.AvailableFor(method, quote) {
				return common.NewMessage("This payment method is not available")
			}
		}
		if ext.Provider == model.PaymentProviderAdtyEpay {
			if len(scene.ChannelIDs) > 0 && !slices.Contains(scene.ChannelIDs, ext.ChannelId) {
				return common.NewMessage("This payment method is not available")
			}
			var channelMethod model.AdtyEpayChannelMethod
			if err := tx.Where("channel_id = ? AND method = ?", ext.ChannelId, method).First(&channelMethod).Error; err != nil {
				return err
			}
			if !channelMethod.Enabled || (channelMethod.MinCNY.IsPositive() && ext.PayAmount.LessThan(channelMethod.MinCNY)) || (channelMethod.MaxCNY.IsPositive() && ext.PayAmount.GreaterThan(channelMethod.MaxCNY)) {
				return common.NewMessage("This payment method is not available")
			}
		}
		snapshot, err := common.Marshal(struct {
			Version  int64                  `json:"version"`
			Scene    model.AdtyPaymentScene `json:"scene"`
			UserRole int                    `json:"user_role"`
			SiteID   int                    `json:"site_id"`
		}{config.Version, *scene, user.Role, user.SiteId})
		if err != nil {
			return err
		}
		ext.PaymentScene, ext.PaymentSceneName, ext.PaymentSceneVersion, ext.PaymentSceneSnapshot = scene.Key, scene.Name, config.Version, string(snapshot)
		return ReserveAdtySiteGiftTx(tx, ext)
	}
	return model.CreateAdtyTopUpWithPolicy(topUp, ext, prepare, validate)
}
