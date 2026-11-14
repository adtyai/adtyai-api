package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 分站（P11）。和钱有关的设置没有内置值（约定 I），测试里设好：d 0.8、下限 0.5、k 上限 3、加赠额外 2 点、试用 $5/$20。
func prepareSiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := prepareReferralDB(t)
	savedReseller := *adty_setting.GetResellerSetting()
	savedLegal := *adty_setting.GetLegalSetting()
	t.Cleanup(func() {
		*adty_setting.GetResellerSetting() = savedReseller
		*adty_setting.GetLegalSetting() = savedLegal
		InvalidateAdtySiteCache()
	})
	cfg := adty_setting.GetResellerSetting()
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled = true
	cfg.RootDomains = []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(2.0), new(5.0), new(20.0)
	cfg.InviteCodeEnabled = true
	adty_setting.GetLegalSetting().SiteAgreementVersion = 3
	InvalidateAdtySiteCache()
	return db
}

func dec(value string) *decimal.Decimal {
	d := decimal.RequireFromString(value)
	return &d
}

func openActiveSite(t *testing.T, db *gorm.DB, prefix string) (*model.User, *model.AdtySite) {
	t.Helper()
	owner := referralUser(t, db, prefix+"-owner")
	site, err := OpenAdtySite(owner.Id, prefix, model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	require.NoError(t, err)
	return owner, site
}

func siteCustomer(t *testing.T, db *gorm.DB, site *model.AdtySite, name string) *model.User {
	t.Helper()
	user := referralUser(t, db, name)
	require.NoError(t, model.AdtySetUserSiteTx(db, user.Id, site.Id))
	require.NoError(t, db.Create(&model.AdtySiteCustomer{UserId: user.Id, SiteId: site.Id, JoinedAt: time.Now().Unix()}).Error)
	user.SiteId = site.Id
	return user
}

func TestRaisingSiteCostFactorWaitsAndLoweringAppliesNow(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, i18n.Init())
	var sent []string
	oldSend := AdtySendSiteEmailAsync
	AdtySendSiteEmailAsync = func(_ *model.AdtySite, _, _, content string) {
		sent = append(sent, content)
	}
	t.Cleanup(func() { AdtySendSiteEmailAsync = oldSend })
	_, site := openActiveSite(t, db, "d-delay")
	current, ok := AdtySiteCostFactor(site)
	require.True(t, ok)
	require.True(t, current.Equal(decimal.RequireFromString("0.8")))

	raised, err := AdjustAdtySite(site.Id, 1, AdtySiteAdjust{CostFactor: dec("0.9"), Reason: "raise"})
	require.NoError(t, err)
	assert.Nil(t, raised.CostFactor, "a higher d must not apply immediately")
	require.NotNil(t, raised.PendingCostFactor)
	assert.True(t, raised.PendingCostFactor.Equal(decimal.RequireFromString("0.9")))
	assert.Positive(t, raised.PendingCostFactorAt)
	require.NotEmpty(t, sent)
	assert.Contains(t, sent[0], "0.9")
	still, ok := AdtySiteCostFactor(raised)
	require.True(t, ok)
	assert.True(t, still.Equal(decimal.RequireFromString("0.8")))

	lowered, err := AdjustAdtySite(site.Id, 1, AdtySiteAdjust{CostFactor: dec("0.6"), Reason: "lower"})
	require.NoError(t, err)
	require.NotNil(t, lowered.CostFactor)
	assert.True(t, lowered.CostFactor.Equal(decimal.RequireFromString("0.6")))
	assert.Nil(t, lowered.PendingCostFactor)
	assert.Zero(t, lowered.PendingCostFactorAt)

	again, err := AdjustAdtySite(site.Id, 1, AdtySiteAdjust{CostFactor: dec("0.7"), Reason: "raise-again"})
	require.NoError(t, err)
	require.NotNil(t, again.PendingCostFactor)
	applied, err := ApplyDueAdtySiteCostFactors(again.PendingCostFactorAt)
	require.NoError(t, err)
	assert.Equal(t, int64(1), applied)
	stored, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	require.NotNil(t, stored.CostFactor)
	assert.True(t, stored.CostFactor.Equal(decimal.RequireFromString("0.7")))
	assert.Nil(t, stored.PendingCostFactor)
}

func TestGlobalCostFactorIncreaseIsHeld(t *testing.T) {
	previous := adty_setting.DefaultResellerSetting()
	current := 0.8
	previous.CostFactor = &current
	previous.MarkupIncreaseDelayHours = 24
	next := previous
	raised := 0.9
	next.CostFactor = &raised
	now := int64(1_700_000_000)
	require.True(t, adty_setting.HoldGlobalCostFactorIncrease(&next, &previous, now))
	require.NotNil(t, next.CostFactor)
	assert.InDelta(t, 0.8, *next.CostFactor, 0.0001)
	require.NotNil(t, next.PendingCostFactor)
	assert.InDelta(t, 0.9, *next.PendingCostFactor, 0.0001)
	assert.Equal(t, now+24*3600, next.PendingCostFactorAt)

	lower := 0.7
	next.CostFactor = &lower
	assert.False(t, adty_setting.HoldGlobalCostFactorIncrease(&next, &previous, now))
	assert.InDelta(t, 0.7, *next.CostFactor, 0.0001)
	assert.Nil(t, next.PendingCostFactor)
}

func TestMinCostFactorIncreaseIsHeld(t *testing.T) {
	previous := adty_setting.DefaultResellerSetting()
	current := 0.5
	previous.MinCostFactor = &current
	previous.MarkupIncreaseDelayHours = 24
	next := previous
	raised := 0.7
	next.MinCostFactor = &raised
	now := int64(1_700_000_000)
	require.True(t, adty_setting.HoldMinCostFactorIncrease(&next, &previous, now))
	require.NotNil(t, next.MinCostFactor)
	assert.InDelta(t, 0.5, *next.MinCostFactor, 0.0001)
	require.NotNil(t, next.PendingMinCostFactor)
	assert.InDelta(t, 0.7, *next.PendingMinCostFactor, 0.0001)
	assert.Equal(t, now+24*3600, next.PendingMinCostFactorAt)

	lower := 0.4
	next.MinCostFactor = &lower
	assert.False(t, adty_setting.HoldMinCostFactorIncrease(&next, &previous, now))
	assert.InDelta(t, 0.4, *next.MinCostFactor, 0.0001)
	assert.Nil(t, next.PendingMinCostFactor)
}

func TestAdtyResellerSettingDefaults(t *testing.T) {
	cfg := adty_setting.DefaultResellerSetting()
	assert.False(t, cfg.Enabled)
	assert.Nil(t, cfg.CostFactor, "no built-in cost factor")
	assert.Nil(t, cfg.MinCostFactor)
	assert.Nil(t, cfg.MaxMarkup)
	assert.Nil(t, cfg.MaxSiteBonusExtra)
	assert.Nil(t, cfg.TrialGrantMaxUSD)
	assert.Nil(t, cfg.TrialGrantDailyUSD)
	assert.Equal(t, adty_setting.ResellerUnknownHostMain, cfg.UnknownHostMode)
	assert.Equal(t, 24, cfg.MarkupIncreaseDelayHours)
	assert.Equal(t, 4, cfg.SettleHour)
	assert.Equal(t, 7, cfg.FreezeDays)
	assert.Equal(t, 7, cfg.CloseNoticeDays)
	assert.True(t, cfg.SiteLoginOnly)
	assert.True(t, cfg.SelfUseCheckEnabled)
	assert.True(t, cfg.MigrationCheckEnabled)
	assert.False(t, cfg.CustomDomainEnabled)
	assert.False(t, cfg.SenderDomainEnabled)
	assert.False(t, cfg.InviteCodeEnabled)
	assert.Equal(t, 30, cfg.FingerprintWindowDays)
	assert.Equal(t, 20, cfg.CustomerLookupPerHour)
	assert.Equal(t, 2, cfg.MaxCustomDomainsPerSite)
	assert.Equal(t, 3, cfg.CustomDomainCreatesPerHour)
	assert.Contains(t, cfg.ReservedSubdomains, "www")
	assert.Equal(t, "masked", cfg.Visibility["customer_email"])
}

func TestOpenSiteRequiresSettings(t *testing.T) {
	db := prepareSiteDB(t)
	owner := referralUser(t, db, "needs-settings")
	adty_setting.GetResellerSetting().CostFactor = nil
	_, err := OpenAdtySite(owner.Id, "needs", model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	assert.ErrorIs(t, err, ErrAdtySiteDisabled)

	cases := []struct {
		name    string
		mutate  func(s *adty_setting.ResellerSetting)
		resend  bool
		wantErr bool
	}{
		{"enabled without money settings", func(s *adty_setting.ResellerSetting) { s.MaxMarkup = nil }, false, true},
		{"enabled without root domain", func(s *adty_setting.ResellerSetting) { s.RootDomains = nil }, false, true},
		{"cost factor above 1 is allowed", func(s *adty_setting.ResellerSetting) { s.CostFactor = new(1.2) }, false, false},
		{"cost factor below minimum", func(s *adty_setting.ResellerSetting) { s.CostFactor = new(0.4) }, false, true},
		{"cost factor not positive", func(s *adty_setting.ResellerSetting) { s.CostFactor = new(0.0) }, false, true},
		{"markup limit below 1 is allowed", func(s *adty_setting.ResellerSetting) { s.MaxMarkup = new(0.9) }, false, false},
		{"markup limit not positive", func(s *adty_setting.ResellerSetting) { s.MaxMarkup = new(0.0) }, false, true},
		{"custom domains without Cloudflare", func(s *adty_setting.ResellerSetting) { s.CustomDomainEnabled = true }, false, true},
		{"custom domains with Cloudflare", func(s *adty_setting.ResellerSetting) {
			s.CustomDomainEnabled, s.CloudflareApiToken, s.CloudflareZoneId, s.CnameTarget = true, "tok", "zone", "customers.sites.example.com"
		}, false, false},
		{"sender domains without Resend key", func(s *adty_setting.ResellerSetting) { s.SenderDomainEnabled = true }, false, true},
		{"sender domains with Resend key", func(s *adty_setting.ResellerSetting) { s.SenderDomainEnabled = true }, true, false},
		{"complete", func(s *adty_setting.ResellerSetting) {}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := adty_setting.DefaultResellerSetting()
			s.Enabled, s.RootDomains = true, []string{"sites.example.com"}
			s.CostFactor, s.MinCostFactor, s.MaxMarkup = new(0.8), new(0.5), new(3.0)
			s.MaxSiteBonusExtra, s.TrialGrantMaxUSD, s.TrialGrantDailyUSD = new(0.0), new(0.0), new(0.0)
			tc.mutate(&s)
			err := adty_setting.ValidateResellerSetting(&s, tc.resend)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubdomainRules(t *testing.T) {
	db := prepareSiteDB(t)
	_, _ = openActiveSite(t, db, "taken")
	for _, tc := range []struct {
		subdomain string
		want      error
	}{
		{"www", ErrAdtySiteSubdomainUsed},
		{"API", ErrAdtySiteSubdomainUsed},
		{"taken", ErrAdtySiteSubdomainUsed},
		{"-dash", ErrAdtySiteSubdomainBad},
		{"dash-", ErrAdtySiteSubdomainBad},
		{"under_score", ErrAdtySiteSubdomainBad},
		{"dot.ted", ErrAdtySiteSubdomainBad},
		{"", ErrAdtySiteSubdomainBad},
		{"a234567890123456789012345678901234567890123456789012345678901234", ErrAdtySiteSubdomainBad},
		{"  Good-Name1 ", nil},
	} {
		owner := referralUser(t, db, "sub-"+time.Now().Format("150405.000000000"))
		site, err := OpenAdtySite(owner.Id, tc.subdomain, model.AdtySiteOpenedByAdmin, AdtySitePreset{})
		if tc.want != nil {
			assert.ErrorIs(t, err, tc.want, tc.subdomain)
			continue
		}
		require.NoError(t, err, tc.subdomain)
		assert.Equal(t, "good-name1", site.Subdomain)
	}
}

func TestApplyRejectsUnpublishedAgreement(t *testing.T) {
	db := prepareSiteDB(t)
	legal := adty_setting.GetLegalSetting()
	previous := legal.SiteAgreementVersion
	legal.SiteAgreementVersion = 0
	t.Cleanup(func() { legal.SiteAgreementVersion = previous })
	owner := referralUser(t, db, "unpublished")
	_, err := ApplyAdtySite(owner.Id, "zero", "", "0", "")
	assert.ErrorIs(t, err, ErrAdtySiteAgreement)
}

func TestOpenSiteByApplyThenApprove(t *testing.T) {
	db := prepareSiteDB(t)
	owner := referralUser(t, db, "applicant")
	_, err := ApplyAdtySite(owner.Id, "applied", "tg @me", "2", "")
	assert.ErrorIs(t, err, ErrAdtySiteAgreement, "an old agreement version is rejected")

	site, err := ApplyAdtySite(owner.Id, "applied", "tg @me", "3", "")
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusPending, site.Status)
	assert.Equal(t, model.AdtySiteOpenedByApply, site.OpenedBy)
	assert.Equal(t, "3", site.AgreementVer)
	assert.Positive(t, site.AgreementAt)
	_, err = ApplyAdtySite(owner.Id, "second", "", "3", "")
	assert.ErrorIs(t, err, ErrAdtySiteAlreadyOwner, "a pending application counts as a site")

	approved, err := ReviewAdtySiteApplication(site.Id, 1, true, dec("0.7"), dec("2"), "ok")
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, approved.Status)
	assert.True(t, approved.CostFactor.Equal(decimal.RequireFromString("0.7")))
	assert.True(t, approved.MaxMarkup.Equal(decimal.NewFromInt(2)))
	_, err = ReviewAdtySiteApplication(site.Id, 1, true, nil, nil, "")
	assert.ErrorIs(t, err, ErrAdtySiteStatus)

	other := referralUser(t, db, "rejected-applicant")
	rejected, err := ApplyAdtySite(other.Id, "rejected", "", "3", "")
	require.NoError(t, err)
	rejected, err = ReviewAdtySiteApplication(rejected.Id, 1, false, nil, nil, "no")
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, rejected.Status)
	_, err = ApplyAdtySite(other.Id, "rejected-again", "", "3", "")
	assert.NoError(t, err, "a rejected applicant can apply again")
}

func TestOpenSiteByAdmin(t *testing.T) {
	db := prepareSiteDB(t)
	owner := referralUser(t, db, "admin-opened")
	_, err := OpenAdtySite(owner.Id, "low", model.AdtySiteOpenedByAdmin, AdtySitePreset{CostFactor: dec("0.4")})
	assert.Error(t, err, "below min_cost_factor")
	_, err = OpenAdtySite(owner.Id, "high", model.AdtySiteOpenedByAdmin, AdtySitePreset{MaxMarkup: dec("5")})
	assert.Error(t, err, "above max_markup")

	site, err := OpenAdtySite(owner.Id, "admin-site", model.AdtySiteOpenedByAdmin, AdtySitePreset{CostFactor: dec("0.6"), OperatorId: 9})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, site.Status)
	assert.Equal(t, "sites.example.com", site.RootDomain)
	var audits []model.AdtySiteAudit
	require.NoError(t, db.Where("site_id = ?", site.Id).Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, "open", audits[0].Action)
	assert.Equal(t, 9, audits[0].OperatorId)
	assert.Equal(t, site.Id, AdtySiteById(site.Id).Id, "the cache sees the new site")
}

func TestOpenSiteByInviteCodeAppliesPreset(t *testing.T) {
	db := prepareSiteDB(t)
	code := &model.AdtySiteInviteCode{MaxUses: 2, CostFactor: dec("0.65"), MaxMarkup: dec("2.5")}
	require.NoError(t, SaveAdtySiteInviteCode(code))
	require.Len(t, code.Code, 12)

	owner := referralUser(t, db, "invited")
	site, err := ApplyAdtySite(owner.Id, "invited", "", "3", " "+code.Code+" ")
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, site.Status)
	assert.Equal(t, model.AdtySiteOpenedByInvite, site.OpenedBy)
	assert.True(t, site.CostFactor.Equal(decimal.RequireFromString("0.65")))
	assert.True(t, site.MaxMarkup.Equal(decimal.RequireFromString("2.5")))
	var stored model.AdtySiteInviteCode
	require.NoError(t, db.First(&stored, code.Id).Error)
	assert.Equal(t, 1, stored.UsedCount)

	_, err = ApplyAdtySite(referralUser(t, db, "bad-code").Id, "bad-code", "", "3", "NOPE")
	assert.ErrorIs(t, err, ErrAdtySiteInviteInvalid)
	expired := &model.AdtySiteInviteCode{ExpireAt: time.Now().Unix() - 10}
	require.NoError(t, SaveAdtySiteInviteCode(expired))
	_, err = ApplyAdtySite(referralUser(t, db, "expired-code").Id, "expired-code", "", "3", expired.Code)
	assert.ErrorIs(t, err, ErrAdtySiteInviteInvalid)

	adty_setting.GetResellerSetting().InviteCodeEnabled = false
	_, err = ApplyAdtySite(referralUser(t, db, "codes-off").Id, "codes-off", "", "3", code.Code)
	assert.ErrorIs(t, err, ErrAdtySiteInviteOff)
}

func TestInviteCodeConcurrentUseRespectsMax(t *testing.T) {
	db := prepareSiteDB(t)
	code := &model.AdtySiteInviteCode{MaxUses: 3}
	require.NoError(t, SaveAdtySiteInviteCode(code))
	owners := make([]*model.User, 10)
	for i := range owners {
		owners[i] = referralUser(t, db, "race-"+string(rune('a'+i)))
	}
	var opened atomic.Int32
	var wg sync.WaitGroup
	for i, owner := range owners {
		wg.Go(func() {
			if _, err := ApplyAdtySite(owner.Id, "race-"+string(rune('a'+i)), "", "3", code.Code); err == nil {
				opened.Add(1)
			}
		})
	}
	wg.Wait()
	assert.EqualValues(t, 3, opened.Load())
	var stored model.AdtySiteInviteCode
	require.NoError(t, db.First(&stored, code.Id).Error)
	assert.Equal(t, 3, stored.UsedCount)
}

func TestSiteCustomerCannotOpenSite(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "home")
	customer := siteCustomer(t, db, site, "home-customer")
	_, err := OpenAdtySite(customer.Id, "mine", model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	assert.ErrorIs(t, err, ErrAdtySiteCustomer)
	_, err = ApplyAdtySite(customer.Id, "mine", "", "3", "")
	assert.ErrorIs(t, err, ErrAdtySiteCustomer)
}

func TestOpenSiteCreatesAffiliateStop(t *testing.T) {
	db := prepareSiteDB(t)
	owner := referralUser(t, db, "fresh-owner")
	_, err := OpenAdtySite(owner.Id, "fresh", model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	require.NoError(t, err)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.Positive(t, affiliate.SiteOwnerSince)
	_, err = Referral.AffiliateByCode(affiliate.InviteCode)
	assert.ErrorIs(t, err, ErrReferralCodeInvalid)
}

func TestSiteCustomerCannotBecomeAffiliate(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "no-aff")
	customer := siteCustomer(t, db, site, "no-aff-customer")
	_, err := Referral.EnsureAffiliate(customer.Id)
	assert.ErrorIs(t, err, ErrReferralSiteCustomer)

	code, err := newInviteCode()
	require.NoError(t, err)
	row := model.ReferralAffiliate{UserId: customer.Id, InviteCode: code, Status: model.ReferralAffiliateStatusApproved, AcquisitionEnabled: true}
	require.NoError(t, db.Create(&row).Error)
	_, err = Referral.AffiliateByCode(code)
	assert.ErrorIs(t, err, ErrReferralCodeInvalid)
	main := referralUser(t, db, "no-aff-main")
	require.NoError(t, db.Create(&model.ReferralBinding{InviteeUserId: main.Id, AffiliateId: row.Id}).Error)
	affiliateID, rate := adtyReferralSnapshot(main.Id)
	assert.Zero(t, affiliateID)
	assert.True(t, rate.IsZero())
}

func TestClosingSiteBlocksWithdrawal(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "close-wd")
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ReferralCommissionAccount{}).Where("affiliate_id = ?", affiliate.Id).Update("available_amount", 40).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusClosing).Error)
	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("10"), AccountType: model.ReferralWithdrawToBalance, IdempotencyKey: "close-wd"})
	assert.Error(t, err)
	assert.InDelta(t, 40, account(t, db, affiliate.Id).AvailableAmount, 1e-9)

	referralSetting().Enabled = false
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusClosed).Error)
	paid, err := Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("10"), AccountType: model.ReferralWithdrawToBalance, IdempotencyKey: "close-wd-off"})
	require.NoError(t, err)
	assert.Equal(t, model.ReferralWithdrawalStatusPaid, paid.Status)
	assert.InDelta(t, 30, account(t, db, affiliate.Id).AvailableAmount, 1e-9)

	_, err = Referral.CreateWithdrawal(ReferralWithdrawalCreateInput{UserId: owner.Id, Amount: usd("25"), AccountType: model.ReferralWithdrawToAlipay,
		AccountName: "A", QRImageURL: "javascript:alert(1)", IdempotencyKey: "qr", ExpectedCNYRate: decimal.NewFromInt(7)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "QR")
}

func TestOpenSiteMarksAffiliate(t *testing.T) {
	db := prepareSiteDB(t)
	owner := referralUser(t, db, "affiliate-owner")
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	require.Zero(t, affiliate.SiteOwnerSince)

	pending, err := ApplyAdtySite(owner.Id, "aff", "", "3", "")
	require.NoError(t, err)
	var stored model.ReferralAffiliate
	require.NoError(t, db.First(&stored, affiliate.Id).Error)
	assert.Zero(t, stored.SiteOwnerSince, "a pending application does not stop commissions yet")

	_, err = ReviewAdtySiteApplication(pending.Id, 1, true, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, db.First(&stored, affiliate.Id).Error)
	assert.Positive(t, stored.SiteOwnerSince)
	_, err = Referral.AffiliateByCode(affiliate.InviteCode)
	assert.ErrorIs(t, err, ErrReferralCodeInvalid, "the referral link stops counting")
}

func TestNoCommissionForSiteCustomer(t *testing.T) {
	db := prepareSiteDB(t)
	affiliate, invitee := inviterAndInvitee(t, db, "site-cust")
	_, site := openActiveSite(t, db, "cust-site")
	require.NoError(t, model.AdtySetUserSiteTx(db, invitee.Id, site.Id))
	affiliateID, rate := adtyReferralSnapshot(invitee.Id)
	assert.Zero(t, affiliateID)
	assert.True(t, rate.IsZero())

	require.NoError(t, model.AdtySetUserSiteTx(db, invitee.Id, 0))
	affiliateID, _ = adtyReferralSnapshot(invitee.Id)
	assert.Equal(t, affiliate.Id, affiliateID, "main-site customers still earn commission")
}

func requireArchivedCloseIdentity(t *testing.T, db *gorm.DB, userId, sourceSiteId int) model.AdtyUserSiteMigration {
	t.Helper()
	var membership model.AdtySiteCustomer
	require.NoError(t, db.Where("user_id = ?", userId).First(&membership).Error)
	assert.Equal(t, userId, membership.UserId)
	assert.Equal(t, sourceSiteId, membership.SiteId)
	assert.Equal(t, model.AdtySiteCustomerArchived, membership.Status)
	assert.NotZero(t, membership.ArchivedAt)
	var live int64
	require.NoError(t, model.AdtyLiveSiteCustomer(db.Model(&model.AdtySiteCustomer{})).Where("user_id = ?", userId).Count(&live).Error)
	assert.Zero(t, live, "inherited customers have no live site membership on main")
	var mappings []model.AdtyUserSiteMigration
	require.NoError(t, db.Where("user_id = ?", userId).Find(&mappings).Error)
	require.Len(t, mappings, 1)
	assert.Equal(t, userId, mappings[0].UserId)
	assert.Equal(t, sourceSiteId, mappings[0].SourceSiteId)
	assert.Zero(t, mappings[0].TargetSiteId)
	assert.NotZero(t, mappings[0].Id)
	return mappings[0]
}

func TestCloseSiteMovesCustomersToMain(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "closing")
	customer := siteCustomer(t, db, site, "closing-customer")
	authBefore := customer.AuthVersion
	require.NoError(t, db.Model(customer).Update("quota", 1500).Error)
	require.NoError(t, db.Create(&model.AdtySitePrice{SiteId: site.Id, Markup: decimal.RequireFromString("1.5"), Version: 1}).Error)
	team, err := CreateAdtyTeam(customer.Id, site.Id, "closing-team", decimal.RequireFromString("0.9"))
	require.NoError(t, err)

	_, err = RequestAdtySiteClose(owner.Id, "retiring")
	require.NoError(t, err)
	before := time.Now().Unix()
	scheduled, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Reason: "owner asked"})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosing, scheduled.Status)
	assert.Zero(t, scheduled.CloseTransferTo)
	assert.GreaterOrEqual(t, scheduled.CloseExecuteAt, before+7*86400, "customers get close_notice_days of notice")

	var stored model.User
	require.NoError(t, RunAdtySiteCloseOnce(time.Now().Unix()))
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Equal(t, site.Id, stored.SiteId, "nothing moves before the notice period ends")

	require.NoError(t, RunAdtySiteCloseOnce(scheduled.CloseExecuteAt))
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, site.Id, stored.InheritedFromSiteId)
	assert.Equal(t, 1500, stored.Quota)
	assert.Equal(t, authBefore+1, stored.AuthVersion)
	requireArchivedCloseIdentity(t, db, customer.Id, site.Id)
	var inheritedTeam model.AdtyTeam
	require.NoError(t, db.First(&inheritedTeam, team.Team.Id).Error)
	assert.Equal(t, team.Team.Id, inheritedTeam.Id)
	assert.Zero(t, inheritedTeam.SiteId)
	assert.Equal(t, site.Id, inheritedTeam.SourceSiteId)
	assert.Equal(t, model.AdtyTeamStatusActive, inheritedTeam.Status)
	assert.Equal(t, customer.Id, inheritedTeam.LeaderUserId)
	closed, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)
	assert.Equal(t, "closing", closed.Subdomain, "the subdomain stays reserved for redirects")

	_, err = OpenAdtySite(owner.Id, "reopened", model.AdtySiteOpenedByAdmin, AdtySitePreset{})
	assert.NoError(t, err, "a former owner may open a new site")
}

func TestCloseSiteRejectsTransferToAnotherSite(t *testing.T) {
	db := prepareSiteDB(t)
	_, from := openActiveSite(t, db, "from")
	_, to := openActiveSite(t, db, "to")
	cheap := siteCustomer(t, db, from, "cheap-customer")
	special := siteCustomer(t, db, from, "special-customer")
	require.NoError(t, db.Create(&[]model.AdtySitePrice{
		{SiteId: from.Id, Markup: decimal.RequireFromString("1.2"), Version: 1},
		{SiteId: from.Id, UserId: special.Id, Markup: decimal.RequireFromString("2.5"), Version: 1},
		{SiteId: to.Id, Markup: decimal.RequireFromString("2"), Version: 1},
		{SiteId: to.Id, Group: "vip", Markup: decimal.RequireFromString("1.1"), Version: 1},
	}).Error)

	_, err := ScheduleAdtySiteClose(from.Id, 1, AdtySiteClosePlan{TransferTo: from.Id})
	assert.ErrorIs(t, err, ErrAdtySiteTransfer)
	_, err = ScheduleAdtySiteClose(from.Id, 1, AdtySiteClosePlan{TransferTo: to.Id, Immediate: true})
	assert.ErrorIs(t, err, ErrAdtySiteTransfer)

	var stored model.User
	require.NoError(t, db.First(&stored, cheap.Id).Error)
	assert.Equal(t, from.Id, stored.SiteId, "a rejected transfer must not move customers")
	active, err := GetAdtySite(from.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, active.Status)
	assert.Zero(t, active.CloseTransferTo)

	closed, err := ScheduleAdtySiteClose(from.Id, 1, AdtySiteClosePlan{TransferTo: 0, Immediate: true})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)
	require.NoError(t, db.First(&stored, cheap.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, from.Id, stored.InheritedFromSiteId)
	stored = model.User{}
	require.NoError(t, db.First(&stored, special.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, from.Id, stored.InheritedFromSiteId)
	requireArchivedCloseIdentity(t, db, cheap.Id, from.Id)
	requireArchivedCloseIdentity(t, db, special.Id, from.Id)
	var carried int64
	require.NoError(t, db.Model(&model.AdtySitePrice{}).Where("site_id = ? AND user_id IN ?", to.Id, []int{cheap.Id, special.Id}).Count(&carried).Error)
	assert.Zero(t, carried, "customer prices must not be carried onto another site")
}

func TestExecuteAdtySiteCloseIgnoresPersistedTransferTo(t *testing.T) {
	db := prepareSiteDB(t)
	_, from := openActiveSite(t, db, "queued-from")
	_, to := openActiveSite(t, db, "queued-to")
	customer := siteCustomer(t, db, from, "queued-customer")
	require.NoError(t, db.Model(customer).Update("quota", 800).Error)
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", from.Id).Updates(map[string]any{
		"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_transfer_to": to.Id,
	}).Error)

	require.NoError(t, ExecuteAdtySiteClose(from.Id, 10))
	var stored model.User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.NotEqual(t, to.Id, stored.SiteId)
	assert.Equal(t, from.Id, stored.InheritedFromSiteId)
	assert.Equal(t, 800, stored.Quota)
	requireArchivedCloseIdentity(t, db, customer.Id, from.Id)
	closed, err := GetAdtySite(from.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)
	stillOpen, err := GetAdtySite(to.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, stillOpen.Status)
}

func TestCloseSiteInheritsSameEmailAsSeparateMainIdentity(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, model.EnsureSiteEmailUniqueIndex(db))
	_, site := openActiveSite(t, db, "same-mail")
	const shared = "shared-close@example.com"
	mainUser := referralUser(t, db, "same-mail-main")
	require.NoError(t, db.Model(mainUser).Updates(map[string]any{"email": shared, "quota": 2000}).Error)
	customer := siteCustomer(t, db, site, "same-mail-site")
	require.NoError(t, db.Model(customer).Updates(map[string]any{"email": "Shared-Close@Example.com", "quota": 700}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: customer.Id, Type: model.LogTypeConsume, Quota: 15}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: mainUser.Id, Type: model.LogTypeConsume, Quota: 40}).Error)

	closed, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Immediate: true})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)

	var inherited, original model.User
	require.NoError(t, db.First(&inherited, customer.Id).Error)
	require.NoError(t, db.First(&original, mainUser.Id).Error)
	assert.NotEqual(t, original.Id, inherited.Id)
	assert.Zero(t, inherited.SiteId)
	assert.Zero(t, original.SiteId)
	assert.Equal(t, site.Id, inherited.InheritedFromSiteId)
	assert.Zero(t, original.InheritedFromSiteId)
	assert.Equal(t, 700, inherited.Quota)
	assert.Equal(t, 2000, original.Quota)
	assert.Equal(t, "shared-close@example.com", model.NormalizeEmail(inherited.Email))
	assert.Equal(t, shared, model.NormalizeEmail(original.Email))
	requireArchivedCloseIdentity(t, db, customer.Id, site.Id)
	var mainMemberships int64
	require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", mainUser.Id).Count(&mainMemberships).Error)
	assert.Zero(t, mainMemberships)
	var mainMappings int64
	require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id = ?", mainUser.Id).Count(&mainMappings).Error)
	assert.Zero(t, mainMappings)
	var logs []model.Log
	require.NoError(t, db.Where("user_id IN ?", []int{customer.Id, mainUser.Id}).Order("user_id").Find(&logs).Error)
	require.Len(t, logs, 2)
	byUser := map[int]int{}
	for _, row := range logs {
		byUser[row.UserId] = row.Quota
	}
	assert.Equal(t, 15, byUser[customer.Id])
	assert.Equal(t, 40, byUser[mainUser.Id])
	_, err = model.GetUniqueUserByEmail(shared, 0)
	assert.ErrorIs(t, err, model.ErrEmailAmbiguous)
	var byName model.User
	require.NoError(t, db.Where("username = ?", customer.Username).First(&byName).Error)
	assert.Equal(t, customer.Id, byName.Id)
}

func TestCloseSiteMappingAndArchiveAreIdempotent(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "retry-close")
	customer := siteCustomer(t, db, site, "retry-close-customer")
	require.NoError(t, db.Model(customer).Update("quota", 900).Error)

	closed, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Immediate: true})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)

	first := requireArchivedCloseIdentity(t, db, customer.Id, site.Id)
	var stored model.User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, site.Id, stored.InheritedFromSiteId)
	assert.Equal(t, 900, stored.Quota)
	assert.Equal(t, customer.Id, stored.Id)

	require.NoError(t, ExecuteAdtySiteClose(site.Id, time.Now().Unix()+10))
	second := requireArchivedCloseIdentity(t, db, customer.Id, site.Id)
	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, first.CreatedAt, second.CreatedAt)
	var mappingCount int64
	require.NoError(t, db.Model(&model.AdtyUserSiteMigration{}).Where("user_id = ?", customer.Id).Count(&mappingCount).Error)
	assert.Equal(t, int64(1), mappingCount)
	var memberships []model.AdtySiteCustomer
	require.NoError(t, db.Where("user_id = ?", customer.Id).Find(&memberships).Error)
	require.Len(t, memberships, 1)
	assert.Equal(t, model.AdtySiteCustomerArchived, memberships[0].Status)
	assert.Equal(t, first.SourceSiteId, memberships[0].SiteId)
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Zero(t, stored.SiteId)
	assert.Equal(t, site.Id, stored.InheritedFromSiteId)
	assert.Equal(t, customer.Id, stored.Id)
	assert.Equal(t, 900, stored.Quota)
}

func TestForcedCloseIgnoresClawBackFlag(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "forced")
	var clawed []int
	saved := AdtySiteClawBackTx
	AdtySiteClawBackTx = func(tx *gorm.DB, s *model.AdtySite, _ int) error {
		clawed = append(clawed, s.Id)
		return assert.AnError
	}
	t.Cleanup(func() { AdtySiteClawBackTx = saved })
	_, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{ClawBack: true, Immediate: true, Reason: "abuse"})
	require.NoError(t, err)
	assert.Empty(t, clawed, "close must not invoke the clawback hook")
	closed, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.False(t, closed.CloseClawBack, "ClawBack on the close plan must be ignored")

	// 已经排期且历史标志为 true 的关站，执行时同样不能走扣回。
	_, queued := openActiveSite(t, db, "forced-queued")
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", queued.Id).Updates(map[string]any{
		"status": model.AdtySiteStatusClosing, "close_execute_at": 1, "close_claw_back": true,
	}).Error)
	require.NoError(t, ExecuteAdtySiteClose(queued.Id, 10))
	assert.Empty(t, clawed, "a previously flagged close must not invoke the clawback hook")
	closedQueued, err := GetAdtySite(queued.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closedQueued.Status)
}

func TestSetAdtySiteStatusRejectsPauseAndResume(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "no-pause")

	paused, err := SetAdtySiteStatus(site.Id, 1, model.AdtySiteStatusPaused, "maintenance")
	assert.Nil(t, paused)
	assert.ErrorIs(t, err, ErrAdtySitePauseRetired)
	stored, err := GetAdtySite(site.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusActive, stored.Status)

	require.NoError(t, db.Model(&model.AdtySite{}).Where("id = ?", site.Id).Update("status", model.AdtySiteStatusPaused).Error)
	resumed, err := SetAdtySiteStatus(site.Id, 1, model.AdtySiteStatusActive, "resume")
	assert.Nil(t, resumed)
	assert.ErrorIs(t, err, ErrAdtySitePauseRetired)
	stored, err = GetAdtySite(site.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusPaused, stored.Status, "leftover paused rows must not be flipped by the retired switch")

	requested, err := RequestAdtySiteClose(site.OwnerUserId, "owner asked to close")
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusPaused, requested.Status)
	assert.NotZero(t, requested.CloseRequestedAt)

	closed, err := ScheduleAdtySiteClose(site.Id, 1, AdtySiteClosePlan{Immediate: true, Reason: "retire leftover"})
	require.NoError(t, err)
	assert.Equal(t, model.AdtySiteStatusClosed, closed.Status)
	assert.Zero(t, closed.CloseTransferTo)
}
