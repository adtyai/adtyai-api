package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// adtyTelegramHTTP 是所有出站 Telegram 调用共用的 client，**必须带 Timeout**。
//
// 投递任务是一条单线程串行的 goroutine（ticker 每分钟一次，逐条发 20 行）。
// 用 `http.DefaultClient`（无 timeout）时，一次 TCP 卡住（对端不响应 / 丢包 /
// DNS 卡住）就会让它永久停在那行，而且：
//   - 队列只增不减；
//   - 没有任何错误日志（函数没返回，SysError 不执行）；
//   - ticker 也不再触发（goroutine 卡在上一次循环里）。
//
// 「没有新消息发出」的外观和「没有消息要发」一模一样，不会自愈。
//
// 所以超时不是「让失败更快」，是让整条重试机制保持活着。
var adtyTelegramHTTP = &http.Client{Timeout: 15 * time.Second}

// adtyTelegramInteractiveHTTP 用于**交互式**调用（保存 bot token 时验证身份）。
//
// 它和投递那个 15 秒的 client 分开，是因为等待的人不同：投递是后台 goroutine，
// 15 秒没人等；而 `getMe` 是用户点了「保存」之后同步等结果——**15 秒会让人以为
// 卡死，然后重复点，产生第二个保存请求。**
var adtyTelegramInteractiveHTTP = &http.Client{Timeout: 5 * time.Second}

// adtyTelegramAPIBase 是出站 API 的根地址，**留作变量给测试替换**。
//
// 写死字面量时，验超时的用例只能自己拼一个 URL 去调 client——那样验的是
// `http.Client` 的通用行为，**不是「这两处调用真的走这个 client」**。
// 同仓库的 `adtyModelsDevURL`（`service/adty_official_price.go`）就是这个形态。
var adtyTelegramAPIBase = "https://api.telegram.org"

// AdtyTelegramSender delivers a notice. Tests leave it unset so no message leaves the process.
//
// chat 是字符串而不是 int64：公告要发到分站自己的频道，站长手里只有 `@their_channel`
// 这类名字，让他们自己去翻数字 id 会让每个分站都来问一遍。数字 id（`-100…`）一样
// 能放进字符串，两种写法都只是 Telegram API 的 chat_id 参数。
var AdtyTelegramSender func(botToken string, chat string, text string) error

func init() {
	if os.Getenv("ADTY_TELEGRAM_ALLOW_NETWORK") == "1" {
		AdtyTelegramGetMe = adtyTelegramGetMeHTTP
		// 之前这里是空的：`AdtyTelegramSender` 全仓没有任何赋值，于是
		// RetryAdtyTelegram 里的发送分支永远进不去，队列只会一路 attempts+1
		// 直到 dead。开关和 getMe 用的是同一个，避免两个「是否允许联网」的口径。
		AdtyTelegramSender = adtyTelegramSendMessageHTTP
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

// QueueAdtyPublicTelegram 把一条对外公告排进公开频道队列。
//
// channel 是目标频道（`@name` 或 `-100…`）。bot 是按 purpose 存的，没有站点维度：
// 主站公告走 admin bot，分站公告走 user bot，站点之间的区别落在 channel 上——
// 那个 bot 必须是目标频道的管理员。以前 purpose 写死成 admin，分站公告只能借用
// 主站 bot，而主站 bot 未必在分站频道里。
//
// 正文先脱敏再入队，不因命中敏感串就拒——和工单那四个字段同一条口径：拒单既不
// 告诉管理员哪句有问题，也挡住了正常发布。私有词（订单/退款/保证金…）仍然拒，
// 因为那些内容本来就不该出现在公开频道，脱敏解决不了「不该发」。
func QueueAdtyPublicTelegram(channel, purpose, body string) error {
	channel, body = strings.TrimSpace(channel), strings.TrimSpace(body)
	if channel == "" || body == "" || (purpose != model.AdtyTelegramPurposeUser && purpose != model.AdtyTelegramPurposeAdmin) {
		return ErrAdtyTelegramInvalid
	}
	if adtyTelegramPrivateWords.MatchString(body) {
		return ErrAdtyTelegramPrivate
	}
	body = redactAdtySecrets(body)
	// EventKey 必须把 channel 算进去：同一段文字发到两个频道是两条投递，
	// 只哈希 body 会让第二条被唯一索引当成重复丢掉。
	sum := sha256.Sum256([]byte(channel + "\n" + body))
	return model.DB.Create(&model.AdtyTelegramDelivery{
		EventKey: "public:" + hex.EncodeToString(sum[:16]),
		Purpose:  purpose, Body: body, Channel: channel,
		Status: model.AdtyTelegramDeliveryWait, Public: true,
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
		if AdtyTelegramSender != nil {
			var bot model.AdtyTelegramBot
			_ = model.DB.Where("purpose = ?", row.Purpose).Limit(1).Find(&bot).Error
			token, err := model.OpenAdtyPurposeSecret("telegram:"+row.Purpose, bot.TokenCipher)
			if err != nil {
				last = "bot token unavailable"
			} else if row.Public {
				// 公开公告发到频道，目标取自行上的 Channel。下面这条分支以前不存在：
				// `!row.Public` 把公开行整个排除在发送之外，它们只会 attempts+1
				// 一路走到 dead，队列等于没有出口。
				if row.Channel == "" {
					last = "channel missing"
				} else if AdtyTelegramSender(token, row.Channel, row.Body) == nil {
					status, last = model.AdtyTelegramDeliverySent, ""
				} else {
					last = "channel send failed"
				}
			} else {
				var binding model.AdtyTelegramBinding
				_ = model.DB.First(&binding, row.BindingId).Error
				if AdtyTelegramSender(token, strconv.FormatInt(binding.TelegramUserId, 10), row.Body) == nil {
					status, last = model.AdtyTelegramDeliverySent, ""
				}
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

// adtyTelegramSendMessageHTTP 发一条消息到 chat（`@name` 或数字 id）。
// 只在 ADTY_TELEGRAM_ALLOW_NETWORK=1 时被装上，测试里永远是 nil。
func adtyTelegramSendMessageHTTP(token, chat, text string) error {
	payload := url.Values{}
	payload.Set("chat_id", chat)
	payload.Set("text", text)
	payload.Set("disable_web_page_preview", "true")
	endpoint := adtyTelegramAPIBase + "/bot" + token + "/sendMessage"
	response, err := adtyTelegramHTTP.PostForm(endpoint, payload)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var body struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram sendMessage status %d", response.StatusCode)
	}
	if err := common.DecodeJson(response.Body, &body); err != nil {
		return err
	}
	if !body.Ok {
		return fmt.Errorf("telegram sendMessage rejected: %s", body.Description)
	}
	return nil
}

// startAdtyTelegramDeliveryTask 定期把待发/失败的投递重试出去。
//
// `RetryAdtyTelegram` 在此之前零生产调用方：入队的行没有任何东西去消费，
// 队列只增不减。这条任务就是它的出口。
func startAdtyTelegramDeliveryTask() {
	// 没有发送实现时，这条任务只会把队列一路推到 dead，而且全程没有错误。
	// 「开关没打开」和「没有消息要发」在外部看起来完全一样，所以必须喊一声。
	//
	// **判据不能只看 sender 为 nil。** 一个本来就不用 Telegram 的部署（本地开发、
	// 不带通知的单体部署、CI）永远是 nil，那样每次启动都会打一条 ERROR——
	// 后果不是多一行日志，是**这条日志从此不被读**。
	// 所以只在「库里真的配了 bot」时才喊：那才是「配了但发不出去」。
	if AdtyTelegramSender == nil && adtyTelegramHasConfiguredBots() {
		common.SysError(common.LogText(
			"telegram delivery is disabled but a bot is configured: ADTY_TELEGRAM_ALLOW_NETWORK is not \"1\", so no message can leave this process; queued rows will be marked failed and eventually dead"))
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := RetryAdtyTelegram(); err != nil {
				common.SysError(common.LogText("telegram delivery task: %s", err.Error()))
			}
		}
	}()
}

func init() {
	adtyMasterTasks = append(adtyMasterTasks, startAdtyTelegramDeliveryTask)
}

func adtyTelegramGetMeHTTP(token string) (int64, string, error) {
	request, err := http.NewRequest(http.MethodGet, adtyTelegramAPIBase+"/bot"+token+"/getMe", nil)
	if err != nil {
		return 0, "", ErrAdtyTelegramUnverified
	}
	response, err := adtyTelegramInteractiveHTTP.Do(request)
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

// adtyTelegramHasConfiguredBots 说明库里是否真的配过「启用且带 token」的 bot。
//
// 它只回答一个问题：**这个部署本来是想发 Telegram 的吗？** 不是的话，
// 「sender 为 nil」是正常状态，不该报警。
//
// 库不可用时返回 false —— 启动阶段库还没好是常态，那种情况下打一条
// 「Telegram 没配」是噪音，而真正的连接问题有它自己的日志。
func adtyTelegramHasConfiguredBots() bool {
	if model.DB == nil {
		return false
	}
	var count int64
	err := model.DB.Model(&model.AdtyTelegramBot{}).
		Where("enabled = ? AND token_cipher <> ?", true, "").
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}
