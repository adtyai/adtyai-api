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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLegacyTaskEarningReviewClosesRefundAfterCustomerTransfer(t *testing.T) {
	for _, mj := range []bool{false, true} {
		for _, released := range []bool{false, true} {
			t.Run(fmt.Sprintf("mj_%t_released_%t", mj, released), func(t *testing.T) {
				db := prepareSiteDB(t)
				require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Channel{}, &model.Token{}))
				owner, site := openActiveSite(t, db, "legacy-root-review")
				customer := siteCustomer(t, db, site, "legacy-root-customer")
				require.NoError(t, db.Model(customer).Update("quota", perUnit("275")).Error)
				if !released {
					require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
				}
				AdtySiteEarningHook(site.Id, customer.Id, perUnit("125"), 1.25, .8, model.AdtySiteEarningUsage)
				// Transfer clears current risk. It cannot establish how an old site's
				// particular charge was classified, even with unrelated positive earnings.
				_, nextSite := openActiveSite(t, db, "legacy-root-next-site")
				require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).
					Updates(map[string]any{"site_id": nextSite.Id, "risk_flag": "", "reviewed": false}).Error)
				require.NoError(t, db.Model(customer).Update("site_id", nextSite.Id).Error)
				AdtySiteEarningHook(site.Id, customer.Id, perUnit("50"), 1.25, .8, model.AdtySiteEarningUsage)
				var key string
				if mj {
					task := &model.Midjourney{UserId: customer.Id, Quota: int(perUnit("125")), Status: "FAILURE",
						BillingSiteId: site.Id, BillingSiteMarkup: 1.25, BillingSiteCostFactor: .8}
					require.NoError(t, db.Create(task).Error)
					require.True(t, RefundMidjourneyQuota(context.Background(), task, "legacy failed"))
					key = fmt.Sprintf("mj:%d:refund", task.Id)
				} else {
					task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, .8)
					require.True(t, RefundTaskQuota(context.Background(), task, "legacy failed"))
					key = fmt.Sprintf("task:%d:refund", task.ID)
				}
				var event model.AdtySiteEarningEvent
				require.NoError(t, db.Where("operation_key = ?", key).First(&event).Error)
				assert.Equal(t, model.AdtyEarningNeedsReview, event.ReviewStatus)
				assert.False(t, event.Projected)
				var wallet model.User
				require.NoError(t, db.First(&wallet, customer.Id).Error)
				assert.Equal(t, int(perUnit("400")), wallet.Quota, "earnings review must not delay the customer refund")
				_, err := RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 30))
				require.NoError(t, err)
				affiliate, err := Referral.EnsureAffiliate(owner.Id)
				require.NoError(t, err)
				assert.Zero(t, account(t, db, affiliate.Id).AvailableAmount, "unresolved old earnings hold positive settlement")
				decision, opposite := model.AdtyEarningOriginalSkipped, model.AdtyEarningOriginalReleased
				if released {
					decision, opposite = opposite, decision
				}
				require.NoError(t, ReviewAdtySiteEarningRoot(key, 1, decision, "verified the original task charge evidence"))
				require.NoError(t, ReviewAdtySiteEarningRoot(key, 1, decision, "same decision replay"))
				require.ErrorIs(t, ReviewAdtySiteEarningRoot(key, 1, opposite, "conflicting decision"), model.ErrBillingOperationConflict)
				require.NoError(t, RecoverBillingOperations(context.Background(), 100))
				_, err = RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 30))
				require.NoError(t, err)
				assert.InDelta(t, 18, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
				var auditCount int64
				require.NoError(t, db.Model(&model.AdtySiteAudit{}).Where("site_id = ? AND action = ?", site.Id, "earning_review").Count(&auditCount).Error)
				assert.EqualValues(t, 1, auditCount)
			})
		}
	}
}

func TestLegacyPositiveAdjustmentUsesOneRootReviewThroughRefund(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Token{}, &model.Channel{}))
	owner, site := openActiveSite(t, db, "legacy-positive-review")
	customer := siteCustomer(t, db, site, "legacy-positive-customer")
	require.NoError(t, db.Model(customer).Update("quota", perUnit("275")).Error)
	require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
	AdtySiteEarningHook(site.Id, customer.Id, perUnit("125"), 1.25, .8, model.AdtySiteEarningUsage)
	task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, .8)
	require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, false))
	RecalculateTaskQuota(context.Background(), task, int(perUnit("150")), common.NewMessage("extra usage"))
	key := fmt.Sprintf("task:%d:adjust:%d", task.ID, int(perUnit("150")))
	var event model.AdtySiteEarningEvent
	require.NoError(t, db.Where("operation_key = ?", key).First(&event).Error)
	assert.Equal(t, model.AdtyEarningNeedsReview, event.ReviewStatus)
	require.NoError(t, ReviewAdtySiteEarningRoot(key, 1, model.AdtyEarningOriginalSkipped, "original charge remained skipped"))
	require.True(t, RefundTaskQuota(context.Background(), task, "later failed"))
	require.NoError(t, RecoverBillingOperations(context.Background(), 100))
	_, err := RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 30))
	require.NoError(t, err)
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.Zero(t, account(t, db, affiliate.Id).AvailableAmount)
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningSkipped].IsZero())
	event = model.AdtySiteEarningEvent{}
	require.NoError(t, db.Where("operation_key = ?", fmt.Sprintf("task:%d:refund", task.ID)).First(&event).Error)
	assert.Equal(t, model.AdtyEarningOriginalSkipped, event.ReviewStatus)
}

func TestLegacyReviewRequiresMatchingCustomerAndOriginalSiteAudit(t *testing.T) {
	for _, matching := range []bool{false, true} {
		t.Run(fmt.Sprintf("matching_%t", matching), func(t *testing.T) {
			db := prepareSiteDB(t)
			require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Token{}, &model.Channel{}))
			_, site := openActiveSite(t, db, "legacy-review-proof")
			customer := siteCustomer(t, db, site, "legacy-review-proof-customer")
			require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
			AdtySiteEarningHook(site.Id, customer.Id, perUnit("125"), 1.25, .8, model.AdtySiteEarningUsage)
			task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, .8)
			if matching {
				require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
			} else {
				require.NoError(t, recordSiteAuditTx(db, site.Id, "customer_review", 1,
					map[string]any{"user_id": customer.Id + 1}, map[string]any{"convert_skipped": true}, "another customer"))
				require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, false))
			}
			require.True(t, RefundTaskQuota(context.Background(), task, "legacy failed"))
			key := fmt.Sprintf("task:%d:refund", task.ID)
			var event model.AdtySiteEarningEvent
			require.NoError(t, db.Where("operation_key = ?", key).First(&event).Error)
			if matching {
				assert.Equal(t, model.AdtyEarningOriginalReleased, event.ReviewStatus)
				assert.True(t, event.Projected)
			} else {
				require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
				require.NoError(t, RecoverBillingOperations(context.Background(), 100))
				require.NoError(t, db.Where("operation_key = ?", key).First(&event).Error)
				assert.Equal(t, model.AdtyEarningNeedsReview, event.ReviewStatus, "blanket review cannot resolve an already isolated legacy root")
				assert.False(t, event.Projected)
			}
		})
	}
}

func TestRiskReleaseSurvivesZeroEventRootTransferAndStaysInOriginalSite(t *testing.T) {
	for _, releaseOriginal := range []bool{false, true} {
		t.Run(fmt.Sprintf("release_original_%t", releaseOriginal), func(t *testing.T) {
			db := prepareSiteDB(t)
			require.NoError(t, db.AutoMigrate(&model.Token{}))
			_, from := openActiveSite(t, db, "zero-root-from")
			_, to := openActiveSite(t, db, "zero-root-to")
			customer := siteCustomer(t, db, from, "zero-root-customer")
			require.NoError(t, db.Model(customer).Update("quota", perUnit("400")).Error)
			require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
			root, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "request:zero-root", UserId: customer.Id,
				SiteId: from.Id, SiteMarkup: 1.25, SiteCostFactor: .8}, 0)
			require.NoError(t, err)
			if releaseOriginal {
				require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
			}
			_, err = ScheduleAdtySiteClose(from.Id, 1, AdtySiteClosePlan{TransferTo: to.Id, Immediate: true})
			require.NoError(t, err)
			var moved model.AdtySiteCustomer
			require.NoError(t, db.Where("user_id = ?", customer.Id).First(&moved).Error)
			assert.False(t, moved.EarningRiskReleased, "transfer resets the new site's risk release")
			require.NoError(t, db.Model(&moved).Update("risk_flag", "self_use").Error)
			require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true)) // B's review cannot release A's root.
			key := "direct:zero-root-chunk"
			require.NoError(t, model.QueueDirectBillingDelta(model.BillingOperation{OperationKey: key, EarningRootKey: root.OperationKey,
				UserId: customer.Id, SiteId: from.Id, SiteMarkup: 1.25, SiteCostFactor: .8}, int(perUnit("125"))))
			_, _, err = model.ApplyBillingSettlement(key)
			require.NoError(t, err)
			var event model.AdtySiteEarningEvent
			require.NoError(t, db.Where("operation_key = ?", key).First(&event).Error)
			assert.Equal(t, releaseOriginal, event.RiskReleased)
			assert.Equal(t, "self_use", event.RiskFlag)
			// A previous approval is also insufficient for a newly frozen risky
			// request after a later risk reset within the same site.
			require.NoError(t, db.Model(&moved).Updates(map[string]any{"reviewed": false, "earning_risk_released": false}).Error)
			newRoot, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "request:new-risk-after-old-review",
				UserId: customer.Id, SiteId: to.Id, SiteMarkup: 1.25, SiteCostFactor: .8}, 0)
			require.NoError(t, err)
			require.NoError(t, model.QueueBillingSettlement(newRoot.OperationKey, int(perUnit("50")), false))
			_, _, err = model.ApplyBillingSettlement(newRoot.OperationKey)
			require.NoError(t, err)
			event = model.AdtySiteEarningEvent{}
			require.NoError(t, db.Where("operation_key = ?", newRoot.OperationKey).First(&event).Error)
			assert.False(t, event.RiskReleased, "a historical review cannot release a later risky root")
		})
	}
}

func TestLegacyEarningReviewProjectionFailureRecoversSettlementHold(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Token{}, &model.Channel{}))
	owner, site := openActiveSite(t, db, "legacy-review-recovery")
	customer := siteCustomer(t, db, site, "legacy-review-recovery-customer")
	AdtySiteEarningHook(site.Id, customer.Id, perUnit("175"), 1.25, .8, model.AdtySiteEarningUsage)
	task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, .8)
	require.True(t, RefundTaskQuota(context.Background(), task, "legacy failed"))
	key := fmt.Sprintf("task:%d:refund", task.ID)
	fault := errors.New("earning projection temporarily unavailable")
	const callback = "test:legacy-earning-projection-fault"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "adty_site_earning_daily" {
			tx.AddError(fault)
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove(callback) })
	require.ErrorIs(t, ReviewAdtySiteEarningRoot(key, 1, model.AdtyEarningOriginalReleased, "verified previously payable original charge"), fault)
	require.NoError(t, db.Callback().Create().Remove(callback))
	require.NoError(t, RecoverBillingOperations(context.Background(), 100))
	_, err := RunAdtySiteSettlementOnce(time.Now().AddDate(0, 0, 30))
	require.NoError(t, err)
	var stored model.AdtySite
	require.NoError(t, db.First(&stored, site.Id).Error)
	assert.Empty(t, stored.SettleHoldReason, "a committed review must recover without an operator repeating it")
	affiliate, err := Referral.EnsureAffiliate(owner.Id)
	require.NoError(t, err)
	assert.InDelta(t, 18, account(t, db, affiliate.Id).AvailableAmount, 1e-9)
}

func TestLegacyRootDecisionSerializesAdjustmentWithoutCustomer(t *testing.T) {
	db := prepareSiteDB(t)
	if db.Dialector.Name() != "postgres" {
		t.Skip("requires PostgreSQL row locks")
	}
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Midjourney{}, &model.Token{}, &model.Channel{}))
	_, site := openActiveSite(t, db, "legacy-review-lock")
	customer := siteCustomer(t, db, site, "legacy-review-lock-customer")
	require.NoError(t, db.Model(customer).Update("quota", perUnit("400")).Error)
	task := siteTask(t, db, customer, site, int(perUnit("125")), 1.25, .8)
	RecalculateTaskQuota(context.Background(), task, int(perUnit("150")), common.NewMessage("first adjustment"))
	firstKey := fmt.Sprintf("task:%d:adjust:%d", task.ID, int(perUnit("150")))
	require.NoError(t, db.Where("user_id = ?", customer.Id).Delete(&model.AdtySiteCustomer{}).Error)
	nextKey, err := model.QueueTaskBillingOperation(task, int(perUnit("100")), false)
	require.NoError(t, err)
	decisionWritten, staleRootRead, releaseDecision := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var writtenOnce, readOnce sync.Once
	const updateCallback = "test:pause-legacy-root-decision"
	const queryCallback = "test:observe-legacy-root-snapshot"
	require.NoError(t, db.Callback().Update().After("gorm:update").Register(updateCallback, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && updates["earning_review_status"] == model.AdtyEarningOriginalSkipped {
			writtenOnce.Do(func() { close(decisionWritten); <-releaseDecision })
		}
	}))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		rows, ok := tx.Statement.Dest.(*[]model.BillingOperation)
		if ok && len(*rows) == 1 && (*rows)[0].Kind == "earning_root" && (*rows)[0].EarningReviewStatus == model.AdtyEarningNeedsReview {
			readOnce.Do(func() { close(staleRootRead) })
		}
	}))
	t.Cleanup(func() { db.Callback().Update().Remove(updateCallback); db.Callback().Query().Remove(queryCallback) })
	reviewDone, applyDone := make(chan error, 1), make(chan error, 1)
	go func() {
		reviewDone <- ReviewAdtySiteEarningRoot(firstKey, 1, model.AdtyEarningOriginalSkipped, "original charge evidence")
	}()
	select {
	case <-decisionWritten:
	case <-time.After(5 * time.Second):
		t.Fatal("review did not reach the root decision")
	}
	go func() { _, _, err := model.ApplyBillingSettlement(nextKey); applyDone <- err }()
	select {
	case <-staleRootRead:
	case <-time.After(5 * time.Second):
		close(releaseDecision)
		t.Fatal("adjustment did not read the pre-commit root snapshot")
	}
	close(releaseDecision)
	require.NoError(t, <-reviewDone)
	require.NoError(t, <-applyDone)
	var event model.AdtySiteEarningEvent
	require.NoError(t, db.Where("operation_key = ?", nextKey).First(&event).Error)
	assert.Equal(t, model.AdtyEarningOriginalSkipped, event.ReviewStatus, "the final event must re-read the serialized root decision")
}

func TestReviewConvertsNullableHistoricalDurableRiskEvent(t *testing.T) {
	db := prepareSiteDB(t)
	_, site := openActiveSite(t, db, "nullable-risk-event")
	customer := siteCustomer(t, db, site, "nullable-risk-customer")
	require.NoError(t, db.Model(customer).Update("quota", perUnit("400")).Error)
	require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
	root, err := model.ReserveBillingOperation(model.BillingOperation{OperationKey: "request:nullable-old-risk", UserId: customer.Id,
		SiteId: site.Id, SiteMarkup: 1.25, SiteCostFactor: .8}, int(perUnit("125")))
	require.NoError(t, err)
	require.NoError(t, model.QueueBillingSettlement(root.OperationKey, int(perUnit("125")), false))
	_, _, err = model.ApplyBillingSettlement(root.OperationKey)
	require.NoError(t, err)
	require.NoError(t, projectAdtySiteEarningEvent(context.Background(), root.OperationKey))
	require.NoError(t, db.Model(&model.AdtySiteEarningEvent{}).Where("operation_key = ?", root.OperationKey).
		Updates(map[string]any{"risk_released": nil, "review_status": nil}).Error)
	require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
	var event model.AdtySiteEarningEvent
	require.NoError(t, db.Where("operation_key = ?", root.OperationKey).First(&event).Error)
	assert.True(t, event.RiskReleased)
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningUsage].Equal(usd("45")))
	assert.True(t, earningTotals(t, db, site.Id)[model.AdtySiteEarningSkipped].IsZero())
}

func TestUndatedLegacyMidjourneyDoesNotInheritUnboundOldReview(t *testing.T) {
	db := prepareSiteDB(t)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}, &model.Channel{}, &model.Token{}))
	_, site := openActiveSite(t, db, "undated-legacy-mj")
	customer := siteCustomer(t, db, site, "undated-legacy-mj-customer")
	require.NoError(t, db.Model(&model.AdtySiteCustomer{}).Where("user_id = ?", customer.Id).Update("risk_flag", "self_use").Error)
	require.NoError(t, ReviewAdtySiteCustomer(customer.Id, 1, true))
	task := &model.Midjourney{UserId: customer.Id, Quota: int(perUnit("125")), Status: "FAILURE", SubmitTime: 0,
		BillingSiteId: site.Id, BillingSiteMarkup: 1.25, BillingSiteCostFactor: .8}
	require.NoError(t, db.Create(task).Error)
	require.True(t, RefundMidjourneyQuota(context.Background(), task, "undated historical task"))
	var event model.AdtySiteEarningEvent
	require.NoError(t, db.Where("operation_key = ?", fmt.Sprintf("mj:%d:refund", task.Id)).First(&event).Error)
	assert.Equal(t, model.AdtyEarningNeedsReview, event.ReviewStatus, "an unbound earlier review cannot prove that it covered this undated charge")
	assert.False(t, event.Projected)
}
