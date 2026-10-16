package model

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckRecentBackup(t *testing.T) {
	orig := pgRestoreList
	defer func() { pgRestoreList = orig }()
	// 假 pg_restore：内容以 PGDMP 开头视为可读，其余为垃圾
	pgRestoreList = func(p string) error {
		b, _ := os.ReadFile(p)
		if !strings.HasPrefix(string(b), "PGDMP") {
			return errors.New("not a dump")
		}
		return nil
	}
	now := time.Now()
	write := func(dir, name, body string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(-age)
		_ = os.Chtimes(p, mt, mt)
	}
	cases := []struct {
		name  string
		setup func(dir string)
		ok    bool
	}{
		{"no dump", func(d string) { write(d, "x.txt", "PGDMP", 0) }, false},
		{"empty", func(d string) { write(d, "a.dump", "", time.Minute) }, false},
		{"old", func(d string) { write(d, "a.dump", "PGDMP..", 3*time.Hour) }, false},
		{"garbage", func(d string) { write(d, "a.dump", "hello", time.Minute) }, false},
		{"newest garbage beats older good", func(d string) {
			write(d, "a.dump", "PGDMP..", 30*time.Minute)
			write(d, "b.dump", "junk", time.Minute)
		}, false},
		{"future mtime", func(d string) { write(d, "a.dump", "PGDMP..", -3*time.Hour) }, false},
		{"good", func(d string) {
			write(d, "old.dump", "PGDMP..", 5*time.Hour)
			write(d, "new.dump", "PGDMP..", 10*time.Minute)
		}, true},
	}
	for _, c := range cases {
		dir := t.TempDir()
		c.setup(dir)
		_, err := checkRecentBackup(dir, 2*time.Hour, now)
		if (err == nil) != c.ok {
			t.Errorf("%s: ok=%v err=%v", c.name, c.ok, err)
		}
	}
}

func TestRequireRecentAcceptBackupEnv(t *testing.T) {
	t.Setenv("ADTY_ACCEPT_BACKUP_SKIP", "")
	t.Setenv("ADTY_ACCEPT_BACKUP_DIR", t.TempDir())
	if err := RequireRecentAcceptBackup(); err == nil {
		t.Error("empty dir should be rejected (skip defaults off)")
	}
	t.Setenv("ADTY_ACCEPT_BACKUP_MAX_AGE", "bogus")
	if err := RequireRecentAcceptBackup(); err == nil {
		t.Error("invalid max age should be rejected")
	}
}

func TestCleanupAcceptReconcileDataRequiresBackup(t *testing.T) {
	t.Setenv("ADTY_ACCEPT_BACKUP_SKIP", "")
	t.Setenv("ADTY_ACCEPT_BACKUP_DIR", t.TempDir())
	// 守卫在任何 DB 操作前拒跑：DB 为 nil 也不会被碰到
	if _, err := CleanupAcceptReconcileDataForTest("accept-recon-", "ACCEPT-RECON-"); err == nil || !strings.Contains(err.Error(), "accept backup guard") {
		t.Fatalf("want backup guard error, got %v", err)
	}
	if _, err := CleanupAcceptUsersForTest([]string{"accept-rf-"}, nil); err == nil || !strings.Contains(err.Error(), "accept backup guard") {
		t.Fatalf("want backup guard error, got %v", err)
	}
}
