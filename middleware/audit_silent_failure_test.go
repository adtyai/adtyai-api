package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0042：兜底审计写失败时必须留下 SysError，且不能让业务请求失败。
//
// 修前两处都是 `_ = model.WriteOfficialAuditLog(...)`：审计行丢了、日志里
// 也没有任何痕迹，事后无法判断「这次操作到底有没有被审计到」。
func setupAuditFailureTest(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db

	// 故意不建 audit_logs：写审计必然失败，但业务本身不受影响。
	buf := &bytes.Buffer{}
	previousWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = buf

	gin.SetMode(gin.TestMode)
	restore := func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		gin.DefaultErrorWriter = previousWriter
	}
	t.Cleanup(restore)
	return buf, restore
}

func TestTokenAuditFailureIsLoggedNotSwallowed(t *testing.T) {
	logs, _ := setupAuditFailureTest(t)

	router := gin.New()
	// 真实链路里 UserAuth 在审计中间件之前就把身份放进 context，这里照排。
	router.Use(RequestId(), func(c *gin.Context) {
		c.Set("id", 42)
		c.Set("username", "audited-user")
		c.Set("role", common.RoleCommonUser)
		c.Next()
	}, TokenOperationAudit())
	router.POST("/api/token/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/token/", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// ① 业务请求必须照常成功——审计是旁路，写失败不能把用户打回去。
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// ② 必须有一条 SysError 能定位到「哪条链路、哪个用户、哪个 action」。
	out := logs.String()
	require.NotEmpty(t, out, "audit write failure must not be silent")
	assert.Contains(t, out, "official audit write failed")
	assert.Contains(t, out, "chain=token route")
	assert.Contains(t, out, "user_id=42")
	assert.Contains(t, out, "action=token.create")
	assert.Contains(t, out, "POST /api/token/")
}

func TestAccessTokenAuditFailureIsLoggedNotSwallowed(t *testing.T) {
	logs, _ := setupAuditFailureTest(t)

	router := gin.New()
	router.Use(RequestId())
	// 直接走中间件内部那一对 begin/finish，避免为了造一个真 PAT 而重复整套鉴权。
	router.GET("/api/user/self", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set("username", "pat-user")
		c.Set("role", common.RoleCommonUser)
		c.Set("use_access_token", true)
		beginAccessTokenAudit(c, &model.UserBase{Id: 7, Username: "pat-user", Role: common.RoleCommonUser}, "ref-1")
		defer finishAccessTokenAudit(c)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/self", nil))

	// 个人访问令牌链路的审计同样不能把请求打回去。
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	out := logs.String()
	assert.Contains(t, out, "official audit write failed")
	assert.Contains(t, out, "chain=access token request")
}
