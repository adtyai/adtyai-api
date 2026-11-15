package service

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/logsinktest"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0048：SQLite 夹具把业务库 `DB` 和日志库 `LOG_DB` 指向同一个连接。
//
// 机制（不是猜测，是这条夹具的必然结果）：DSN 带 `_txlock=immediate`，业务事务
// 一 BEGIN 就持有单写者模型的写锁；`recordSiteAuditTx` 随后经 `LOG_DB` 的第二条
// 连接写 `audit_logs`，被自己的锁挡住，等满 busy_timeout 后失败。
//
// 失败本身是设计允许的——审计是旁路，`WriteOfficialAuditLog` 出错不得回滚业务。
// 缺陷在于它**被静默吞掉**：`audit_logs` 没有行，日志里也没有任何痕迹，事后
// 无法判断「这次站点操作到底有没有被审计到」。所以判据是行为，不是赋值处数：
//
//	一次站点操作之后，`audit_logs` 有行 **或** 有一条可定位的 ERROR；
//	两者都没有，就是缺陷。

// errorLines 取出带 marker 的 ERROR 行。`logsinktest` 的行首是级别前缀，
// 所以 SysLog 打的 LOG 行不会被误算成「看得见的失败」。
func errorLines(buffer *logsinktest.Buffer, marker string) []string {
	var out []string
	for _, line := range strings.Split(buffer.String(), "\n") {
		if strings.HasPrefix(line, "ERROR ") && strings.Contains(line, marker) {
			out = append(out, line)
		}
	}
	return out
}

type siteAuditFixture struct {
	SiteID  int
	AdminID int
}

// setupSiteAuditFixture 起一个和 `service/referral_test.go` 同形的单文件 SQLite
// 夹具：WAL + BEGIN IMMEDIATE + 4 条连接，只有 busy_timeout 从 10s 缩到 300ms
// （等多久不改变机制，只决定这条用例跑几秒）。
//
// logTarget:
//   - "same-file"：`LOG_DB` 和 `DB` 是同一个连接——缺陷现场，日志写入必被自锁挡住；
//   - "no-schema"：`LOG_DB` 是另一个没有 `audit_logs` 的空库——反向用例，
//     人为让日志写入必然失败。
func setupSiteAuditFixture(t *testing.T, logTarget string) *siteAuditFixture {
	t.Helper()
	dir := t.TempDir()
	open := func(name string) *gorm.DB {
		dsn := filepath.Join(dir, name) + "?_pragma=busy_timeout(300)&_pragma=journal_mode(WAL)&_txlock=immediate"
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(4)
		sqlDB.SetMaxIdleConns(4)
		t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
		return db
	}
	business := open("business.db")
	logDB := business
	if logTarget == "no-schema" {
		logDB = open("log.db")
	}

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = business, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitColumnNamesForTests()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitColumnNamesForTests()
		InvalidateAdtySiteCache()
	})

	require.NoError(t, business.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteAudit{}))
	if logTarget == "same-file" {
		// 审计表建在业务文件里：同一张表、同一个文件、同一把写锁。
		require.NoError(t, model.MigrateAuditLogs())
	}

	saved := *adty_setting.GetResellerSetting()
	t.Cleanup(func() { *adty_setting.GetResellerSetting() = saved })
	cfg := adty_setting.GetResellerSetting()
	*cfg = adty_setting.DefaultResellerSetting()
	cfg.Enabled = true
	cfg.MaxMarkup = new(3.0)

	admin := &model.User{
		Username: "site-audit-admin", Password: "hashed-password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, business.Create(admin).Error)
	site := &model.AdtySite{
		OwnerUserId:  admin.Id,
		Subdomain:    "audit-visible",
		RootDomain:   "sites.example.com",
		Status:       model.AdtySiteStatusActive,
		OpenedBy:     model.AdtySiteOpenedByAdmin,
		ContactInfo:  "owner@example.com",
		AgreementVer: "1",
	}
	require.NoError(t, business.Create(site).Error)
	return &siteAuditFixture{SiteID: site.Id, AdminID: admin.Id}
}

func auditRowsFor(t *testing.T, db *gorm.DB, siteID int) int64 {
	t.Helper()
	var rows int64
	require.NoError(t, db.Table("audit_logs").
		Where("site_id = ? AND action = ?", siteID, "adty.site.adjust").Count(&rows).Error)
	return rows
}

// 验收 1：SQLite 夹具上做一次真实的站点操作，`audit_logs` 有行或有一条可定位的
// ERROR —— 二者不能都没有。
func TestSiteOperationAuditIsPersistedOrLoggedSQLite(t *testing.T) {
	fx := setupSiteAuditFixture(t, "same-file")
	buffer := logsinktest.Capture(t)

	updated, err := AdjustAdtySite(fx.SiteID, fx.AdminID, AdtySiteAdjust{
		MaxMarkup: dec("2.00"),
		Reason:    "align markup with tier",
	})
	require.NoError(t, err, "审计写失败不得回滚业务")
	require.NotNil(t, updated)

	// 业务本身必须落库：审计是旁路，它挂了不该把这次调整也带走。
	var stored model.AdtySite
	require.NoError(t, model.DB.Where("id = ?", fx.SiteID).Take(&stored).Error)
	require.NotNil(t, stored.MaxMarkup)
	assert.True(t, dec("2.00").Equal(*stored.MaxMarkup), "业务改动必须提交")

	rows := auditRowsFor(t, model.LOG_DB, fx.SiteID)
	errLines := errorLines(buffer, "official site audit write failed")
	if rows == 0 && len(errLines) == 0 {
		t.Fatalf("审计行没写进 audit_logs，也没有一条可定位的 ERROR：这次站点操作是否被审计完全无从判断")
	}
	if rows == 0 {
		// 这条夹具的必然分支：业务事务持着写锁，日志库的第二条连接等满
		// busy_timeout 后失败。失败可以被接受，静默不行。
		require.Len(t, errLines, 1)
		t.Logf("audit_logs 无对应行（日志库与业务库同一把写锁），ERROR 可见：%s", errLines[0])
		assertLocatableSiteAuditError(t, errLines[0], fx)
	} else {
		t.Logf("audit_logs 有 %d 行，日志写入没有被自锁挡住", rows)
	}
}

// 验收 2（反向）：把日志写入人为改成必然失败（LOG_DB 指向没有 audit_logs 的库），
// 必须能观察到一条带 site_id / operator_id / action / object_id 的 ERROR，
// 且业务照常提交。
func TestSiteOperationAuditFailureIsObservableSQLite(t *testing.T) {
	fx := setupSiteAuditFixture(t, "no-schema")
	buffer := logsinktest.Capture(t)

	updated, err := AdjustAdtySite(fx.SiteID, fx.AdminID, AdtySiteAdjust{
		MaxMarkup: dec("2.50"),
		Reason:    "reverse check",
	})
	require.NoError(t, err, "审计写失败不得回滚业务")
	require.NotNil(t, updated)
	assert.True(t, dec("2.50").Equal(*updated.MaxMarkup))

	assert.False(t, model.LOG_DB.Migrator().HasTable("audit_logs"),
		"反向用例的夹具里没有 audit_logs 表，日志写入必然失败")

	errLines := errorLines(buffer, "official site audit write failed")
	require.Len(t, errLines, 1, "日志写入失败必须留下一条 ERROR，而不是被吞掉")
	assertLocatableSiteAuditError(t, errLines[0], fx)
}

func assertLocatableSiteAuditError(t *testing.T, line string, fx *siteAuditFixture) {
	t.Helper()
	assert.Contains(t, line, "site_id="+strconv.Itoa(fx.SiteID), "ERROR 必须能定位到站点")
	assert.Contains(t, line, "operator_id="+strconv.Itoa(fx.AdminID), "ERROR 必须能定位到操作者")
	assert.Contains(t, line, "action=adty.site.adjust", "ERROR 必须能定位到动作")
	assert.Contains(t, line, "object_id="+strconv.Itoa(fx.SiteID), "ERROR 必须能定位到对象")
	assert.Contains(t, line, "error=", "ERROR 必须带上底层错误")
}
