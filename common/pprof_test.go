package common

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitorCPUFailureDoesNotPanic(t *testing.T) {
	previousRead, previousSleep := readCPUPercent, monitorIdleSleep
	monitorIdleSleep = 0
	t.Cleanup(func() {
		readCPUPercent = previousRead
		monitorIdleSleep = previousSleep
	})

	readCPUPercent = func(time.Duration, bool) ([]float64, error) {
		return nil, errors.New("cpu unavailable")
	}
	require.NotPanics(t, monitorCPUOnce)

	readCPUPercent = func(time.Duration, bool) ([]float64, error) {
		return nil, nil
	}
	require.NotPanics(t, monitorCPUOnce)
}
