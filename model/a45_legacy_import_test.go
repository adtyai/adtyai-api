package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sampleA45JSON() []byte {
	return []byte(`{
  "_comment": "placeholder",
  "TopupOrderTimeoutMinutes": 60,
  "AffTopupCommissionRate": 0.10,
  "AffSubscriptionCommissionRate": 0.10,
  "AffMinWithdrawAmount": 10,
  "AffWithdrawFeeRate": 0,
  "BalanceLowAlertThreshold": 0
}`)
}

func TestParseA45LegacyJSON_SampleOK(t *testing.T) {
	vals, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)
	require.Equal(t, "60", vals["TopupOrderTimeoutMinutes"])
	require.Equal(t, "0.1", vals["AffTopupCommissionRate"])
	require.Equal(t, "0.1", vals["AffSubscriptionCommissionRate"])
	require.Equal(t, "10", vals["AffMinWithdrawAmount"])
	require.Equal(t, "0", vals["AffWithdrawFeeRate"])
	require.Equal(t, "0", vals["BalanceLowAlertThreshold"])
	require.NoError(t, ValidateA45LegacyValues(vals))
}

func TestParseA45LegacyFile_RepoSample(t *testing.T) {
	// Resolve sample relative to this test file → ../../scripts from model/
	// When tests run from module root, use scripts/ path.
	candidates := []string{
		filepath.Join("scripts", "a45_legacy_defaults.sample.json"),
		filepath.Join("..", "scripts", "a45_legacy_defaults.sample.json"),
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Skip("sample json not found from test cwd")
	}
	vals, err := ParseA45LegacyFile(path)
	require.NoError(t, err)
	require.Len(t, vals, 6)
	require.NoError(t, ValidateA45LegacyValues(vals))
}

func TestValidateA45LegacyValues_OutOfRange(t *testing.T) {
	vals, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)
	vals["AffTopupCommissionRate"] = "0.9" // max 0.5
	err = ValidateA45LegacyValues(vals)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBusinessParamRange)

	vals2, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)
	vals2["TopupOrderTimeoutMinutes"] = "3" // min 5
	err = ValidateA45LegacyValues(vals2)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBusinessParamRange)
}

func TestParseA45LegacyJSON_MissingKey(t *testing.T) {
	_, err := ParseA45LegacyJSON([]byte(`{"TopupOrderTimeoutMinutes": 60}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
}

func TestParseA45LegacyJSON_UnexpectedKey(t *testing.T) {
	raw := []byte(`{
  "TopupOrderTimeoutMinutes": 60,
  "AffTopupCommissionRate": 0.1,
  "AffSubscriptionCommissionRate": 0.1,
  "AffMinWithdrawAmount": 10,
  "AffWithdrawFeeRate": 0,
  "BalanceLowAlertThreshold": 0,
  "AffFreezeDays": 7
}`)
	_, err := ParseA45LegacyJSON(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected")
}

func TestPlanA45LegacyImport_DryRunNoWrite(t *testing.T) {
	vals, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)
	calls := 0
	current := func(k string) string {
		calls++
		return "old-" + k
	}
	changes, err := PlanA45LegacyImport(vals, current)
	require.NoError(t, err)
	require.Len(t, changes, 6)
	require.Equal(t, 6, calls)
	plan := FormatA45LegacyPlan(changes, A45PlanModeDryRun)
	require.Contains(t, plan, "dry-run")
	require.Contains(t, plan, "TopupOrderTimeoutMinutes")
	require.Contains(t, plan, "old-TopupOrderTimeoutMinutes")
	// PlanA45LegacyImport is pure: no ApplyBusinessParamUpdates / no backup write.
}

func setupA45SQLite(t *testing.T) {
	t.Helper()
	prevRedis := common.RedisEnabled
	prevBatch := common.BatchUpdateEnabled
	common.OptionMapRWMutex.Lock()
	var prevOptions map[string]string
	if common.OptionMap != nil {
		prevOptions = make(map[string]string, len(common.OptionMap))
		for k, v := range common.OptionMap {
			prevOptions[k] = v
		}
	}
	prevOptionsNil := common.OptionMap == nil
	common.OptionMapRWMutex.Unlock()
	bpMu.Lock()
	prevTimeout := bpTopupOrderTimeoutMinutes
	prevTopupRate := bpAffTopupCommissionRate
	prevSubRate := bpAffSubscriptionCommissionRate
	prevMin := bpAffMinWithdrawAmount
	prevFee := bpAffWithdrawFeeRate
	prevAlert := bpBalanceLowAlertThreshold
	bpMu.Unlock()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "a45.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB, previousLogDB := DB, LOG_DB
	DB = db
	LOG_DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	for _, d := range BusinessParamCatalog() {
		common.OptionMap[d.Key] = d.Default
	}
	// Seed distinct "old" values for the six keys so apply changes something.
	olds := map[string]string{
		"TopupOrderTimeoutMinutes":      "30",
		"AffTopupCommissionRate":        "0.05",
		"AffSubscriptionCommissionRate": "0.05",
		"AffMinWithdrawAmount":          "5",
		"AffWithdrawFeeRate":            "0.01",
		"BalanceLowAlertThreshold":      "1",
	}
	for k, v := range olds {
		common.OptionMap[k] = v
		require.NoError(t, db.Save(&Option{Key: k, Value: v}).Error)
		require.NoError(t, applyBusinessParamRuntime(k, v))
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.RedisEnabled = prevRedis
		common.BatchUpdateEnabled = prevBatch
		common.OptionMapRWMutex.Lock()
		if prevOptionsNil {
			common.OptionMap = nil
		} else {
			common.OptionMap = prevOptions
		}
		common.OptionMapRWMutex.Unlock()
		bpMu.Lock()
		bpTopupOrderTimeoutMinutes = prevTimeout
		bpAffTopupCommissionRate = prevTopupRate
		bpAffSubscriptionCommissionRate = prevSubRate
		bpAffMinWithdrawAmount = prevMin
		bpAffWithdrawFeeRate = prevFee
		bpBalanceLowAlertThreshold = prevAlert
		bpMu.Unlock()
	})
}

func TestSetupA45SQLiteRestoresWithdrawMinimum(t *testing.T) {
	require.NoError(t, applyBusinessParamRuntime("AffMinWithdrawAmount", "0"))
	t.Run("setup", func(t *testing.T) {
		setupA45SQLite(t)
		require.Equal(t, 5.0, GetAffMinWithdrawAmount())
	})
	require.Equal(t, 0.0, GetAffMinWithdrawAmount())
}

func TestApplyAndRollbackA45Legacy_SQLite(t *testing.T) {
	setupA45SQLite(t)
	vals, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)

	backup := filepath.Join(t.TempDir(), "a45.rollback.json")

	// Before apply
	require.Equal(t, "30", BusinessParamCurrentValue("TopupOrderTimeoutMinutes"))

	changes, err := ApplyA45LegacyImport(vals, backup)
	require.NoError(t, err)
	require.Len(t, changes, 6)
	require.Equal(t, "30", changes[0].Old)
	require.Equal(t, "60", BusinessParamCurrentValue("TopupOrderTimeoutMinutes"))
	require.Equal(t, "0.1", BusinessParamCurrentValue("AffTopupCommissionRate"))
	require.Equal(t, 60, GetTopupOrderTimeoutMinutes())

	var opt Option
	require.NoError(t, DB.Where("`key` = ?", "TopupOrderTimeoutMinutes").First(&opt).Error)
	require.Equal(t, "60", opt.Value)

	// Backup should hold pre-apply values
	bakVals, err := ReadA45LegacyBackup(backup)
	require.NoError(t, err)
	require.Equal(t, "30", bakVals["TopupOrderTimeoutMinutes"])
	require.Equal(t, "0.05", bakVals["AffTopupCommissionRate"])

	rbChanges, err := RollbackA45LegacyImport(backup)
	require.NoError(t, err)
	require.Len(t, rbChanges, 6)
	require.Equal(t, "30", BusinessParamCurrentValue("TopupOrderTimeoutMinutes"))
	require.Equal(t, "0.05", BusinessParamCurrentValue("AffTopupCommissionRate"))
	require.Equal(t, 30, GetTopupOrderTimeoutMinutes())
}

func TestApplyA45Legacy_RequiresDB(t *testing.T) {
	previousDB := DB
	DB = nil
	t.Cleanup(func() { DB = previousDB })
	vals, err := ParseA45LegacyJSON(sampleA45JSON())
	require.NoError(t, err)
	_, err = ApplyA45LegacyImport(vals, filepath.Join(t.TempDir(), "b.json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "DB not initialized")
}

func TestFormatA45LegacyPlan_ModeWording(t *testing.T) {
	changes := []A45LegacyChange{{Key: "TopupOrderTimeoutMinutes", Old: "30", New: "15"}}
	applied := FormatA45LegacyPlan(changes, A45PlanModeApply)
	require.NotContains(t, applied, "dry-run")
	require.NotContains(t, applied, "no DB write")
	require.Contains(t, applied, "applied")
	rb := FormatA45LegacyPlan(changes, A45PlanModeRollback)
	require.NotContains(t, rb, "dry-run")
	require.Contains(t, rb, "rolled back")
	require.Contains(t, FormatA45LegacyPlan(changes, A45PlanModeDryRun), "dry-run")
}
