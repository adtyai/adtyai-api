package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConcurrentSetupCreatesOneRoot(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}, &model.Setup{}))
	require.NoError(t, model.EnsureOneRootUserIndex(db))
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

	var wg sync.WaitGroup
	for _, name := range []string{"root-a", "root-b"} {
		wg.Add(1)
		go func(username string) {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			body := `{"username":"` + username + `","password":"password-123","confirmPassword":"password-123"}`
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			PostSetup(ctx)
		}(name)
	}
	wg.Wait()

	var roots int64
	require.NoError(t, db.Model(&model.User{}).Where("role = ?", common.RoleRootUser).Count(&roots).Error)
	require.Equal(t, int64(1), roots)
}
