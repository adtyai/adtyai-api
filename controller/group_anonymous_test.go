package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAnonymousUserGroupsDoesNotQueryUsers(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	previousRedis := common.RedisEnabled
	common.RedisEnabled = true
	t.Cleanup(func() { common.RedisEnabled = previousRedis })

	queries := 0
	const hook = "f186_count_user_queries"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Dest == nil {
			return
		}
		if err := tx.Statement.Parse(tx.Statement.Dest); err != nil || tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Table == "users" {
			queries++
		}
	}))
	t.Cleanup(func() { db.Callback().Query().Remove(hook) })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)
	GetUserGroups(ctx)

	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Zero(t, queries)
}
