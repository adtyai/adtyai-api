package service

import (
	"strconv"
	"time"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func init() {
	perfmetrics.GroupHealthObserver = observeAdtyGroupHealth
}

type AdtyGroupHealthView struct {
	Group              string                 `json:"group"`
	Status             string                 `json:"status"`
	RequestSuccessRate *float64               `json:"request_success_rate"`
	RequestSamples     int                    `json:"request_samples"`
	ProbeRate          *float64               `json:"probe_rate"`
	ProbeSamples       int                    `json:"probe_samples"`
	Scope              string                 `json:"scope"`
	UpdatedAt          int64                  `json:"updated_at"`
	Series             []AdtyGroupHealthPoint `json:"series"`
}

type AdtyGroupHealthPoint struct {
	Ts      int64 `json:"ts"`
	Success int   `json:"success"`
	Fail    int   `json:"fail"`
}

func observeAdtyGroupHealth(group, modelName string, userId int, outcome perfmetrics.Outcome) {
	if model.DB == nil || group == "" {
		return
	}
	kind, record := adtyGroupHealthFromOutcome(string(outcome))
	if !record {
		return
	}
	siteId := 0
	if userId > 0 {
		var user model.User
		if err := model.DB.Select("site_id").First(&user, userId).Error; err == nil {
			siteId = user.SiteId
		}
	}
	_ = RecordAdtyGroupHealth(siteId, group, modelName, model.AdtyGroupHealthRequest, kind, time.Now())
}

func adtyGroupHealthFromOutcome(outcome string) (string, bool) {
	switch outcome {
	case string(perfmetrics.OutcomeSuccess):
		return model.AdtyGroupHealthSuccess, true
	case string(perfmetrics.OutcomeFailure):
		return model.AdtyGroupHealthUpstream, true
	default:
		return model.AdtyGroupHealthCustomer, false
	}
}

func RecordAdtyGroupHealth(siteId int, group, modelName, kind, outcome string, now time.Time) error {
	if group == "" || (kind != model.AdtyGroupHealthRequest && kind != model.AdtyGroupHealthProbe) {
		return ErrAdtyTranslationInvalid
	}
	if outcome != model.AdtyGroupHealthSuccess && outcome != model.AdtyGroupHealthUpstream && outcome != model.AdtyGroupHealthCustomer {
		return ErrAdtyTranslationInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model.AdtyGroupHealthSample{
			SiteId: siteId, GroupName: group, ModelName: modelName, Kind: kind, Outcome: outcome, CreatedAt: now.Unix(),
		}).Error; err != nil {
			return err
		}
		var state model.AdtyGroupHealthState
		found := tx.First(&state, 1)
		state.Id = 1
		state.HeartbeatAt = now.Unix()
		if state.SuccessMin == "" {
			state.SuccessMin = "0.95"
		}
		if state.StaleSeconds == 0 {
			state.StaleSeconds = 6 * 3600
		}
		if found.Error == gorm.ErrRecordNotFound {
			return tx.Create(&state).Error
		}
		if found.Error != nil {
			return found.Error
		}
		return tx.Model(&state).Updates(map[string]any{"heartbeat_at": now.Unix()}).Error
	})
}

func RecordAdtyGroupProbe(siteId int, group, modelName string, ok bool, now time.Time) error {
	outcome := model.AdtyGroupHealthUpstream
	if ok {
		outcome = model.AdtyGroupHealthSuccess
	}
	return RecordAdtyGroupHealth(siteId, group, modelName, model.AdtyGroupHealthProbe, outcome, now)
}

func PutAdtyGroupHealthConfig(successMin string, staleSeconds int) error {
	value, err := decimal.NewFromString(successMin)
	if err != nil || value.IsNegative() || value.GreaterThan(decimal.NewFromInt(1)) || staleSeconds < 60 {
		return ErrAdtyTranslationInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var state model.AdtyGroupHealthState
		found := tx.First(&state, 1)
		state.Id = 1
		state.SuccessMin = successMin
		state.StaleSeconds = staleSeconds
		if found.Error == gorm.ErrRecordNotFound {
			return tx.Create(&state).Error
		}
		if found.Error != nil {
			return found.Error
		}
		return tx.Model(&state).Updates(map[string]any{"success_min": successMin, "stale_seconds": staleSeconds}).Error
	})
}

func ListAdtyGroupHealthForUser(userId int, userGroup, modelName string, allowed []string, now time.Time) ([]AdtyGroupHealthView, error) {
	var user model.User
	if err := model.DB.Select("id", "site_id").First(&user, userId).Error; err != nil {
		return nil, err
	}
	rows, err := listAdtyGroupHealth(user.SiteId, modelName, now)
	if err != nil {
		return nil, err
	}
	allow := map[string]struct{}{}
	for _, group := range allowed {
		allow[group] = struct{}{}
	}
	if userGroup != "" {
		allow[userGroup] = struct{}{}
	}
	filtered := make([]AdtyGroupHealthView, 0, len(allow))
	seen := map[string]struct{}{}
	scope := "platform"
	if user.SiteId > 0 {
		scope = "site"
	}
	for _, row := range rows {
		if _, ok := allow[row.Group]; !ok {
			continue
		}
		seen[row.Group] = struct{}{}
		filtered = append(filtered, row)
	}
	for group := range allow {
		if _, ok := seen[group]; ok || group == "" || group == "auto" {
			continue
		}
		filtered = append(filtered, AdtyGroupHealthView{
			Group: group, Status: model.AdtyGroupHealthNoData, Scope: scope, Series: adtyHealthSeries(now, nil),
		})
	}
	return filtered, nil
}

func listAdtyGroupHealth(siteId int, modelName string, now time.Time) ([]AdtyGroupHealthView, error) {
	var state model.AdtyGroupHealthState
	_ = model.DB.First(&state, 1).Error
	if state.SuccessMin == "" {
		state.SuccessMin = "0.95"
	}
	if state.StaleSeconds == 0 {
		state.StaleSeconds = 6 * 3600
	}
	minRate, _ := decimal.NewFromString(state.SuccessMin)
	since := now.Add(-24 * time.Hour).Unix()
	query := model.DB.Where("site_id = ? AND created_at >= ?", siteId, since)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	var samples []model.AdtyGroupHealthSample
	if err := query.Order("created_at ASC").Find(&samples).Error; err != nil {
		return nil, err
	}
	type tally struct {
		success int
		fail    int
		refuse  int
		probeOK int
		probeNo int
		updated int64
		series  map[int64]*AdtyGroupHealthPoint
	}
	grouped := map[string]*tally{}
	for _, sample := range samples {
		row := grouped[sample.GroupName]
		if row == nil {
			row = &tally{series: map[int64]*AdtyGroupHealthPoint{}}
			grouped[sample.GroupName] = row
		}
		if sample.CreatedAt > row.updated {
			row.updated = sample.CreatedAt
		}
		if sample.Kind == model.AdtyGroupHealthProbe {
			if sample.Outcome == model.AdtyGroupHealthSuccess {
				row.probeOK++
			} else if sample.Outcome == model.AdtyGroupHealthUpstream {
				row.probeNo++
			}
			continue
		}
		if sample.Outcome == model.AdtyGroupHealthCustomer {
			row.refuse++
			continue
		}
		window := sample.CreatedAt - (sample.CreatedAt % (15 * 60))
		point := row.series[window]
		if point == nil {
			point = &AdtyGroupHealthPoint{Ts: window}
			row.series[window] = point
		}
		if sample.Outcome == model.AdtyGroupHealthSuccess {
			row.success++
			point.Success++
		} else {
			row.fail++
			point.Fail++
		}
	}
	interrupted := state.HeartbeatAt > 0 && now.Unix()-state.HeartbeatAt > int64(state.StaleSeconds)
	scope := "platform"
	if siteId > 0 {
		scope = "site"
	}
	out := make([]AdtyGroupHealthView, 0, len(grouped))
	for group, row := range grouped {
		view := AdtyGroupHealthView{Group: group, Status: model.AdtyGroupHealthNoData, Scope: scope, UpdatedAt: row.updated, Series: adtyHealthSeries(now, row.series)}
		requestTotal := row.success + row.fail
		view.RequestSamples = requestTotal
		view.ProbeSamples = row.probeOK + row.probeNo
		if interrupted {
			view.Status = model.AdtyGroupHealthInterrupted
			out = append(out, view)
			continue
		}
		if requestTotal == 0 {
			view.Status = model.AdtyGroupHealthNoData
		} else {
			rate := float64(row.success) / float64(requestTotal)
			view.RequestSuccessRate = &rate
			if decimal.NewFromFloat(rate).GreaterThanOrEqual(minRate) {
				view.Status = model.AdtyGroupHealthOK
			} else {
				view.Status = model.AdtyGroupHealthDegraded
				_ = openAdtyGroupHealthAlert(siteId, group, "degraded")
			}
		}
		if view.ProbeSamples > 0 {
			rate := float64(row.probeOK) / float64(view.ProbeSamples)
			view.ProbeRate = &rate
		}
		if view.Status == model.AdtyGroupHealthOK {
			_ = closeAdtyGroupHealthAlert(siteId, group)
		}
		out = append(out, view)
	}
	return out, nil
}

func adtyHealthSeries(now time.Time, points map[int64]*AdtyGroupHealthPoint) []AdtyGroupHealthPoint {
	starts := AdtyHealthWindowStarts(now)
	out := make([]AdtyGroupHealthPoint, len(starts))
	for i, ts := range starts {
		if point := points[ts]; point != nil {
			out[i] = *point
			continue
		}
		out[i] = AdtyGroupHealthPoint{Ts: ts}
	}
	return out
}

func openAdtyGroupHealthAlert(siteId int, group, kind string) error {
	var count int64
	if err := model.DB.Model(&model.AdtyGroupHealthAlert{}).Where("site_id = ? AND group_name = ? AND open = ?", siteId, group, true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return model.DB.Create(&model.AdtyGroupHealthAlert{SiteId: siteId, GroupName: group, Kind: kind, Open: true}).Error
}

func closeAdtyGroupHealthAlert(siteId int, group string) error {
	return model.DB.Model(&model.AdtyGroupHealthAlert{}).Where("site_id = ? AND group_name = ? AND open = ?", siteId, group, true).Update("open", false).Error
}

func AdtyGroupHealthDoesNotReroute(tokenId int) (string, error) {
	var token model.Token
	if err := model.DB.Select("group").First(&token, tokenId).Error; err != nil {
		return "", err
	}
	return token.Group, nil
}

func initGroupHealthThreshold(value string) string {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "0.95"
	}
	return strconv.FormatFloat(parsed, 'f', -1, 64)
}
