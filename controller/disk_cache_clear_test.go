package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClearDiskCacheKeepsOpenBodyAndUsage(t *testing.T) {
	require.NoError(t, i18n.Init())
	cacheDir := t.TempDir()
	saved := common.GetDiskCacheConfig()
	common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: true, ThresholdMB: 0, MaxSizeMB: 1024, Path: cacheDir})
	t.Cleanup(func() { common.SetDiskCacheConfig(saved) })

	payload := []byte("still-in-use")
	before := common.GetDiskCacheStats().CurrentDiskUsageBytes
	storage, err := common.CreateBodyStorage(payload)
	require.NoError(t, err)
	require.True(t, storage.IsDisk())
	t.Cleanup(func() { _ = storage.Close() })
	require.Equal(t, before+int64(len(payload)), common.GetDiskCacheStats().CurrentDiskUsageBytes)

	dir := filepath.Join(cacheDir, "new-api-body-cache")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	old := time.Now().Add(-11 * time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(dir, entries[0].Name()), old, old))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/performance/disk_cache", nil)
	ClearDiskCache(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Equal(t, before+int64(len(payload)), common.GetDiskCacheStats().CurrentDiskUsageBytes)

	reader, err := storage.NewReader()
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, payload, body)

	require.NoError(t, storage.Close())
	require.Equal(t, before, common.GetDiskCacheStats().CurrentDiskUsageBytes)
}
