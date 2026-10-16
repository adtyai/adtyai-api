package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// B12 恢复演练真解密：下载密文 → age -d -i <identity> → pg_restore 到临时库 → 轻量校验 → 删库删文件。
//
// 环境变量（私钥只以“文件路径”出现；文件内容绝不读进内存/日志/库/告警，只交给 age CLI）：
//   ADTY_RESTORE_DRILL_IDENTITY_FILE     age 私钥 identity 文件路径；空或文件不存在 → 保持 restore_decrypt_not_configured
//   ADTY_RESTORE_DRILL_PG_DSN            可选：有 CREATEDB 权限的管理 DSN（建/删临时库）；空 → ADTY_BACKUP_PG_DSN → SQL_DSN
//   ADTY_RESTORE_DRILL_TEMP_DB_PREFIX    可选：临时库名前缀，默认 adty_restore_drill_（仅 [a-z0-9_]）
//   ADTY_RESTORE_DRILL_TIMEOUT_MINUTES   可选：整条链超时，默认 30
//   ADTY_RESTORE_DRILL_TMPDIR            可选：临时文件根目录，默认系统 TMPDIR（每次建 0700 私有子目录，结束整目录删除）
// 任务开关仍是 ADTY_RESTORE_DRILL_TASK_ENABLED（默认 false）。

const defaultRestoreDrillDBPrefix = "adty_restore_drill_"

// AgeRestoreConfig 真解密配置；Run* 钩子可注入（验收用假 age / 假 pg_restore）。
type AgeRestoreConfig struct {
	IdentityFile string
	AdminDSN     string
	DBPrefix     string
	Timeout      time.Duration
	TmpRoot      string
	Store        BackupStore // nil → R2 from env

	// 钩子：nil 走真实 CLI / SQL
	RunAge       func(ctx context.Context, identityFile, in, out string) error
	RunPgRestore func(ctx context.Context, dsn, dumpPath string) error
	CreateDB     func(ctx context.Context, adminDSN, name string) error
	DropDB       func(ctx context.Context, adminDSN, name string) error
	Check        func(ctx context.Context, dsn string) (map[string]any, error)
	OnCleanup    func(tmpDir, dbName string) // 验收观察清理
}

// RestoreDecryptFromEnv 配了 identity 且文件存在 → 真实现；否则默认未接线桩（同日 skip 逻辑依赖此判断）。
func RestoreDecryptFromEnv() RestoreDecryptFn {
	id := backupEnv("ADTY_RESTORE_DRILL_IDENTITY_FILE")
	if id == "" {
		return defaultRestoreDecrypt
	}
	if st, err := os.Stat(id); err != nil || st.IsDir() {
		return defaultRestoreDecrypt
	}
	cfg := AgeRestoreConfig{
		IdentityFile: id,
		AdminDSN:     firstNonEmpty(backupEnv("ADTY_RESTORE_DRILL_PG_DSN"), backupEnv("ADTY_BACKUP_PG_DSN"), backupEnv("SQL_DSN")),
		DBPrefix:     backupEnv("ADTY_RESTORE_DRILL_TEMP_DB_PREFIX"),
		TmpRoot:      backupEnv("ADTY_RESTORE_DRILL_TMPDIR"),
	}
	if m, err := strconv.Atoi(backupEnv("ADTY_RESTORE_DRILL_TIMEOUT_MINUTES")); err == nil && m > 0 {
		cfg.Timeout = time.Duration(m) * time.Minute
	}
	return NewAgeRestoreDecrypt(cfg)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

var dbPrefixRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// NewAgeRestoreDecrypt 返回真实 RestoreDecryptFn。
func NewAgeRestoreDecrypt(cfg AgeRestoreConfig) RestoreDecryptFn {
	if cfg.DBPrefix == "" || !dbPrefixRe.MatchString(cfg.DBPrefix) {
		cfg.DBPrefix = defaultRestoreDrillDBPrefix
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	if cfg.RunAge == nil {
		cfg.RunAge = runAgeDecryptCLI
	}
	if cfg.RunPgRestore == nil {
		cfg.RunPgRestore = runPgRestoreCLI
	}
	if cfg.CreateDB == nil {
		cfg.CreateDB = createTempDB
	}
	if cfg.DropDB == nil {
		cfg.DropDB = dropTempDB
	}
	if cfg.Check == nil {
		cfg.Check = checkRestoredDB
	}
	return func(ctx context.Context, obj *BackupObject) (extra map[string]any, err error) {
		if cfg.IdentityFile == "" {
			return nil, errors.New(ErrRestoreDecryptNotConfigured)
		}
		if _, e := os.Stat(cfg.IdentityFile); e != nil {
			return nil, errors.New(ErrRestoreDecryptNotConfigured)
		}
		if cfg.AdminDSN == "" {
			return nil, errors.New("restore admin dsn not configured")
		}
		store := cfg.Store
		if store == nil {
			s, e := NewR2BackupStoreFromEnv()
			if e != nil {
				return nil, e
			}
			store = s
		}
		ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		start := time.Now()

		tmpDir, e := os.MkdirTemp(cfg.TmpRoot, "adty-restore-*")
		if e != nil {
			return nil, fmt.Errorf("tmpdir: %v", e)
		}
		_ = os.Chmod(tmpDir, 0o700)
		dbName := ""
		dbCreated := false
		defer func() {
			// 总是清理：先删库（用独立 ctx，超时/取消后也能删），再删整个临时目录（密文+明文）
			if dbCreated {
				dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Minute)
				if de := cfg.DropDB(dctx, cfg.AdminDSN, dbName); de != nil && err == nil {
					err = fmt.Errorf("drop temp db: %v", de)
				}
				dcancel()
			}
			_ = os.RemoveAll(tmpDir)
			if cfg.OnCleanup != nil {
				cfg.OnCleanup(tmpDir, dbName)
			}
		}()

		encPath := filepath.Join(tmpDir, "backup.dump.age")
		plainPath := filepath.Join(tmpDir, "backup.dump")
		f, e := os.OpenFile(encPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if e != nil {
			return nil, e
		}
		h := sha256.New()
		e = store.Get(ctx, obj.Key, io.MultiWriter(f, h))
		_ = f.Close()
		if e != nil {
			return nil, fmt.Errorf("download: %v", e)
		}
		gotSHA := hex.EncodeToString(h.Sum(nil))
		if obj.SHA256 != "" && !strings.EqualFold(gotSHA, strings.TrimSpace(obj.SHA256)) {
			return nil, fmt.Errorf("download sha256 mismatch head=%s got=%s", obj.SHA256, gotSHA)
		}
		if e := cfg.RunAge(ctx, cfg.IdentityFile, encPath, plainPath); e != nil {
			return nil, fmt.Errorf("age decrypt: %v", e)
		}
		_ = os.Remove(encPath)

		dbName = fmt.Sprintf("%s%s_%d", cfg.DBPrefix, time.Now().In(ShanghaiLocation()).Format("20060102_150405"), os.Getpid())
		if e := cfg.CreateDB(ctx, cfg.AdminDSN, dbName); e != nil {
			return nil, fmt.Errorf("create temp db: %v", sanitizeDSN(e.Error(), cfg.AdminDSN))
		}
		dbCreated = true
		tempDSN, e := DSNWithDatabase(cfg.AdminDSN, dbName)
		if e != nil {
			return nil, e
		}
		if e := cfg.RunPgRestore(ctx, tempDSN, plainPath); e != nil {
			return nil, fmt.Errorf("pg_restore: %v", sanitizeDSN(sanitizeDSN(e.Error(), tempDSN), cfg.AdminDSN))
		}
		_ = os.Remove(plainPath) // 明文尽早删
		checks, e := cfg.Check(ctx, tempDSN)
		if e != nil {
			return nil, fmt.Errorf("verify: %v", sanitizeDSN(e.Error(), tempDSN))
		}
		extra = map[string]any{
			"skeleton":        false,
			"restore":         "ok",
			"download_sha256": gotSHA,
			"temp_db":         dbName,
			"identity_file":   filepath.Base(cfg.IdentityFile), // 只记文件名，不记内容
			"duration_ms":     time.Since(start).Milliseconds(),
		}
		for k, v := range checks {
			extra["check_"+k] = v
		}
		return extra, nil
	}
}

// sanitizeStderr 截断并抹掉任何疑似私钥行。
func sanitizeStderr(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(strings.ToUpper(l), "AGE-SECRET-KEY") {
			l = "[redacted]"
		}
		keep = append(keep, l)
	}
	s = strings.TrimSpace(strings.Join(keep, " "))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func sanitizeDSN(s, dsn string) string {
	if dsn == "" {
		return s
	}
	return strings.ReplaceAll(s, dsn, "***")
}

func runAgeDecryptCLI(ctx context.Context, identityFile, in, out string) error {
	cmd := exec.CommandContext(ctx, "age", "-d", "-i", identityFile, "-o", out, in)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v %s", err, sanitizeStderr(stderr.String()))
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		return errors.New("empty plaintext")
	}
	_ = os.Chmod(out, 0o600)
	return nil
}

func runPgRestoreCLI(ctx context.Context, dsn, dumpPath string) error {
	cmd := exec.CommandContext(ctx, "pg_restore", "--no-owner", "--no-privileges", "--exit-on-error", "-d", dsn, dumpPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v %s", err, sanitizeStderr(sanitizeDSN(stderr.String(), dsn)))
	}
	return nil
}

var safeIdentRe = regexp.MustCompile(`^[a-z0-9_]{1,63}$`)

func createTempDB(ctx context.Context, adminDSN, name string) error {
	if !safeIdentRe.MatchString(name) {
		return errors.New("bad temp db name")
	}
	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE DATABASE "`+name+`"`)
	return err
}

func dropTempDB(ctx context.Context, adminDSN, name string) error {
	if !safeIdentRe.MatchString(name) {
		return errors.New("bad temp db name")
	}
	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	return err
}

// checkRestoredDB 轻量校验：SELECT 1、public 表数>0、关键表（存在才查）行数与余额合计。
func checkRestoredDB(ctx context.Context, dsn string) (map[string]any, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var one int
	if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return nil, err
	}
	out := map[string]any{"select1": one}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil {
		return nil, err
	}
	if tables == 0 {
		return nil, errors.New("restored db has no public tables")
	}
	out["public_tables"] = tables
	for _, t := range []string{"users", "logs", "ops_job_runs", "redemptions", "topups"} {
		var reg sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.'||$1)::text`, t).Scan(&reg); err != nil {
			return nil, err
		}
		if !reg.Valid {
			continue
		}
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM "`+t+`"`).Scan(&n); err != nil {
			return nil, err
		}
		out["rows_"+t] = n
	}
	// 余额合计：users.quota 存在才算
	var hasQuota bool
	_ = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='quota')`).Scan(&hasQuota)
	if hasQuota {
		var sum sql.NullInt64
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(sum(quota),0) FROM users`).Scan(&sum); err != nil {
			return nil, err
		}
		out["users_quota_sum"] = sum.Int64
	}
	return out, nil
}

// DSNWithDatabase 把 DSN（URL 或 key=value）的库名换成 name。
func DSNWithDatabase(dsn, name string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", errors.New("bad admin dsn url")
		}
		u.Path = "/" + name
		return u.String(), nil
	}
	re := regexp.MustCompile(`(^|\s)dbname=('[^']*'|\S*)`)
	if re.MatchString(dsn) {
		return re.ReplaceAllString(dsn, "${1}dbname="+name), nil
	}
	return strings.TrimSpace(dsn) + " dbname=" + name, nil
}
