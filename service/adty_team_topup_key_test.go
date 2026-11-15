package service

import (
	"testing"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 团队 Key 的第二次预扣必须能入账（`ISS-R4-0067`）。
//
// 同一个 operation 会有第二次预扣（发送前补扣、阶梯价上调），两次共用同一个
// `ref`（operation_key）。只按 `kind:ref` 构幂等键时，第二次必撞唯一索引，
// 于是**每个需要补扣的团队 Key 请求都失败**。
func TestAdtyTeamReserveTopUpDoesNotCollideWithUniqueKeyPG(t *testing.T) {
	db := announcementTestDB(t)
	pgtest.RecordServerVersion(t, db)

	leader := ticketTestUser(t, db, "team-key-leader", 1)
	member := ticketTestUser(t, db, "team-key-member", 1)
	site := model.AdtySite{Subdomain: "team-key", RootDomain: "example.com", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	team := model.AdtyTeam{SiteId: site.Id, LeaderUserId: leader.Id, Name: "key-team",
		Status: model.AdtyTeamStatusActive, Discount: decimal.NewFromInt(1), SharedQuota: 10_000_000}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&model.AdtyTeamMember{
		TeamId: team.Id, UserId: member.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive,
	}).Error)

	token := model.Token{UserId: member.Id, Name: "team-key-token", Key: "sk-team-key-1"}
	require.NoError(t, db.Create(&token).Error)
	require.NoError(t, db.Create(&model.AdtyTeamKey{
		TeamId: team.Id, UserId: member.Id, TokenId: token.Id, Status: model.AdtyTeamMemberActive,
	}).Error)

	// 两次预扣，同一个 ref（= operation_key），金额不同。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		handled, err := reserveAdtyTeamQuotaHook(tx, token.Id, member.Id, 2_000_000, "request:team-grow-1")
		require.True(t, handled)
		return err
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		handled, err := reserveAdtyTeamQuotaHook(tx, token.Id, member.Id, 1_000_000, "request:team-grow-1")
		require.True(t, handled,
			"第二次预扣必须成功：修前它撞 reserve:<ref> 的唯一索引（2067/23505）")
		return err
	}))

	var refreshed model.AdtyTeam
	require.NoError(t, db.First(&refreshed, team.Id).Error)
	assert.EqualValues(t, 7_000_000, refreshed.SharedQuota, "10,000,000 − 2,000,000 − 1,000,000")

	var rows []model.AdtyTeamLedger
	require.NoError(t, db.Where("team_id = ? AND type = ?", team.Id, "reserve").Order("id").Find(&rows).Error)
	require.Len(t, rows, 2, "两次预扣是两行流水")
	assert.EqualValues(t, -2_000_000, rows[0].DeltaQuota)
	assert.EqualValues(t, -1_000_000, rows[1].DeltaQuota)
	assert.NotEqual(t, rows[0].IdempotencyKey, rows[1].IdempotencyKey, "两个事件必须两个键")
}

// 反过来：**其余 kind 的幂等键不能带上余额**，否则重放会重复入账。
//
// 这一条是我按 `kind:ref:after` 全量改时发现的：`topup` / `credit` / `spend`
// 一个 ref 就代表一个事件，而唯一索引正是「同一事件入账两次」的防线。
// 把余额算进键之后，重放时余额已变、键不同 → 新行成立 → 团队余额被重复加。
func TestAdtyTeamLedgerKeyKeepsOtherKindsIdempotent(t *testing.T) {
	assert.Equal(t, "topup:EPU-1", adtyTeamLedgerKey("topup", "EPU-1", 9_000_000))
	assert.Equal(t, "credit:ref-9", adtyTeamLedgerKey("credit", "ref-9", 9_000_000))
	assert.Equal(t, "spend:ref-9", adtyTeamLedgerKey("spend", "ref-9", 9_000_000),
		"非 reserve 的键必须与余额无关，否则重放会重复入账")

	assert.Equal(t, "reserve:op-1:7000000", adtyTeamLedgerKey("reserve", "op-1", 7_000_000))
	assert.NotEqual(t, adtyTeamLedgerKey("reserve", "op-1", 7_000_000),
		adtyTeamLedgerKey("reserve", "op-1", 9_000_000),
		"同一 operation 的两次预扣是不同的键")
}
