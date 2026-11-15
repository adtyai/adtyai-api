package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这条守的是 `ISS-R4-0111`：**站点收益不得二次入账到 referral 佣金账户。**
//
// 修前结算在同一个事务里把 walletTotal 写两处：站点收益钱包（可提现）和
// referral 佣金账户（也能提现）。站长两本账各提一次 => 平台付双份现金。
// `docs/adty/features.md:216` 原文写明「站长收益提现不走这套 `referral_*` 账户」。
//
// 这条只测「结算后 referral 账户没被记钱 + 站点收益钱包有那笔钱」——它不测提现，
// 因为拦截提现会把开站前合法挣到的邀请佣金一起锁死（见 0111 的裁定 B）。
func TestSiteEarningSettlementDoesNotCreditReferralAccountPG(t *testing.T) {
	db := prepareSiteDB(t)
	pgtest.RecordServerVersion(t, db)

	owner, site := openActiveSite(t, db, "0111-double")
	customer := siteCustomer(t, db, site, "0111-double-customer")
	sitePrice(t, db, site.Id, 0, "", "1.25")

	saved := adtySiteEarningDayFn
	t.Cleanup(func() { adtySiteEarningDayFn = saved })
	adtySiteEarningDayFn = func() string { return "2026-09-01" }
	consumeOnSite(t, customer, perUnit("100")) // k=1.25, d=0.8 => +$20 收益

	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)

	_, err = RunAdtySiteSettlementOnce(time.Date(2026, 10, 1, 4, 0, 0, 0, time.Local))
	require.NoError(t, err)

	// ① 收益只落在站点钱包。
	assert.True(t, siteWalletBalance(t, db, site.Id, model.AdtySiteWalletEarnings).
		Equal(decimal.RequireFromString("20")), "已结算收益必须落在站点收益钱包")

	// ② referral 佣金账户**不变**——修前这里也是 20，于是能再提一次。
	assert.Zero(t, account(t, db, affiliate.Id).AvailableAmount,
		"站点收益不得再记进 referral 佣金账户（ISS-R4-0111）")

	// ③ referral 侧也不该留一条 site_earning 流水：有流水就说明入账路径还在。
	var entries int64
	require.NoError(t, db.Model(&model.ReferralCommissionLedger{}).
		Where("affiliate_id = ? AND type = ?", affiliate.Id, "site_earning").Count(&entries).Error)
	assert.Zero(t, entries, "referral 侧不该再有 site_earning 入账流水")
}
