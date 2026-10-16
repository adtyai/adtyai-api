package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

// A43 option keys (adtyai-only; official TelegramBotToken untouched for OAuth).
const (
	OptAdtyTelegramBotToken    = "AdtyTelegramBotToken"
	OptTelegramUrgentChatId    = "TelegramUrgentChatId"
	OptTelegramPendingChatId   = "TelegramPendingChatId"
	OptAdtyAlertUrgentEnabled  = "AdtyAlertUrgentEnabled"
	OptAdtyAlertPendingEnabled = "AdtyAlertPendingEnabled"
	OptAdtyAlertKindEnabled    = "AdtyAlertKindEnabled"
)

// AdtyTelegramBotTokenSecretMeta for GET alerts: secret_set + hm_ fingerprint (no plaintext).
func AdtyTelegramBotTokenSecretMeta() (secretSet bool, fingerprint string) {
	tok := setting.GetAdtyTelegramBotToken()
	if strings.TrimSpace(tok) == "" {
		return false, ""
	}
	fp, err := common.AdtySecretFingerprint(tok)
	if err != nil || fp == "" {
		return true, ""
	}
	return true, "hm_" + fp
}

// AlertA45ThresholdKeys are thresholds owned by business-params (A45); GET aggregates them.
var AlertA45ThresholdKeys = []string{
	"SelfApproveAlertAmount",
	"PayUnknownEscalateHours",
	"AlertMergeMinutes",
	"RefundAlertAmount",
	"RefundAlertDailyCount",
	"DiskAlertWarnPct",
	"DiskAlertCritPct",
	"ChannelBalanceAlertUsd",
	"DailyReportTime",
	"BackupRetentionDays",
	"EpayFailureDemoteThreshold",
}

// AlertsConfigView is the GET /api/admin/alerts payload (never includes bot token plaintext).
type AlertsConfigView struct {
	BotToken struct {
		SecretSet   bool   `json:"secret_set"`
		Fingerprint string `json:"fingerprint,omitempty"`
		OptionKey   string `json:"option_key"`
		Note        string `json:"note"`
	} `json:"bot_token"`
	UrgentChatId       string          `json:"urgent_chat_id"`
	PendingChatId      string          `json:"pending_chat_id"`
	UrgentRouteEnabled bool            `json:"urgent_route_enabled"`
	PendingRouteEnabled bool           `json:"pending_route_enabled"`
	KindEnabled        map[string]bool `json:"kind_enabled"`
	MergeMinutes       int             `json:"merge_minutes"`
	Thresholds         map[string]string `json:"thresholds"`
	ThresholdsNote     string          `json:"thresholds_note"`
}

// BuildAlertsConfigView assembles safe GET payload.
func BuildAlertsConfigView() AlertsConfigView {
	var v AlertsConfigView
	set, fp := AdtyTelegramBotTokenSecretMeta()
	v.BotToken.SecretSet = set
	v.BotToken.Fingerprint = fp
	v.BotToken.OptionKey = OptAdtyTelegramBotToken
	v.BotToken.Note = "Alert-only bot token (AdtyTelegramBotToken). Official TelegramBotToken / system_setting.telegram stay for OAuth and are not modified by this API."
	v.UrgentChatId = setting.GetTelegramUrgentChatId()
	v.PendingChatId = setting.GetTelegramPendingChatId()
	v.UrgentRouteEnabled = setting.IsAdtyAlertUrgentEnabled()
	v.PendingRouteEnabled = setting.IsAdtyAlertPendingEnabled()
	v.KindEnabled = setting.GetAdtyAlertKindEnabled()
	v.MergeMinutes = GetAlertMergeMinutes()
	v.Thresholds = map[string]string{}
	for _, k := range AlertA45ThresholdKeys {
		v.Thresholds[k] = BusinessParamCurrentValue(k)
	}
	v.ThresholdsNote = "Thresholds live in A45 business-params; GET aggregates current values. PUT may update AlertMergeMinutes only via this page; other thresholds stay on A45."
	return v
}

// AlertsPutInput is the PUT body (pointers = present).
type AlertsPutInput struct {
	BotToken            *string           `json:"bot_token"`
	UrgentChatId        *string           `json:"urgent_chat_id"`
	PendingChatId       *string           `json:"pending_chat_id"`
	UrgentRouteEnabled  *bool             `json:"urgent_route_enabled"`
	PendingRouteEnabled *bool             `json:"pending_route_enabled"`
	KindEnabled         map[string]bool   `json:"kind_enabled"`
	MergeMinutes        *int              `json:"merge_minutes"`
}

// ApplyAlertsPut persists alert options. Empty/omitted bot_token leaves token unchanged.
// Returns old/new audit-safe snapshots (token only fingerprint/set).
func ApplyAlertsPut(in AlertsPutInput) (oldSnap, newSnap map[string]any, err error) {
	oldSnap = alertsAuditSnap()
	updates := map[string]string{}

	if in.BotToken != nil {
		tok := strings.TrimSpace(*in.BotToken)
		if tok != "" {
			updates[OptAdtyTelegramBotToken] = tok
		}
		// empty string → do not change (leave key out of updates)
	}
	if in.UrgentChatId != nil {
		updates[OptTelegramUrgentChatId] = strings.TrimSpace(*in.UrgentChatId)
	}
	if in.PendingChatId != nil {
		updates[OptTelegramPendingChatId] = strings.TrimSpace(*in.PendingChatId)
	}
	if in.UrgentRouteEnabled != nil {
		updates[OptAdtyAlertUrgentEnabled] = strconv.FormatBool(*in.UrgentRouteEnabled)
	}
	if in.PendingRouteEnabled != nil {
		updates[OptAdtyAlertPendingEnabled] = strconv.FormatBool(*in.PendingRouteEnabled)
	}
	if in.KindEnabled != nil {
		merged := setting.GetAdtyAlertKindEnabled()
		for k, v := range in.KindEnabled {
			merged[k] = v
		}
		b, mErr := json.Marshal(merged)
		if mErr != nil {
			return oldSnap, nil, mErr
		}
		updates[OptAdtyAlertKindEnabled] = string(b)
	}
	if in.MergeMinutes != nil {
		val := strconv.Itoa(*in.MergeMinutes)
		if vErr := ValidateBusinessParam("AlertMergeMinutes", val, nil); vErr != nil {
			return oldSnap, nil, vErr
		}
		updates["AlertMergeMinutes"] = val
	}

	if len(updates) == 0 {
		newSnap = alertsAuditSnap()
		return oldSnap, newSnap, nil
	}
	for k, v := range updates {
		if err = UpdateOption(k, v); err != nil {
			return oldSnap, nil, fmt.Errorf("%s: %w", k, err)
		}
	}
	newSnap = alertsAuditSnap()
	return oldSnap, newSnap, nil
}

func alertsAuditSnap() map[string]any {
	set, fp := AdtyTelegramBotTokenSecretMeta()
	return map[string]any{
		"bot_token_secret_set":  set,
		"bot_token_fingerprint": fp,
		"urgent_chat_id":        setting.GetTelegramUrgentChatId(),
		"pending_chat_id":       setting.GetTelegramPendingChatId(),
		"urgent_route_enabled":  setting.IsAdtyAlertUrgentEnabled(),
		"pending_route_enabled": setting.IsAdtyAlertPendingEnabled(),
		"kind_enabled":          setting.GetAdtyAlertKindEnabled(),
		"merge_minutes":         GetAlertMergeMinutes(),
	}
}

// SeedAlertsOptionsIntoOptionMap registers defaults (called from InitOptionMap).
func SeedAlertsOptionsIntoOptionMap() {
	common.OptionMap[OptAdtyTelegramBotToken] = ""
	common.OptionMap[OptTelegramUrgentChatId] = ""
	common.OptionMap[OptTelegramPendingChatId] = ""
	common.OptionMap[OptAdtyAlertUrgentEnabled] = "true"
	common.OptionMap[OptAdtyAlertPendingEnabled] = "true"
	common.OptionMap[OptAdtyAlertKindEnabled] = setting.GetAdtyAlertKindEnabledJSON()
}
