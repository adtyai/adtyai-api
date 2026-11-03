package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
