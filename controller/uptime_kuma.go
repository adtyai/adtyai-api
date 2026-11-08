package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	requestTimeout   = 30 * time.Second
	httpTimeout      = 10 * time.Second
	uptimeCacheTTL   = 15 * time.Second
	uptimeKeySuffix  = "_24"
	apiStatusPath    = "/api/status-page/"
	apiHeartbeatPath = "/api/status-page/heartbeat/"
)

type Monitor struct {
	Name   string  `json:"name"`
	Uptime float64 `json:"uptime"`
	Status int     `json:"status"`
	Group  string  `json:"group,omitempty"`
}

type UptimeGroupResult struct {
	CategoryName string    `json:"categoryName"`
	Monitors     []Monitor `json:"monitors"`
}

type uptimeKumaStatusCache struct {
	sync.Mutex
	configuration string
	expiresAt     time.Time
	results       []UptimeGroupResult
	refresh       singleflight.Group
}

var publicUptimeStatusCache uptimeKumaStatusCache

func (cache *uptimeKumaStatusCache) get(configuration string) ([]UptimeGroupResult, bool) {
	cache.Lock()
	defer cache.Unlock()
	if cache.configuration != configuration || !time.Now().Before(cache.expiresAt) {
		return nil, false
	}
	return cache.results, true
}

func getAndDecode(ctx context.Context, client *http.Client, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 status")
	}

	return common.DecodeJson(resp.Body, dest)
}

func fetchGroupData(ctx context.Context, client *http.Client, groupConfig map[string]any) UptimeGroupResult {
	url, _ := groupConfig["url"].(string)
	slug, _ := groupConfig["slug"].(string)
	categoryName, _ := groupConfig["categoryName"].(string)

	result := UptimeGroupResult{
		CategoryName: categoryName,
		Monitors:     []Monitor{},
	}

	if url == "" || slug == "" {
		return result
	}

	baseURL := strings.TrimSuffix(url, "/")

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]struct {
			Status int `json:"status"`
		} `json:"heartbeatList"`
		UptimeList map[string]float64 `json:"uptimeList"`
	}

	// The status and heartbeat endpoints are independent; one failure must not
	// cancel the other request after it has already been dispatched.
	var g errgroup.Group
	g.Go(func() error {
		return getAndDecode(ctx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(ctx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})

	if g.Wait() != nil {
		return result
	}

	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}

		for _, m := range pg.MonitorList {
			// Group names often identify channels. The public page does not
			// show them, and this unauthenticated response must not either.
			monitor := Monitor{
				Name: m.Name,
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				monitor.Status = heartbeats[0].Status
			}

			result.Monitors = append(result.Monitors, monitor)
		}
	}

	return result
}

func GetUptimeKumaStatus(c *gin.Context) {
	settings := console_setting.GetConsoleSetting()
	if !settings.UptimeKumaEnabled {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []UptimeGroupResult{}})
		return
	}
	configuration := settings.UptimeKumaGroups
	groups := console_setting.GetUptimeKumaGroups()
	if len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []UptimeGroupResult{}})
		return
	}

	if results, ok := publicUptimeStatusCache.get(configuration); ok {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
		return
	}

	refresh := publicUptimeStatusCache.refresh.DoChan(configuration, func() (any, error) {
		if results, ok := publicUptimeStatusCache.get(configuration); ok {
			return results, nil
		}
		// A disconnected visitor must not cancel work shared by other callers.
		// The independent deadline still bounds every refresh, including failures.
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		client := &http.Client{Timeout: httpTimeout}
		results := make([]UptimeGroupResult, len(groups))
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(4)
		for i, group := range groups {
			g.Go(func() error {
				results[i] = fetchGroupData(gCtx, client, group)
				return nil
			})
		}
		_ = g.Wait()
		publicUptimeStatusCache.Lock()
		publicUptimeStatusCache.configuration = configuration
		publicUptimeStatusCache.results = results
		publicUptimeStatusCache.expiresAt = time.Now().Add(uptimeCacheTTL)
		publicUptimeStatusCache.Unlock()
		return results, nil
	})
	select {
	case <-c.Request.Context().Done():
		return
	case result := <-refresh:
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result.Val.([]UptimeGroupResult)})
	}
}
