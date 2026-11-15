package minimax

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/pgtest"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The rebuilt OpenAI payload is counted with the object/array rule from
// relay/channel/openai/relay_image.go, not with the raw array length: a url
// entry plus a b64_json entry for the same image bills once, and entries that
// carry no image payload bill nothing.
func TestMiniMaxImageResponseCountMatchesOpenAIRule(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int64
	}{
		{"empty array", `{"data":[]}`, 0},
		{"null data", `{"data":null,"created":1}`, 0},
		{"missing data", `{"created":1}`, 0},
		{"object with url", `{"data":{"url":"https://x/1.png"}}`, 1},
		{"object without payload", `{"data":{"revised_prompt":"p"}}`, 0},
		{"two urls", `{"data":[{"url":"https://x/1.png"},{"url":"https://x/2.png"}]}`, 2},
		{"two b64 entries", `{"data":[{"b64_json":"a"},{"b64_json":"b"}]}`, 2},
		{"split shape bills once", `{"data":[{"url":"https://x/1.png"},{"b64_json":"a"}]}`, 1},
		{"url and b64 of different images", `{"data":[{"url":"https://x/1.png"},{"url":"https://x/2.png"},{"b64_json":"a"}]}`, 2},
		{"empty url string ignored", `{"data":[{"url":""}]}`, 0},
		{"payload-free entry ignored", `{"data":[{"b64_json":"a"},{"revised_prompt":"p"}]}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, miniMaxImagePayloadCount([]byte(tc.body)))
		})
	}
}

// Upstream returned fewer images than the request reserved (or none at all):
// the settled count must follow what was delivered, and must stay at the
// reserved quantity when the response carries no image payload at all.
func TestMiniMaxImageHandlerBillsDeliveredImageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opened, err := model.OpenAdtyTestDatabase()
	if errors.Is(err, model.ErrAdtyTestDSNUnset) {
		t.Skip("ADTY_TEST_PG_DSN 未设置，需要 PostgreSQL 才能跑这个测试")
	}
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })
	pgtest.RecordServerVersion(t, opened.DB)
	require.NoError(t, opened.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}, &model.Option{}, &model.Log{}))
	require.NoError(t, model.MigrateAdtyTablesOn(opened.DB))

	const startQuota = 1_000_000
	const pricePerImageUSD = 0.02
	// 0.02 USD × 500000 Quota/USD = 10000 Quota per image, as integers.
	const quotaPerImage = 10000
	const reservedImages = 4

	oldBatchUpdate, oldRedisEnabled := common.BatchUpdateEnabled, common.RedisEnabled
	common.BatchUpdateEnabled, common.RedisEnabled = false, false
	t.Cleanup(func() { common.BatchUpdateEnabled, common.RedisEnabled = oldBatchUpdate, oldRedisEnabled })

	cases := []struct {
		name      string
		upstream  string
		delivered int
		billed    int
	}{
		{
			name: "fewer images than requested",
			upstream: `{"base_resp":{"status_code":0},"data":{"image_urls":[` +
				`"https://upstream/1.png","https://upstream/2.png","https://upstream/3.png"]}}`,
			delivered: 3,
			billed:    3,
		},
		{
			name:      "only one image delivered",
			upstream:  `{"base_resp":{"status_code":0},"data":{"image_urls":["https://upstream/1.png"]}}`,
			delivered: 1,
			billed:    1,
		},
		{
			name:      "base64 payloads",
			upstream:  `{"base_resp":{"status_code":0},"data":{"image_base64":["aW1hZ2UtMQ==","aW1hZ2UtMg=="]}}`,
			delivered: 2,
			billed:    2,
		},
		{
			name:      "no image payload keeps the reservation",
			upstream:  `{"base_resp":{"status_code":0},"data":{}}`,
			delivered: 0,
			billed:    reservedImages,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := &model.User{Username: "minimax-" + strings.ReplaceAll(tc.name, " ", "-"), Password: "x", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: startQuota}
			require.NoError(t, model.DB.Create(user).Error)
			require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
				return model.CreateAdtyFundBatchesTx(tx, user.Id, fmt.Sprintf("order-minimax-%d", user.Id), int64(startQuota), 0)
			}))
			token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("sk-minimax-%d", user.Id), Name: "minimax", Status: common.TokenStatusEnabled, RemainQuota: startQuota}
			require.NoError(t, model.DB.Create(token).Error)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			info := &relaycommon.RelayInfo{
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "image-01"},
				UserId:          user.Id,
				TokenId:         token.Id,
				TokenKey:        token.Key,
				OriginModelName: "image-01",
				UsingGroup:      "default",
				StartTime:       time.Now(),
				PriceData: hosttypes.PriceData{
					UsePrice:       true,
					ModelPrice:     pricePerImageUSD,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
				},
			}
			// The client asked for four images; that is what gets reserved.
			require.Nil(t, service.PrepareImageBillingForRequest(ctx, info, reservedImages))
			require.Equal(t, reservedImages*quotaPerImage, info.PriceData.QuotaToPreConsume)

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tc.upstream)),
			}

			usage, apiErr := miniMaxImageHandler(ctx, resp, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, float64(tc.billed), info.PriceData.OtherRatios()["n"],
				"the billable count must follow the delivered payloads")
			// The client receives exactly the images that were billed.
			var out dto.ImageResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
			require.Len(t, out.Data, tc.delivered)

			usage.TotalTokens, usage.PromptTokens = 1, 1
			require.Nil(t, service.PostTextConsumeQuota(ctx, info, usage, nil))

			quota, quotaErr := model.GetUserQuota(user.Id, true)
			require.NoError(t, quotaErr)
			assert.Equal(t, startQuota-tc.billed*quotaPerImage, quota,
				"settlement must follow the delivered count, not the reserved %d", reservedImages)
		})
	}
}
