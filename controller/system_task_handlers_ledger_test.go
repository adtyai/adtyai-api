package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerSystemTaskHandlersRegistered(t *testing.T) {
	RegisterScheduledSystemTasks()

	for _, typ := range []string{
		model.SystemTaskTypeHourlyRollup,
		model.SystemTaskTypePartitionPrecreate,
		model.SystemTaskTypeProbeCleanup,
		model.SystemTaskTypeDailyReconcile,
	} {
		h, ok := service.LookupSystemTaskHandler(typ)
		require.True(t, ok, "handler %s missing", typ)
		require.NotNil(t, h)
		sch, ok := h.(service.ScheduledSystemTaskHandler)
		require.True(t, ok, "%s not scheduled", typ)
		assert.True(t, sch.Enabled())
		assert.Greater(t, sch.Interval(), time.Duration(0))
		assert.Equal(t, typ, sch.Type())
	}

	hr, _ := service.LookupSystemTaskHandler(model.SystemTaskTypeHourlyRollup)
	assert.Equal(t, time.Hour, hr.(service.ScheduledSystemTaskHandler).Interval())
	pr, _ := service.LookupSystemTaskHandler(model.SystemTaskTypePartitionPrecreate)
	assert.Equal(t, 24*time.Hour, pr.(service.ScheduledSystemTaskHandler).Interval())
	pc, _ := service.LookupSystemTaskHandler(model.SystemTaskTypeProbeCleanup)
	assert.Equal(t, 24*time.Hour, pc.(service.ScheduledSystemTaskHandler).Interval())
	dr, _ := service.LookupSystemTaskHandler(model.SystemTaskTypeDailyReconcile)
	assert.Equal(t, 30*time.Minute, dr.(service.ScheduledSystemTaskHandler).Interval())
}
