package controller

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicUptimeStatusDoesNotCopyGroupName(t *testing.T) {
	data, err := os.ReadFile("uptime_kuma.go")
	require.NoError(t, err)
	require.NotContains(t, string(data), "Group: pg.Name")
	require.NotContains(t, string(data), "Group:  pg.Name")
}
