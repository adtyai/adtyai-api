//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func adminCtx(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "accept-health-admin")
	return c, w
}

func main() {
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	must(model.EnsureOpsJobRunsTable(model.DB))
	must(model.EnsureOpsAlertTable(model.DB))
	must(model.EnsureHealthProbesTable(model.DB))
	fmt.Println("InitDB + health_probes + ops_job_runs OK")

	ctx := context.Background()
	now := time.Now().UTC()

	// wipe accept namespace probes (epay accept-* + postgres/redis main leftover from prior)
	_ = model.DB.Where("target_type IN ?", []string{
		model.HealthTargetEpay, model.HealthTargetPostgres, model.HealthTargetRedis,
		model.HealthTargetBepusdt, model.HealthTargetChatwoot,
	}).Delete(&model.HealthProbe{})

	// (1) table exists + CHECK rejects bad type
	if !model.DB.Migrator().HasTable(&model.HealthProbe{}) {
		panic("(1) health_probes missing")
	}
	bad := model.DB.Exec(`INSERT INTO health_probes (target_type, target_id, ok, latency_ms, error, checked_at)
		VALUES ('badtype','x',true,1,'',NOW())`)
	if bad.Error == nil {
		panic("(1) CHECK should reject bad target_type")
	}
	fmt.Println("(1) table + CHECK OK")

	// (2) write probes: epay ok then fail; postgres via ProbePostgres
	epayID := "accept-epay-1"
	_, err := model.RecordHealthProbe(model.HealthTargetEpay, epayID, true, 12, "", now.Add(-2*time.Hour))
	must(err)
	failAt := now.Add(-30 * time.Minute)
	_, err = model.RecordHealthProbe(model.HealthTargetEpay, epayID, false, 500, "timeout", failAt)
	must(err)
	pg, err := model.ProbePostgres(ctx)
	must(err)
	if !pg.Ok {
		panic(fmt.Sprintf("(2) postgres probe should ok: %s", pg.Error))
	}
	rd, err := model.ProbeRedis(ctx)
	must(err)
	if rd.Ok {
		panic("(2) redis disabled should ok=false")
	}
	if !strings.Contains(rd.Error, "skipped") {
		panic(fmt.Sprintf("(2) redis skip reason want skipped got %q", rd.Error))
	}
	fmt.Println("(2) RecordHealthProbe + ProbePostgres/Redis OK")

	// (3) GET aggregate + last_failure
	c, w := adminCtx(http.MethodGet, "/api/admin/health")
	controller.AdminGetHealth(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(3) GET status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	var getResp struct {
		Success bool                         `json:"success"`
		Data    model.HealthSummaryResponse `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Success {
		panic("(3) success=false")
	}
	if !getResp.Data.Refreshed {
		panic("(3) refreshed should be true (GET sync probes)")
	}
	var epayView *model.HealthProbeView
	var pgView *model.HealthProbeView
	for i := range getResp.Data.Probes {
		p := &getResp.Data.Probes[i]
		if p.TargetType == model.HealthTargetEpay && p.TargetId == epayID {
			epayView = p
		}
		if p.TargetType == model.HealthTargetPostgres && p.TargetId == model.HealthTargetIDMain {
			pgView = p
		}
	}
	if epayView == nil {
		panic("(3) epay probe missing in summary")
	}
	if epayView.Ok {
		panic("(3) epay latest should be fail")
	}
	if epayView.LastFailureAt == nil {
		panic("(3) epay last_failure_at nil")
	}
	// allow 2s skew
	if epayView.LastFailureAt.Sub(failAt).Abs() > 2*time.Second {
		panic(fmt.Sprintf("(3) last_failure_at=%v want ~%v", epayView.LastFailureAt, failAt))
	}
	if pgView == nil || !pgView.Ok {
		panic("(3) postgres latest should be ok")
	}
	if epayView.Status != "fail" || pgView.Status != "ok" {
		panic(fmt.Sprintf("(3) status want epay=fail pg=ok got %q/%q", epayView.Status, pgView.Status))
	}
	for _, p := range getResp.Data.Probes {
		if p.Status == "skipped" && p.Ok {
			panic("(3) skipped row with ok=true")
		}
	}
	fmt.Println("(3) GET aggregate + last_failure OK")

	// (4) POST refresh
	c, w = adminCtx(http.MethodPost, "/api/admin/health/refresh")
	controller.AdminRefreshHealth(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(4) POST refresh status want 200 got %d body=%s", w.Code, w.Body.String()))
	}
	must(json.Unmarshal(w.Body.Bytes(), &getResp))
	if !getResp.Success || !getResp.Data.Refreshed {
		panic("(4) refresh response bad")
	}
	fmt.Println("(4) POST refresh OK")

	// (5) cleanup retention: old success (>7d) deleted; old failure (>30d) deleted;
	//     recent success/failure kept; ops_job_runs probe_cleanup
	oldOK := now.Add(-8 * 24 * time.Hour)
	oldFail := now.Add(-31 * 24 * time.Hour)
	keepOK := now.Add(-3 * 24 * time.Hour)
	keepFail := now.Add(-10 * 24 * time.Hour)
	_, err = model.RecordHealthProbe(model.HealthTargetEpay, "cleanup-old-ok", true, 1, "", oldOK)
	must(err)
	_, err = model.RecordHealthProbe(model.HealthTargetEpay, "cleanup-old-fail", false, 1, "old", oldFail)
	must(err)
	_, err = model.RecordHealthProbe(model.HealthTargetEpay, "cleanup-keep-ok", true, 1, "", keepOK)
	must(err)
	_, err = model.RecordHealthProbe(model.HealthTargetEpay, "cleanup-keep-fail", false, 1, "keep", keepFail)
	must(err)

	cleanup, err := model.RunProbeCleanup(ctx, true)
	must(err)
	if cleanup.Skipped {
		panic("(5) cleanup should not skip with force")
	}
	if cleanup.DeletedSuccess < 1 {
		panic(fmt.Sprintf("(5) deleted_success want >=1 got %d", cleanup.DeletedSuccess))
	}
	if cleanup.DeletedFailure < 1 {
		panic(fmt.Sprintf("(5) deleted_failure want >=1 got %d", cleanup.DeletedFailure))
	}

	var nOldOK, nOldFail, nKeepOK, nKeepFail int64
	must(model.DB.Model(&model.HealthProbe{}).Where("target_id = ?", "cleanup-old-ok").Count(&nOldOK).Error)
	must(model.DB.Model(&model.HealthProbe{}).Where("target_id = ?", "cleanup-old-fail").Count(&nOldFail).Error)
	must(model.DB.Model(&model.HealthProbe{}).Where("target_id = ?", "cleanup-keep-ok").Count(&nKeepOK).Error)
	must(model.DB.Model(&model.HealthProbe{}).Where("target_id = ?", "cleanup-keep-fail").Count(&nKeepFail).Error)
	if nOldOK != 0 || nOldFail != 0 {
		panic(fmt.Sprintf("(5) old rows should be gone ok=%d fail=%d", nOldOK, nOldFail))
	}
	if nKeepOK != 1 || nKeepFail != 1 {
		panic(fmt.Sprintf("(5) keep rows want 1/1 got ok=%d fail=%d", nKeepOK, nKeepFail))
	}

	runKey := now.Format("2006-01-02")
	job, err := model.GetOpsJobRun(model.OpsJobProbeCleanup, runKey)
	must(err)
	if job.Status != model.OpsJobStatusOK {
		panic(fmt.Sprintf("(5) ops_job_runs status want ok got %s", job.Status))
	}
	fmt.Println("(5) probe_cleanup retention + ops_job_runs OK")

	// (6) handler registered
	controller.RegisterScheduledSystemTasks()
	fmt.Println("(6) RegisterScheduledSystemTasks (incl probe_cleanup) OK")

	fmt.Println("ALL ACCEPT HEALTH PASSED")
}
