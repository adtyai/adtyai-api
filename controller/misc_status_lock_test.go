package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// `/api/status` 是匿名热路径，公告是一次真实的 DB 查询，必须发生在 OptionMap 读锁之外
// （ISS-R4-0123）。这条用例在公告查询执行的那一刻探一次写锁：拿得到说明没有读锁在手，
// 拿不到就是还在全局读锁里做 I/O——那会让后台保存设置整体排在匿名页面加载后面。
func TestStatusQueriesAnnouncementsOutsideOptionLock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "status-lock.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.AdtyNotificationPublication{}))
	require.NoError(t, db.Create(&model.AdtyNotificationPublication{
		Audience: "all_users", Kind: "system", Title: "Maintenance", Content: "Maintenance window",
		Status: "published", Revision: 1, ReadKey: strings.Repeat("a", 64), PublishedAt: 1, UpdatedAt: 1,
	}).Error)

	setting := console_setting.GetConsoleSetting()
	previousSetting := *setting
	t.Cleanup(func() { *setting = previousSetting })
	setting.AnnouncementsEnabled = true

	var queried, lockHeld atomic.Bool
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:status-lock-probe", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "adty_notification_publications") {
			return
		}
		queried.Store(true)
		// 只有「当前 goroutine 没有读锁在手」时 TryLock 才拿得到；拿到要立刻还回去。
		if common.OptionMapRWMutex.TryLock() {
			common.OptionMapRWMutex.Unlock()
			return
		}
		lockHeld.Store(true)
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:status-lock-probe") })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	GetStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, queried.Load(), "公告查询必须真的发生，否则这条用例什么都没测")
	assert.Contains(t, recorder.Body.String(), "Maintenance window")
	assert.False(t, lockHeld.Load(), "公告查询不能发生在 OptionMap 读锁里")
}
