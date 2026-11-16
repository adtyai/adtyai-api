package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTeamTopupOrderCreditsPrincipalWithoutGift(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-topup-leader")
	created, err := CreateAdtyTeam(leader.Id, 0, "Topup", decimal.NewFromInt(1))
	require.NoError(t, err)
	order, err := CreateAdtyTeamTopupOrder(leader.Id, created.Team.Id, decimal.NewFromInt(1), "team-pay-1")
	require.NoError(t, err)
	require.True(t, order.BonusUSD.IsZero())
	require.True(t, order.BonusRate.IsZero())
	require.EqualValues(t, 500000, order.CreditQuota)

	replay, err := CreateAdtyTeamTopupOrder(leader.Id, created.Team.Id, decimal.NewFromInt(1), "team-pay-1")
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, replay.TradeNo)

	err = db.Transaction(func(tx *gorm.DB) error {
		_, _, _, _, err := model.CompleteAdtyTopUpTx(tx, order.TradeNo, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		return err
	})
	require.NoError(t, err)
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.EqualValues(t, 500000, team.SharedQuota)
	var user model.User
	require.NoError(t, db.First(&user, leader.Id).Error)
	require.Equal(t, 0, user.Quota)

	err = db.Transaction(func(tx *gorm.DB) error {
		already, _, _, _, err := model.CompleteAdtyTopUpTx(tx, order.TradeNo, model.AdtyPaymentFacts{
			Provider: model.PaymentProviderAdtyEpay, Source: model.AdtyPaymentSourceManual,
		})
		require.True(t, already)
		return err
	})
	require.NoError(t, err)
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	require.EqualValues(t, 500000, team.SharedQuota)
	_ = common.QuotaPerUnit
}

// ISS-R4-0068：幂等键的作用域是「本团队」。同一个 key 在别的团队里必须能用，
// 否则 B 团队只会拿到 Invalid parameters，且查不出自己撞的是 A 团队的单号。
func TestTeamTopupIdempotencyKeyIsScopedToTeam(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leaderA := referralUser(t, db, "team-key-a")
	teamA, err := CreateAdtyTeam(leaderA.Id, 0, "KeyA", decimal.NewFromInt(1))
	require.NoError(t, err)
	leaderB := referralUser(t, db, "team-key-b")
	teamB, err := CreateAdtyTeam(leaderB.Id, 0, "KeyB", decimal.NewFromInt(1))
	require.NoError(t, err)

	const key = "shared-client-key"
	first, err := CreateAdtyTeamTopupOrder(leaderA.Id, teamA.Team.Id, decimal.NewFromInt(1), key)
	require.NoError(t, err)

	second, err := CreateAdtyTeamTopupOrder(leaderB.Id, teamB.Team.Id, decimal.NewFromInt(1), key)
	require.NoError(t, err, "另一个团队用同一个 key 不能被 A 团队的单号挡住")
	require.NotEqual(t, first.TradeNo, second.TradeNo)
	require.Equal(t, teamB.Team.Id, second.TeamId)

	// 同团队同 key 同参数：仍然是同一张单。
	replay, err := CreateAdtyTeamTopupOrder(leaderA.Id, teamA.Team.Id, decimal.NewFromInt(1), key)
	require.NoError(t, err)
	require.Equal(t, first.TradeNo, replay.TradeNo)

	// 同团队同 key 改金额：是调用方换了参数，必须拒绝而不是新建同号单。
	_, err = CreateAdtyTeamTopupOrder(leaderA.Id, teamA.Team.Id, decimal.NewFromInt(2), key)
	require.ErrorIs(t, err, ErrAdtyTeamInvalid)
}

// 旧规则（单号只由 key 派生）下的老单不必迁移：同团队重放仍回到老单，
// 别的团队用同一个 key 则直接建自己的单，不再被挡住。
func TestTeamTopupReplaysLegacyTradeNoOnlyForSameTeam(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leaderA := referralUser(t, db, "team-legacy-a")
	teamA, err := CreateAdtyTeam(leaderA.Id, 0, "LegacyA", decimal.NewFromInt(1))
	require.NoError(t, err)
	leaderB := referralUser(t, db, "team-legacy-b")
	teamB, err := CreateAdtyTeam(leaderB.Id, 0, "LegacyB", decimal.NewFromInt(1))
	require.NoError(t, err)

	const key = "legacy-shared-key"
	legacyNo := adtyTeamTopupLegacyTradeNo("TTEAM", key)
	require.Len(t, legacyNo, len("TTEAM")+20, "老单号格式：前缀 + sha1(key)[:20]")
	require.NoError(t, db.Create(&model.AdtyTopupOrder{
		TradeNo: legacyNo, UserId: leaderA.Id, TeamId: teamA.Team.Id,
		Provider: model.PaymentProviderAdtyEpay, Method: "team",
		FaceUSD: decimal.NewFromInt(1), PayCurrency: "USD",
	}).Error)

	replay, err := CreateAdtyTeamTopupOrder(leaderA.Id, teamA.Team.Id, decimal.NewFromInt(1), key)
	require.NoError(t, err)
	require.Equal(t, legacyNo, replay.TradeNo, "同团队的重放必须回到升级前那张单")

	fresh, err := CreateAdtyTeamTopupOrder(leaderB.Id, teamB.Team.Id, decimal.NewFromInt(1), key)
	require.NoError(t, err)
	require.NotEqual(t, legacyNo, fresh.TradeNo, "不能把别的团队的老单当成自己的重放")
	require.Equal(t, teamB.Team.Id, fresh.TeamId)
}

// 幂等键是客户端自由文本，必须限长（与仓库内其它幂等键一致，取 64）。
func TestTeamTopupRejectsOverlongIdempotencyKey(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "team-key-long")
	team, err := CreateAdtyTeam(leader.Id, 0, "KeyLong", decimal.NewFromInt(1))
	require.NoError(t, err)

	_, err = CreateAdtyTeamTopupOrder(leader.Id, team.Team.Id, decimal.NewFromInt(1), strings.Repeat("k", adtyTeamTopupKeyMaxLen+1))
	require.ErrorIs(t, err, ErrAdtyTeamIdempotencyKey)

	_, err = CreateAdtyTeamTopupOrder(leader.Id, team.Team.Id, decimal.NewFromInt(1), strings.Repeat("k", adtyTeamTopupKeyMaxLen))
	require.NoError(t, err, "64 个字符是允许的上限")
}
