//go:build ignore

// B12 restore_drill 骨架验收（云 PG；假 R2；无 age 私钥、无 pg_restore）。
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// listOverrideStore 让 List 返回指定 sha，Head 走底层（模拟 List/Head 不一致）。
type listOverrideStore struct {
	model.BackupStore
	listSHA string
}

func (s *listOverrideStore) List(ctx context.Context, prefix string) ([]model.BackupObject, error) {
	objs, err := s.BackupStore.List(ctx, prefix)
	for i := range objs {
		objs[i].SHA256 = s.listSHA
	}
	return objs, err
}

func must(err error) {
	if err != nil {
		panic(err)
	}
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
	ctx := context.Background()
	loc := model.ShanghaiLocation()

	var def string
	must(model.DB.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='ops_job_runs_job_check'`).Scan(&def).Error)
	if !strings.Contains(def, "restore_drill") {
		panic("(0) job CHECK missing restore_drill: " + def)
	}
	fmt.Println("(0) ops_job_runs CHECK includes restore_drill OK")

	// (1) non-Sunday → skip (no ops_job_runs row)
	monday := time.Date(2026, 10, 12, 10, 0, 0, 0, loc) // Monday
	store := model.NewMemBackupStore()
	store.Objects[model.BackupObjectKey("2026-10-11")] = model.BackupObject{
		Key: model.BackupObjectKey("2026-10-11"), Size: 42, SHA256: strings.Repeat("a", 64),
	}
	r1, err := model.RunRestoreDrill(ctx, "", false, &model.RestoreDrillDeps{
		Store: store, Now: func() time.Time { return monday },
		Decrypt: func(context.Context, *model.BackupObject) (map[string]any, error) {
			return map[string]any{"fake": true}, nil
		},
	})
	must(err)
	if !r1.Skipped || r1.Reason != "not_sunday_window" {
		b, _ := json.Marshal(r1)
		panic("(1) monday should skip: " + string(b))
	}
	fmt.Println("(1) non-Sunday skip OK")

	// (2) Sunday before window → skip
	sunEarly := time.Date(2026, 10, 11, 2, 0, 0, 0, loc) // Sunday 02:00 < after-hour 4
	r1b, err := model.RunRestoreDrill(ctx, "", false, &model.RestoreDrillDeps{
		Store: store, Now: func() time.Time { return sunEarly },
		Decrypt: func(context.Context, *model.BackupObject) (map[string]any, error) {
			return map[string]any{"fake": true}, nil
		},
	})
	must(err)
	if !r1b.Skipped || r1b.Reason != "not_sunday_window" {
		panic("(1b) sunday before window should skip")
	}
	fmt.Println("(1b) Sunday before after-hour skip OK")

	// (3) Sunday success with fake decrypt
	sunday := time.Date(2026, 10, 11, 10, 0, 0, 0, loc)
	runKey := sunday.Format("2006-01-02")
	_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, runKey)
	r2, err := model.RunRestoreDrill(ctx, runKey, false, &model.RestoreDrillDeps{
		Store: store, Now: func() time.Time { return sunday },
		Decrypt: func(_ context.Context, obj *model.BackupObject) (map[string]any, error) {
			return map[string]any{"rows_checked": 1, "hook": "fake_ok", "object": obj.Key}, nil
		},
	})
	must(err)
	if r2.Status != model.OpsJobStatusOK || r2.ObjectKey == "" || r2.Size != 42 {
		b, _ := json.Marshal(r2)
		panic("(2) success path: " + string(b))
	}
	row, err := model.GetOpsJobRun(model.OpsJobRestoreDrill, runKey)
	must(err)
	if row.Status != model.OpsJobStatusOK || !strings.Contains(row.Detail, "skeleton") || !strings.Contains(row.Detail, "fake_ok") {
		panic("(2) ops_job_runs detail: " + row.Detail)
	}
	fmt.Printf("(2) restore_drill ok key=%s size=%d OK\n", r2.ObjectKey, r2.Size)

	// (4) same-day dedupe
	r3, err := model.RunRestoreDrill(ctx, runKey, false, &model.RestoreDrillDeps{
		Store: store, Now: func() time.Time { return sunday },
		Decrypt: func(context.Context, *model.BackupObject) (map[string]any, error) {
			panic("should not decrypt on dedupe")
		},
	})
	must(err)
	if !r3.Skipped {
		panic("(3) same-day should skip")
	}
	fmt.Println("(3) same-day dedupe OK")

	// (5) default decrypt stub → failed + urgent
	var sent []string
	model.OpsTelegramSender = func(level, kind, text string) error {
		sent = append(sent, level+"|"+kind+"|"+text)
		return nil
	}
	failKey := "2026-10-04" // another Sunday
	_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, failKey)
	store2 := model.NewMemBackupStore()
	store2.Objects[model.BackupObjectKey("2026-10-03")] = model.BackupObject{
		Key: model.BackupObjectKey("2026-10-03"), Size: 10, SHA256: strings.Repeat("b", 64),
	}
	rf, err := model.RunRestoreDrill(ctx, failKey, true /*force*/, &model.RestoreDrillDeps{
		Store: store2, Now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, loc) },
		// Decrypt nil → default stub
	})
	must(err)
	if rf.Status != model.OpsJobStatusFailed || !strings.Contains(rf.Error, model.ErrRestoreDecryptNotConfigured) {
		b, _ := json.Marshal(rf)
		panic("(4) default stub should fail: " + string(b))
	}
	frow, _ := model.GetOpsJobRun(model.OpsJobRestoreDrill, failKey)
	if frow == nil || frow.Status != model.OpsJobStatusFailed {
		panic("(4) failed row missing")
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "urgent|restore_drill_failed|") {
		panic(fmt.Sprintf("(4) urgent alert: %v", sent))
	}
	fmt.Println("(4) default decrypt stub → failed + urgent OK")

	// (4b) same-day non-force retry with stub → skip, no new urgent, attempt unchanged
	att := frow.Attempt
	for i := 0; i < 2; i++ {
		rs, err := model.RunRestoreDrill(ctx, failKey, false, &model.RestoreDrillDeps{
			Store: store2, Now: func() time.Time { return time.Date(2026, 10, 4, 13+i, 0, 0, 0, loc) },
		})
		must(err)
		if !rs.Skipped || rs.Reason != "decrypt_not_configured_already" {
			b, _ := json.Marshal(rs)
			panic("(4b) should skip already-not-configured: " + string(b))
		}
	}
	frow2, _ := model.GetOpsJobRun(model.OpsJobRestoreDrill, failKey)
	if len(sent) != 1 || frow2 == nil || frow2.Attempt != att {
		panic(fmt.Sprintf("(4b) no spam: sent=%d attempt %d→%v", len(sent), att, frow2))
	}
	fmt.Println("(4b) same-day not-configured retry skipped, no extra urgent OK")

	// (4c) ops injects real Decrypt → same-day retry allowed and succeeds
	rw, err := model.RunRestoreDrill(ctx, failKey, false, &model.RestoreDrillDeps{
		Store: store2, Now: func() time.Time { return time.Date(2026, 10, 4, 15, 0, 0, 0, loc) },
		Decrypt: func(context.Context, *model.BackupObject) (map[string]any, error) {
			return map[string]any{"hook": "wired"}, nil
		},
	})
	must(err)
	if rw.Skipped || rw.Status != model.OpsJobStatusOK || len(sent) != 1 {
		b, _ := json.Marshal(rw)
		panic("(4c) wired retry should run ok: " + string(b))
	}
	fmt.Println("(4c) wired Decrypt same-day retry OK")

	// (4d) sha: Head empty sha → locate fail; list/head mismatch → fail; matching → ok
	shaKey := model.BackupObjectKey("2026-10-03")
	noSHA := model.NewMemBackupStore()
	noSHA.Objects[shaKey] = model.BackupObject{Key: shaKey, Size: 10, SHA256: "  "}
	if _, _, e := model.FindLatestDailyBackupObject(ctx, noSHA); e == nil || !strings.Contains(e.Error(), "sha256 meta missing") {
		panic(fmt.Sprintf("(4d) empty sha should fail: %v", e))
	}
	mm := &listOverrideStore{BackupStore: store2, listSHA: strings.Repeat("c", 64)}
	if _, _, e := model.FindLatestDailyBackupObject(ctx, mm); e == nil || !strings.Contains(e.Error(), "mismatch") {
		panic(fmt.Sprintf("(4d) sha mismatch should fail: %v", e))
	}
	okS := &listOverrideStore{BackupStore: store2, listSHA: strings.Repeat("b", 64)}
	if o, _, e := model.FindLatestDailyBackupObject(ctx, okS); e != nil || o.SHA256 != strings.Repeat("b", 64) {
		panic(fmt.Sprintf("(4d) matching sha should pass: %v", e))
	}
	fmt.Println("(4d) sha256 empty→fail, mismatch→fail, match→ok OK")

	// (6) object missing → failed + alert
	sent = nil
	missKey := "2026-09-27"
	_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, missKey)
	rm, err := model.RunRestoreDrill(ctx, missKey, true, &model.RestoreDrillDeps{
		Store: model.NewMemBackupStore(),
		Now:   func() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, loc) },
		Decrypt: func(context.Context, *model.BackupObject) (map[string]any, error) {
			return nil, errors.New("should not reach")
		},
	})
	must(err)
	if rm.Status != model.OpsJobStatusFailed || !strings.Contains(rm.Error, "locate") {
		b, _ := json.Marshal(rm)
		panic("(5) missing object: " + string(b))
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "restore_drill_failed") {
		panic(fmt.Sprintf("(5) alert: %v", sent))
	}
	fmt.Println("(5) missing object → failed + urgent OK")

	// (8) fake real-pipeline: Get → fake age → fake create/restore/check → cleanup
	{
		_ = os.Unsetenv("ADTY_RESTORE_DRILL_IDENTITY_FILE")
		tmpRoot, _ := os.MkdirTemp("", "adty-accept-rd-")
		defer os.RemoveAll(tmpRoot)
		idFile := filepath.Join(tmpRoot, "fake-identity.txt")
		must(os.WriteFile(idFile, []byte("not-a-real-key\n"), 0o600))
		blob := []byte("fake-encrypted-blob")
		sum := sha256.Sum256(blob)
		shaHex := hex.EncodeToString(sum[:])
		fkey := model.BackupObjectKey("2026-09-19")
		fs := model.NewMemBackupStore()
		fs.Objects[fkey] = model.BackupObject{Key: fkey, Size: int64(len(blob)), SHA256: shaHex}
		fs.Data[fkey] = blob
		work := filepath.Join(tmpRoot, "work")
		must(os.MkdirAll(work, 0o700))
		var calls []string
		var cleaned bool
		mk := func(restoreErr error) model.RestoreDecryptFn {
			return model.NewAgeRestoreDecrypt(model.AgeRestoreConfig{
				IdentityFile: idFile, AdminDSN: "postgres://u:p@h/db", TmpRoot: work, Store: fs,
				RunAge: func(_ context.Context, id, in, out string) error {
					calls = append(calls, "age")
					b, _ := os.ReadFile(in)
					return os.WriteFile(out, append([]byte("PLAIN:"), b...), 0o600)
				},
				CreateDB:     func(_ context.Context, _, n string) error { calls = append(calls, "create:"+n); return nil },
				DropDB:       func(_ context.Context, _, n string) error { calls = append(calls, "drop:"+n); return nil },
				RunPgRestore: func(context.Context, string, string) error { calls = append(calls, "restore"); return restoreErr },
				Check: func(context.Context, string) (map[string]any, error) {
					return map[string]any{"select1": 1, "rows_users": 0}, nil
				},
				OnCleanup: func(string, string) { cleaned = true },
			})
		}
		fk := "2026-09-20"
		_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, fk)
		sent = nil
		r8, err := model.RunRestoreDrill(ctx, fk, true, &model.RestoreDrillDeps{Store: fs, Decrypt: mk(nil),
			Now: func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, loc) }})
		must(err)
		b8, _ := json.Marshal(r8.Detail)
		if r8.Status != model.OpsJobStatusOK || r8.Detail["restore"] != "ok" || r8.Detail["skeleton"] != false ||
			r8.Detail["download_sha256"] != shaHex || !cleaned || strings.Contains(string(b8), "not-a-real-key") {
			panic("(8) fake pipeline: " + string(b8))
		}
		if len(calls) != 4 || !strings.HasPrefix(calls[3], "drop:adty_restore_drill_") {
			panic(fmt.Sprintf("(8) calls %v", calls))
		}
		if ents, _ := os.ReadDir(work); len(ents) != 0 {
			panic("(8) tmp not cleaned")
		}
		fmt.Printf("(8) fake pipeline ok, drop+tmp cleanup, no key in detail OK %s\n", b8)
		// (8b) restore fails → still drop + cleanup, urgent
		calls, cleaned = nil, false
		_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, fk)
		r8b, err := model.RunRestoreDrill(ctx, fk, true, &model.RestoreDrillDeps{Store: fs, Decrypt: mk(errors.New("boom")),
			Now: func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, loc) }})
		must(err)
		if r8b.Status != model.OpsJobStatusFailed || !cleaned || len(calls) != 4 || !strings.HasPrefix(calls[3], "drop:") {
			panic(fmt.Sprintf("(8b) fail cleanup: %v %v", calls, r8b.Error))
		}
		if ents, _ := os.ReadDir(work); len(ents) != 0 {
			panic("(8b) tmp not cleaned")
		}
		fmt.Println("(8b) restore failure → drop temp db + tmp cleanup + failed OK")
		// (8c) download sha mismatch → fail before age
		calls = nil
		fs.Data[fkey] = []byte("tampered")
		_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, fk)
		r8c, _ := model.RunRestoreDrill(ctx, fk, true, &model.RestoreDrillDeps{Store: fs, Decrypt: mk(nil),
			Now: func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, loc) }})
		if r8c.Status != model.OpsJobStatusFailed || !strings.Contains(r8c.Error, "sha256 mismatch") || len(calls) != 0 {
			panic(fmt.Sprintf("(8c) %v %v", r8c.Error, calls))
		}
		fmt.Println("(8c) download sha mismatch → failed before decrypt OK")
		must(model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, fk))

		// (9) missing identity → not_configured
		if r, _ := model.NewAgeRestoreDecrypt(model.AgeRestoreConfig{IdentityFile: filepath.Join(tmpRoot, "nope")})(ctx, &model.BackupObject{}); r != nil {
			panic("(9)")
		}
		_, e9 := model.NewAgeRestoreDecrypt(model.AgeRestoreConfig{IdentityFile: filepath.Join(tmpRoot, "nope")})(ctx, &model.BackupObject{})
		if e9 == nil || e9.Error() != model.ErrRestoreDecryptNotConfigured {
			panic(fmt.Sprintf("(9) missing file: %v", e9))
		}
		_ = os.Setenv("ADTY_RESTORE_DRILL_IDENTITY_FILE", filepath.Join(tmpRoot, "nope"))
		_, e9b := model.RestoreDecryptFromEnv()(ctx, &model.BackupObject{})
		_ = os.Unsetenv("ADTY_RESTORE_DRILL_IDENTITY_FILE")
		_, e9c := model.RestoreDecryptFromEnv()(ctx, &model.BackupObject{})
		if e9b == nil || e9b.Error() != model.ErrRestoreDecryptNotConfigured || e9c == nil || e9c.Error() != model.ErrRestoreDecryptNotConfigured {
			panic(fmt.Sprintf("(9) env: %v %v", e9b, e9c))
		}
		fmt.Println("(9) missing/empty identity → restore_decrypt_not_configured OK")
	}

	// (10) real chain: pg_dump small source db → age -r → Mem store → age -d -i → pg_restore temp db → check → drop
	acceptRealAgeChain(ctx, loc, &sent)

	// (7) hook registered
	controller.RegisterScheduledSystemTasks()
	h, ok := service.LookupSystemTaskHandler(model.SystemTaskTypeRestoreDrill)
	if !ok || h == nil {
		panic("(6) restore_drill handler missing")
	}
	fmt.Println("(6) restore_drill system task registered OK")

	// cleanup — 不污染生产 last runs
	must(model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, runKey))
	must(model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, failKey))
	must(model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, missKey))
	fmt.Println("(7) cleanup fake restore_drill rows OK")
	fmt.Println("ACCEPT_RESTORE_DRILL_PASS")
}

func execSQL(dsn, q string) {
	db, err := sql.Open("pgx", dsn)
	must(err)
	defer db.Close()
	_, err = db.Exec(q)
	must(err)
}

func countTempDBs(dsn, prefix string) int {
	db, err := sql.Open("pgx", dsn)
	must(err)
	defer db.Close()
	var n int
	must(db.QueryRow(`SELECT count(*) FROM pg_database WHERE datname LIKE $1`, prefix+"%").Scan(&n))
	return n
}

// acceptRealAgeChain 有 age 才跑；identity 优先 ADTY_ACCEPT_AGE_IDENTITY_FILE（运维测试钥路径），否则临时 age-keygen。
// 私钥文件内容不读入本进程；公钥用 age-keygen -y 推出。
func acceptRealAgeChain(ctx context.Context, loc *time.Location, sent *[]string) {
	if _, err := exec.LookPath("age"); err != nil {
		fmt.Println("(10) SKIP real age chain: age not on PATH")
		return
	}
	adminDSN := os.Getenv("SQL_DSN")
	tmpRoot, _ := os.MkdirTemp("", "adty-accept-age-")
	defer os.RemoveAll(tmpRoot)
	idFile := os.Getenv("ADTY_ACCEPT_AGE_IDENTITY_FILE")
	src := "ops test key"
	if idFile == "" {
		idFile = filepath.Join(tmpRoot, "id.txt")
		must(exec.Command("age-keygen", "-o", idFile).Run())
		src = "temp age-keygen"
	}
	pub, err := exec.Command("age-keygen", "-y", idFile).Output()
	must(err)
	rcpt := strings.TrimSpace(string(pub))

	// 源库：小 users 表
	srcDB := fmt.Sprintf("adty_accept_rd_src_%d", os.Getpid())
	execSQL(adminDSN, `DROP DATABASE IF EXISTS "`+srcDB+`"`)
	execSQL(adminDSN, `CREATE DATABASE "`+srcDB+`"`)
	defer execSQL(adminDSN, `DROP DATABASE IF EXISTS "`+srcDB+`" WITH (FORCE)`)
	srcDSN, err := model.DSNWithDatabase(adminDSN, srcDB)
	must(err)
	execSQL(srcDSN, `CREATE TABLE users(id int primary key, quota bigint); INSERT INTO users VALUES (1,100),(2,250);`)

	enc := filepath.Join(tmpRoot, "b.dump.age")
	sh := exec.Command("bash", "-c", `set -o pipefail; pg_dump -Fc --no-owner -d "$SRC" | age -r "$RCPT" -o "$OUT"`)
	sh.Env = append(os.Environ(), "SRC="+srcDSN, "RCPT="+rcpt, "OUT="+enc)
	if out, err := sh.CombinedOutput(); err != nil {
		panic("(10) dump|age: " + err.Error() + " " + strings.ReplaceAll(string(out), srcDSN, "***"))
	}
	b, _ := os.ReadFile(enc)
	sum := sha256.Sum256(b)
	key := model.BackupObjectKey("2026-09-12")
	st := model.NewMemBackupStore()
	must(st.Put(ctx, key, enc, int64(len(b)), hex.EncodeToString(sum[:])))

	work := filepath.Join(tmpRoot, "work")
	must(os.MkdirAll(work, 0o700))
	prefix := "adty_accept_rdtmp_"
	runKey := "2026-09-13"
	run := func(id string) *model.RestoreDrillResult {
		_ = model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, runKey)
		r, err := model.RunRestoreDrill(ctx, runKey, true, &model.RestoreDrillDeps{Store: st,
			Decrypt: model.NewAgeRestoreDecrypt(model.AgeRestoreConfig{IdentityFile: id, AdminDSN: adminDSN,
				DBPrefix: prefix, TmpRoot: work, Store: st}),
			Now: func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, loc) }})
		must(err)
		return r
	}
	*sent = nil
	r := run(idFile)
	bd, _ := json.Marshal(r.Detail)
	if r.Status != model.OpsJobStatusOK || r.Detail["check_rows_users"] != int64(2) || r.Detail["check_users_quota_sum"] != int64(350) {
		panic("(10) real chain: " + r.Error + " " + string(bd))
	}
	if strings.Contains(strings.ToUpper(string(bd)), "AGE-SECRET-KEY") {
		panic("(10) key material in detail")
	}
	if ents, _ := os.ReadDir(work); len(ents) != 0 || countTempDBs(adminDSN, prefix) != 0 {
		panic("(10) leftover tmp/db")
	}
	fmt.Printf("(10) real age chain (%s) ok: %s; temp db dropped, tmp cleaned OK\n", src, bd)

	// (10b) wrong identity → failed, no leftover, no key in error/alert
	wrong := filepath.Join(tmpRoot, "wrong.txt")
	must(exec.Command("age-keygen", "-o", wrong).Run())
	rw := run(wrong)
	all := rw.Error + strings.Join(*sent, "|")
	row, _ := model.GetOpsJobRun(model.OpsJobRestoreDrill, runKey)
	if row != nil {
		all += row.Detail
	}
	if rw.Status != model.OpsJobStatusFailed || !strings.Contains(rw.Error, "age decrypt") || strings.Contains(strings.ToUpper(all), "AGE-SECRET-KEY") {
		panic("(10b) wrong key: " + rw.Error)
	}
	if ents, _ := os.ReadDir(work); len(ents) != 0 || countTempDBs(adminDSN, prefix) != 0 {
		panic("(10b) leftover tmp/db")
	}
	fmt.Println("(10b) wrong identity → failed, no tmp/db leftover, no key in error/alert/detail OK")

	// (10c) corrupt plaintext (valid age, not a dump) → pg_restore fails after CREATE DATABASE → db dropped
	bad := filepath.Join(tmpRoot, "bad.age")
	c := exec.Command("bash", "-c", `echo notadump | age -r "$RCPT" -o "$OUT"`)
	c.Env = append(os.Environ(), "RCPT="+rcpt, "OUT="+bad)
	must(c.Run())
	bb, _ := os.ReadFile(bad)
	s2 := sha256.Sum256(bb)
	must(st.Put(ctx, key, bad, int64(len(bb)), hex.EncodeToString(s2[:])))
	rc := run(idFile)
	if rc.Status != model.OpsJobStatusFailed || !strings.Contains(rc.Error, "pg_restore") || strings.Contains(rc.Error, adminDSN) {
		panic("(10c) corrupt dump: " + rc.Error)
	}
	if ents, _ := os.ReadDir(work); len(ents) != 0 || countTempDBs(adminDSN, prefix) != 0 {
		panic("(10c) leftover tmp/db")
	}
	fmt.Println("(10c) pg_restore failure after CREATE DATABASE → temp db dropped, plaintext removed OK")
	must(model.DeleteOpsJobRunForTest(model.OpsJobRestoreDrill, runKey))
}
