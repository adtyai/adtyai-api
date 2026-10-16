//go:build ignore

// 联调一次性：force 跑备份 + restore_drill（跳过周日窗 / 03:00）。
//
// 用途：联调机 MinIO 假 R2 + 测试 age 钥整链验证。
//   - 私钥只走 ADTY_RESTORE_DRILL_IDENTITY_FILE 文件路径（内容不进代码/库/日志）
//   - 本脚本不会设置 ADTY_RESTORE_DRILL_TASK_ENABLED / ADTY_BACKUP_TASK_ENABLED
//   - 定时任务仍保持默认（演练默认关）；生产密钥/真 R2 等总裁确认
//
// 用法（仓库根）：
//   ./scripts/run_manual_backup_restore_drill.sh
//   # .sh 会 go build 后 exec 二进制，缺配置时 exit 2 可原样传到 shell
//   # （勿直接依赖 go run：go run 会把非 0 收成 1）
//
// 必填 env：
//   ADTY_BACKUP_AGE_RECIPIENT              age1… 公钥
//   ADTY_RESTORE_DRILL_IDENTITY_FILE       age 私钥 identity 文件路径（须存在）
//   ADTY_BACKUP_R2_ENDPOINT / BUCKET / ACCESS_KEY_ID / SECRET_ACCESS_KEY
//   SQL_DSN 或 ADTY_BACKUP_PG_DSN          备份源库；演练管理 DSN 另可配 ADTY_RESTORE_DRILL_PG_DSN（CREATEDB）
//
// 缺 recipient / identity → exit 2；业务失败 → exit 1。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func env(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func failPre(msg string) {
	fmt.Fprintf(os.Stderr, "manual_backup_restore_drill: %s\n", msg)
	os.Exit(2)
}

func preflight() {
	rcpt := env("ADTY_BACKUP_AGE_RECIPIENT")
	if rcpt == "" {
		failPre("ADTY_BACKUP_AGE_RECIPIENT missing (need age1… test public key)")
	}
	if !strings.HasPrefix(rcpt, "age1") {
		failPre("ADTY_BACKUP_AGE_RECIPIENT must be a public key (age1...)")
	}
	id := env("ADTY_RESTORE_DRILL_IDENTITY_FILE")
	if id == "" {
		failPre("ADTY_RESTORE_DRILL_IDENTITY_FILE missing (path to age identity; private key stays on disk)")
	}
	st, err := os.Stat(id)
	if err != nil || st.IsDir() {
		failPre("ADTY_RESTORE_DRILL_IDENTITY_FILE not found or not a file: " + id)
	}
	for _, k := range []string{
		"ADTY_BACKUP_R2_ENDPOINT",
		"ADTY_BACKUP_R2_BUCKET",
		"ADTY_BACKUP_R2_ACCESS_KEY_ID",
		"ADTY_BACKUP_R2_SECRET_ACCESS_KEY",
	} {
		if env(k) == "" {
			failPre(k + " missing (联调 MinIO/假 R2 四项)")
		}
	}
	if env("ADTY_BACKUP_PG_DSN") == "" && env("SQL_DSN") == "" {
		failPre("SQL_DSN or ADTY_BACKUP_PG_DSN required")
	}
	// 明确不碰定时开关；若运维误开，仅警告
	if strings.EqualFold(env("ADTY_RESTORE_DRILL_TASK_ENABLED"), "true") {
		fmt.Fprintln(os.Stderr, "WARN: ADTY_RESTORE_DRILL_TASK_ENABLED=true — 本脚本仍 force 跑一次；联调建议关掉定时")
	}
}

func main() {
	preflight()

	if env("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true

	if err := model.InitDB(); err != nil {
		fmt.Fprintf(os.Stderr, "InitDB: %v\n", err)
		os.Exit(1)
	}
	if err := model.InitLogDB(); err != nil {
		fmt.Fprintf(os.Stderr, "InitLogDB: %v\n", err)
		os.Exit(1)
	}
	if err := model.EnsureOpsJobRunsTable(model.DB); err != nil {
		fmt.Fprintf(os.Stderr, "EnsureOpsJobRunsTable: %v\n", err)
		os.Exit(1)
	}
	_ = model.EnsureOpsAlertTable(model.DB)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	summary := map[string]any{
		"mode":                    "manual_force",
		"task_enabled_unchanged":  true,
		"restore_drill_task_env":  env("ADTY_RESTORE_DRILL_TASK_ENABLED"),
		"restore_drill_task_note": "default off; this script does not enable it",
		"started_at":              time.Now().In(model.ShanghaiLocation()).Format(time.RFC3339),
	}

	fmt.Println("==> RunDailyBackup force=true (runKey=today Shanghai)")
	bak, err := model.RunDailyBackup(ctx, "", true, nil)
	if err != nil {
		summary["backup_error"] = err.Error()
		printJSON(summary)
		fmt.Fprintf(os.Stderr, "backup fatal: %v\n", err)
		os.Exit(1)
	}
	summary["backup"] = bak
	if bak == nil || bak.Status != model.OpsJobStatusOK {
		printJSON(summary)
		fmt.Fprintln(os.Stderr, "backup failed (see JSON)")
		os.Exit(1)
	}
	fmt.Printf("backup ok run_key=%s object_key=%s size=%d sha256=%s\n",
		bak.RunKey, bak.ObjectKey, bak.Size, bak.SHA256)

	if key, ok, e := model.LastOKOpsJobRunKey(model.OpsJobBackup); e == nil {
		if ok {
			summary["last_backup"] = key
		} else {
			summary["last_backup"] = nil
		}
	}

	fmt.Println("==> RunRestoreDrill force=true (skip Sunday window; Decrypt from env identity)")
	drill, err := model.RunRestoreDrill(ctx, "", true, nil)
	if err != nil {
		summary["restore_error"] = err.Error()
		printJSON(summary)
		fmt.Fprintf(os.Stderr, "restore_drill fatal: %v\n", err)
		os.Exit(1)
	}
	summary["restore_drill"] = drill
	if drill == nil || drill.Status != model.OpsJobStatusOK {
		printJSON(summary)
		fmt.Fprintln(os.Stderr, "restore_drill failed (see JSON)")
		os.Exit(1)
	}
	fmt.Printf("restore_drill ok run_key=%s backup_day=%s object_key=%s\n",
		drill.RunKey, drill.BackupDay, drill.ObjectKey)
	if drill.Detail != nil {
		if v, ok := drill.Detail["temp_db"]; ok {
			summary["temp_db"] = v
			summary["temp_db_cleanup"] = "always_drop_on_exit"
		}
		if v, ok := drill.Detail["skeleton"]; ok {
			summary["skeleton"] = v
		}
		if v, ok := drill.Detail["restore"]; ok {
			summary["restore"] = v
		}
	}

	summary["ok"] = true
	summary["finished_at"] = time.Now().In(model.ShanghaiLocation()).Format(time.RFC3339)
	printJSON(summary)
	fmt.Println("manual backup + restore_drill OK")
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
