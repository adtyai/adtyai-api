package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func hangUntilDone(w http.ResponseWriter, r *http.Request) {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
	case <-timer.C:
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"models":[],"version":"0","status":"success"}`))
}

func TestOllamaAdminRequestsStopWhenContextEnds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(hangUntilDone))
	t.Cleanup(upstream.Close)

	calls := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "list", run: func(ctx context.Context) error {
			_, err := FetchOllamaModels(ctx, upstream.URL, "enabled-key")
			return err
		}},
		{name: "delete", run: func(ctx context.Context) error {
			return DeleteOllamaModel(ctx, upstream.URL, "enabled-key", "llama3")
		}},
		{name: "version", run: func(ctx context.Context) error {
			_, err := FetchOllamaVersion(ctx, upstream.URL, "enabled-key")
			return err
		}},
		{name: "stream", run: func(ctx context.Context) error {
			return PullOllamaModelStream(ctx, upstream.URL, "enabled-key", "llama3", func(OllamaPullResponse) {})
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()
			err := call.run(ctx)
			elapsed := time.Since(start)
			require.Error(t, err)
			require.Less(t, elapsed, time.Second, "elapsed %s", elapsed)
		})
	}
}
