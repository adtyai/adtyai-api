package model

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

// blockEvalHook 挡住第一次 EVAL，用来确认签到在缓存写完之前不会返回。
type blockEvalHook struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (hook *blockEvalHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() == "eval" {
		hook.once.Do(func() { close(hook.entered) })
		<-hook.release
	}
	return ctx, nil
}

func (*blockEvalHook) AfterProcess(context.Context, redis.Cmder) error { return nil }

func (*blockEvalHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (*blockEvalHook) AfterProcessPipeline(context.Context, []redis.Cmder) error { return nil }

func TestCheckinSyncsQuotaCacheBeforeReturn(t *testing.T) {
	db := adtyTestDB(t)
	useUserCacheMiniRedis(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	saved := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = operation_setting.CheckinSetting{Enabled: true, MinQuota: 5000, MaxQuota: 5000}
	t.Cleanup(func() { *operation_setting.GetCheckinSetting() = saved })

	user := &User{Username: "chk-cache", Password: "hashed-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000, AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, populateUserCache(*user))

	hook := &blockEvalHook{entered: make(chan struct{}), release: make(chan struct{})}
	common.RDB.AddHook(hook)
	done := make(chan error, 1)
	go func() {
		record, err := UserCheckin(user.Id)
		if err == nil && record.QuotaAwarded != 5000 {
			err = fmt.Errorf("check-in award %d", record.QuotaAwarded)
		}
		done <- err
	}()

	select {
	case <-hook.entered:
	case err := <-done:
		require.NoError(t, err)
		t.Fatal("check-in finished without updating the quota cache")
	case <-time.After(5 * time.Second):
		t.Fatal("check-in did not reach the quota cache update")
	}
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-done:
		require.NoError(t, err)
		t.Fatal("check-in returned before the quota cache update finished")
	default:
	}
	close(hook.release)
	require.NoError(t, <-done)

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 6000, cached.Quota)
}
