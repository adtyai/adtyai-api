package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// 团队充值下单和 hosted/pay、epay/pay 是同类的建单入口：每张单都会真实打一次支付网关，
// 而单号由客户端的 Idempotency-Key 决定——换一个 key 就是一张新单。所以它必须和另外三个
// 入口一样按用户限速（ISS-R4-0127）。
func TestTeamTopupOrderRateLimited(t *testing.T) {
	oldEnable, oldNum, oldDuration := common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 3
	t.Cleanup(func() {
		common.CriticalRateLimitEnable = oldEnable
		common.CriticalRateLimitNum = oldNum
		common.CriticalRateLimitDuration = oldDuration
	})

	f := newTeamAuthFixture(t)
	created := f.decodeCreated(t, f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Limit Desk","leader_id":%d,"discount":"0.80"}`, f.mainLeader.Id), f.rootTok))

	post := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/api/adty/teams/%d/topups", created.Team.Id), strings.NewReader(`{"amount_usd":"1"}`))
		req.Host = teamAuthMainHost
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.mainLeaderTok)
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		f.engine.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < common.CriticalRateLimitNum; i++ {
		rec := post(fmt.Sprintf("team-limit-%d", i))
		require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "order %d: %s", i+1, rec.Body.String())
	}
	blocked := post("team-limit-blocked")
	require.Equal(t, http.StatusTooManyRequests, blocked.Code, blocked.Body.String())
	require.NotEmpty(t, blocked.Header().Get("Retry-After"))

	// 限额内的每一次不同 key 都落一张单；被限速的那次不再落单。
	var orders int64
	require.NoError(t, f.db.Model(&model.AdtyTopupOrder{}).Where("team_id = ?", created.Team.Id).Count(&orders).Error)
	require.EqualValues(t, common.CriticalRateLimitNum, orders)
}
