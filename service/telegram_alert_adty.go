package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

// AdtyTelegramHTTPDoer allows accept tests to stub Telegram Bot API.
type AdtyTelegramHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// AdtyTelegramHTTPClient is the HTTP client used by AdtySendTelegramAlert.
// Accept scripts may replace it with a mock; production uses a short-timeout client.
var AdtyTelegramHTTPClient AdtyTelegramHTTPDoer = &http.Client{Timeout: 10 * time.Second}

// AdtySendTelegramAlert sends a message to the urgent or pending chat.
// When bot/chat is missing, only records ops_alert_events (no HTTP).
// kind should describe the alert; for test use "alert_test".
func AdtySendTelegramAlert(level, kind, text string) error {
	level = strings.ToLower(strings.TrimSpace(level))
	if level != model.OpsAlertLevelUrgent && level != model.OpsAlertLevelPending {
		return fmt.Errorf("level must be urgent or pending")
	}
	if level == model.OpsAlertLevelUrgent && !setting.IsAdtyAlertUrgentEnabled() {
		return fmt.Errorf("urgent route disabled")
	}
	if level == model.OpsAlertLevelPending && !setting.IsAdtyAlertPendingEnabled() {
		return fmt.Errorf("pending route disabled")
	}

	token := strings.TrimSpace(setting.GetAdtyTelegramBotToken())
	var chatID string
	if level == model.OpsAlertLevelUrgent {
		chatID = setting.GetTelegramUrgentChatId()
	} else {
		chatID = setting.GetTelegramPendingChatId()
	}

	payload := map[string]any{
		"level":   level,
		"kind":    kind,
		"text":    text,
		"chat_id": chatID,
	}

	if token == "" || chatID == "" {
		payload["sent"] = false
		payload["reason"] = "bot_token or chat_id not configured"
		_ = model.RecordOpsAlert(level, kind, payload)
		return fmt.Errorf("alert bot not configured: need AdtyTelegramBotToken and %s chat id", level)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	body, _ := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    text,
	})
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		payload["sent"] = false
		payload["reason"] = err.Error()
		_ = model.RecordOpsAlert(level, kind, payload)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := AdtyTelegramHTTPClient.Do(req)
	if err != nil {
		payload["sent"] = false
		payload["reason"] = err.Error()
		_ = model.RecordOpsAlert(level, kind, payload)
		return fmt.Errorf("telegram send failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload["sent"] = false
		payload["http_status"] = resp.StatusCode
		payload["response"] = string(respBody)
		_ = model.RecordOpsAlert(level, kind, payload)
		return fmt.Errorf("telegram API status %d: %s", resp.StatusCode, string(respBody))
	}
	payload["sent"] = true
	payload["http_status"] = resp.StatusCode
	_ = model.RecordOpsAlert(level, kind, payload)
	return nil
}
