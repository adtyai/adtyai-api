package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo         *relaycommon.RelayInfo
	funding           FundingSource
	preConsumedQuota  int    // 实际预扣额度（信任用户可能为 0）
	tokenConsumed     int    // 令牌额度实际扣减量
	trusted           bool   // 是否命中信任额度旁路
	fundingSettled    bool   // funding.Settle 已成功，资金来源已提交
	settled           bool   // Settle 全部完成（资金 + 令牌）
	refunded          bool   // Refund 已调用
	lang              string // 请求声明的语言，为空表示未声明
	mu                sync.Mutex
	operationKey      string
	requestContext    *gin.Context
	realtimeShortfall int
}

// Settle 根据实际消耗额度提交持久化结算决定，再原子调整钱包与令牌。
// 旧的非持久化会话保留原有的分步资金来源接口。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.operationKey != "" {
		if actualQuota < 0 {
			return model.ErrBillingOperationConflict
		}
		if s.settled {
			return nil
		}
		if err := model.QueueBillingSettlement(s.operationKey, actualQuota, false, s.relayInfo.RealtimeAlreadyCharged, s.relayInfo.RealtimeTokenAlreadyCharged); err != nil {
			return err
		}
		// The durable intent owns this reservation from here, including if an
		// apply fails. A failure handler must not issue a competing full refund.
		s.fundingSettled = true
		activeBillingReservations.Delete(s.operationKey)
		row, _, err := model.ApplyBillingSettlement(s.operationKey)
		if err != nil {
			return err
		}
		s.relayInfo.BillingShortfall = max(0, s.relayInfo.BillingShortfall-s.realtimeShortfall) + row.Shortfall
		s.settled = true
		if s.requestContext != nil {
			s.requestContext.Set(adtyBillingOperationContextKey, s.operationKey)
		}
		projectBillingEarnings(s.operationKey)
		return nil
	}
	if s.settled {
		return nil
	}
	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		return nil
	}
	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）
	if !s.fundingSettled {
		charged, err := s.funding.Settle(delta)
		if err != nil {
			return err
		}
		s.fundingSettled = true
		// 二开（余额下限）：补扣最多扣到 0，没扣到的部分由平台承担，记下来写日志 admin_info。
		if delta > 0 && charged < delta {
			s.relayInfo.BillingShortfall += delta - charged
		}
	}
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground {
		if delta > 0 {
			tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
		} else {
			tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
		}
		if tokenErr != nil {
			// 资金来源已提交，令牌调整失败只能记录日志；标记 settled 防止 Refund 误退资金
			common.SysLog(common.LogText("error adjusting token quota after funding settled (userId=%d, tokenId=%d, delta=%d): %s",
				s.relayInfo.UserId, s.relayInfo.TokenId, delta, tokenErr.Error()))
		}
	}
	s.settled = true
	return tokenErr
}

// Refund 持久化退款决定并幂等退还所有预扣费；失败由恢复任务继续处理。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.operationKey != "" {
		defer s.mu.Unlock()
		if s.settled || s.refunded || s.fundingSettled {
			return
		}
		if err := model.QueueBillingSettlement(s.operationKey, 0, true); err != nil {
			common.SysError(common.LogText("queue billing refund failed: %s", err.Error()))
			return
		}
		s.refunded = true
		activeBillingReservations.Delete(s.operationKey)
		if _, _, err := model.ApplyBillingSettlement(s.operationKey); err != nil {
			common.SysError(common.LogText("billing refund remains pending: %s", err.Error()))
		}
		return
	}
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, common.LogText("user %d request failed, refunding the pre-consumed quota (token_quota=%s, funding=%s)",
		s.relayInfo.UserId,
		logger.FormatQuota(s.tokenConsumed),
		s.funding.Source(),
	))

	// 复制需要的值到闭包中
	tokenId := s.relayInfo.TokenId
	tokenKey := s.relayInfo.TokenKey
	isPlayground := s.relayInfo.IsPlayground
	tokenConsumed := s.tokenConsumed
	funding := s.funding

	gopool.Go(func() {
		// 1) 退还资金来源
		if err := funding.Refund(); err != nil {
			common.SysLog(common.LogText("error refunding billing source: %s", err.Error()))
		}
		// 2) 退还令牌额度
		if tokenConsumed > 0 && !isPlayground {
			if err := model.IncreaseTokenQuota(tokenId, tokenKey, tokenConsumed); err != nil {
				common.SysLog(common.LogText("error refunding token quota: %s", err.Error()))
			}
		}
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	return s.reserve(targetQuota, false)
}

// ReserveBeforeSubmit 在发起上游请求前补扣，钱包余额必须覆盖差额。
func (s *BillingSession) ReserveBeforeSubmit(targetQuota int) error {
	return s.reserve(targetQuota, true)
}

func (s *BillingSession) reserve(targetQuota int, requireAvailableQuota bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.operationKey != "" {
		if s.settled || s.refunded || s.fundingSettled {
			return model.ErrBillingOperationConflict
		}
		row, err := model.ReserveBillingOperation(s.durableSpec(), targetQuota)
		if err != nil {
			return err
		}
		s.preConsumedQuota, s.tokenConsumed = row.ReservedQuota, row.TokenReservedQuota
		s.syncRelayInfo()
		return nil
	}

	if s.relayInfo != nil {
		_, imageRequest := s.relayInfo.Request.(*dto.ImageRequest)
		requireAvailableQuota = requireAvailableQuota || imageRequest || s.relayInfo.ImageRequestCount > 0
	}
	if s.settled || s.refunded || s.trusted && !requireAvailableQuota || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if err := s.reserveFunding(delta); err != nil {
		return err
	}
	if err := s.reserveToken(delta); err != nil {
		s.rollbackFundingReserve(delta)
		return err
	}

	s.preConsumedQuota += delta
	s.tokenConsumed += delta
	if requireAvailableQuota {
		s.trusted = false
	}
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 令牌预扣 -> 资金来源预扣。
// 任一步骤失败时原子回滚已完成的步骤。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	if s.operationKey != "" {
		row, err := model.ReserveBillingOperation(s.durableSpec(), quota)
		if err != nil {
			code, status := types.ErrorCodeUpdateDataError, http.StatusInternalServerError
			if errors.Is(err, model.ErrBillingWalletInsufficient) {
				code, status = types.ErrorCodeInsufficientUserQuota, http.StatusForbidden
			}
			if errors.Is(err, model.ErrBillingTokenInsufficient) {
				code, status = types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden
			}
			return types.NewErrorWithStatusCode(err, code, status, types.ErrOptionWithSkipRetry())
		}
		s.preConsumedQuota, s.tokenConsumed = row.ReservedQuota, row.TokenReservedQuota
		s.syncRelayInfo()
		c.Set(adtyDurableBillingContextKey, true)
		c.Set(adtyBillingOperationContextKey, s.operationKey)
		activeBillingReservations.Store(s.operationKey, struct{}{})
		if c.Request != nil {
			context.AfterFunc(c.Request.Context(), func() { activeBillingReservations.Delete(s.operationKey) })
		}
		return nil
	}
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, common.LogText("user %d has enough quota, trusted without pre-consume (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, common.LogText("user %d needs pre-consume %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣令牌额度 ----
	if effectiveQuota > 0 {
		if err := PreConsumeTokenQuota(s.relayInfo, effectiveQuota); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		s.tokenConsumed = effectiveQuota
	}

	// ---- 2) 预扣资金来源 ----
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		// 预扣费失败，回滚令牌额度
		if s.tokenConsumed > 0 && !s.relayInfo.IsPlayground {
			if rollbackErr := model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed); rollbackErr != nil {
				common.SysLog(common.LogText("error rolling back token quota (userId=%d, tokenId=%d, amount=%d, fundingErr=%s): %s",
					s.relayInfo.UserId, s.relayInfo.TokenId, s.tokenConsumed, err.Error(), rollbackErr.Error()))
			}
			s.tokenConsumed = 0
		}
		if errors.Is(err, ErrInsufficientWalletQuota) {
			userQuota, quotaErr := model.GetUserQuota(s.relayInfo.UserId, false)
			if quotaErr != nil {
				userQuota = 0
			}
			return types.NewErrorWithStatusCode(
				errors.New(i18n.T(c, i18n.MsgQuotaUserInsufficient, map[string]any{"Remaining": logger.FormatQuota(userQuota)})),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	s.preConsumedQuota = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		// 二开（余额下限）：补预扣一律是条件扣减，余额不够就拒绝请求，不再记欠费（余额不会变负）。
		// 图片张数、任务重试价、分组切换后的阶梯价都在发往上游前确定，拒绝时上游不会被调用。
		if err := funding.PreConsume(delta); err != nil {
			if errors.Is(err, ErrInsufficientWalletQuota) {
				return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

func (s *BillingSession) rollbackFundingReserve(delta int) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.IncreaseUserQuota(funding.userId, delta, false); err != nil {
			common.SysLog(common.LogText("error rolling back wallet funding reserve: %s", err.Error()))
		} else {
			funding.consumed -= delta
		}
	}
}

func (s *BillingSession) reserveToken(delta int) error {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return nil
	}
	if err := PreConsumeTokenQuota(s.relayInfo, delta); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}

// shouldTrust 统一信任额度检查。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := operation_setting.GetQuotaSetting().TrustQuotaUSD * common.QuotaPerUnit
	if trustQuota <= 0 || math.IsNaN(trustQuota) || math.IsInf(trustQuota, 0) {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = float64(tokenQuota) > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		// 二开（余额下限）：钱包请求一律预扣，余额不够就拒绝。TrustQuotaUSD 设置项保留，
		// 但对钱包不再生效：信任旁路不预扣，结算时只能扣到 0，差额要平台承担。
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 创建钱包计费会话
// ---------------------------------------------------------------------------

// NewBillingSession 创建只用钱包资金源的 BillingSession。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	// 用户 ID 无效时直接报错，结果和删订阅前默认扣费路径的一样（原来在订阅检查里报这个错）
	if relayInfo.UserId <= 0 {
		return nil, types.NewError(errors.New("invalid userId"), types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				errors.New(i18n.T(c, i18n.MsgQuotaUserInsufficient, map[string]any{"Remaining": logger.FormatQuota(userQuota)})),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				errors.New(i18n.T(c, i18n.MsgQuotaPreConsumeInsufficient, map[string]any{"Remaining": logger.FormatQuota(userQuota), "Required": logger.FormatQuota(preConsumedQuota)})),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		if relayInfo.RequestId == "" {
			relayInfo.RequestId = common.NewRequestId()
		}
		session := &BillingSession{
			relayInfo:      relayInfo,
			funding:        &WalletFunding{userId: relayInfo.UserId},
			lang:           i18n.StatedLang(c),
			operationKey:   "request:" + relayInfo.RequestId,
			requestContext: c,
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	return tryWallet()
}

func (s *BillingSession) durableSpec() model.BillingOperation {
	row := model.BillingOperation{OperationKey: s.operationKey, UserId: s.relayInfo.UserId, TokenId: s.relayInfo.TokenId, TokenUnlimited: s.relayInfo.TokenUnlimited, LeaseOwner: billingProcessLeaseOwner}
	if s.relayInfo.IsPlayground {
		row.TokenId = 0
	}
	if k, d, ok := AdtySiteMarkupFromContext(s.requestContext); ok {
		row.SiteId, row.SiteMarkup, row.SiteCostFactor = s.requestContext.GetInt(AdtySiteIdContextKey), k, d
	}
	return row
}
