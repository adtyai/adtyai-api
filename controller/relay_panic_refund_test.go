package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panicRefundBilling struct{ refunds int }

func (b *panicRefundBilling) Refund(*gin.Context)           { b.refunds++ }
func (b *panicRefundBilling) Settle(int) error              { return nil }
func (b *panicRefundBilling) NeedsRefund() bool             { return true }
func (b *panicRefundBilling) GetPreConsumedQuota() int      { return 1 }
func (b *panicRefundBilling) Reserve(int) error             { return nil }
func (b *panicRefundBilling) ReserveBeforeSubmit(int) error { return nil }

func TestRelayPanicRefundsPrecharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	billing := &panicRefundBilling{}
	info := &relaycommon.RelayInfo{Billing: billing}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	got := noteRelayPanic(ctx, info, nil, errors.New("boom"))
	require.NotNil(t, got)
	assert.Equal(t, 1, billing.refunds)

	again := noteRelayPanic(ctx, info, got, errors.New("boom"))
	require.NotNil(t, again)
	assert.Equal(t, 1, billing.refunds, "an error already handled by the refund defer is not refunded twice")
}
