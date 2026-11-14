package service

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCollectAdtyFundAnomaliesDoesNotRestrictFundWrites(t *testing.T) {
	db := prepareFloorDB(t)
	owner, site := openActiveSite(t, db, "detect-norestrict")
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	user, token := floorCustomer(t, db, site, "detect-cust", 10_000)
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).
		Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).
		Update("cycle_tier_usd", "100").Error)
	require.NoError(t, db.Model(&model.AdtySiteWallet{}).
		Where("site_id = ? AND kind = ?", site.Id, model.AdtySiteWalletMargin).
		Update("cycle_used_usd", "91").Error)

	kinds, err := CollectAdtyFundAnomalies(site.Id)
	require.NoError(t, err)
	require.Contains(t, kinds, AdtyAnomalyMargin)
	rows, err := ListAdtyAnomalyDispositions(site.Id)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Equal(t, "open", rows[0].Status)

	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("10"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "detect-earn",
	})
	require.NoError(t, err)

	root := restrictionRoot(t, db, "detect-root")
	_, err = model.AdjustUserQuotaWithLedger(user.Id, root.Id, common.RoleRootUser, "add", 100, "unrestricted after detect", "detect-adjust")
	require.NoError(t, err)

	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "detect-bill"
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	_ = owner
}

func TestManualAnomalyReviewRestrictsRelatedFundWritesOnly(t *testing.T) {
	db := prepareFloorDB(t)
	root := restrictionRoot(t, db, "review-root")
	_, site := openActiveSite(t, db, "review-scope")
	_, otherSite := openActiveSite(t, db, "review-other")
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	require.NoError(t, EnsureAdtySiteWallets(otherSite.Id))
	user, token := floorCustomer(t, db, site, "review-cust", 10_000)
	otherUser, otherToken := floorCustomer(t, db, site, "review-other-cust", 10_000)
	otherSiteUser := siteCustomer(t, db, otherSite, "review-offsite")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", otherSiteUser.Id).Update("quota", 10_000).Error)

	orderRow := openFundAnomaly(t, db, AdtyAnomalyRefundAmountMismatch, site.Id, user.Id, "ORD-RESTRICT")
	require.NoError(t, ReviewAdtyAnomalyDisposition(orderRow.Id, root.Id, "reviewing", "hold this order"))

	require.ErrorIs(t, creditUserOrder(t, db, user.Id, "ORD-RESTRICT", 50), model.ErrAdtyFundWriteRestricted)
	require.Equal(t, 10_000, restrictionUserQuota(t, db, user.Id), "a blocked order credit must not change the balance")
	require.Equal(t, int64(0), orderLedgerCount(t, db, user.Id, "ORD-RESTRICT"))

	require.NoError(t, creditUserOrder(t, db, user.Id, "ORD-OTHER", 25))
	require.Equal(t, 10_025, restrictionUserQuota(t, db, user.Id))

	_, err := model.AdjustUserQuotaWithLedger(otherUser.Id, root.Id, common.RoleRootUser, "add", 40, "independent account", "order-other-acct")
	require.NoError(t, err)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("3"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "order-unrelated-wallet",
	})
	require.NoError(t, err, "an order-scoped review must not block unattributed site wallet writes")
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("1"), AdtySiteWalletChange{
		Type: "gift_reserve", IdempotencyKey: "order-wallet-restricted", RefType: "topup", RefID: "ORD-RESTRICT",
	})
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)
	require.True(t, siteWalletBalance(t, db, site.Id, model.AdtySiteWalletEarnings).Equal(decimal.RequireFromString("3")))

	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "order-bill-ok"
	require.Nil(t, PreConsumeBilling(ctx, 100, info), "an order-scoped review must not block independent billing")

	accountRow := openFundAnomaly(t, db, AdtyAnomalyGiftGuaranteeShort, site.Id, user.Id, "")
	require.NoError(t, ReviewAdtyAnomalyDisposition(accountRow.Id, root.Id, "reviewing", "hold this account"))

	before := restrictionUserQuota(t, db, user.Id)
	_, err = model.AdjustUserQuotaWithLedger(user.Id, root.Id, common.RoleRootUser, "add", 80, "blocked account", "acct-adjust")
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)
	require.Equal(t, before, restrictionUserQuota(t, db, user.Id))
	require.Equal(t, int64(0), adjustLedgerCount(t, db, user.Id, "acct-adjust"))

	ctx2, info2, _ := floorRelay(t, user, token)
	info2.RequestId = "acct-bill-blocked"
	apiErr := PreConsumeBilling(ctx2, 100, info2)
	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, model.ErrAdtyFundWriteRestricted)
	require.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Nil(t, info2.Billing)
	require.Equal(t, before, floorBalance(t, user.Id))

	_, err = model.AdjustUserQuotaWithLedger(otherUser.Id, root.Id, common.RoleRootUser, "add", 15, "other account continues", "acct-other-ok")
	require.NoError(t, err)
	ctx3, info3, _ := floorRelay(t, otherUser, otherToken)
	info3.RequestId = "acct-other-bill"
	require.Nil(t, PreConsumeBilling(ctx3, 100, info3))
	_, err = model.AdjustUserQuotaWithLedger(otherSiteUser.Id, root.Id, common.RoleRootUser, "add", 15, "other site continues", "acct-offsite-ok")
	require.NoError(t, err)

	siteRow := openFundAnomaly(t, db, AdtyAnomalyBalanceLedgerGap, otherSite.Id, 0, "")
	require.NoError(t, ReviewAdtyAnomalyDisposition(siteRow.Id, root.Id, "reviewing", "hold this site"))
	_, err = ApplyAdtySiteWalletChange(otherSite.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "site-wallet-blocked",
	})
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)
	require.True(t, siteWalletBalance(t, db, otherSite.Id, model.AdtySiteWalletEarnings).IsZero())
	_, err = model.AdjustUserQuotaWithLedger(otherSiteUser.Id, root.Id, common.RoleRootUser, "add", 20, "site blocked", "site-adjust-blocked")
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("2"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "other-site-wallet-ok",
	})
	require.NoError(t, err)
}

func TestClosingAnomalyReviewLiftsFundWriteRestriction(t *testing.T) {
	db := prepareFloorDB(t)
	root := restrictionRoot(t, db, "close-root")
	_, site := openActiveSite(t, db, "close-scope")
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	user, token := floorCustomer(t, db, site, "close-cust", 10_000)

	row := openFundAnomaly(t, db, AdtyAnomalyBalanceLedgerGap, site.Id, 0, "")
	require.NoError(t, ReviewAdtyAnomalyDisposition(row.Id, root.Id, "reviewing", "hold while reviewing"))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("5"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "close-wallet-blocked",
	})
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)
	_, err = model.AdjustUserQuotaWithLedger(user.Id, root.Id, common.RoleRootUser, "add", 30, "blocked while open", "close-adjust-blocked")
	require.ErrorIs(t, err, model.ErrAdtyFundWriteRestricted)

	require.NoError(t, ReviewAdtyAnomalyDisposition(row.Id, root.Id, "closed", "cleared"))
	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("5"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "close-wallet-ok",
	})
	require.NoError(t, err)
	_, err = model.AdjustUserQuotaWithLedger(user.Id, root.Id, common.RoleRootUser, "add", 30, "allowed after close", "close-adjust-ok")
	require.NoError(t, err)
	ctx, info, _ := floorRelay(t, user, token)
	info.RequestId = "close-bill-ok"
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
}

func restrictionRoot(t *testing.T, db *gorm.DB, name string) *model.User {
	t.Helper()
	user := referralUser(t, db, name)
	require.NoError(t, db.Model(user).Update("role", common.RoleRootUser).Error)
	user.Role = common.RoleRootUser
	return user
}

func openFundAnomaly(t *testing.T, db *gorm.DB, kind string, siteID, accountID int, orderRef string) model.AdtyAnomalyDisposition {
	t.Helper()
	ref := orderRef
	if ref == "" {
		ref = strconv.Itoa(siteID)
	}
	row := model.AdtyAnomalyDisposition{
		Kind: kind, Ref: ref, Status: "open", Note: "detected",
		SiteId: siteID, AccountId: accountID, OrderRef: orderRef,
	}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func creditUserOrder(t *testing.T, db *gorm.DB, userID int, tradeNo string, delta int64) error {
	t.Helper()
	return db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := model.AdtyLockForUpdate(tx).First(&user, userID).Error; err != nil {
			return err
		}
		after := int64(user.Quota) + delta
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("quota", after).Error; err != nil {
			return err
		}
		return model.InsertAdtyLedgerTx(tx, &model.AdtyBalanceLedger{
			UserId: userID, DeltaQuota: delta, BalanceBefore: int64(user.Quota), BalanceAfter: after,
			Type: model.AdtyLedgerTypeTopUp, RefType: "topup", RefId: tradeNo,
		})
	})
}

func restrictionUserQuota(t *testing.T, db *gorm.DB, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, db.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func orderLedgerCount(t *testing.T, db *gorm.DB, userID int, tradeNo string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).
		Where("user_id = ? AND ref_type = ? AND ref_id = ?", userID, "topup", tradeNo).
		Count(&n).Error)
	return n
}

func adjustLedgerCount(t *testing.T, db *gorm.DB, userID int, idemKey string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AdtyBalanceLedger{}).
		Where("user_id = ? AND idempotency_key = ?", userID, idemKey).
		Count(&n).Error)
	return n
}

func siteWalletBalance(t *testing.T, db *gorm.DB, siteID int, kind string) decimal.Decimal {
	t.Helper()
	var wallet model.AdtySiteWallet
	require.NoError(t, db.Where("site_id = ? AND kind = ?", siteID, kind).First(&wallet).Error)
	return wallet.Balance
}
