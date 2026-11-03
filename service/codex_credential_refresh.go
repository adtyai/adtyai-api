package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

type CodexCredentialRefreshOptions struct {
	ResetCaches bool
}

type CodexOAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	AccountID   string `json:"account_id,omitempty"`
	LastRefresh string `json:"last_refresh,omitempty"`
	Email       string `json:"email,omitempty"`
	Type        string `json:"type,omitempty"`
	Expired     string `json:"expired,omitempty"`
}

// ErrCodexChannelKeyChanged 表示保存前渠道密钥已经被别的刷新写过，不能再用旧密钥覆盖。
var ErrCodexChannelKeyChanged = errors.New("codex channel key changed before it could be saved")

// SaveCodexChannelKey 只在密钥仍是读出来的那一版时写入。写入失败或被并发刷新抢走都返回错误。
func SaveCodexChannelKey(channelID int, oldKey, newKey string) error {
	result := model.DB.Model(&model.Channel{}).Where("id = ? AND key = ?", channelID, oldKey).Update("key", newKey)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCodexChannelKeyChanged
	}
	return nil
}

func parseCodexOAuthKey(raw string) (*CodexOAuthKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("codex channel: empty oauth key")
	}
	var key CodexOAuthKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	return &key, nil
}

var codexChannelRefreshLocks sync.Map

func codexChannelRefreshLock(channelID int) *sync.Mutex {
	value, _ := codexChannelRefreshLocks.LoadOrStore(channelID, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func codexRefreshLockKey(channelID int) int64 {
	return 0x434f440000000000 + int64(channelID)
}

// lockCodexRefreshAcrossProcesses 挡住另一个主进程同时换同一渠道的票。
func lockCodexRefreshAcrossProcesses(channelID int) (func(), error) {
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return func() {}, nil
	}
	sqlDB, err := model.DB.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", codexRefreshLockKey(channelID)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", codexRefreshLockKey(channelID))
		_ = conn.Close()
	}, nil
}

func RefreshCodexChannelCredential(ctx context.Context, channelID int, opts CodexCredentialRefreshOptions) (*CodexOAuthKey, *model.Channel, error) {
	lock := codexChannelRefreshLock(channelID)
	lock.Lock()
	defer lock.Unlock()
	unlockDB, err := lockCodexRefreshAcrossProcesses(channelID)
	if err != nil {
		return nil, nil, err
	}
	defer unlockDB()

	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, nil, err
	}
	if ch == nil {
		return nil, nil, fmt.Errorf("channel not found")
	}
	if ch.Type != constant.ChannelTypeCodex {
		return nil, nil, fmt.Errorf("channel type is not Codex")
	}

	oauthKey, err := parseCodexOAuthKey(strings.TrimSpace(ch.Key))
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, nil, fmt.Errorf("codex channel: refresh_token is required to refresh credential")
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := RefreshCodexOAuthTokenWithProxy(refreshCtx, oauthKey.RefreshToken, ch.GetSetting().Proxy)
	if err != nil {
		return nil, nil, err
	}

	oauthKey.AccessToken = res.AccessToken
	oauthKey.RefreshToken = res.RefreshToken
	oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
	oauthKey.Expired = res.ExpiresAt.Format(time.RFC3339)
	if strings.TrimSpace(oauthKey.Type) == "" {
		oauthKey.Type = "codex"
	}

	if strings.TrimSpace(oauthKey.AccountID) == "" {
		if accountID, ok := ExtractCodexAccountIDFromJWT(oauthKey.AccessToken); ok {
			oauthKey.AccountID = accountID
		}
	}
	if strings.TrimSpace(oauthKey.Email) == "" {
		if email, ok := ExtractEmailFromJWT(oauthKey.AccessToken); ok {
			oauthKey.Email = email
		}
	}

	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, nil, err
	}

	if err := SaveCodexChannelKey(ch.Id, ch.Key, string(encoded)); err != nil {
		return nil, nil, err
	}

	if opts.ResetCaches {
		model.InitChannelCache()
	}

	return oauthKey, ch, nil
}
