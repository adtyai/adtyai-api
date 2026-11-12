package service

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// VerificationScopeAdtyNoticeConfigure covers Bot token and translator credential changes.
const VerificationScopeAdtyNoticeConfigure = "adty.notice.configure"

const (
	AdtyNoticeActionTelegramSave = "telegram.save"
	AdtyNoticeActionTelegramTest = "telegram.test"
	AdtyNoticeActionTranslation  = "translation.save"
	AdtyNoticeActionTranslateTry = "translation.test"
)

// AdtyNoticeContext binds a high-risk notice setting to one target. A changed target invalidates the proof.
type AdtyNoticeContext struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

func adtyIsNoticeVerificationScope(scope string) bool {
	return scope == VerificationScopeAdtyNoticeConfigure
}

func adtyBindNoticeOperation(scope string, fields map[string]json.RawMessage, raw json.RawMessage) (any, bool) {
	if !adtyIsNoticeVerificationScope(scope) {
		return nil, false
	}
	var context AdtyNoticeContext
	if len(fields) != 2 || common.Unmarshal(raw, &context) != nil {
		return nil, false
	}
	switch context.Action {
	case AdtyNoticeActionTelegramSave, AdtyNoticeActionTelegramTest, AdtyNoticeActionTranslation, AdtyNoticeActionTranslateTry:
	default:
		return nil, false
	}
	if context.Target == "" || len(context.Target) > 32 {
		return nil, false
	}
	return context, true
}

// AdtyNoticeOperation builds the step-up operation for a Bot or translator change.
func AdtyNoticeOperation(action, target string) VerificationOperation {
	payload, _ := common.Marshal(AdtyNoticeContext{Action: action, Target: target})
	return VerificationOperation{Scope: VerificationScopeAdtyNoticeConfigure, Context: payload}
}

func init() {
	accessTokenVerificationScopes[VerificationScopeAdtyNoticeConfigure] = "option:write"
}
