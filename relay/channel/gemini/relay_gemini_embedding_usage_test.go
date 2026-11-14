package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGeminiEmbeddingRelayInfo(estimate int) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{
		OriginModelName: "text-embedding-004",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "text-embedding-004",
		},
	}
	info.SetEstimatePromptTokens(estimate)
	return info
}

func newGeminiEmbeddingContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	return c, recorder
}

func TestGeminiEmbeddingHandlerUsesUpstreamUsageMetadata(t *testing.T) {
	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)

	body, err := common.Marshal(dto.GeminiBatchEmbeddingResponse{
		Embeddings: []*dto.ContentEmbedding{{Values: []float64{0.1, 0.2}}},
		UsageMetadata: &dto.GeminiEmbeddingUsageMetadata{
			PromptTokenCount: 42,
		},
	})
	require.NoError(t, err)

	usage, newAPIError := GeminiEmbeddingHandler(c, info, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 42, usage.PromptTokens)
	require.Equal(t, 42, usage.TotalTokens)
	require.Equal(t, 0, usage.CompletionTokens)
}

func TestGeminiEmbeddingHandlerFallsBackWhenUsageMetadataMissing(t *testing.T) {
	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)

	body, err := common.Marshal(dto.GeminiBatchEmbeddingResponse{
		Embeddings: []*dto.ContentEmbedding{{Values: []float64{0.1, 0.2}}},
	})
	require.NoError(t, err)

	usage, newAPIError := GeminiEmbeddingHandler(c, info, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 10, usage.TotalTokens)
}

func TestNativeGeminiEmbeddingHandlerUsesUpstreamUsageMetadata(t *testing.T) {
	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)

	body, err := common.Marshal(dto.GeminiEmbeddingResponse{
		Embedding: dto.ContentEmbedding{Values: []float64{0.1, 0.2}},
		UsageMetadata: &dto.GeminiEmbeddingUsageMetadata{
			PromptTokenCount: 42,
		},
	})
	require.NoError(t, err)

	usage, newAPIError := NativeGeminiEmbeddingHandler(c, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}, info)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 42, usage.PromptTokens)
	require.Equal(t, 42, usage.TotalTokens)
	require.Equal(t, 0, usage.CompletionTokens)
}

func TestNativeGeminiEmbeddingHandlerFallsBackWhenUsageMetadataMissing(t *testing.T) {
	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)

	body, err := common.Marshal(dto.GeminiEmbeddingResponse{
		Embedding: dto.ContentEmbedding{Values: []float64{0.1, 0.2}},
	})
	require.NoError(t, err)

	usage, newAPIError := NativeGeminiEmbeddingHandler(c, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}, info)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 10, usage.TotalTokens)
}

func TestNativeGeminiBatchEmbeddingHandlerUsesUpstreamUsageMetadata(t *testing.T) {
	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)
	info.IsGeminiBatchEmbedding = true

	body, err := common.Marshal(dto.GeminiBatchEmbeddingResponse{
		Embeddings: []*dto.ContentEmbedding{{Values: []float64{0.1}}, {Values: []float64{0.2}}},
		UsageMetadata: &dto.GeminiEmbeddingUsageMetadata{
			PromptTokenCount: 77,
		},
	})
	require.NoError(t, err)

	usage, newAPIError := NativeGeminiEmbeddingHandler(c, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}, info)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 77, usage.PromptTokens)
	require.Equal(t, 77, usage.TotalTokens)
}

func TestGeminiEmbeddingUsageMetadataIgnoresNonPositivePromptCount(t *testing.T) {
	require.Nil(t, (*dto.GeminiEmbeddingUsageMetadata)(nil).GeminiUsageMetadata())
	require.Nil(t, (&dto.GeminiEmbeddingUsageMetadata{PromptTokenCount: 0}).GeminiUsageMetadata())
	require.Nil(t, (&dto.GeminiEmbeddingUsageMetadata{PromptTokenCount: -3}).GeminiUsageMetadata())

	c, _ := newGeminiEmbeddingContext()
	info := newGeminiEmbeddingRelayInfo(10)
	body, err := common.Marshal(dto.GeminiBatchEmbeddingResponse{
		Embeddings:    []*dto.ContentEmbedding{{Values: []float64{0.1}}},
		UsageMetadata: &dto.GeminiEmbeddingUsageMetadata{PromptTokenCount: 0},
	})
	require.NoError(t, err)

	usage, newAPIError := GeminiEmbeddingHandler(c, info, &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, newAPIError)
	require.Equal(t, 10, usage.PromptTokens)
}
