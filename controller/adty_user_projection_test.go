package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 客户接口不露渠道、倍率和上游信息（任务 7.1）。

func TestAdminUserSiteFiltersDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("database DSN not configured")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			require.NoError(t, db.AutoMigrate(&model.AdtySite{}))
			require.NoError(t, db.Create(&model.AdtySite{Id: 7, Subdomain: "north", Brand: `{"name":"North Studio","smtp_password":"secret-mail"}`}).Error)
			require.NoError(t, db.Create(&model.AdtySite{Id: 8, Subdomain: "south"}).Error)
			users := []model.User{
				{Username: "site-user-main", Role: common.RoleCommonUser, SiteId: 0, Status: 1},
				{Username: "site-user-north", Role: common.RoleCommonUser, SiteId: 7, Status: 1},
				{Username: "site-user-disabled", Role: common.RoleCommonUser, SiteId: 7, Status: 2},
				{Username: "site-user-south", Role: common.RoleCommonUser, SiteId: 8, Status: 1},
				{Username: "site-user-admin", Role: common.RoleAdminUser, SiteId: 7, Status: 1},
				{Username: "site-user-root", Role: common.RoleRootUser, SiteId: 7, Status: 1},
				{Username: "site-user-deleted", Role: common.RoleCommonUser, SiteId: 7, Status: 1},
				{Username: "site-user-orphan", Role: common.RoleCommonUser, SiteId: 77, Status: 1},
			}
			require.NoError(t, db.Create(&users).Error)
			require.NoError(t, db.Delete(&users[6]).Error)
			for _, tc := range []struct {
				name, path  string
				handler     gin.HandlerFunc
				role, total int
				want        []int
			}{
				{"all sites", "/api/user/?sort_by=id&sort_order=asc", GetAllUsers, common.RoleRootUser, 8, []int{0, 1, 2, 3, 4, 5, 6, 7}},
				{"site page one", "/api/user/?site_id=7&sort_by=id&sort_order=asc&page_size=2", GetAllUsers, common.RoleRootUser, 5, []int{1, 2}},
				{"site page two", "/api/user/?site_id=7&sort_by=id&sort_order=asc&page_size=2&p=2", GetAllUsers, common.RoleRootUser, 5, []int{4, 5}},
				{"main site zero", "/api/user/?site_id=0", GetAllUsers, common.RoleRootUser, 1, []int{0}},
				{"unknown site", "/api/user/?site_id=900", GetAllUsers, common.RoleRootUser, 0, []int{}},
				{"admin role ceiling", "/api/user/?site_id=7&sort_by=id&sort_order=asc", GetAllUsers, common.RoleAdminUser, 3, []int{1, 2, 6}},
				{"search site status role", "/api/user/search?site_id=7&keyword=site-user&status=1&role=1&group=default", SearchUsers, common.RoleRootUser, 1, []int{1}},
				{"search deleted", "/api/user/search?site_id=7&status=-1", SearchUsers, common.RoleRootUser, 1, []int{6}},
				{"search main site", "/api/user/search?site_id=0&keyword=site-user", SearchUsers, common.RoleRootUser, 1, []int{0}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
					c.Set("role", tc.role)
					tc.handler(c)
					var response struct {
						Success bool `json:"success"`
						Data    struct {
							Total int `json:"total"`
							Items []struct {
								ID       int    `json:"id"`
								SiteID   int    `json:"site_id"`
								SiteName string `json:"site_name"`
							} `json:"items"`
						} `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					require.True(t, response.Success, recorder.Body.String())
					require.Equal(t, tc.total, response.Data.Total)
					require.Len(t, response.Data.Items, len(tc.want))
					for i, index := range tc.want {
						view := response.Data.Items[i]
						assert.Equal(t, users[index].Id, view.ID)
						assert.Equal(t, users[index].SiteId, view.SiteID)
						name := map[int]string{0: common.SystemName, 7: "North Studio", 8: "south", 77: ""}[view.SiteID]
						assert.Equal(t, name, view.SiteName)
					}
					assert.NotContains(t, recorder.Body.String(), "secret-mail")
				})
			}
			for _, siteID := range []string{"-1", "north", "1.5", "999999999999999999999999999999", ""} {
				for _, handler := range []gin.HandlerFunc{GetAllUsers, SearchUsers} {
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodGet, "/api/user/?site_id="+siteID, nil)
					c.Set("role", common.RoleRootUser)
					handler(c)
					assert.Equal(t, http.StatusBadRequest, recorder.Code, "invalid site_id %q", siteID)
					assert.NotContains(t, recorder.Body.String(), "site-user-")
				}
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/user/"+fmt.Sprint(users[1].Id), nil)
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(users[1].Id)}}
			c.Set("role", common.RoleRootUser)
			GetUser(c)
			assert.Contains(t, recorder.Body.String(), `"site_name":"North Studio"`)
			assert.NotContains(t, recorder.Body.String(), "secret-mail")
			require.NoError(t, db.Model(&users[5]).Update("site_id", 9).Error)
			for _, tc := range []struct {
				role int
				ids  []int
			}{
				{common.RoleRootUser, []int{0, 7, 8, 9, 77}},
				{common.RoleAdminUser, []int{0, 7, 8, 77}},
			} {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/user/sites", nil)
				c.Set("role", tc.role)
				GetUserSites(c)
				var response struct {
					Success bool `json:"success"`
					Data    []struct {
						SiteID   int    `json:"site_id"`
						SiteName string `json:"site_name"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				require.True(t, response.Success, recorder.Body.String())
				require.Len(t, response.Data, len(tc.ids))
				for i, id := range tc.ids {
					assert.Equal(t, id, response.Data[i].SiteID)
				}
				assert.Equal(t, "North Studio", response.Data[1].SiteName)
				assert.NotContains(t, recorder.Body.String(), "brand")
				assert.NotContains(t, recorder.Body.String(), "secret-mail")
				assert.NotContains(t, recorder.Body.String(), "quota")
			}
		})
	}
}

func TestAdminUserBalancesPreserveRawQuotaAndExposeLiveQuota(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	first := model.User{Username: "durable-first", Quota: 100, QuotaPending: -80, Role: common.RoleCommonUser}
	second := model.User{Username: "durable-second", Quota: 50, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)
	type balanceView struct {
		ID        int  `json:"id"`
		Quota     int  `json:"quota"`
		LiveQuota *int `json:"live_quota"`
	}
	for _, tc := range []struct {
		name, path string
		handler    gin.HandlerFunc
		list       bool
	}{
		{"list", "/api/user/?sort_by=quota&sort_order=asc", GetAllUsers, true},
		{"search", "/api/user/search?keyword=durable&sort_by=quota&sort_order=asc", SearchUsers, true},
		{"detail", "/api/user/detail", GetUser, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			c.Set("role", common.RoleRootUser)
			c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(first.Id)}}
			tc.handler(c)
			var response struct {
				Success bool            `json:"success"`
				Data    json.RawMessage `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			var view balanceView
			if tc.list {
				var page struct {
					Items []balanceView `json:"items"`
				}
				require.NoError(t, common.Unmarshal(response.Data, &page))
				require.Len(t, page.Items, 2)
				view = page.Items[0]
				assert.Equal(t, first.Id, view.ID, "the available-balance sort must use raw plus pending")
			} else {
				require.NoError(t, common.Unmarshal(response.Data, &view))
			}
			require.NotNil(t, view.LiveQuota)
			assert.Equal(t, 20, *view.LiveQuota)
			assert.Equal(t, 100, view.Quota, "the raw audit/edit contract must stay intact")
			assert.NotContains(t, recorder.Body.String(), "quota_pending")
		})
	}
}

func TestUserAndSiteBalanceViewsIncludeDurablePending(t *testing.T) {
	for _, tc := range []struct{ raw, pending int }{{100, -80}, {-80, 100}} {
		user, identity := setupSecurityEnrollmentTest(t)
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]any{
			"quota": tc.raw, "quota_pending": tc.pending,
		}).Error)
		response := securityEnrollmentRequest(http.MethodGet, "/api/user/self", "", "", identity, GetSelf)
		var self struct {
			Success bool `json:"success"`
			Data    struct {
				Quota int `json:"quota"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &self))
		require.True(t, self.Success, response.Body.String())
		assert.Equal(t, 20, self.Data.Quota)
		loaded, err := model.GetUserById(user.Id, false)
		require.NoError(t, err)
		assert.Equal(t, 20, buildSelfUserData(loaded)["quota"], "login and profile share the same balance projection")
		require.NoError(t, model.DB.AutoMigrate(&model.AdtySiteCustomer{}))
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("site_id", 7).Error)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/adty/admin/sites/7/customers", nil)
		c.Params = gin.Params{{Key: "id", Value: "7"}}
		c.Set("role", common.RoleRootUser)
		GetAdtyAdminSiteCustomers(c)
		var customers struct {
			Success bool `json:"success"`
			Data    struct {
				Items []struct {
					Quota int `json:"quota"`
				} `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &customers))
		require.True(t, customers.Success, recorder.Body.String())
		require.Len(t, customers.Data.Items, 1)
		assert.Equal(t, 20, customers.Data.Items[0].Quota)
	}
}

func adtyTaskFixture() *model.Task {
	return &model.Task{
		TaskID:    "task_public",
		ChannelId: 42,
		Properties: model.Properties{
			Input: "a cat", UpstreamModelName: "vendor-real-model", OriginModelName: "video-1",
		},
	}
}

func TestUserTasksHideChannelAndUpstreamModel(t *testing.T) {
	task := adtyTaskFixture()

	userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.Zero(t, userView.ChannelId)
	props, ok := userView.Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, props.UpstreamModelName)
	assert.Equal(t, "video-1", props.OriginModelName)

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
	assert.Equal(t, 42, adminView.ChannelId)
	assert.Equal(t, "vendor-real-model", adminView.Properties.(model.Properties).UpstreamModelName)
}

func TestVideoFetchHidesChannel(t *testing.T) {
	// The video fetch API returns TaskModel2Dto straight to API tokens.
	view := relay.TaskModel2Dto(adtyTaskFixture())
	assert.Zero(t, view.ChannelId)
}

func TestUserMidjourneyHideChannelId(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}))
	require.NoError(t, db.Create(&model.Midjourney{UserId: 7, MjId: "mj-1", ChannelId: 42, Action: "IMAGINE"}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/mj/self?p=1&page_size=10", nil)
	c.Set("id", 7)
	GetUserMidjourney(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"mj_id":"mj-1"`)
	assert.Contains(t, recorder.Body.String(), `"channel_id":0`)
	assert.NotContains(t, recorder.Body.String(), `"channel_id":42`)
}

func TestSelfStatIgnoresChannelForUsers(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = oldLogDB })
	now := common.GetTimestamp()
	require.NoError(t, db.Create([]*model.Log{
		{UserId: 7, Username: "stat-user", Type: model.LogTypeConsume, Quota: 100, ChannelId: 1, CreatedAt: now},
		{UserId: 7, Username: "stat-user", Type: model.LogTypeConsume, Quota: 50, ChannelId: 2, CreatedAt: now},
	}).Error)

	stat := func(role int) string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/stat?type=2&channel=1", nil)
		c.Set("id", 7)
		c.Set("username", "stat-user")
		c.Set("role", role)
		GetLogsSelfStat(c)
		return recorder.Body.String()
	}
	assert.Contains(t, stat(common.RoleCommonUser), `"quota":150`, "the channel filter is ignored for users")
	assert.Contains(t, stat(common.RoleAdminUser), `"quota":100`)
}

func TestRelayErrorMasksUpstreamURL(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://secret-upstream.example.com")

	apiErr := types.NewOpenAIError(errors.New("Post \"https://secret-upstream.example.com/v1/chat/completions\": dial tcp: timeout"),
		types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	message := apiErr.ToOpenAIError(common.UpstreamAddresses(c)...).Message

	assert.NotContains(t, message, "secret-upstream.example.com")
	assert.True(t, strings.Contains(message, "timeout"), message)
}
