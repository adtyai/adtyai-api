package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestCodexAutoRefreshPublishesCommittedCredentialsAfterPageFailure(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.InitChannelCache()
		common.MemoryCacheEnabled = previousCache
	})
	channels := make([]model.Channel, codexCredentialRefreshBatchSize)
	for i := range channels {
		channels[i] = model.Channel{Name: "codex-page", Type: constant.ChannelTypeCodex, Status: common.ChannelStatusEnabled}
	}
	channels[0].Key = `{"access_token":"old-access","refresh_token":"old-refresh","account_id":"acc"}`
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, model.InitChannelCache())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(SetCodexOAuthTokenURLForTest(server.URL))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("cache-fail-codex-page", func(tx *gorm.DB) {
		if limit, ok := tx.Statement.Clauses["LIMIT"].Expression.(clause.Limit); ok && limit.Offset >= codexCredentialRefreshBatchSize {
			_ = tx.AddError(errors.New("next credential page unavailable"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("cache-fail-codex-page")) })
	runCodexCredentialAutoRefreshOnce()
	stored, err := model.GetChannelById(channels[0].Id, true)
	require.NoError(t, err)
	require.Contains(t, stored.Key, `"access_token":"new-access"`, "token rotation committed before the later query failed")
	cached, err := model.CacheGetChannel(channels[0].Id)
	require.NoError(t, err)
	assert.Contains(t, cached.Key, `"access_token":"new-access"`)
}

func TestSaveCodexChannelKeyDoesNotOverwrite(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{Name: "codex-race", Type: 1, Key: `{"refresh_token":"old"}`}
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, SaveCodexChannelKey(channel.Id, channel.Key, `{"refresh_token":"new"}`))
	err := SaveCodexChannelKey(channel.Id, `{"refresh_token":"old"}`, `{"refresh_token":"lost"}`)
	assert.ErrorIs(t, err, ErrCodexChannelKeyChanged)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, `{"refresh_token":"new"}`, stored.Key)
}

func TestCodexRefreshWaitsForAnotherProcess(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{
		Name: "codex-remote", Type: constant.ChannelTypeCodex, Status: 1,
		Key: `{"access_token":"at-1","refresh_token":"rt-1","account_id":"acc"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	conn, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", codexRefreshLockKey(channel.Id))
	require.NoError(t, err)

	arrived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		_, _ = w.Write([]byte(`{"access_token":"at-2","refresh_token":"rt-2","expires_in":3600}`))
	}))
	t.Cleanup(server.Close)
	restore := SetCodexOAuthTokenURLForTest(server.URL)
	t.Cleanup(restore)

	done := make(chan error, 1)
	go func() {
		_, _, err := RefreshCodexChannelCredential(context.Background(), channel.Id, CodexCredentialRefreshOptions{})
		done <- err
	}()
	select {
	case <-arrived:
		t.Fatal("refresh called upstream while another process held the lock")
	case <-time.After(300 * time.Millisecond):
	}
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", codexRefreshLockKey(channel.Id))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not finish after the other process released the lock")
	}
}

func TestRefreshCodexChannelCredentialSerializesTokenRotation(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))

	var mu sync.Mutex
	var seen []string
	firstArrived := make(chan struct{})
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.FormValue("refresh_token")
		mu.Lock()
		seen = append(seen, token)
		first := len(seen) == 1
		mu.Unlock()
		if first {
			close(firstArrived)
			<-releaseFirst
			_, _ = w.Write([]byte(`{"access_token":"at-2","refresh_token":"rt-2","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"at-3","refresh_token":"rt-3","expires_in":3600}`))
	}))
	t.Cleanup(server.Close)
	restore := SetCodexOAuthTokenURLForTest(server.URL)
	t.Cleanup(restore)

	channel := &model.Channel{
		Name: "codex-lock", Type: constant.ChannelTypeCodex, Status: 1,
		Key: `{"access_token":"at-1","refresh_token":"rt-1","account_id":"acc"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	errCh := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := RefreshCodexChannelCredential(context.Background(), channel.Id, CodexCredentialRefreshOptions{})
			errCh <- err
		}()
	}
	close(start)
	select {
	case <-firstArrived:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not reach the token endpoint")
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	require.Equal(t, []string{"rt-1"}, append([]string(nil), seen...))
	mu.Unlock()
	close(releaseFirst)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	mu.Lock()
	require.Equal(t, []string{"rt-1", "rt-2"}, seen)
	mu.Unlock()

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Contains(t, stored.Key, `"refresh_token":"rt-3"`)
}

// 这条守的是 `ISS-R4-0103` 的 (a) 面：**跨进程 advisory lock 必须受调用方 deadline 约束。**
//
// 修前拿锁用的是 `context.Background()`，调用方给的 `ctx` 根本没传进来。另一个进程
// 卡死但连接还在时，`pg_advisory_lock` 会无限期阻塞，连带这条渠道的 `sync.Mutex`
// 一起占住，之后该渠道所有刷新和相关 HTTP 请求永不返回。
//
// 断言：ctx 1s 到期前后必须带着 `context.DeadlineExceeded` 返回，且**不能**在没拿到
// 锁的情况下已经去上游换票。
func TestCodexRefreshLockHonorsCallerDeadline(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	channel := &model.Channel{
		Name: "codex-lock-deadline", Type: constant.ChannelTypeCodex, Status: 1,
		Key: `{"access_token":"at-1","refresh_token":"rt-1","account_id":"acc"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	conn, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", codexRefreshLockKey(channel.Id))
	require.NoError(t, err)
	held := true
	unlockOther := func() {
		if !held {
			return
		}
		held = false
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", codexRefreshLockKey(channel.Id))
	}
	t.Cleanup(func() {
		unlockOther()
		_ = conn.Close()
	})

	upstreamCalled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled <- struct{}{}
		_, _ = w.Write([]byte(`{"access_token":"at-2","refresh_token":"rt-2","expires_in":3600}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(SetCodexOAuthTokenURLForTest(server.URL))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := RefreshCodexChannelCredential(ctx, channel.Id, CodexCredentialRefreshOptions{})
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded, "拿锁必须跟随调用方 deadline")
	case <-time.After(5 * time.Second):
		// 修前就在这里挂住：1s 的 ctx 对 advisory lock 完全不生效。
		unlockOther()
		t.Fatal("refresh 在调用方 deadline 之后仍在等跨进程锁（ISS-R4-0103：pg_advisory_lock 无超时）")
	}
	unlockOther()
	select {
	case <-upstreamCalled:
		t.Fatal("没拿到锁就不该去上游换票")
	case <-time.After(200 * time.Millisecond):
	}
}

// 这条守的是 `ISS-R4-0103` 的 (b) 面：解锁失败时**不能**把带锁的连接还回池。
//
// session 级 advisory lock 跟着物理连接走，`conn.Close()` 只是还池。修前解锁错误被
// `_, _ =` 吞掉、连接照旧还池，于是这把锁留在了池里，下一个拿到这条连接的人
// 以为自己在别处加锁。丢弃物理连接后会话结束，锁必须随之释放。
//
// 断言方式取「物理会话是否还在」：advisory lock 在同会话内可重入，
// 直接在池里再 `pg_try_advisory_lock` 一次会假阳性（拿到的可能就是同一条连接）。
func TestDiscardedCodexRefreshConnReleasesSessionLock(t *testing.T) {
	db := adtyTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	conn, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	key := codexRefreshLockKey(987654321)
	_, err = conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", key)
	require.NoError(t, err)
	var pid int
	require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&pid))

	discardCodexRefreshConn(conn)

	other, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })

	// 会话结束是服务端异步收尾，给一小段时间轮询。
	alive := 1
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		require.NoError(t, other.QueryRowContext(context.Background(),
			"SELECT count(*) FROM pg_stat_activity WHERE pid = $1", pid).Scan(&alive))
		if alive == 0 {
			break
		}
	}
	require.Zero(t, alive, "带锁的连接必须被丢弃（物理会话结束），不能只 Close 还回池")
}
