package service

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const billingRefundSweepInterval = 30 * time.Second

// StartBillingRefundSweeper refunds durable holds whose owning process is no
// longer heartbeating. It never refunds a live node's open hold.
func StartBillingRefundSweeper() {
	go func() {
		sweepBillingRefundsOnce()
		ticker := time.NewTicker(billingRefundSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			sweepBillingRefundsOnce()
		}
	}()
}

func sweepBillingRefundsOnce() {
	if _, err := model.SweepBillingRefunds(common.GetTimestamp()); err != nil {
		common.SysError(common.LogText("billing refund sweep failed: %s", err.Error()))
	}
}

func (s *BillingSession) persistBillingHold() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistBillingHoldLocked()
}

func (s *BillingSession) persistBillingHoldLocked() error {
	if !s.shouldRecordHoldLocked() {
		return nil
	}
	s.ensureRequestIDLocked()
	job := s.billingHoldLocked()
	if strings.TrimSpace(job.RequestID) == "" {
		return errors.New("billing hold request id is empty")
	}
	return model.SaveOpenBillingHold(&job)
}

func (s *BillingSession) shouldRecordHoldLocked() bool {
	if s == nil || s.settled || s.refunded || s.fundingSettled {
		return false
	}
	return s.needsRefundLocked()
}

func (s *BillingSession) ensureRequestIDLocked() {
	if s.relayInfo == nil || strings.TrimSpace(s.relayInfo.RequestId) != "" {
		return
	}
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub != nil && strings.TrimSpace(sub.requestId) != "" {
		s.relayInfo.RequestId = sub.requestId
		return
	}
	s.relayInfo.RequestId = common.NewRequestId()
}

func (s *BillingSession) billingHoldLocked() model.BillingRefundJob {
	job := model.BillingRefundJob{
		Source:     s.fundingSourceLocked(),
		TokenQuota: s.tokenConsumed,
		NodeName:   common.NodeName,
	}
	if s.relayInfo != nil {
		job.RequestID = s.relayInfo.RequestId
		job.UserID = s.relayInfo.UserId
		job.TokenID = s.relayInfo.TokenId
		job.TokenKey = s.relayInfo.TokenKey
		job.Playground = s.relayInfo.IsPlayground
	}
	if wallet, ok := s.funding.(*WalletFunding); ok && wallet != nil {
		job.UserID = wallet.userId
		job.Bonus = wallet.bonusConsumed
		job.Paid = wallet.paidConsumed
	}
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub != nil {
		if sub.preConsumed > 0 {
			job.SubscriptionRequestID = sub.requestId
			job.SubscriptionID = sub.subscriptionId
		}
		if s.extraReserved > 0 {
			job.SubscriptionID = sub.subscriptionId
			job.ExtraReserved = s.extraReserved
		}
	}
	return job
}

func (s *BillingSession) fundingSourceLocked() string {
	if s.funding == nil {
		return ""
	}
	return s.funding.Source()
}

func (s *BillingSession) markBillingHoldSettledLocked() error {
	if s.relayInfo == nil {
		return nil
	}
	return model.MarkBillingHoldSettled(s.relayInfo.RequestId)
}

func (s *BillingSession) abortUnrecordedConsume(cause error) *types.NewAPIError {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abortUnrecordedConsumeLocked(cause)
	return types.NewError(cause, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
}

func (s *BillingSession) abortUnrecordedConsumeLocked(cause error) {
	job := s.billingHoldLocked()
	if err := model.MarkBillingRefundDue(&job); err == nil {
		if applyErr := refundWithRetry(func() error { return model.ApplyBillingRefund(job.RequestID) }); applyErr != nil {
			common.SysError(common.LogText("billing refund apply failed for %s: %s", job.RequestID, applyErr.Error()))
		} else {
			s.zeroConsumedLocked()
		}
		s.refunded = true
		return
	} else if errors.Is(err, model.ErrBillingRefundClosed) {
		s.refunded = true
		return
	}
	common.SysError(common.LogText("billing refund mark due failed for %s: %s", job.RequestID, cause.Error()))
	s.fallbackRefundLocked()
}

func (s *BillingSession) fallbackRefundLocked() {
	if s.funding != nil {
		if err := refundWithRetry(s.funding.Refund); err != nil {
			common.SysError(common.LogText("billing fallback refund failed: %s", err.Error()))
			return
		}
	}
	if s.extraReserved > 0 && s.funding != nil && s.funding.Source() == BillingSourceSubscription && s.relayInfo != nil && s.relayInfo.SubscriptionId > 0 {
		extra := s.extraReserved
		subscriptionID := s.relayInfo.SubscriptionId
		if err := refundWithRetry(func() error {
			return model.PostConsumeUserSubscriptionDelta(subscriptionID, -int64(extra))
		}); err != nil {
			common.SysError(common.LogText("billing fallback subscription refund failed: %s", err.Error()))
			return
		}
		s.extraReserved = 0
	}
	if s.tokenConsumed > 0 && s.relayInfo != nil && !s.relayInfo.IsPlayground {
		tokenID := s.relayInfo.TokenId
		tokenKey := s.relayInfo.TokenKey
		amount := s.tokenConsumed
		if err := refundWithRetry(func() error {
			return model.IncreaseTokenQuota(tokenID, tokenKey, amount)
		}); err != nil {
			common.SysError(common.LogText("billing fallback token refund failed: %s", err.Error()))
			return
		}
	}
	s.refunded = true
	s.zeroConsumedLocked()
}

func (s *BillingSession) zeroConsumedLocked() {
	s.tokenConsumed = 0
	s.extraReserved = 0
	s.preConsumedQuota = 0
	if wallet, ok := s.funding.(*WalletFunding); ok && wallet != nil {
		wallet.consumed, wallet.bonusConsumed, wallet.paidConsumed = 0, 0, 0
	}
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub != nil {
		sub.preConsumed = 0
	}
}
