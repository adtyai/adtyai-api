package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0017：绑定团队的 Key 在结算补扣时扣团队共享余额、退还时退回团队，
// 成员个人钱包全程不动。预扣侧有 AdtyTeamReserveHook，结算侧必须有一支对称的
// 钩子，否则差额落到成员个人钱包（补扣扣错人，退还凭空多给），团队余额停在预扣值。
type teamSettleFixture struct {
	db     *gorm.DB
	member *model.User
	token  *model.Token
	team   model.AdtyTeam
}

func newTeamSettleFixture(t *testing.T, personalQuota int, teamQuota int64) *teamSettleFixture {
	t.Helper()
	db := prepareReferralDB(t)
	leader := referralUser(t, db, "team-settle-leader")
	member := referralUser(t, db, "team-settle-member")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", member.Id).Update("quota", personalQuota).Error)
	member.Quota = personalQuota
	created, err := CreateAdtyTeam(leader.Id, 0, "Settle", decimal.NewFromInt(1))
	require.NoError(t, err)
	_, err = JoinAdtyTeam(member.Id, created.Team.Id)
	require.NoError(t, err)
	token := &model.Token{UserId: member.Id, Key: fmt.Sprintf("team-settle-%d", member.Id), Name: "team-settle",
		RemainQuota: 100_000_000, UnlimitedQuota: true, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, BindAdtyTeamKey(member.Id, token.Id))
	require.NoError(t, CreditAdtyTeam(int64(leader.Id), created.Team.Id, teamQuota, "team-settle-credit"))
	var team model.AdtyTeam
	require.NoError(t, db.First(&team, created.Team.Id).Error)
	return &teamSettleFixture{db: db, member: member, token: token, team: team}
}

func (f *teamSettleFixture) reserve(t *testing.T, key string, quota int) {
	t.Helper()
	_, err := model.ReserveBillingOperation(model.BillingOperation{
		OperationKey: key, UserId: f.member.Id, TokenId: f.token.Id, Kind: "request",
	}, quota)
	require.NoError(t, err)
}

func (f *teamSettleFixture) settle(t *testing.T, key string, actual int) *model.BillingOperation {
	t.Helper()
	require.NoError(t, model.QueueBillingSettlement(key, actual, false))
	row, applied, err := model.ApplyBillingSettlement(key)
	require.NoError(t, err)
	require.True(t, applied)
	return row
}

func (f *teamSettleFixture) teamQuota(t *testing.T) int64 {
	t.Helper()
	var team model.AdtyTeam
	require.NoError(t, f.db.First(&team, f.team.Id).Error)
	return team.SharedQuota
}

func (f *teamSettleFixture) personalQuota(t *testing.T) int {
	t.Helper()
	var user model.User
	require.NoError(t, f.db.First(&user, f.member.Id).Error)
	return user.Quota
}

// ledgerSum 汇总团队流水中「预扣/结算/退还」三类：充值那条 credit 是夹具的初始
// 入账，不算在这次请求的余额变化里。
func (f *teamSettleFixture) ledgerSum(t *testing.T) (int64, int64) {
	t.Helper()
	var rows []model.AdtyTeamLedger
	require.NoError(t, f.db.Where("team_id = ? AND type IN ?", f.team.Id,
		[]string{"reserve", "settle", "refund"}).Find(&rows).Error)
	var sum int64
	for _, row := range rows {
		sum += row.DeltaQuota
	}
	return sum, int64(len(rows))
}

// 补扣：预扣 50,000、实际 150,000，额外 100,000 必须从团队共享余额扣。
func TestTeamKeySettleTopUpComesFromSharedBalancePG(t *testing.T) {
	f := newTeamSettleFixture(t, 500_000, 50_000_000)
	f.reserve(t, "request:team-topup-1", 50_000)
	require.EqualValues(t, 49_950_000, f.teamQuota(t), "预扣从团队余额扣")

	row := f.settle(t, "request:team-topup-1", 150_000)

	assert.EqualValues(t, 49_850_000, f.teamQuota(t), "补扣 100,000 也从团队共享余额扣")
	assert.EqualValues(t, 500_000, f.personalQuota(t), "成员个人钱包全程不动")
	assert.EqualValues(t, 150_000, row.AppliedQuota)
	assert.Zero(t, row.Shortfall)
}

// 退还：预扣 50,000、实际 10,000，未用的 40,000 必须退回团队，不能进成员钱包。
func TestTeamKeySettleRefundReturnsToSharedBalancePG(t *testing.T) {
	f := newTeamSettleFixture(t, 500_000, 50_000_000)
	f.reserve(t, "request:team-refund-1", 50_000)
	require.EqualValues(t, 49_950_000, f.teamQuota(t))

	row := f.settle(t, "request:team-refund-1", 10_000)

	assert.EqualValues(t, 49_990_000, f.teamQuota(t), "未用的 40,000 退回团队共享余额")
	assert.EqualValues(t, 500_000, f.personalQuota(t), "成员个人钱包不得被凭空加钱")
	assert.EqualValues(t, 10_000, row.AppliedQuota)
}

// 团队余额不够补扣时，扣到 0 为止，差额记 shortfall；成员钱包仍然不动。
func TestTeamKeySettleTopUpStopsAtTeamBalancePG(t *testing.T) {
	f := newTeamSettleFixture(t, 500_000, 60_000)
	f.reserve(t, "request:team-short-1", 50_000)
	require.EqualValues(t, 10_000, f.teamQuota(t))

	row := f.settle(t, "request:team-short-1", 150_000)

	assert.EqualValues(t, 0, f.teamQuota(t), "团队余额扣到 0 为止")
	assert.EqualValues(t, 500_000, f.personalQuota(t), "差额不得转嫁给成员个人钱包")
	assert.EqualValues(t, 90_000, row.Shortfall)
	assert.EqualValues(t, 60_000, row.AppliedQuota)
}

// 预扣与结算交错：团队余额的净变化必须等于实际消耗，成员个人余额净变化为 0，
// 团队流水合计等于团队余额变化。
func TestTeamKeyReserveSettleKeepsSharedBalanceConservedPG(t *testing.T) {
	f := newTeamSettleFixture(t, 500_000, 50_000_000)
	actuals := []int{30_000, 60_000, 45_000, 55_000}

	var wg sync.WaitGroup
	errs := make(chan error, len(actuals)*2)
	for i, actual := range actuals {
		key := fmt.Sprintf("request:team-conc-%d", i)
		wg.Add(1)
		go func(i, actual int, key string) {
			defer wg.Done()
			if _, err := model.ReserveBillingOperation(model.BillingOperation{
				OperationKey: key, UserId: f.member.Id, TokenId: f.token.Id, Kind: "request",
			}, 50_000); err != nil {
				errs <- err
				return
			}
			// 让别的预扣/结算穿插进来，制造团队行锁上的真实交错。
			time.Sleep(time.Duration(i%3) * 2 * time.Millisecond)
			if err := model.QueueBillingSettlement(key, actual, false); err != nil {
				errs <- err
				return
			}
			if _, _, err := model.ApplyBillingSettlement(key); err != nil {
				errs <- err
			}
		}(i, actual, key)
	}
	wg.Wait()
	close(errs)
	var collected []error
	for err := range errs {
		collected = append(collected, err)
	}
	require.Empty(t, collected, "concurrent reserve/settle must not fail")

	var consumed int64
	for _, actual := range actuals {
		consumed += int64(actual)
	}
	assert.EqualValues(t, 50_000_000-consumed, f.teamQuota(t), "团队余额净变化 = 实际消耗")
	assert.EqualValues(t, 500_000, f.personalQuota(t), "成员个人余额净变化为 0")
	ledgerDelta, rows := f.ledgerSum(t)
	assert.EqualValues(t, -consumed, ledgerDelta, "团队流水合计 = 团队余额变化")
	assert.EqualValues(t, int64(len(actuals))*2, rows, "每笔一笔预扣 + 一笔结算流水")
}

// 团队 Key 的准入门槛是团队共享余额：个人余额为 0 的成员拿着有余额的团队 Key 必须放行。
func TestTeamKeyAdmissionUsesSharedBalancePG(t *testing.T) {
	f := newTeamSettleFixture(t, 0, 5_000_000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, f.token.Id)
	info := &relaycommon.RelayInfo{
		UserId: f.member.Id, TokenId: f.token.Id, TokenKey: f.token.Key, TokenUnlimited: true,
		RequestId: "team-admission-1", StartTime: time.Now(), UsingGroup: "default", UserGroup: "default",
		PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}

	session, apiErr := NewBillingSession(ctx, info, 50_000)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.EqualValues(t, 4_950_000, f.teamQuota(t), "预扣从团队共享余额扣")
	assert.EqualValues(t, 0, f.personalQuota(t), "个人钱包不动")
}
