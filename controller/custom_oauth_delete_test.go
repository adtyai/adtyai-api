package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestDeleteCustomOAuthProviderKeepsBindingThatArrivesDuringDelete(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}))
	provider := &model.CustomOAuthProvider{
		Name: "Acme", Slug: "acme-del", Enabled: true, ClientId: "id", ClientSecret: "secret",
		AuthorizationEndpoint: "https://example.com/auth", TokenEndpoint: "https://example.com/token",
		UserInfoEndpoint: "https://example.com/user",
	}
	require.NoError(t, model.CreateCustomOAuthProvider(provider))
	user := &model.User{Username: "oauth-del", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(user).Error)

	tx := db.Begin()
	require.NoError(t, tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&model.CustomOAuthProvider{}, provider.Id).Error)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/custom-oauth/"+strconv.Itoa(provider.Id), nil)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(provider.Id)}}
		DeleteCustomOAuthProvider(ctx)
		done <- recorder
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting int
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%custom_oauth_providers%' AND pid <> pg_backend_pid()`).Scan(&waiting).Error)
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			var queries []string
			_ = db.Raw(`SELECT query FROM pg_stat_activity WHERE query LIKE '%custom_oauth%'`).Scan(&queries).Error
			t.Fatalf("delete did not wait on the provider lock; queries=%v", queries)
		}
		time.Sleep(20 * time.Millisecond)
	}

	require.NoError(t, tx.Create(&model.UserOAuthBinding{UserId: user.Id, ProviderId: provider.Id, ProviderUserId: "ext-1", CreatedAt: time.Now()}).Error)
	require.NoError(t, tx.Commit().Error)
	recorder := <-done
	require.Contains(t, recorder.Body.String(), "still has linked users")
	require.Contains(t, recorder.Body.String(), `"success":false`)

	var left model.CustomOAuthProvider
	require.NoError(t, db.First(&left, provider.Id).Error)
	var bindings int64
	require.NoError(t, db.Model(&model.UserOAuthBinding{}).Where("provider_id = ?", provider.Id).Count(&bindings).Error)
	require.Equal(t, int64(1), bindings)
}
