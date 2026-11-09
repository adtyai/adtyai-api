package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resendCall struct {
	Method, Path, Auth, Body, Idempotency string
}

func fakeResend(t *testing.T, status int, response string, header http.Header) (*AdtyResendClient, *[]resendCall) {
	t.Helper()
	calls := &[]resendCall{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, resendCall{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(body), r.Header.Get("Idempotency-Key")})
		for k, v := range header {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return &AdtyResendClient{BaseURL: server.URL, HTTP: server.Client(), APIKey: func() string { return "re_test" }}, calls
}

func TestAdtyResendClientRequests(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		call       func(c *AdtyResendClient) (string, error)
		method     string
		path       string
		bodyFields map[string]any
	}{
		{"send email", func(c *AdtyResendClient) (string, error) {
			return c.SendEmail(ctx, AdtyResendEmail{From: "News <n@x.y>", To: []string{"a@b.c"}, Subject: "s", HTML: "h", Headers: map[string]string{"List-Unsubscribe": "<https://u>"}})
		}, http.MethodPost, "/emails", map[string]any{"from": "News <n@x.y>", "headers": map[string]any{"List-Unsubscribe": "<https://u>"}}},
		{"create domain", func(c *AdtyResendClient) (string, error) {
			d, err := c.CreateDomain(ctx, AdtyResendDomainCreate{Name: "news.x.y"})
			return d.ID, err
		}, http.MethodPost, "/domains", map[string]any{"name": "news.x.y"}},
		{"get domain", func(c *AdtyResendClient) (string, error) {
			d, err := c.GetDomain(ctx, "did")
			return d.ID, err
		}, http.MethodGet, "/domains/did", nil},
		{"delete domain", func(c *AdtyResendClient) (string, error) { return "", c.DeleteDomain(ctx, "did") }, http.MethodDelete, "/domains/did", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, calls := fakeResend(t, http.StatusOK, `{"object":"x","id":"new-id","name":"news.x.y","status":"not_started","records":[]}`, nil)
			id, err := tc.call(client)
			require.NoError(t, err)
			require.Len(t, *calls, 1)
			got := (*calls)[0]
			assert.Equal(t, tc.method, got.Method)
			assert.Equal(t, tc.path, got.Path)
			assert.Equal(t, "Bearer re_test", got.Auth)
			if id != "" {
				assert.Equal(t, "new-id", id)
			}
			if tc.bodyFields != nil {
				var body map[string]any
				require.NoError(t, common.Unmarshal([]byte(got.Body), &body))
				for key, want := range tc.bodyFields {
					assert.Equal(t, want, body[key], key)
				}
			}
		})
	}
}

func TestAdtyResendClientIdempotencyKey(t *testing.T) {
	client, calls := fakeResend(t, http.StatusOK, `{"id":"e1"}`, nil)
	_, err := client.SendEmail(context.Background(), AdtyResendEmail{From: "a <a@b.c>", To: []string{"x@y.z"}, Subject: "s", HTML: "h", IdempotencyKey: "trigger-1-7"})
	require.NoError(t, err)
	assert.Equal(t, "trigger-1-7", (*calls)[0].Idempotency)
}

func TestAdtyResendClientErrors(t *testing.T) {
	ctx := context.Background()

	client, _ := fakeResend(t, http.StatusUnprocessableEntity, `{"statusCode":422,"name":"validation_error","message":"Invalid from"}`, nil)
	_, err := client.SendEmail(ctx, AdtyResendEmail{Subject: "system notice"})
	var apiErr *AdtyResendError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 422, apiErr.StatusCode)
	assert.Equal(t, "validation_error", apiErr.Name)
	assert.Equal(t, "Invalid from", apiErr.Message)
	assert.False(t, errors.Is(err, ErrAdtyResendRateLimited))

	client, _ = fakeResend(t, http.StatusTooManyRequests, `{"statusCode":429,"name":"rate_limit_exceeded","message":"Too many requests"}`, http.Header{"Retry-After": {"2"}})
	_, err = client.SendEmail(ctx, AdtyResendEmail{Subject: "system notice"})
	require.ErrorIs(t, err, ErrAdtyResendRateLimited)
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 2*time.Second, apiErr.RetryAfter)

	client, _ = fakeResend(t, http.StatusTooManyRequests, `{"statusCode":429,"name":"monthly_quota_exceeded","message":"quota"}`, nil)
	_, err = client.SendEmail(ctx, AdtyResendEmail{Subject: "system notice"})
	require.ErrorIs(t, err, ErrAdtyResendQuotaExceeded)
	assert.False(t, errors.Is(err, ErrAdtyResendRateLimited))

	client, calls := fakeResend(t, http.StatusOK, `{"id":"x"}`, nil)
	client.APIKey = func() string { return " " }
	_, err = client.SendEmail(ctx, AdtyResendEmail{Subject: "system notice"})
	require.ErrorIs(t, err, ErrAdtyResendNotConfigured)
	assert.Empty(t, *calls)
}

func TestAdtyResendDefaultKeyReadsOption(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	oldMap := common.OptionMap
	common.OptionMap = map[string]string{"ResendApiKey": "re_from_option"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldMap
		common.OptionMapRWMutex.Unlock()
	})
	client := NewAdtyResendClient()
	assert.Equal(t, "https://api.resend.com", client.BaseURL)
	assert.Equal(t, "re_from_option", client.APIKey())
}
