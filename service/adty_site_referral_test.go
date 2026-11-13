package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestMainTeamAgreementPriceHasNoSupplyFloor(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	leader := referralUser(t, db, "agree-leader")
	main, err := CreateAdtyTeam(leader.Id, 0, "Agree", decimal.NewFromInt(1))
	require.NoError(t, err)
	require.NoError(t, SaveAdtyMainTeamAgreementPrice(main.Team.Id, "0.50"))
	var stored model.AdtyTeam
	require.NoError(t, db.First(&stored, main.Team.Id).Error)
	require.True(t, stored.Discount.Equal(decimal.RequireFromString("0.50")))

	owner := referralUser(t, db, "agree-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "agree", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	siteLeader := referralUser(t, db, "agree-site-leader")
	require.NoError(t, db.Model(siteLeader).Update("site_id", site.Id).Error)
	siteTeam, err := CreateAdtyTeam(siteLeader.Id, site.Id, "Site", decimal.RequireFromString("0.90"))
	require.NoError(t, err)
	require.Error(t, SaveAdtyMainTeamAgreementPrice(siteTeam.Team.Id, "0.50"))
}

func TestOwnerSiteTeamDiscountRespectsSupplyPrice(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "team-discount-owner")
	factor := decimal.RequireFromString("0.70")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "team-discount", Status: model.AdtySiteStatusActive, CostFactor: &factor}
	require.NoError(t, db.Create(&site).Error)
	leader := referralUser(t, db, "team-discount-leader")
	require.NoError(t, db.Model(leader).Update("site_id", site.Id).Error)
	created, err := CreateAdtyTeam(leader.Id, site.Id, "Desk", decimal.RequireFromString("0.80"))
	require.NoError(t, err)
	require.Error(t, SaveAdtyOwnerSiteTeamDiscount(site.Id, owner.Id, created.Team.Id, "0.60"))
	require.NoError(t, SaveAdtyOwnerSiteTeamDiscount(site.Id, owner.Id, created.Team.Id, "0.75"))
	var stored model.AdtyTeam
	require.NoError(t, db.First(&stored, created.Team.Id).Error)
	require.True(t, stored.Discount.Equal(decimal.RequireFromString("0.75")))
	require.EqualValues(t, 0, stored.SharedQuota)
	other := referralUser(t, db, "team-discount-other")
	require.Error(t, SaveAdtyOwnerSiteTeamDiscount(site.Id, other.Id, created.Team.Id, "0.90"))
}

func TestSiteReferralRateStaysUnsetUntilRootSetsCap(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "sref-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "sref", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	err := SaveAdtyOwnerSiteReferralRate(site.Id, owner.Id, "0.05")
	require.ErrorIs(t, err, ErrAdtySiteReferralUnset)

	require.NoError(t, SaveAdtySiteReferralCap("0.10"))
	require.Error(t, SaveAdtyOwnerSiteReferralRate(site.Id, owner.Id, "0.11"))
	require.NoError(t, SaveAdtyOwnerSiteReferralRate(site.Id, owner.Id, "0.08"))
	var stored model.AdtySiteReferralRate
	require.NoError(t, db.First(&stored, site.Id).Error)
	require.True(t, stored.Rate.Equal(decimal.RequireFromString("0.08")))

	other := referralUser(t, db, "sref-other")
	require.Error(t, SaveAdtyOwnerSiteReferralRate(site.Id, other.Id, "0.01"))
}

func TestSiteInviteBindsOnlyNewCustomersOnTheSameSite(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	owner := referralUser(t, db, "sref-bind-owner")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "sref-bind", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, SaveAdtySiteReferralCap("0.20"))
	require.NoError(t, SaveAdtyOwnerSiteReferralRate(site.Id, owner.Id, "0.05"))

	inviter := referralUser(t, db, "sref-inviter")
	invitee := referralUser(t, db, "sref-invitee")
	require.NoError(t, db.Model(inviter).Update("site_id", site.Id).Error)
	require.NoError(t, db.Model(invitee).Update("site_id", site.Id).Error)
	require.NoError(t, BindAdtyOwnerSiteInvite(site.Id, owner.Id, inviter.Id, invitee.Id))

	other := referralUser(t, db, "sref-cross")
	require.Error(t, BindAdtyOwnerSiteInvite(site.Id, owner.Id, inviter.Id, other.Id))
	require.Error(t, BindAdtyOwnerSiteInvite(site.Id, owner.Id, inviter.Id, inviter.Id))

	paid := referralUser(t, db, "sref-paid")
	require.NoError(t, db.Model(paid).Update("site_id", site.Id).Error)
	require.NoError(t, db.Create(&model.TopUp{UserId: paid.Id, TradeNo: "sref-paid-order", Status: common.TopUpStatusSuccess}).Error)
	require.Error(t, BindAdtyOwnerSiteInvite(site.Id, owner.Id, inviter.Id, paid.Id))
}
