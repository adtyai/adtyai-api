package controller

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamChannelTestSeesEventsBeyondEightKilobytes(t *testing.T) {
	event := `{"choices":[{"delta":{"content":"ok"}}]}`
	big := "data: " + strings.Repeat(" ", 9<<10) + event + "\n"
	body, err := readTestResponseBody(io.NopCloser(strings.NewReader(big)), true)
	require.NoError(t, err)
	require.NoError(t, validateTestResponseBody(body, true))

	var later bytes.Buffer
	later.WriteString("data: " + event + "\n")
	later.Write(bytes.Repeat([]byte("x"), 8<<10))
	later.WriteString("\ndata: {\"error\":{\"message\":\"later failed\"}}\n")
	body, err = readTestResponseBody(io.NopCloser(bytes.NewReader(later.Bytes())), true)
	require.NoError(t, err)
	require.Error(t, validateTestResponseBody(body, true))
	require.Contains(t, validateTestResponseBody(body, true).Error(), "later failed")
}
