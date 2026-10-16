package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// B12 每日备份：pg_dump 全量 → age 公钥加密 → R2 → 校验 → 保留期清理。
// 服务端只持有 age 公钥（recipient），从不需要私钥；恢复/演练由运维持私钥离线做。

const (
	BackupDailyPrefix = "daily/"
	backupKindFailed  = "backup_failed"
)

// BackupObject 远端对象元信息。
type BackupObject struct {
	Key    string
	Size   int64
	SHA256 string // 上传时写入的 x-amz-meta-sha256；List 可为空
}

// BackupStore 远端存储（R2 / 验收用内存实现）。
type BackupStore interface {
	Put(ctx context.Context, key, localPath string, size int64, sha256Hex string) error
	Head(ctx context.Context, key string) (*BackupObject, error)
	List(ctx context.Context, prefix string) ([]BackupObject, error)
	Delete(ctx context.Context, key string) error
	// Get 把对象内容写到 w（恢复演练下载密文用）。
	Get(ctx context.Context, key string, w io.Writer) error
}

// BackupDeps 可注入依赖；nil 字段用生产默认（pg_dump / age CLI / R2）。
type BackupDeps struct {
	Dump    func(ctx context.Context, w io.Writer) error
	Encrypt func(ctx context.Context, r io.Reader, w io.Writer) error
	Store   BackupStore
	Now     func() time.Time
}

// OpsTelegramSender 由 controller 注册为 service.AdtySendTelegramAlert（避免 model→service 循环引用）。
// nil 时退回 RecordOpsAlert 落库。
var OpsTelegramSender func(level, kind, text string) error

func pushOpsAlert(level, kind, text string, payload map[string]any) {
	if OpsTelegramSender != nil {
		if err := OpsTelegramSender(level, kind, text); err == nil {
			return
		} else {
			common.SysError(common.LogText("ops alert %s send failed: %v", kind, err))
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["text"] = text
	_ = RecordOpsAlert(level, kind, payload)
}

func backupEnv(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func defaultBackupDump(ctx context.Context, w io.Writer) error {
	dsn := backupEnv("ADTY_BACKUP_PG_DSN")
	if dsn == "" {
		dsn = backupEnv("SQL_DSN")
	}
	if dsn == "" {
		return errors.New("pg dsn not configured")
	}
	cmd := exec.CommandContext(ctx, "pg_dump", "-Fc", "--no-owner", "-d", dsn)
	cmd.Stdout = w
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// 不回带 DSN；stderr 截断
		msg := stderr.String()
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("pg_dump failed: %v %s", err, strings.ReplaceAll(msg, dsn, "***"))
	}
	return nil
}

func defaultBackupEncrypt(ctx context.Context, r io.Reader, w io.Writer) error {
	rcpt := backupEnv("ADTY_BACKUP_AGE_RECIPIENT")
	if rcpt == "" {
		return errors.New("age recipient not configured")
	}
	if !strings.HasPrefix(rcpt, "age1") {
		return errors.New("age recipient must be a public key (age1...)")
	}
	cmd := exec.CommandContext(ctx, "age", "-r", rcpt)
	cmd.Stdin = r
	cmd.Stdout = w
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age encrypt failed: %v %s", err, stderr.String())
	}
	return nil
}

func (d *BackupDeps) fill() error {
	if d.Dump == nil {
		d.Dump = defaultBackupDump
	}
	if d.Encrypt == nil {
		d.Encrypt = defaultBackupEncrypt
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Store == nil {
		s, err := NewR2BackupStoreFromEnv()
		if err != nil {
			return err
		}
		d.Store = s
	}
	return nil
}

// BackupObjectKey daily/YYYY-MM-DD/adtyai-YYYY-MM-DD.dump.age
func BackupObjectKey(runKey string) string {
	return BackupDailyPrefix + runKey + "/adtyai-" + runKey + ".dump.age"
}

// IsBackupRetentionExempt 升级前手工备份（pre-v*/ pre/ 任意段以 pre 开头）永不自动删。
func IsBackupRetentionExempt(key string) bool {
	if !strings.HasPrefix(key, BackupDailyPrefix) {
		return true
	}
	for _, seg := range strings.Split(key, "/") {
		if strings.HasPrefix(strings.ToLower(seg), "pre") {
			return true
		}
	}
	return false
}

// BackupResult 返回给 system task。
type BackupResult struct {
	RunKey        string   `json:"run_key"`
	Skipped       bool     `json:"skipped"`
	Status        string   `json:"status"`
	ObjectKey     string   `json:"object_key,omitempty"`
	Size          int64    `json:"size,omitempty"`
	SHA256        string   `json:"sha256,omitempty"`
	RetentionDays int      `json:"retention_days"`
	Deleted       []string `json:"deleted"`
	Exempt        []string `json:"exempt"`
	Error         string   `json:"error,omitempty"`
}

// RunDailyBackup 执行一次备份。runKey 空=今天（上海）。
func RunDailyBackup(ctx context.Context, runKey string, force bool, deps *BackupDeps) (*BackupResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps == nil {
		deps = &BackupDeps{}
	}
	d := *deps
	if d.Now == nil {
		d.Now = time.Now
	}
	if runKey == "" {
		runKey = d.Now().In(ShanghaiLocation()).Format("2006-01-02")
	}
	ret := GetBackupRetentionDays()
	out := &BackupResult{RunKey: runKey, RetentionDays: ret, Deleted: []string{}, Exempt: []string{}}

	row, skip, lock, err := beginOpsJobRun(ctx, OpsJobBackup, runKey, force)
	if err != nil {
		return nil, err
	}
	if skip {
		out.Skipped = true
		out.Status = OpsJobStatusOK
		if row != nil {
			out.Status = row.Status
		}
		return out, nil
	}
	defer lock.Release()

	fail := func(stage string, e error) (*BackupResult, error) {
		out.Status = OpsJobStatusFailed
		out.Error = stage + ": " + e.Error()
		_ = finishOpsJobRun(row, OpsJobStatusFailed, map[string]any{
			"stage": stage, "error": out.Error, "object_key": out.ObjectKey,
		})
		pushOpsAlert(OpsAlertLevelUrgent, backupKindFailed,
			fmt.Sprintf("[adtyai] 备份失败 %s（%s）：%s", runKey, stage, e.Error()),
			map[string]any{"run_key": runKey, "stage": stage})
		return out, nil
	}

	if err := d.fill(); err != nil {
		return fail("config", err)
	}

	tmp, err := os.CreateTemp("", "adty-backup-*.dump.age")
	if err != nil {
		return fail("tmpfile", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	// dump | encrypt → tmp，同时算 sha256；明文不落盘
	pr, pw := io.Pipe()
	dumpErr := make(chan error, 1)
	go func() {
		e := d.Dump(ctx, pw)
		_ = pw.CloseWithError(e)
		dumpErr <- e
	}()
	h := sha256.New()
	encErr := d.Encrypt(ctx, pr, io.MultiWriter(tmp, h))
	_ = pr.CloseWithError(io.ErrClosedPipe)
	dErr := <-dumpErr
	_ = tmp.Close()
	if dErr != nil {
		return fail("dump", dErr)
	}
	if encErr != nil {
		return fail("encrypt", encErr)
	}
	st, err := os.Stat(tmpPath)
	if err != nil {
		return fail("encrypt", err)
	}
	if st.Size() == 0 {
		return fail("encrypt", errors.New("empty output"))
	}
	out.Size = st.Size()
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	out.ObjectKey = BackupObjectKey(runKey)
	touchOpsJobHeartbeat(row)

	if err := d.Store.Put(ctx, out.ObjectKey, tmpPath, out.Size, out.SHA256); err != nil {
		return fail("upload", err)
	}
	touchOpsJobHeartbeat(row)
	obj, err := d.Store.Head(ctx, out.ObjectKey)
	if err != nil {
		return fail("verify", err)
	}
	if obj.Size != out.Size || (obj.SHA256 != "" && obj.SHA256 != out.SHA256) || obj.SHA256 == "" {
		return fail("verify", fmt.Errorf("mismatch size %d/%d sha %q", obj.Size, out.Size, obj.SHA256))
	}

	// 保留期：只在 daily/ 下按日期段判断；pre* 豁免；清理失败不算备份失败，但发待处理告警
	today, _, _ := ParseBusinessDate(runKey)
	cutoff := today.AddDate(0, 0, -ret).Format("2006-01-02")
	var retErr error
	if objs, e := d.Store.List(ctx, BackupDailyPrefix); e != nil {
		retErr = e
	} else {
		for _, o := range objs {
			if IsBackupRetentionExempt(o.Key) {
				out.Exempt = append(out.Exempt, o.Key)
				continue
			}
			parts := strings.SplitN(strings.TrimPrefix(o.Key, BackupDailyPrefix), "/", 2)
			if _, e := time.Parse("2006-01-02", parts[0]); e != nil {
				out.Exempt = append(out.Exempt, o.Key) // 不认识的路径不碰
				continue
			}
			if parts[0] < cutoff {
				if e := d.Store.Delete(ctx, o.Key); e != nil {
					retErr = e
					continue
				}
				out.Deleted = append(out.Deleted, o.Key)
			}
		}
	}
	detail := map[string]any{
		"object_key":     out.ObjectKey,
		"size":           out.Size,
		"sha256":         out.SHA256,
		"retention_days": ret,
		"cutoff":         cutoff,
		"deleted":        out.Deleted,
		"exempt_count":   len(out.Exempt),
	}
	if retErr != nil {
		detail["retention_error"] = retErr.Error()
		pushOpsAlert(OpsAlertLevelPending, "backup_retention_failed",
			fmt.Sprintf("[adtyai] 备份 %s 已成功，旧备份清理失败：%s", runKey, retErr.Error()),
			map[string]any{"run_key": runKey})
	}
	if err := finishOpsJobRun(row, OpsJobStatusOK, detail); err != nil {
		return out, err
	}
	out.Status = OpsJobStatusOK
	return out, nil
}

// MemBackupStore 验收/测试用内存存储。
type MemBackupStore struct {
	mu      sync.Mutex
	Objects map[string]BackupObject
	Data    map[string][]byte // 可选：对象内容（Get 用）
	FailPut bool
}

func NewMemBackupStore() *MemBackupStore {
	return &MemBackupStore{Objects: map[string]BackupObject{}, Data: map[string][]byte{}}
}

func (m *MemBackupStore) Get(_ context.Context, key string, w io.Writer) error {
	m.mu.Lock()
	b, ok := m.Data[key]
	m.mu.Unlock()
	if !ok {
		return errors.New("not found")
	}
	_, err := w.Write(b)
	return err
}

func (m *MemBackupStore) Put(_ context.Context, key, localPath string, size int64, sha string) error {
	if m.FailPut {
		return errors.New("forced upload failure")
	}
	b, _ := os.ReadFile(localPath)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Objects[key] = BackupObject{Key: key, Size: size, SHA256: sha}
	if m.Data == nil {
		m.Data = map[string][]byte{}
	}
	if b != nil {
		m.Data[key] = b
	}
	return nil
}

func (m *MemBackupStore) Head(_ context.Context, key string) (*BackupObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.Objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &o, nil
}

func (m *MemBackupStore) List(_ context.Context, prefix string) ([]BackupObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []BackupObject
	for k, o := range m.Objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *MemBackupStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Objects, key)
	return nil
}
