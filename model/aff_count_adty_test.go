package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// 审查 N-P2-13：邀请注册后 aff_count +1，即使未配置邀请奖励。
func TestAffCountIncrementsWithoutInviterBonus(t *testing.T) {
	truncateTables(t)
	prev := common.QuotaForInviter
	common.QuotaForInviter = 0
	t.Cleanup(func() { common.QuotaForInviter = prev })
	inviter := createReserveTestUser(t, 0)
	u := &User{Username: "invitee_n213", Password: "password-123456", DisplayName: "x"}
	require.NoError(t, u.Insert(inviter.Id))
	var cnt int
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Select("aff_count").Scan(&cnt).Error)
	require.Equal(t, 1, cnt)
}
