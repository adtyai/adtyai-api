/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSiteSecretProcessRestart(t *testing.T) {
	const tokenKey = "adty_reseller_setting.cloudflare_apiToken"
	const widgetKey = "adty_reseller_setting.turnstile_widget_secret"
	const cryptoKey = "synthetic-encryption-key-for-local-tests-only"
	if path := os.Getenv("ADTY_SECRET_RESTART_PATH"); path != "" {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		defer sqlDB.Close()
		DB = db
		common.OptionMap = map[string]string{}
		require.NoError(t, db.AutoMigrate(&Option{}))
		if os.Getenv("ADTY_SECRET_RESTART_MODE") == "write" {
			require.NoError(t, UpdateOptionsBulk(map[string]string{tokenKey: "restart-token", widgetKey: `{"widget":"restart-widget"}`}))
			return
		}
		cfg := adty_setting.GetResellerSetting()
		cfg.CloudflareApiToken = "stale-token"
		cfg.TurnstileWidgetSecret = map[string]string{"widget": "stale-widget"}
		loadOptionsFromDatabase()
		if os.Getenv("CRYPTO_SECRET") == cryptoKey {
			require.Equal(t, "restart-token", cfg.CloudflareApiToken)
			require.Equal(t, map[string]string{"widget": "restart-widget"}, cfg.TurnstileWidgetSecret)
		} else {
			require.Empty(t, cfg.CloudflareApiToken)
			require.Empty(t, cfg.TurnstileWidgetSecret)
		}
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "options.db")
	for i, key := range []string{cryptoKey, cryptoKey, cryptoKey + "-wrong", "", cryptoKey} {
		mode := "read"
		if i == 0 {
			mode = "write"
		}
		cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestSiteSecretProcessRestart$", "-test.v")
		cmd.Env = append(os.Environ(), "ADTY_SECRET_RESTART_PATH="+path, "ADTY_SECRET_RESTART_MODE="+mode, "CRYPTO_SECRET="+key)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "fresh process %d: %s", i, output)
		require.NotContains(t, string(output), "restart-token")
		require.NotContains(t, string(output), "restart-widget")
	}
}

func TestSiteSecretOptionsDatabaseMatrix(t *testing.T) {
	const tokenKey = "adty_reseller_setting.cloudflare_apiToken"
	const widgetKey = "adty_reseller_setting.turnstile_widget_secret"
	const secret = "synthetic-cloudflare-token"
	const cryptoKey = "synthetic-encryption-key-for-local-tests-only"
	t.Setenv("CRYPTO_SECRET", cryptoKey)
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			require.NoError(t, db.AutoMigrate(&Option{}))
			cfg := adty_setting.GetResellerSetting()
			saved, oldOptions := *cfg, common.OptionMap
			oldRegister := common.RegisterEnabled
			common.OptionMap = map[string]string{}
			t.Cleanup(func() { *cfg, common.OptionMap, common.RegisterEnabled = saved, oldOptions, oldRegister })
			var sqlLog bytes.Buffer
			db = db.Session(&gorm.Session{Logger: logger.New(log.New(&sqlLog, "", 0), logger.Config{SlowThreshold: time.Second, LogLevel: logger.Info})})
			DB = db
			stored := func(key string) string {
				t.Helper()
				var row Option
				require.NoError(t, db.Where(&Option{Key: key}).First(&row).Error)
				return row.Value
			}

			require.NoError(t, UpdateOption(tokenKey, secret))
			first := stored(tokenKey)
			require.True(t, strings.HasPrefix(first, "adty:secret:v1:"), "the durable token must be encrypted")
			assert.NotContains(t, first, secret)
			assert.Equal(t, secret, cfg.CloudflareApiToken)
			require.NoError(t, UpdateOption(tokenKey, secret))
			assert.NotEqual(t, first, stored(tokenKey), "each encryption must use a new random nonce")
			values := map[string]string{tokenKey: secret, widgetKey: `{"widget":"synthetic-widget-secret"}`}
			require.NoError(t, UpdateOptionsBulk(values))
			for key, value := range values {
				assert.NotContains(t, stored(key), value)
				plain, exists, err := ReadOptionFromDB(key)
				require.NoError(t, err)
				assert.True(t, exists)
				assert.Equal(t, value, plain)
			}
			tokenCipher := stored(tokenKey)
			if dialect == "mysql" {
				require.NoError(t, db.Exec("ALTER TABLE options MODIFY `key` VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error)
				for _, alias := range []string{"Adty_reseller_setting.cloudflare_apiToken", tokenKey + " ", "adty_reseller_setting.cloudflare_ápiToken"} {
					for _, writer := range []func(string, string) error{
						UpdateOption,
						func(key, value string) error { return UpdateOptionsBulk(map[string]string{key: value}) },
					} {
						err := writer(alias, "synthetic-alias-secret")
						require.Error(t, err, "collation aliases must not bypass protected option handling")
						assert.Equal(t, tokenCipher, stored(tokenKey))
					}
				}
			}
			rollback := errors.New("synthetic transaction rollback")
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := SaveOptionsTx(tx, map[string]string{tokenKey: "synthetic-rollback-secret"}); err != nil {
					return err
				}
				plain, exists, err := ReadOptionForUpdate(tx, tokenKey)
				require.NoError(t, err)
				assert.True(t, exists)
				assert.Equal(t, "synthetic-rollback-secret", plain)
				return rollback
			})
			require.ErrorIs(t, err, rollback)
			assert.Equal(t, tokenCipher, stored(tokenKey))
			assert.Equal(t, secret, cfg.CloudflareApiToken)

			failure := errors.New("synthetic storage failure")
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("secret_write_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == "options" {
					tx.AddError(failure)
				}
			}))
			err = UpdateOptionsBulk(map[string]string{tokenKey: "synthetic-failed-write"})
			require.ErrorIs(t, err, failure)
			require.NoError(t, db.Callback().Update().Remove("secret_write_failure"))
			assert.Equal(t, tokenCipher, stored(tokenKey))
			assert.Equal(t, secret, cfg.CloudflareApiToken)
			assert.NotContains(t, sqlLog.String(), "synthetic-failed-write")

			// A legacy database upgrades on its first options load, then remains stable.
			require.NoError(t, db.Save(&Option{Key: tokenKey, Value: secret}).Error)
			sqlLog.Reset()
			all, err := AllOption()
			require.NoError(t, err)
			assert.Contains(t, all, &Option{Key: tokenKey, Value: secret})
			upgraded := stored(tokenKey)
			assert.NotEqual(t, secret, upgraded)
			_, err = AllOption()
			require.NoError(t, err)
			assert.Equal(t, upgraded, stored(tokenKey), "repeated loads must not re-encrypt historical rows")
			assert.NotContains(t, sqlLog.String(), secret, "migration SQL logs must not contain plaintext")
			for _, bad := range []string{"adty:secret:v9:unknown", "adty:secret:v1:truncated", upgraded[:len(upgraded)-5] + "AAAAA"} {
				require.NoError(t, db.Save(&Option{Key: tokenKey, Value: bad}).Error)
				require.NoError(t, db.Save(&Option{Key: "RegisterEnabled", Value: "false"}).Error)
				common.RegisterEnabled = true
				_, err := AllOption()
				require.Error(t, err)
				assert.NotContains(t, err.Error(), bad)
				loadOptionsFromDatabase()
				assert.False(t, common.RegisterEnabled, "an unreadable provider secret must not block unrelated security settings")
				assert.Empty(t, cfg.CloudflareApiToken, "a failed reload must invalidate the old token")
				assert.Empty(t, cfg.TurnstileWidgetSecret)
				assert.Equal(t, bad, stored(tokenKey), "corrupted ciphertext must never be replaced with plaintext")
			}
			require.NoError(t, db.Save(&Option{Key: tokenKey, Value: upgraded}).Error)
			t.Setenv("CRYPTO_SECRET", cryptoKey+"-wrong")
			loadOptionsFromDatabase()
			assert.Empty(t, cfg.CloudflareApiToken)
			_, err = AllOption()
			require.Error(t, err)
			t.Setenv("CRYPTO_SECRET", cryptoKey)
			require.NoError(t, db.Save(&Option{Key: widgetKey, Value: upgraded}).Error)
			_, err = AllOption()
			require.Error(t, err, "AAD must bind ciphertext to its option key")
			require.NoError(t, UpdateOptionsBulk(values))
			t.Setenv("CRYPTO_SECRET", "")
			err = UpdateOptionsBulk(map[string]string{tokenKey: "must-not-persist", "adty_reseller_setting.freeze_days": "99"})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "must-not-persist")
			assert.Equal(t, secret, cfg.CloudflareApiToken)
			var count int64
			require.NoError(t, db.Model(&Option{}).Where(&Option{Key: "adty_reseller_setting.freeze_days"}).Count(&count).Error)
			assert.Zero(t, count)
			t.Setenv("CRYPTO_SECRET", cryptoKey)
			require.Error(t, UpdateOption(tokenKey, upgraded), "a client must not install a storage ciphertext as a token")
			require.NoError(t, UpdateOptionsBulk(map[string]string{tokenKey: "", widgetKey: "{}"}))
			assert.Empty(t, stored(tokenKey))
			assert.Empty(t, cfg.CloudflareApiToken)
			assert.Empty(t, cfg.TurnstileWidgetSecret)
		})
	}
}

func TestSiteSecretUpgradeKeepsConcurrentReplacement(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	const key = "adty_reseller_setting.cloudflare_apiToken"
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := isolatedMigrationTestDatabase(t, dialect)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(4)
			require.NoError(t, db.AutoMigrate(&Option{}))
			require.NoError(t, db.Create(&Option{Key: key, Value: "synthetic-legacy-token"}).Error)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			DB = db.WithContext(ctx)
			read, proceed := make(chan struct{}), make(chan struct{})
			var release sync.Once
			defer release.Do(func() { close(proceed) })
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("secret_snapshot_barrier", func(tx *gorm.DB) {
				if tx.Statement.Table == "options" && tx.Statement.Clauses["WHERE"].Expression == nil {
					close(read)
					select {
					case <-proceed:
					case <-ctx.Done():
						tx.AddError(ctx.Err())
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("secret_snapshot_barrier")) })
			result := make(chan error, 1)
			go func() { _, err := AllOption(); result <- err }()
			select {
			case <-read:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return SaveOptionsTx(tx, map[string]string{key: "synthetic-replacement-token"})
			}))
			release.Do(func() { close(proceed) })
			require.NoError(t, <-result)
			plain, exists, err := ReadOptionFromDB(key)
			require.NoError(t, err)
			assert.True(t, exists)
			assert.Equal(t, "synthetic-replacement-token", plain)
		})
	}
}
