package model

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// adtyExit 留给测试替换。生产环境直接退出，不能改用 FatalLog，否则测试挡不住。
var adtyExit = func(code int) {
	os.Exit(code)
}

// adtyDSNIsPostgreSQL 和 chooseDB 用同样的前缀判断。没设 DSN 时上游用 SQLite。
func adtyDSNIsPostgreSQL(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// CheckAdtyPostgreSQL 在连库之前检查 SQL_DSN 和 LOG_SQL_DSN：不是 PostgreSQL 就写明是哪一个并退出。
// LOG_SQL_DSN 为空时日志和主库同库，不单独检查。
func CheckAdtyPostgreSQL() {
	var problems []string
	if !adtyDSNIsPostgreSQL(os.Getenv("SQL_DSN")) {
		problems = append(problems, "SQL_DSN")
	}
	if logDSN := os.Getenv("LOG_SQL_DSN"); logDSN != "" && !adtyDSNIsPostgreSQL(logDSN) {
		problems = append(problems, "LOG_SQL_DSN")
	}
	if len(problems) == 0 {
		return
	}
	common.SysError("ADTY requires PostgreSQL; check " + strings.Join(problems, " and "))
	adtyExit(1)
}

// MigrateAdtyTables 建二开的表。只由主节点的 migrateDB 调用；上游测试在 SQLite 上也会走到这里，只用 AutoMigrate。
func MigrateAdtyTables() error {
	if err := DB.AutoMigrate(&AdtyGroup{}, &AdtyTopupOrder{}, &PaymentOrphanEvent{}, &AdtyBalanceLedger{}, &AdtyEpayChannel{}, &AdtyEpayChannelMethod{}, &AdtyUserFingerprint{}, &AdtyUserLimit{}, &AdtyOfficialPrice{}); err != nil {
		return err
	}
	if err := migrateAdtyMarketingTables(); err != nil {
		return err
	}
	if err := migrateAdtyReferralTables(); err != nil {
		return err
	}
	return migrateAdtySiteTables()
}

// migrateAdtySiteTables 建分站的表（P11）。
func migrateAdtySiteTables() error {
	return DB.AutoMigrate(AdtySiteTables()...)
}

// migrateAdtyReferralTables 建推广返佣的 12 张表（任务 6.1）。
func migrateAdtyReferralTables() error {
	return DB.AutoMigrate(ReferralTables()...)
}

// migrateAdtyTables 是挂到上游迁移上的同名入口，测试也直接调用它。
func migrateAdtyTables() error {
	return MigrateAdtyTables()
}

// SeedAdtyGroups 只在表里还没有任何行（含已软删除）时插入，要在设置载入之后调用。
// 已有名字不覆盖，设置里后加的键也不补。
func SeedAdtyGroups() error {
	var count int64
	if err := DB.Unscoped().Model(&AdtyGroup{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	groups, err := adtyGroupSeedSources()
	if err != nil {
		return err
	}
	if _, ok := groups[AdtyDefaultGroupID]; !ok {
		groups[AdtyDefaultGroupID] = ""
	}
	rows := make([]AdtyGroup, 0, len(groups))
	for id, desc := range groups {
		rows = append(rows, AdtyGroup{
			ID:    id,
			Names: adtyFillLangs(id),
			Descs: adtyFillLangs(desc),
		})
	}
	return DB.Create(&rows).Error
}

// ClearAdtyPlaceholderGroupRatio 去掉上游默认 GroupGroupRatio 里的 edit_this。
// 它不是分组，留着的话分组校验上线后这个配置一保存就会被拒。
func ClearAdtyPlaceholderGroupRatio() error {
	nested := map[string]map[string]float64{}
	if err := adtyUnmarshalObject(ratio_setting.GroupGroupRatio2JSONString(), &nested); err != nil {
		return err
	}
	changed := false
	for outer, inner := range nested {
		if _, ok := inner["edit_this"]; !ok {
			continue
		}
		delete(inner, "edit_this")
		changed = true
		if len(inner) == 0 {
			delete(nested, outer)
		}
	}
	if !changed {
		return nil
	}
	raw, err := common.Marshal(nested)
	if err != nil {
		return err
	}
	return UpdateOption("GroupGroupRatio", string(raw))
}

func adtyUnmarshalObject(raw string, dest any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return common.Unmarshal([]byte(raw), dest)
}

func adtyGroupSeedSources() (map[string]string, error) {
	groups := map[string]string{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if !adtySeedableGroupID(id) {
			return
		}
		if _, ok := groups[id]; !ok {
			groups[id] = ""
		}
	}
	for id := range ratio_setting.GetGroupRatioCopy() {
		add(id)
	}
	for id, desc := range setting.GetUserUsableGroupsCopy() {
		add(id)
		if !adtySeedableGroupID(id) {
			continue
		}
		desc = strings.TrimSpace(desc)
		if desc != "" {
			groups[id] = desc
		}
	}
	topup := map[string]float64{}
	if err := adtyUnmarshalObject(common.TopupGroupRatio2JSONString(), &topup); err != nil {
		return nil, err
	}
	for id := range topup {
		add(id)
	}
	for _, id := range setting.GetAutoGroups() {
		add(id)
	}
	nested := map[string]map[string]float64{}
	if err := adtyUnmarshalObject(ratio_setting.GroupGroupRatio2JSONString(), &nested); err != nil {
		return nil, err
	}
	for outer, inner := range nested {
		add(outer)
		for id := range inner {
			add(id)
		}
	}
	special := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	if special != nil {
		for owner, inner := range special.ReadAll() {
			add(owner)
			for key := range inner {
				add(adtyStripGroupPrefix(key))
			}
		}
	}
	limits := map[string][2]int{}
	if err := adtyUnmarshalObject(setting.ModelRequestRateLimitGroup2JSONString(), &limits); err != nil {
		return nil, err
	}
	for id := range limits {
		add(id)
	}
	return groups, nil
}
