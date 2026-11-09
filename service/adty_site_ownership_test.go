package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type siteOwnershipFixture struct {
	db              *gorm.DB
	root, old, next *model.User
	site            model.AdtySite
}

func newSiteOwnershipFixture(t *testing.T, dialect string) siteOwnershipFixture {
	t.Helper()
	db := prepareReferralDialectDB(t, dialect)
	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved; InvalidateAdtySiteCache() })
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(0.0), new(5.0), new(10.0)
	f := siteOwnershipFixture{db: db, root: referralUser(t, db, "handover-root"), old: referralUser(t, db, "handover-old"), next: referralUser(t, db, "handover-next")}
	require.NoError(t, db.Model(f.root).Update("role", common.RoleRootUser).Error)
	f.site = model.AdtySite{OwnerUserId: f.old.Id, Subdomain: "handover-site", RootDomain: "sites.example.com", Status: model.AdtySiteStatusActive,
		AgreementVer: AdtyCurrentSiteAgreementVersion(), AgreementAt: 123}
	require.NoError(t, db.Create(&f.site).Error)
	for _, user := range []*model.User{f.old, f.next} {
		_, err := Referral.EnsureAffiliate(user.Id)
		require.NoError(t, err)
	}
	require.NoError(t, db.Model(&model.ReferralAffiliate{}).Where("user_id = ?", f.old.Id).Update("site_owner_since", 100).Error)
	return f
}

func (f siteOwnershipFixture) input() AdtySiteOwnerHandoverInput {
	return AdtySiteOwnerHandoverInput{SiteId: f.site.Id, OperatorId: f.root.Id, ExpectedOwnerUserId: f.old.Id,
		ExpectedOwnershipVersion: new(int64(0)), NewOwnerUserId: f.next.Id, Reason: "Owner replacement"}
}

func TestSiteOwnerHandoverAtomicAuthorityDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			f := newSiteOwnershipFixture(t, dialect)
			staff, err := CreateAdtyStaff(f.old.Id, f.site.Id, AdtyStaffInput{Username: "handover-staff", DisplayName: "Support", Password: "employee-password-42", Role: "support"})
			require.NoError(t, err)
			require.NoError(t, f.db.Model(&model.ReferralCommissionAccount{}).Where("user_id = ?", f.old.Id).Update("available_amount", 12.34).Error)
			input := f.input()
			input.OperatorId = f.next.Id
			_, err = HandoverAdtySiteOwner(input)
			require.Error(t, err, "service must not trust a caller's Root claim")
			for _, invalid := range []struct {
				column string
				value  any
			}{{"status", common.UserStatusDisabled}, {"site_id", f.site.Id}, {"staff_site_id", f.site.Id}} {
				require.NoError(t, f.db.Model(f.next).Update(invalid.column, invalid.value).Error)
				_, err = HandoverAdtySiteOwner(f.input())
				require.ErrorIs(t, err, ErrAdtySiteOwnershipTarget)
				require.NoError(t, f.db.Model(f.next).Updates(map[string]any{"status": common.UserStatusEnabled, "site_id": 0, "staff_site_id": 0}).Error)
			}
			for _, status := range []string{model.AdtySiteStatusPending, model.AdtySiteStatusClosing, model.AdtySiteStatusClosed} {
				require.NoError(t, f.db.Model(&f.site).Update("status", status).Error)
				_, err = HandoverAdtySiteOwner(f.input())
				require.ErrorIs(t, err, ErrAdtySiteStatus)
			}
			require.NoError(t, f.db.Model(&f.site).Update("status", model.AdtySiteStatusPaused).Error)
			const callback = "test:handover-audit-rollback"
			require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "adty_site_audits" {
					tx.AddError(errors.New("audit unavailable"))
				}
			}))
			_, err = HandoverAdtySiteOwner(f.input())
			require.ErrorContains(t, err, "audit unavailable")
			require.NoError(t, f.db.Callback().Create().Remove(callback))
			var rolledBack model.AdtySite
			require.NoError(t, f.db.First(&rolledBack, f.site.Id).Error)
			assert.Equal(t, f.old.Id, rolledBack.OwnerUserId)
			assert.Equal(t, AdtyCurrentSiteAgreementVersion(), rolledBack.AgreementVer)
			var changes int64
			require.NoError(t, f.db.Model(&model.AdtySiteOwnership{}).Count(&changes).Error)
			assert.Zero(t, changes)
			_, _, err = AdtyStaffAccess(staff.UserId, 1, "customers.read")
			require.NoError(t, err, "a failed handover cannot revoke staff")
			var nextAffiliate model.ReferralAffiliate
			require.NoError(t, f.db.Where("user_id = ?", f.next.Id).First(&nextAffiliate).Error)
			assert.Zero(t, nextAffiliate.SiteOwnerSince)

			site, err := HandoverAdtySiteOwner(f.input())
			require.NoError(t, err)
			assert.Equal(t, f.next.Id, site.OwnerUserId)
			assert.EqualValues(t, 1, site.OwnershipVersion)
			assert.Empty(t, site.AgreementVer)
			assert.Zero(t, site.AgreementAt)
			_, err = AdtyOwnerSite(f.old.Id)
			require.ErrorIs(t, err, ErrAdtySiteNotOwner)
			required, _ := AdtySiteTermsStatus(f.next.Id)
			assert.True(t, required)
			_, _, err = AdtyStaffAccess(staff.UserId, 1, "customers.read")
			require.ErrorIs(t, err, ErrAdtyStaffDenied)
			var member model.AdtySiteStaff
			require.NoError(t, f.db.First(&member, staff.Id).Error)
			assert.Equal(t, f.next.Id, member.OwnerUserId)
			assert.Equal(t, "disabled", member.Status)
			assert.Equal(t, "[]", member.Permissions)
			assert.EqualValues(t, 2, member.Version)
			for _, id := range []int{f.old.Id, f.next.Id, staff.UserId} {
				var user model.User
				require.NoError(t, f.db.First(&user, id).Error)
				assert.EqualValues(t, 2, user.AuthVersion)
			}
			var oldAffiliate model.ReferralAffiliate
			require.NoError(t, f.db.Where("user_id = ?", f.old.Id).First(&oldAffiliate).Error)
			assert.EqualValues(t, 100, oldAffiliate.SiteOwnerSince)
			require.NoError(t, f.db.First(&nextAffiliate, nextAffiliate.Id).Error)
			assert.Positive(t, nextAffiliate.SiteOwnerSince)
			assert.InDelta(t, 12.34, account(t, f.db, oldAffiliate.Id).AvailableAmount, 0.000001, "handover must never move balances")
			require.NoError(t, AcceptAdtySiteTerms(f.next.Id, AdtyCurrentSiteAgreementVersion()))
			grants := []string{"customers.read"}
			_, err = MutateAdtyStaff(f.next.Id, site.Id, staff.Id, AdtySiteOwnerActionStaffUpdate,
				AdtyStaffInput{ExpectedVersion: 2, Role: "support", DisplayName: "Support", Permissions: &grants, Status: "enabled"})
			require.NoError(t, err, "new owner can explicitly reauthorize the suspended employee")
			_, _, err = AdtyStaffAccess(staff.UserId, 1, "customers.read")
			require.Error(t, err, "reauthorizing must not revive old staff sessions")
			_, _, err = AdtyStaffAccess(staff.UserId, 3, "customers.read")
			require.NoError(t, err)
			back := f.input()
			back.ExpectedOwnerUserId, back.NewOwnerUserId, back.ExpectedOwnershipVersion = f.next.Id, f.old.Id, new(int64(1))
			_, err = HandoverAdtySiteOwner(back)
			require.NoError(t, err)
			_, err = HandoverAdtySiteOwner(f.input())
			require.ErrorIs(t, err, ErrAdtySiteOwnershipConflict, "A -> B -> A cannot make an old Root form current again")
		})
	}
}

func TestSiteOwnerHandoverSerializesCompetingOwnership(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, race := range []string{"same-site", "same-target", "opening"} {
			t.Run(dialect+"/"+race, func(t *testing.T) {
				f := newSiteOwnershipFixture(t, dialect)
				other := referralUser(t, f.db, "handover-other")
				second := f.input()
				if race == "same-site" {
					second.NewOwnerUserId = other.Id
				}
				if race == "same-target" {
					site := model.AdtySite{OwnerUserId: other.Id, Subdomain: "other-site", Status: model.AdtySiteStatusActive}
					require.NoError(t, f.db.Create(&site).Error)
					second.SiteId, second.ExpectedOwnerUserId = site.Id, other.Id
				}
				start := make(chan struct{})
				results := make(chan error, 2)
				var group sync.WaitGroup
				group.Go(func() { <-start; _, err := HandoverAdtySiteOwner(f.input()); results <- err })
				group.Go(func() {
					<-start
					if race == "opening" {
						_, err := OpenAdtySite(f.next.Id, "new-opening", model.AdtySiteOpenedByAdmin, AdtySitePreset{OperatorId: f.root.Id})
						results <- err
						return
					}
					_, err := HandoverAdtySiteOwner(second)
					results <- err
				})
				close(start)
				group.Wait()
				close(results)
				success := 0
				for err := range results {
					if err == nil {
						success++
					} else {
						assert.True(t, errors.Is(err, ErrAdtySiteAlreadyOwner) || errors.Is(err, ErrAdtySiteOwnershipConflict), "%v", err)
					}
				}
				assert.Equal(t, 1, success)
				var owned int64
				require.NoError(t, f.db.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status <> ?", f.next.Id, model.AdtySiteStatusClosed).Count(&owned).Error)
				assert.LessOrEqual(t, owned, int64(1))
			})
		}
	}
}

func TestSiteOwnershipKeepsDelayedEarningsAndRefundsWithOriginalOwner(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			f := newSiteOwnershipFixture(t, dialect)
			buyer := referralUser(t, f.db, "handover-customer")
			require.NoError(t, f.db.Model(buyer).Updates(map[string]any{"site_id": f.site.Id, "quota": 5000000}).Error)
			require.NoError(t, f.db.Create(&model.AdtySiteCustomer{UserId: buyer.Id, SiteId: f.site.Id, Reviewed: true}).Error)
			require.NoError(t, f.db.Create(&model.AdtySiteEarningDaily{SiteId: f.site.Id, UserId: buyer.Id, Day: "2026-01-01", Kind: model.AdtySiteEarningUsage,
				Earning: decimal.NewFromInt(3), Status: model.AdtySiteEarningPending}).Error)
			oldOp, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "before-handover", UserId: buyer.Id, SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8}, 500000)
			require.NoError(t, err)
			assert.Equal(t, f.old.Id, oldOp.SiteOwnerUserId)
			createOrder := func(trade string, bonus int64) *model.AdtyTopupOrder {
				t.Helper()
				ext := &model.AdtyTopupOrder{TradeNo: trade, UserId: buyer.Id, Provider: model.PaymentProviderAdtyBEpusdt, Method: "usdt.trc20", SiteId: f.site.Id,
					FaceUSD: decimal.NewFromInt(100), BonusUSD: decimal.NewFromInt(bonus), CreditQuota: (100 + bonus) * 500000,
					PayAmount: decimal.NewFromInt(100), PayCurrency: "USDT", ExpireAt: time.Now().Unix() + 900}
				require.NoError(t, model.CreateAdtyTopUp(&model.TopUp{TradeNo: trade, UserId: buyer.Id, Amount: 100, PaymentProvider: ext.Provider, PaymentMethod: ext.Method, Status: common.TopUpStatusPending}, ext))
				return ext
			}
			oldOrder := createOrder("old-owner-order", 2)
			assert.Equal(t, f.old.Id, oldOrder.SiteOwnerUserId)
			site, err := HandoverAdtySiteOwner(f.input())
			require.NoError(t, err)
			balances, err := GetAdtySiteBalances(site)
			require.NoError(t, err)
			assert.True(t, balances.PendingUSD.IsZero(), "old pending earnings are not the new owner's assets")
			newOp, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "after-handover", UserId: buyer.Id, SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8}, 500000)
			require.NoError(t, err)
			assert.Equal(t, f.next.Id, newOp.SiteOwnerUserId)
			newOrder := createOrder("new-owner-order", 1)
			assert.Equal(t, f.next.Id, newOrder.SiteOwnerUserId)
			third := referralUser(t, f.db, "handover-third")
			input := f.input()
			input.ExpectedOwnerUserId, input.NewOwnerUserId, input.ExpectedOwnershipVersion = f.next.Id, third.Id, new(int64(1))
			site, err = HandoverAdtySiteOwner(input)
			require.NoError(t, err)
			for _, key := range []string{oldOp.OperationKey, newOp.OperationKey} {
				require.NoError(t, model.QueueBillingSettlement(key, 500000, false))
				_, _, err = model.ApplyBillingSettlement(key)
				require.NoError(t, err)
				require.NoError(t, projectAdtySiteEarningEvent(context.Background(), key))
			}
			require.NoError(t, model.QueueDirectBillingDelta(model.BillingOperation{OperationKey: "late-refund", EarningRootKey: oldOp.OperationKey, UserId: buyer.Id, SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8}, -250000))
			_, _, err = model.ApplyBillingSettlement("late-refund")
			require.NoError(t, err)
			require.NoError(t, projectAdtySiteEarningEvent(context.Background(), "late-refund"))
			var refund model.AdtySiteEarningEvent
			require.NoError(t, f.db.Where("operation_key = ?", "late-refund").First(&refund).Error)
			assert.Equal(t, f.old.Id, refund.SiteOwnerUserId)
			for _, order := range []*model.AdtyTopupOrder{oldOrder, newOrder} {
				_, err = model.CompleteAdtyTopUp(order.TradeNo, model.AdtyPaymentFacts{Provider: order.Provider, PaidAmount: decimal.NewFromInt(100), FiatAmount: decimal.NewFromInt(100), PaidCurrency: "USDT", Source: model.AdtyPaymentSourceCallback}, "")
				require.NoError(t, err)
			}
			_, err = model.RefundAdtyTopUp(oldOrder.TradeNo, "Original owner refund", f.root.Id, "handover-refund", 50000000)
			require.NoError(t, err)
			_, err = model.RefundAdtyTopUp(oldOrder.TradeNo, "Original owner refund", f.root.Id, "handover-refund", 50000000)
			require.NoError(t, err)
			settled, err := settleAdtySite(site.Id, "2099-01-01", time.Now().Unix())
			require.NoError(t, err)
			require.True(t, settled)
			for _, expected := range []struct {
				user      int
				available float64
			}{{f.old.Id, 3.3}, {f.next.Id, -0.4}, {third.Id, 0}} {
				var accounts []model.ReferralCommissionAccount
				require.NoError(t, f.db.Where("user_id = ?", expected.user).Find(&accounts).Error)
				actual := 0.0
				if len(accounts) > 0 {
					actual = accounts[0].AvailableAmount
				}
				assert.InDelta(t, expected.available, actual, 0.000001, fmt.Sprint(expected.user))
			}
			// Replaying projection and settlement cannot move money a second time.
			require.NoError(t, projectAdtySiteEarningEvent(context.Background(), "late-refund"))
			settled, err = settleAdtySite(site.Id, "2099-01-01", time.Now().Unix())
			require.NoError(t, err)
			assert.False(t, settled)
		})
	}
}

func TestSiteOwnershipTaskInheritsReservationAcrossHandover(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%t", dialect, legacy), func(t *testing.T) {
				f := newSiteOwnershipFixture(t, dialect)
				require.NoError(t, f.db.AutoMigrate(&model.Task{}))
				buyer := referralUser(t, f.db, "handover-task-customer")
				require.NoError(t, f.db.Model(buyer).Update("quota", 5000000).Error)
				op, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "task-original", UserId: buyer.Id, SiteId: f.site.Id, SiteMarkup: 2, SiteCostFactor: 0.8}, 500000)
				require.NoError(t, err)
				if legacy {
					require.NoError(t, f.db.Model(op).Update("site_owner_user_id", 0).Error)
				}
				_, err = HandoverAdtySiteOwner(f.input())
				require.NoError(t, err)
				// A context stamped after reservation cannot replace its owner.
				task := &model.Task{UserId: buyer.Id, TaskID: "handover-task", Quota: 500000,
					PrivateData: model.TaskPrivateData{BillingOperationKey: op.OperationKey,
						BillingContext: &model.TaskBillingContext{SiteId: f.site.Id, SiteOwnerUserId: f.next.Id, SiteMarkup: 2, SiteCostFactor: 0.8}}}
				require.NoError(t, task.InsertWithContext(context.Background()))
				assert.Equal(t, f.old.Id, task.PrivateData.BillingContext.SiteOwnerUserId)
				_, _, err = model.ApplyBillingSettlement(op.OperationKey)
				require.NoError(t, err)
				key, err := model.QueueTaskBillingOperation(task, 250000, false)
				require.NoError(t, err)
				adjustment, _, err := model.ApplyBillingSettlement(key)
				require.NoError(t, err)
				assert.Equal(t, f.old.Id, adjustment.SiteOwnerUserId)
				require.NoError(t, projectAdtySiteEarningEvent(context.Background(), key))
				var event model.AdtySiteEarningEvent
				require.NoError(t, f.db.Where("operation_key = ?", key).First(&event).Error)
				assert.Equal(t, f.old.Id, event.SiteOwnerUserId)
			})
		}
	}
}

func TestSiteOwnershipMidjourneyChargeAndRefundKeepAcceptanceOwner(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			f := newSiteOwnershipFixture(t, dialect)
			require.NoError(t, f.db.AutoMigrate(&model.Midjourney{}))
			buyer := referralUser(t, f.db, "handover-mj-customer")
			require.NoError(t, f.db.Model(buyer).Update("quota", 5000000).Error)
			old := &model.Midjourney{UserId: buyer.Id, Quota: 500000, BillingChargePending: true, BillingOperationKey: "mj:before",
				BillingSiteId: f.site.Id, BillingSiteMarkup: 2, BillingSiteCostFactor: 0.8}
			require.NoError(t, old.Insert())
			legacy := &model.Midjourney{UserId: buyer.Id, Quota: 500000, BillingChargePending: true,
				BillingSiteId: f.site.Id, BillingSiteMarkup: 2, BillingSiteCostFactor: 0.8}
			require.NoError(t, f.db.Create(legacy).Error)
			_, err := HandoverAdtySiteOwner(f.input())
			require.NoError(t, err)
			next := &model.Midjourney{UserId: buyer.Id, Quota: 500000, BillingChargePending: true, BillingOperationKey: "mj:after",
				BillingSiteId: f.site.Id, BillingSiteOwnerUserId: f.old.Id, BillingSiteMarkup: 2, BillingSiteCostFactor: 0.8}
			require.NoError(t, next.Insert())
			assert.Equal(t, f.next.Id, next.BillingSiteOwnerUserId, "a new acceptance freezes SQL ownership, not an earlier mutable context")
			for _, input := range []struct {
				task  *model.Midjourney
				owner int
			}{{old, f.old.Id}, {legacy, f.old.Id}, {next, f.next.Id}} {
				key, err := model.QueueMidjourneyBillingOperation(input.task, false)
				require.NoError(t, err)
				op, _, err := model.ApplyBillingSettlement(key)
				require.NoError(t, err)
				assert.Equal(t, input.owner, op.SiteOwnerUserId)
				require.NoError(t, projectAdtySiteEarningEvent(context.Background(), key))
				// Replay through a fresh SQL copy, as a restarted poller would.
				require.NoError(t, f.db.First(input.task, input.task.Id).Error)
				refund, err := model.QueueMidjourneyBillingOperation(input.task, true)
				require.NoError(t, err)
				op, _, err = model.ApplyBillingSettlement(refund)
				require.NoError(t, err)
				assert.Equal(t, input.owner, op.SiteOwnerUserId)
				require.NoError(t, projectAdtySiteEarningEvent(context.Background(), refund))
				_, applied, err := model.ApplyBillingSettlement(refund)
				require.NoError(t, err)
				assert.False(t, applied)
			}
		})
	}
}

func TestSiteOwnershipForcedCloseClawsBackEachActualOwner(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			f := newSiteOwnershipFixture(t, dialect)
			site, err := HandoverAdtySiteOwner(f.input())
			require.NoError(t, err)
			for _, owner := range []int{f.old.Id, f.next.Id} {
				require.NoError(t, f.db.Model(&model.ReferralCommissionAccount{}).Where("user_id = ?", owner).Update("available_amount", 100).Error)
			}
			for _, input := range []struct {
				owner               int
				available, negative int64
			}{{0, 7, -2}, {f.next.Id, 11, -3}} {
				for i, row := range []model.AdtySiteEarningDaily{
					{Kind: model.AdtySiteEarningUsage, Status: model.AdtySiteEarningAvailable, Earning: decimal.NewFromInt(input.available)},
					{Kind: model.AdtySiteEarningRefund, Status: model.AdtySiteEarningPending, Earning: decimal.NewFromInt(input.negative)},
					{Kind: model.AdtySiteEarningUsage, Status: model.AdtySiteEarningPending, Earning: decimal.NewFromInt(100)},
					{Kind: model.AdtySiteEarningSkipped, Status: model.AdtySiteEarningPending, Earning: decimal.NewFromInt(10)},
					{Kind: model.AdtySiteEarningTrial, Status: model.AdtySiteEarningAvailable, Earning: decimal.NewFromInt(-4)},
				} {
					row.SiteId, row.SiteOwnerUserId, row.UserId, row.Day = site.Id, input.owner, f.root.Id, fmt.Sprintf("2026-01-%02d", i+1)
					require.NoError(t, f.db.Create(&row).Error)
				}
			}
			for range 2 {
				require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return adtySiteClawBackTx(tx, site, f.root.Id) }))
				for _, expected := range []struct {
					owner  int
					amount float64
				}{{f.old.Id, 93}, {f.next.Id, 89}} {
					var account model.ReferralCommissionAccount
					require.NoError(t, f.db.Where("user_id = ?", expected.owner).First(&account).Error)
					assert.InDelta(t, expected.amount, account.AvailableAmount, 0.000001)
				}
			}
		})
	}
}
