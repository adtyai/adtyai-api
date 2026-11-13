package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const adtyTelegramReply = "此机器人仅用于通知，请前往网站操作\n\nThis bot only sends notifications. Please use the website."
const adtyTelegramStaged = "已收到绑定请求，请回到网站确认。\n\nBinding request received. Confirm it on the website."
const adtyTelegramBound = "绑定已确认。\n\nBinding confirmed."

var (
	ErrAdtyTelegramInvalid    = common.NewMessage("Invalid parameters")
	ErrAdtyTelegramForbidden  = common.NewMessage("No permission to perform this action")
	ErrAdtyTelegramUnverified = common.NewMessage("Bot identity is not verified")
	ErrAdtyTelegramPrivate    = common.NewMessage("Private notices stay in a private chat")
	adtyTelegramTokenPattern  = regexp.MustCompile(`^(\d{6,12}):[A-Za-z0-9_-]{20,}$`)
	adtyTelegramPrivateWords  = regexp.MustCompile(`(?i)(订单|退款|保证金|兑换码|refund|deposit|voucher|order)`)
)

// AdtyTelegramGetMe verifies a bot token. The default does not dial Telegram.
var AdtyTelegramGetMe = func(string) (int64, string, error) {
	return 0, "", ErrAdtyTelegramUnverified
}

// AdtyTelegramSender delivers a notice. Tests leave it unset so no message leaves the process.
var AdtyTelegramSender func(botToken string, chatID int64, text string) error

func init() {
	if os.Getenv("ADTY_TELEGRAM_ALLOW_NETWORK") == "1" {
		AdtyTelegramGetMe = adtyTelegramGetMeHTTP
	}
}

type AdtyTelegramBotView struct {
	Purpose           string `json:"purpose"`
	BotId             int64  `json:"bot_id"`
	Username          string `json:"username"`
	TokenMask         string `json:"token_mask"`
	Enabled           bool   `json:"enabled"`
	IdentityVerified  bool   `json:"identity_verified"`
	SavedAt           int64  `json:"saved_at"`
	TestedAt          int64  `json:"tested_at"`
	Pending           int64  `json:"pending"`
	WebhookConfigured bool   `json:"webhook_configured"`
}

type AdtyTelegramSaved struct {
	View          AdtyTelegramBotView `json:"bot"`
	WebhookSecret string              `json:"webhook_secret,omitempty"`
}

type AdtyTelegramMessage struct {
	Text          string `json:"text"`
	ForwardFrom   any    `json:"forward_from"`
	ForwardOrigin any    `json:"forward_origin"`
	From          struct {
		Id       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Chat struct {
		Id   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
}

type AdtyTelegramUpdate struct {
	UpdateId      int64                `json:"update_id"`
	Message       *AdtyTelegramMessage `json:"message"`
	CallbackQuery any                  `json:"callback_query"`
}

type AdtyTelegramBindView struct {
	Status         string `json:"status"`
	TelegramUserId int64  `json:"telegram_user_id"`
	Username       string `json:"username"`
	Purpose        string `json:"purpose"`
}

func ListAdtyTelegramBots() ([]AdtyTelegramBotView, error) {
	out := make([]AdtyTelegramBotView, 0, 2)
	for _, purpose := range []string{model.AdtyTelegramPurposeUser, model.AdtyTelegramPurposeAdmin} {
		view, err := adtyTelegramView(purpose)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func SaveAdtyTelegramBot(purpose, token string, enabled bool) (*AdtyTelegramSaved, error) {
	if purpose != model.AdtyTelegramPurposeUser && purpose != model.AdtyTelegramPurposeAdmin && !strings.HasPrefix(purpose, "site:") {
		return nil, ErrAdtyTelegramInvalid
	}
	token = strings.TrimSpace(token)
	var saved *AdtyTelegramSaved
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var row model.AdtyTelegramBot
		lookup := model.AdtyLockForUpdate(tx).Where("purpose = ?", purpose).Limit(1).Find(&row)
		if lookup.Error != nil {
			return lookup.Error
		}
		secret := ""
		if token != "" {
			match := adtyTelegramTokenPattern.FindStringSubmatch(token)
			if match == nil {
				return ErrAdtyTelegramInvalid
			}
			botId, _ := strconv.ParseInt(match[1], 10, 64)
			username := ""
			verified := false
			if id, name, err := AdtyTelegramGetMe(token); err == nil && id > 0 {
				botId, username, verified = id, name, true
			}
			cipher, err := model.SealAdtyPurposeSecret("telegram:"+purpose, token)
			if err != nil {
				return err
			}
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				return err
			}
			secret = hex.EncodeToString(raw)
			sum := sha256.Sum256([]byte(secret))
			identity := sha256.Sum256([]byte(strconv.FormatInt(botId, 10)))
			row.Purpose = purpose
			row.BotId = botId
			row.Username = username
			row.TokenCipher = cipher
			row.TokenMask = adtyMaskTail(token)
			row.WebhookHash = hex.EncodeToString(sum[:])
			row.IdentityHash = hex.EncodeToString(identity[:])
			row.IdentityVerified = verified
			row.Enabled = enabled
			row.SavedAt = time.Now().Unix()
			if lookup.RowsAffected == 0 {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&row).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.AdtyTelegramBinding{}).
				Where("purpose = ? AND bot_identity <> ?", purpose, row.IdentityHash).
				Update("status", model.AdtyTelegramBindConfirm).Error; err != nil {
				return err
			}
		} else if lookup.RowsAffected == 0 {
			return ErrAdtyTelegramInvalid
		} else if err := tx.Model(&row).Update("enabled", enabled).Error; err != nil {
			return err
		}
		view, err := adtyTelegramViewTx(tx, purpose)
		if err != nil {
			return err
		}
		saved = &AdtyTelegramSaved{View: view, WebhookSecret: secret}
		return nil
	})
	return saved, err
}

func TestAdtyTelegramBot(purpose string, userId int) error {
	if purpose != model.AdtyTelegramPurposeUser && purpose != model.AdtyTelegramPurposeAdmin {
		return ErrAdtyTelegramInvalid
	}
	var row model.AdtyTelegramBot
	if err := model.DB.Where("purpose = ?", purpose).Limit(1).Find(&row).Error; err != nil {
		return err
	}
	if row.Id == 0 || row.TokenCipher == "" {
		return ErrAdtyTelegramInvalid
	}
	if _, _, err := AdtyTelegramGetMe(mustOpenTelegramToken(row)); err != nil && !errors.Is(err, ErrAdtyTelegramUnverified) {
		return err
	}
	if err := model.DB.Model(&row).Update("tested_at", time.Now().Unix()).Error; err != nil {
		return err
	}
	return QueueAdtyTelegramNotice(userId, purpose, "测试通知\n\nTest notice", "telegram-test:"+purpose+":"+strconv.Itoa(userId))
}

func StartAdtyTelegramBind(userId int, purpose string) (string, string, int64, error) {
	if purpose != model.AdtyTelegramPurposeUser && purpose != model.AdtyTelegramPurposeAdmin {
		return "", "", 0, ErrAdtyTelegramInvalid
	}
	var user model.User
	if err := model.DB.Select("id", "site_id", "status", "role").First(&user, userId).Error; err != nil {
		return "", "", 0, err
	}
	if user.Status != common.UserStatusEnabled {
		return "", "", 0, ErrAdtyTelegramForbidden
	}
	if purpose == model.AdtyTelegramPurposeAdmin && user.Role < common.RoleAdminUser {
		var owned int64
		if err := model.DB.Model(&model.AdtySite{}).Where("owner_user_id = ?", user.Id).Count(&owned).Error; err != nil {
			return "", "", 0, err
		}
		if owned == 0 {
			return "", "", 0, ErrAdtyTelegramForbidden
		}
	}
	var bot model.AdtyTelegramBot
	if err := model.DB.Where("purpose = ? AND enabled = ?", purpose, true).Limit(1).Find(&bot).Error; err != nil {
		return "", "", 0, err
	}
	if bot.Id == 0 {
		return "", "", 0, ErrAdtyTelegramInvalid
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", "", 0, err
	}
	code := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(code))
	expires := time.Now().Add(15 * time.Minute).Unix()
	row := model.AdtyTelegramChallenge{
		UserId: user.Id, SiteId: user.SiteId, Purpose: purpose,
		CodeHash: hex.EncodeToString(sum[:]), BotIdentity: bot.IdentityHash, ExpiresAt: expires,
	}
	if err := model.DB.Create(&row).Error; err != nil {
		return "", "", 0, err
	}
	return code, bot.Username, expires, nil
}

func HandleAdtyTelegramUpdate(purpose, secret string, update AdtyTelegramUpdate) (string, error) {
	if purpose != model.AdtyTelegramPurposeUser && purpose != model.AdtyTelegramPurposeAdmin {
		return "", ErrAdtyTelegramInvalid
	}
	var bot model.AdtyTelegramBot
	if err := model.DB.Where("purpose = ?", purpose).Limit(1).Find(&bot).Error; err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(secret))
	if bot.Id == 0 || subtle.ConstantTimeCompare([]byte(bot.WebhookHash), []byte(hex.EncodeToString(sum[:]))) != 1 {
		return "", ErrAdtyTelegramForbidden
	}
	if update.Message == nil || update.Message.Chat.Type != "private" || update.Message.From.Id <= 0 {
		return adtyTelegramReply, nil
	}
	text := strings.TrimSpace(update.Message.Text)
	code, ok := strings.CutPrefix(text, "/start ")
	forwarded := update.Message.ForwardFrom != nil || update.Message.ForwardOrigin != nil || update.CallbackQuery != nil
	if !ok || forwarded || strings.TrimSpace(code) == "" {
		return adtyTelegramReply, nil
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(code)))
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var challenge model.AdtyTelegramChallenge
		lookup := model.AdtyLockForUpdate(tx).Where("code_hash = ? AND purpose = ?", hex.EncodeToString(hash[:]), purpose).Limit(1).Find(&challenge)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 || challenge.UsedAt != 0 || challenge.SeenAt != 0 || challenge.ExpiresAt < time.Now().Unix() || challenge.BotIdentity != bot.IdentityHash {
			return ErrAdtyTelegramInvalid
		}
		return tx.Model(&challenge).Updates(map[string]any{
			"telegram_user_id": update.Message.From.Id, "telegram_username": update.Message.From.Username, "seen_at": time.Now().Unix(),
		}).Error
	})
	if err != nil {
		return adtyTelegramReply, nil
	}
	return adtyTelegramStaged, nil
}

func ConfirmAdtyTelegramBind(userId int, purpose, code string, telegramUserId int64) (*model.AdtyTelegramBinding, error) {
	code = strings.TrimSpace(code)
	if telegramUserId <= 0 || len(code) < 16 {
		return nil, ErrAdtyTelegramInvalid
	}
	sum := sha256.Sum256([]byte(code))
	var binding *model.AdtyTelegramBinding
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := model.AdtyLockForUpdate(tx).First(&user, userId).Error; err != nil {
			return err
		}
		var challenge model.AdtyTelegramChallenge
		lookup := model.AdtyLockForUpdate(tx).Where("code_hash = ?", hex.EncodeToString(sum[:])).Limit(1).Find(&challenge)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 || challenge.UsedAt != 0 || challenge.ExpiresAt < time.Now().Unix() || challenge.SeenAt == 0 {
			return ErrAdtyTelegramInvalid
		}
		if challenge.UserId != user.Id || challenge.SiteId != user.SiteId || challenge.Purpose != purpose || challenge.TelegramUserId != telegramUserId {
			return ErrAdtyTelegramForbidden
		}
		row := model.AdtyTelegramBinding{
			UserId: user.Id, Purpose: purpose, SiteId: user.SiteId,
			TelegramUserId: telegramUserId, BotIdentity: challenge.BotIdentity, Status: model.AdtyTelegramBindActive,
		}
		var existing model.AdtyTelegramBinding
		found := tx.Where("user_id = ? AND purpose = ?", user.Id, purpose).Limit(1).Find(&existing)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			binding = &row
		} else {
			existing.SiteId = user.SiteId
			existing.TelegramUserId = telegramUserId
			existing.BotIdentity = challenge.BotIdentity
			existing.Status = model.AdtyTelegramBindActive
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			binding = &existing
		}
		return tx.Model(&challenge).Update("used_at", time.Now().Unix()).Error
	})
	return binding, err
}

func GetAdtyTelegramBind(userId int, purpose string) (*AdtyTelegramBindView, error) {
	view := &AdtyTelegramBindView{Status: "unbound", Purpose: purpose}
	var row model.AdtyTelegramBinding
	lookup := model.DB.Where("user_id = ? AND purpose = ?", userId, purpose).Limit(1).Find(&row)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return view, nil
	}
	view.Status = row.Status
	view.TelegramUserId = row.TelegramUserId
	return view, nil
}

func QueueAdtyTelegramNotice(userId int, purpose, body, eventKey string) error {
	body = strings.TrimSpace(body)
	if userId <= 0 || strings.TrimSpace(eventKey) == "" || body == "" || adtyTextHasSecret(body) {
		return ErrAdtyTelegramInvalid
	}
	if !strings.Contains(body, "\n\n") {
		body += "\n\nEnglish is not ready"
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var existing model.AdtyTelegramDelivery
		lookup := tx.Where("event_key = ?", eventKey).Limit(1).Find(&existing)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected > 0 {
			return nil
		}
		delivery := model.AdtyTelegramDelivery{
			EventKey: eventKey, UserId: userId, Purpose: purpose, Body: body, Status: model.AdtyTelegramDeliverySkip,
		}
		var binding model.AdtyTelegramBinding
		found := tx.Where("user_id = ? AND purpose = ? AND status = ?", userId, purpose, model.AdtyTelegramBindActive).Limit(1).Find(&binding)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected > 0 {
			var bot model.AdtyTelegramBot
			if err := tx.Where("purpose = ? AND enabled = ?", purpose, true).Limit(1).Find(&bot).Error; err != nil {
				return err
			}
			if bot.Id != 0 && bot.IdentityHash == binding.BotIdentity {
				delivery.Status = model.AdtyTelegramDeliveryWait
				delivery.BindingId = binding.Id
				delivery.SiteId = binding.SiteId
			}
		}
		return tx.Create(&delivery).Error
	})
}

func QueueAdtyPublicTelegram(channel, body string, public bool) error {
	if !public || channel == "" || adtyTelegramPrivateWords.MatchString(body) || adtyTextHasSecret(body) {
		return ErrAdtyTelegramPrivate
	}
	sum := sha256.Sum256([]byte(channel + "\n" + body))
	return model.DB.Create(&model.AdtyTelegramDelivery{
		EventKey: "public:" + hex.EncodeToString(sum[:16]),
		Purpose:  model.AdtyTelegramPurposeAdmin, Body: body, Status: model.AdtyTelegramDeliveryWait, Public: true,
	}).Error
}

func RetryAdtyTelegram() (int, error) {
	var rows []model.AdtyTelegramDelivery
	if err := model.DB.Where("status IN ?", []string{model.AdtyTelegramDeliveryWait, model.AdtyTelegramDeliveryFail}).Limit(20).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		status := model.AdtyTelegramDeliveryFail
		last := "sender unavailable"
		if AdtyTelegramSender != nil && !row.Public {
			var binding model.AdtyTelegramBinding
			var bot model.AdtyTelegramBot
			_ = model.DB.First(&binding, row.BindingId).Error
			_ = model.DB.Where("purpose = ?", row.Purpose).Limit(1).Find(&bot).Error
			token, err := model.OpenAdtyPurposeSecret("telegram:"+row.Purpose, bot.TokenCipher)
			if err == nil && AdtyTelegramSender(token, binding.TelegramUserId, row.Body) == nil {
				status, last = model.AdtyTelegramDeliverySent, ""
			}
		}
		attempts := row.Attempts + 1
		if status != model.AdtyTelegramDeliverySent && attempts >= 5 {
			status = model.AdtyTelegramDeliveryDead
		}
		if err := model.DB.Model(&row).Updates(map[string]any{"status": status, "attempts": attempts, "last_error": last}).Error; err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func adtyTelegramView(purpose string) (AdtyTelegramBotView, error) {
	return adtyTelegramViewTx(model.DB, purpose)
}

func adtyTelegramViewTx(tx *gorm.DB, purpose string) (AdtyTelegramBotView, error) {
	view := AdtyTelegramBotView{Purpose: purpose}
	var row model.AdtyTelegramBot
	if err := tx.Where("purpose = ?", purpose).Limit(1).Find(&row).Error; err != nil {
		return view, err
	}
	if row.Id == 0 {
		return view, nil
	}
	var pending int64
	if err := tx.Model(&model.AdtyTelegramDelivery{}).Where("purpose = ? AND status IN ?", purpose, []string{model.AdtyTelegramDeliveryWait, model.AdtyTelegramDeliveryFail}).Count(&pending).Error; err != nil {
		return view, err
	}
	view.BotId = row.BotId
	view.Username = row.Username
	view.TokenMask = row.TokenMask
	view.Enabled = row.Enabled
	view.IdentityVerified = row.IdentityVerified
	view.SavedAt = row.SavedAt
	view.TestedAt = row.TestedAt
	view.Pending = pending
	view.WebhookConfigured = row.WebhookHash != ""
	return view, nil
}

func adtyMaskTail(value string) string {
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

func mustOpenTelegramToken(row model.AdtyTelegramBot) string {
	plain, err := model.OpenAdtyPurposeSecret("telegram:"+row.Purpose, row.TokenCipher)
	if err != nil {
		return ""
	}
	return plain
}

func adtyTelegramGetMeHTTP(token string) (int64, string, error) {
	request, err := http.NewRequest(http.MethodGet, "https://api.telegram.org/bot"+token+"/getMe", nil)
	if err != nil {
		return 0, "", ErrAdtyTelegramUnverified
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		if response != nil {
			_ = response.Body.Close()
		}
		return 0, "", ErrAdtyTelegramUnverified
	}
	defer response.Body.Close()
	var body struct {
		Ok     bool `json:"ok"`
		Result struct {
			Id       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"result"`
	}
	if common.DecodeJson(response.Body, &body) != nil || !body.Ok || body.Result.Id <= 0 {
		return 0, "", ErrAdtyTelegramUnverified
	}
	return body.Result.Id, body.Result.Username, nil
}
