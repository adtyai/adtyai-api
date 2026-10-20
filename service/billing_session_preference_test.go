package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionOnlyRejectsWithoutActiveSubscription(t *testing.T) {
	truncate(t)
	const userID = 910041
	seedUser(t, userID, 500000)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	session, apiErr := NewBillingSession(c, &relaycommon.RelayInfo{
		UserId:      userID,
		UserSetting: dto.UserSetting{BillingPreference: "subscription_only"},
	}, 1)
	require.Nil(t, session)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	msg := apiErr.Error()
	require.True(t, strings.Contains(msg, "subscription only") || strings.Contains(msg, "仅订阅") || strings.Contains(msg, "僅訂閱"), msg)

	user, err := model.GetUserById(userID, false)
	require.NoError(t, err)
	require.Equal(t, 500000, user.Quota)
}
