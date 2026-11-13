package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

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
