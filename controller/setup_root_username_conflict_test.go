package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupRootSetupDB 给 setup 流程准备一套空库：PG 用隔离 schema，SQLite 用临时文件。
// 两种方言都要跑：建根账号时撞上的唯一冲突在 SQLite/MySQL 上是「事务照常提交」，
// 假成功就是永久的；PostgreSQL 会因事务中止在 COMMIT 时才炸。契约是同一个。
func setupRootSetupDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	require.NoError(t, i18n.Init())
	if dialect == "postgres" {
		db := adtyTestDB(t)
		require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Setup{}))
		return db
	}
	path := filepath.Join(t.TempDir(), "setup-root.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitColumnNamesForTests()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		model.InitColumnNamesForTests()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Setup{}))
	return db
}

func withSetupDisabled(t *testing.T) {
	t.Helper()
	previous := constant.Setup
	constant.Setup = false
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		constant.Setup = previous
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
}

func postSetup(username string) string {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"username":"` + username + `","password":"password-123","confirmPassword":"password-123","SelfUseModeEnabled":false}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	PostSetup(ctx)
	return recorder.Body.String()
}

// ISS-R4-0136：用户名已被一个非 root 账号占用时，建根账号的唯一冲突不能被吞。
// 吞掉就是「实例已初始化、但没有管理员」：setup 入口关闭，只能手工改库。
func TestSetupFailsWhenUsernameAlreadyTakenByNonRoot(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupRootSetupDB(t, dialect)
			require.NoError(t, model.EnsureOneRootUserIndex(db))
			require.NoError(t, model.EnsureUsernameUniqueIndex(db))

			// 前置条件：没有根账号，但 "admin" 已经被一个普通账号占了。
			require.NoError(t, db.Create(&model.User{
				Username: "admin",
				Password: "password-123",
				Role:     common.RoleCommonUser,
				Status:   common.UserStatusEnabled,
			}).Error)
			require.False(t, model.RootUserExists())
			withSetupDisabled(t)

			body := postSetup("admin")

			// 必须真失败：报错、实例没被标记已初始化、库里没有根账号。
			// 这条错误要说清是用户名/邮箱被占，不能是「事务中止」之类的残留。
			require.Contains(t, body, `"success":false`, body)
			require.NotContains(t, body, `"success":true`, body)
			require.Contains(t, body, "already taken", body)
			require.False(t, constant.Setup)
			require.Nil(t, model.GetSetup())
			require.False(t, model.RootUserExists())

			var roots int64
			require.NoError(t, db.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&roots).Error)
			require.Zero(t, roots)

			// 占位账号没被改动，也没有多出半截根账号。
			var users int64
			require.NoError(t, db.Model(&model.User{}).Count(&users).Error)
			require.Equal(t, int64(1), users)
		})
	}
}

// 对照组：用户名没被占时初始化照旧成功，证明上面那半边不是被误伤的。
func TestSetupSucceedsWhenRootUsernameIsFree(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupRootSetupDB(t, dialect)
			require.NoError(t, model.EnsureOneRootUserIndex(db))
			require.NoError(t, model.EnsureUsernameUniqueIndex(db))
			withSetupDisabled(t)

			body := postSetup("fresh-root")

			require.Contains(t, body, `"success":true`, body)
			require.True(t, constant.Setup)
			require.True(t, model.RootUserExists())
			var roots int64
			require.NoError(t, db.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&roots).Error)
			require.Equal(t, int64(1), roots)
		})
	}
}
