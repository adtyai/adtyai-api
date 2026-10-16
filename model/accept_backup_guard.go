package model

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	defaultAcceptBackupDir    = "/workspace/adty-mock/backups"
	defaultAcceptBackupMaxAge = 2 * time.Hour
)

// pgRestoreList 可在单测中替换；默认执行 `pg_restore -l <file>` 校验备份可读。
var pgRestoreList = func(path string) error {
	out, err := exec.Command("pg_restore", "-l", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_restore -l failed: %v: %s", err, firstLine(string(out)))
	}
	return nil
}

// RequireRecentAcceptBackup 验收清理入口守卫：没有近期可用备份就拒跑。
//   - ADTY_ACCEPT_BACKUP_DIR（默认 /workspace/adty-mock/backups）下最新的 *.dump
//   - mtime 在 ADTY_ACCEPT_BACKUP_MAX_AGE 内（Go duration，默认 2h）
//   - 文件非空，且 `pg_restore -l` 成功
//
// 跳过开关 ADTY_ACCEPT_BACKUP_SKIP=1 默认关闭，仅供无库单测使用。
func RequireRecentAcceptBackup() error {
	if os.Getenv("ADTY_ACCEPT_BACKUP_SKIP") == "1" {
		return nil
	}
	dir := os.Getenv("ADTY_ACCEPT_BACKUP_DIR")
	if dir == "" {
		dir = defaultAcceptBackupDir
	}
	maxAge := defaultAcceptBackupMaxAge
	if v := os.Getenv("ADTY_ACCEPT_BACKUP_MAX_AGE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return fmt.Errorf("accept backup guard: invalid ADTY_ACCEPT_BACKUP_MAX_AGE %q (e.g. 2h)", v)
		}
		maxAge = d
	}
	_, err := checkRecentBackup(dir, maxAge, time.Now())
	return err
}

func checkRecentBackup(dir string, maxAge time.Duration, now time.Time) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.dump"))
	if err != nil {
		return "", err
	}
	var newest string
	var newestInfo os.FileInfo
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil || fi.IsDir() {
			continue
		}
		if newestInfo == nil || fi.ModTime().After(newestInfo.ModTime()) {
			newest, newestInfo = f, fi
		}
	}
	if newestInfo == nil {
		return "", fmt.Errorf("accept backup guard: no *.dump in %s; back up the 联调 DB first (pg_dump -Fc) or set ADTY_ACCEPT_BACKUP_DIR", dir)
	}
	if newestInfo.ModTime().After(now.Add(time.Minute)) {
		return "", fmt.Errorf("accept backup guard: newest backup %s has mtime in the future (%s); refusing", newest, newestInfo.ModTime().Format(time.RFC3339))
	}
	if age := now.Sub(newestInfo.ModTime()); age > maxAge {
		return "", fmt.Errorf("accept backup guard: newest backup %s is %s old (> %s); take a fresh backup first", newest, age.Round(time.Minute), maxAge)
	}
	if newestInfo.Size() == 0 {
		return "", fmt.Errorf("accept backup guard: newest backup %s is empty", newest)
	}
	if err := pgRestoreList(newest); err != nil {
		return "", fmt.Errorf("accept backup guard: newest backup %s unreadable: %v", newest, err)
	}
	return newest, nil
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
