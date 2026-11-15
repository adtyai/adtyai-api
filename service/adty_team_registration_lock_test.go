package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0105：注册链接完成入口是匿名的（router 只挂了 IP 级限流），口令校验
// 必须和登录共用同一套失败计数与锁定。否则一枚链接就是全平台已启用用户名的
// 密码预言机：猜错不计数、链接仍然 outstanding，可以一直猜下去。
func TestAdtyTeamRegistrationPasswordGuessesFeedTheLoginLock(t *testing.T) {
	db := prepareReferralDB(t)
	owner := referralUser(t, db, teamRegName("lk"))
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: teamRegName("lks"), Status: model.AdtySiteStatusActive, CostFactor: decimalPtr("0.80")}
	require.NoError(t, db.Create(&site).Error)
	leader := referralUser(t, db, teamRegName("ld"))
	require.NoError(t, db.Model(leader).Update("site_id", site.Id).Error)
	leader.SiteId = site.Id
	hashTeamRegPassword(t, db, leader, "RightPass123")

	issued, err := CreateAdtyOwnerSiteTeamRegistration(owner.Id, site.Id, time.Hour)
	require.NoError(t, err)
	common.ClearLoginFailures(leader.Username)
	t.Cleanup(func() { common.ClearLoginFailures(leader.Username) })
	require.False(t, common.LoginLocked(leader.Username))

	for i := 0; i < common.LoginLockLimit; i++ {
		_, err := CompleteAdtyTeamRegistration(issued.Code, leader.Username, "WrongPass123", "Lock Desk", nil)
		require.ErrorIs(t, err, ErrAdtyTeamRegistrationUnverified, "猜错口令只能回同一句话")
	}
	assert.True(t, common.LoginLocked(leader.Username), "猜错必须计入登录侧同一个失败计数")

	// 锁上之后，密码正确也不能过：同一把锁封住这条入口，文案不变。
	_, err = CompleteAdtyTeamRegistration(issued.Code, leader.Username, "RightPass123", "Lock Desk", nil)
	require.ErrorIs(t, err, ErrAdtyTeamRegistrationUnverified)

	// 计数清掉（相当于锁定窗口过去）之后，密码正确能过：链接没有被猜错消耗。
	common.ClearLoginFailures(leader.Username)
	created, err := CompleteAdtyTeamRegistration(issued.Code, leader.Username, "RightPass123", "Lock Desk", nil)
	require.NoError(t, err)
	assert.Equal(t, leader.Id, created.Team.LeaderUserId)
}
