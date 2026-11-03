package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/ionet"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pagedDeploymentHTTP struct {
	names []string
}

func (f pagedDeploymentHTTP) Do(req *ionet.HTTPRequest) (*ionet.HTTPResponse, error) {
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	page, _ := strconv.Atoi(parsed.Query().Get("page"))
	size, _ := strconv.Atoi(parsed.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	start := (page - 1) * size
	if start > len(f.names) {
		start = len(f.names)
	}
	end := start + size
	if end > len(f.names) {
		end = len(f.names)
	}
	deployments := make([]map[string]any, 0, end-start)
	for _, name := range f.names[start:end] {
		deployments = append(deployments, map[string]any{
			"id": name, "name": name, "status": "running", "created_at": "2026-01-01T00:00:00Z",
		})
	}
	body, err := json.Marshal(map[string]any{"data": map[string]any{
		"deployments": deployments,
		"total":       len(f.names),
	}})
	if err != nil {
		return nil, err
	}
	return &ionet.HTTPResponse{StatusCode: http.StatusOK, Body: body}, nil
}

func withDeploymentSearchClient(t *testing.T, names []string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		"model_deployment.ionet.enabled": "true",
		"model_deployment.ionet.api_key": "test-key",
	}
	common.OptionMapRWMutex.Unlock()
	previousClient := newIoEnterpriseClient
	newIoEnterpriseClient = func(apiKey string) *ionet.Client {
		return ionet.NewClientWithConfig(apiKey, "http://ionet.test", pagedDeploymentHTTP{names: names})
	}
	t.Cleanup(func() {
		newIoEnterpriseClient = previousClient
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
}

func searchDeploymentNames(t *testing.T, rawURL string) (int, []string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	SearchDeployments(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Total int `json:"total"`
			Items []struct {
				Name string `json:"deployment_name"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success, recorder.Body.String())
	names := make([]string, 0, len(body.Data.Items))
	for _, item := range body.Data.Items {
		names = append(names, item.Name)
	}
	return body.Data.Total, names
}

func TestSearchDeploymentsFindsNamesBeyondTheRequestedPage(t *testing.T) {
	names := make([]string, 0, 12)
	for i := 1; i <= 11; i++ {
		names = append(names, "plain-"+strconv.Itoa(i))
	}
	names = append(names, "hidden-target")
	withDeploymentSearchClient(t, names)

	total, found := searchDeploymentNames(t, "/api/deployments/search?keyword=HIDDEN&p=1&page_size=10")
	require.Equal(t, 1, total)
	require.Equal(t, []string{"hidden-target"}, found)

	matches := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		matches = append(matches, "match-"+strconv.Itoa(i))
	}
	withDeploymentSearchClient(t, matches)
	total, found = searchDeploymentNames(t, "/api/deployments/search?keyword=match&p=2&page_size=10")
	require.Equal(t, 12, total)
	require.Equal(t, []string{"match-11", "match-12"}, found)
}
