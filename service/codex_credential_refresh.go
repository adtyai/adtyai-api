package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

const (
	// codexRefreshLockPollInterval 是抢跨进程 advisory lock 的重试间隔。
	codexRefreshLockPollInterval = 200 * time.Millisecond
	// codexRefreshUnlockTimeout 是解锁自己的上限。解锁不能跟着调用方的 ctx 走：
	// 走到这一步时它多半已经到期了，那样解锁必然失败，锁只能靠丢连接回收。
	codexRefreshUnlockTimeout = 5 * time.Second
)

// lockCodexRefreshAcrossProcesses 挡住另一个主进程同时换同一渠道的票。
//
// 拿锁用 pg_try_advisory_lock 而不是 pg_advisory_lock：阻塞型拿锁没有超时，
// 对方进程卡死但连接还在（锁一直不放）时，本次刷新就永久阻塞在 DB 上，还会连带
// 占住 per-channel 的 sync.Mutex，把该渠道之后所有刷新和相关 HTTP 请求一起拖死。
// 这里按 ctx 轮询，调用方的 deadline 到期就返回。
func lockCodexRefreshAcrossProcesses(ctx context.Context, channelID int) (func(), error) {
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return func() {}, nil
	}
	sqlDB, err := model.DB.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	lockKey := codexRefreshLockKey(channelID)
	for {
		var locked bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if locked {
			break
		}
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, fmt.Errorf("codex channel %d: wait for credential refresh lock: %w", channelID, ctx.Err())
		case <-time.After(codexRefreshLockPollInterval):
		}
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), codexRefreshUnlockTimeout)
		defer cancel()
		if _, err := conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
			common.SysError(common.LogText("codex refresh: advisory unlock failed for channel %d: %s", channelID, err.Error()))
			discardCodexRefreshConn(conn)
		}
		_ = conn.Close()
	}, nil
}

// discardCodexRefreshConn 让 database/sql 丢掉这条物理连接，而不是还回池里。
//
// conn.Close() 只是把连接还池，session 级 advisory lock 跟着物理连接走：一次
// 解锁失败就会把锁留在一条会被别的刷新复用的池化连接上。在 Raw 里返回
// driver.ErrBadConn 是 database/sql 标记「这条连接不可再复用」的手段（文档：
// 返回 ErrBadConn 后 Conn 不再可用），物理连接随 Close 一起关掉，锁随之释放。
func discardCodexRefreshConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

func RefreshCodexChannelCredential(ctx context.Context, channelID int, opts CodexCredentialRefreshOptions) (*CodexOAuthKey, *model.Channel, error) {
	lock := codexChannelRefreshLock(channelID)
	lock.Lock()
	defer lock.Unlock()
	unlockDB, err := lockCodexRefreshAcrossProcesses(ctx, channelID)
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
		_ = model.RefreshChannelCache()
	}

	return oauthKey, ch, nil
}
