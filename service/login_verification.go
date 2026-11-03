package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const LoginVerificationTTL = 5 * time.Minute

type LoginChallenge struct {
	RequireVerification bool                       `json:"require_verification"`
	FlowToken           string                     `json:"flow_token"`
	ExpiresAt           int64                      `json:"expires_at"`
	Methods             []VerificationMethodOption `json:"methods"`
}

// LegacyGitHubMigration is a GitHub binding rewrite from the stored login name
// to the numeric account ID. On the login path it waits for the login
// verification and is written in the transaction that issues the session.
type LegacyGitHubMigration struct {
	GitHubID string
	LegacyID string
}

// loginFlowPayload stays comparable; completion compares it with the bound copy.
type loginFlowPayload struct {
	AuthVersion int64  `json:"auth_version"`
	LoginMethod string `json:"login_method"`
	// The GitHub binding rewrite waiting for this verification, if any.
	PendingGitHubID       string `json:"pending_github_id,omitempty"`
	PendingGitHubLegacyID string `json:"pending_github_legacy_id,omitempty"`
}

// LoginVerification is server-owned state read from a primary-authenticated flow.
// It is never constructed from a user ID or an authentication claim in a request.
type LoginVerification struct {
	Flow    *model.AuthFlow
	State   *model.UserVerificationState
	payload loginFlowPayload
	// pendingBackupCode 已核对但还没核销。只在建会话的事务里核销。
	pendingBackupCode string
	// pendingTOTPStep 是已核对但还没占用的时间步。0 表示这次不是 TOTP。
	pendingTOTPStep int64
}

func StartLoginVerification(user *model.User, loginMethod string, migration *LegacyGitHubMigration) (*LoginChallenge, error) {
	if user == nil || user.Id <= 0 || user.AuthVersion <= 0 || loginMethod == "" {
		return nil, model.ErrAuthFlowInvalid
	}
	state, err := model.GetUserVerificationState(user.Id)
	if err != nil {
		return nil, err
	}
	if state.Status != common.UserStatusEnabled || state.AuthVersion != user.AuthVersion {
		return nil, model.ErrUserSessionInactive
	}
	methods, err := securityVerificationPolicy(VerificationScopeLogin, *state)
	if err != nil || len(methods) == 0 {
		if err == nil && migration != nil {
			// The rewrite is written only after a verification, so a login that
			// no longer requires one cannot carry it.
			return nil, ErrVerificationUnavailable
		}
		return nil, err
	}
	available := false
	for _, method := range methods {
		available = available || method.Available
	}
	if !available {
		return nil, ErrVerificationUnavailable
	}
	payload := loginFlowPayload{AuthVersion: state.AuthVersion, LoginMethod: loginMethod}
	if migration != nil {
		payload.PendingGitHubID, payload.PendingGitHubLegacyID = migration.GitHubID, migration.LegacyID
	}
	encoded, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(LoginVerificationTTL)
	token, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose: model.AuthFlowPurposeLoginVerification, UserId: user.Id,
		Payload: string(encoded), ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return &LoginChallenge{RequireVerification: true, FlowToken: token, ExpiresAt: expiresAt.Unix(), Methods: methods}, nil
}

func RequireLoginVerification(token, method string) (*LoginVerification, error) {
	flow, err := model.GetAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeLoginVerification})
	if err != nil {
		return nil, err
	}
	var payload loginFlowPayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil || payload.AuthVersion <= 0 || payload.LoginMethod == "" {
		return nil, model.ErrAuthFlowInvalid
	}
	state, err := model.GetUserVerificationState(flow.UserId)
	if err != nil {
		return nil, err
	}
	if state.Status != common.UserStatusEnabled || state.AuthVersion != payload.AuthVersion {
		return nil, model.ErrUserSessionInactive
	}
	if err := requireLoginVerificationMethod(state, method); err != nil {
		return nil, err
	}
	return &LoginVerification{Flow: flow, State: state, payload: payload}, nil
}

func requireLoginVerificationMethod(state *model.UserVerificationState, method string) error {
	methods, err := securityVerificationPolicy(VerificationScopeLogin, *state)
	if err != nil {
		return err
	}
	for _, option := range methods {
		if option.Method != method {
			continue
		}
		if !option.Available {
			return ErrVerificationUnavailable
		}
		return nil
	}
	return ErrProofMethod
}

func VerifyLoginCode(token, code, ip, userAgent string) (*AuthBundle, *LegacyGitHubMigration, error) {
	verification, err := RequireLoginVerification(token, VerificationMethodTwoFA)
	if err != nil {
		return nil, nil, err
	}
	twoFA, err := model.GetTwoFAByUserId(verification.State.UserID)
	if err != nil {
		return nil, nil, err
	}
	backupCode, totpStep, err := CheckLoginTwoFactor(twoFA, code)
	if err != nil {
		return nil, nil, err
	}
	verification.pendingBackupCode = backupCode
	verification.pendingTOTPStep = totpStep
	return CompleteLoginVerification(token, verification, VerificationMethodTwoFA, ip, userAgent)
}

// CompleteLoginVerification must only run after a concrete factor ceremony.
// Recheck the bound version and method while consuming the flow and creating the
// session atomically; a different request cannot reuse this authorization. The
// returned migration is non-nil when the flow's pending GitHub binding rewrite
// was written together with the session.
func CompleteLoginVerification(token string, verification *LoginVerification, method, ip, userAgent string) (*AuthBundle, *LegacyGitHubMigration, error) {
	if verification == nil || verification.Flow == nil || verification.State == nil {
		return nil, nil, model.ErrAuthFlowInvalid
	}
	session, refreshSecret, err := newLoginSession(verification.State.UserID, verification.payload.AuthVersion, verification.payload.LoginMethod, ip, userAgent)
	if err != nil {
		return nil, nil, err
	}
	var migration *LegacyGitHubMigration
	var consumedBackupID int
	var consumedTOTPStep int64
	if err := model.CreateUserSessionFromLoginFlow(token, session, func(tx *gorm.DB, flow *model.AuthFlow, state *model.UserVerificationState) error {
		var payload loginFlowPayload
		if flow.Id != verification.Flow.Id || common.UnmarshalJsonStr(flow.Payload, &payload) != nil || payload != verification.payload {
			return model.ErrAuthFlowInvalid
		}
		if err := requireLoginVerificationMethod(state, method); err != nil {
			return err
		}
		if verification.pendingBackupCode != "" {
			id, err := model.ConsumeUnusedBackupCodeWithTx(tx, state.UserID, verification.pendingBackupCode)
			if err != nil {
				return err
			}
			if id == 0 {
				return ErrVerificationFailed
			}
			if err := model.ResetTwoFAUsageWithTx(tx, state.UserID); err != nil {
				return err
			}
			consumedBackupID = id
		}
		if verification.pendingTOTPStep > 0 {
			if err := model.ClaimTOTPStepWithTx(tx, state.UserID, verification.pendingTOTPStep); err != nil {
				if errors.Is(err, model.ErrTOTPReplay) {
					return ErrVerificationFailed
				}
				return err
			}
			consumedTOTPStep = verification.pendingTOTPStep
		}
		if payload.PendingGitHubID == "" {
			return nil
		}
		// A binding that was relinked while the challenge was open is left as it
		// is and the login continues without the rewrite.
		written, err := model.MigrateLegacyGitHubBindingWithTx(tx, state.UserID, payload.PendingGitHubLegacyID, payload.PendingGitHubID)
		if err != nil {
			return err
		}
		if written {
			migration = &LegacyGitHubMigration{GitHubID: payload.PendingGitHubID, LegacyID: payload.PendingGitHubLegacyID}
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	bundle, err := issueAuthBundle(session, session.SID+"."+refreshSecret, true)
	if err != nil {
		_, _ = model.RevokeUnretainedLoginSession(session.UserID, session.SID, "token_issue_failed", consumedBackupID, consumedTOTPStep)
		return nil, nil, err
	}
	bundle.consumedBackupCodeID = consumedBackupID
	bundle.consumedTOTPStep = consumedTOTPStep
	return bundle, migration, nil
}

// AbandonIssuedLogin 撤掉没有留下的登录会话。备用码和撤会话在同一个事务里还回去。
func AbandonIssuedLogin(userID int, sid, reason string, bundle *AuthBundle) error {
	backupID := 0
	totpStep := int64(0)
	if bundle != nil {
		backupID = bundle.consumedBackupCodeID
		totpStep = bundle.consumedTOTPStep
	}
	_, err := model.RevokeUnretainedLoginSession(userID, sid, reason, backupID, totpStep)
	if err == nil && bundle != nil {
		bundle.consumedBackupCodeID = 0
		bundle.consumedTOTPStep = 0
	}
	return err
}
