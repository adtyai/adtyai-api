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
package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackupIncludesIndependentLogDatabaseAndPublishesOnlyCompleteSets(t *testing.T) {
	const password = "synthetic-pg-pass"
	const mainDSN = "postgresql://backup:synthetic-pg-pass@localhost/main"
	const logDSN = "postgresql://backup:synthetic-pg-pass@localhost/logs"
	script, err := os.ReadFile("../deploy/adty/backup.sh")
	require.NoError(t, err)
	for _, fixture := range []struct {
		name, main, logs, failure, password, options string
		logArchives                                  int
		wantError                                    bool
	}{
		{name: "independent logs", logs: logDSN, logArchives: 1},
		{name: "shared default"},
		{name: "explicit same database", logs: mainDSN},
		{name: "URI quoted password", main: "postgresql://backup:synthetic%27%20%24%28touch%20injected%29%5C@localhost/main", password: "synthetic' $(touch injected)\\"},
		{name: "keyword quoted password", main: `host=localhost dbname=main user=backup password='synthetic\' quoted\\slash'`, password: "synthetic' quoted\\slash"},
		{name: "pgx literal backslash password", main: `host=localhost dbname=main user=backup password='synthetic\tail'`, password: `synthetic\tail`},
		{name: "query password", main: "postgresql://backup@localhost/main?password=synthetic-pg-pass"},
		{name: "URI pgx runtime options", main: "postgresql://backup:synthetic-pg-pass@localhost/main?search_path=public&statement_cache_capacity=0&default_query_exec_mode=simple_protocol", options: "options='-c search_path=public'"},
		{name: "keyword pgx runtime options", main: "host=localhost dbname=main password=synthetic-pg-pass search_path='public, tenant' description_cache_capacity=0", options: `options='-c search_path=public,\\ tenant'`},
		{name: "existing server options", main: "host=localhost database=main password=synthetic-pg-pass options='-c statement_timeout=1000' search_path=public", options: "options='-c statement_timeout=1000 -c search_path=public'"},
		{name: "PostgreSQL 17 SSL negotiation", main: "postgresql://backup:synthetic-pg-pass@localhost/main?sslnegotiation=postgres&sslmode=require", options: "sslnegotiation='postgres'"},
		{name: "pgx direct SSL negotiation", main: "postgresql://backup:synthetic-pg-pass@localhost/main?sslnegotiation=direct", options: "sslmode='require' sslnegotiation='direct'"},
		{name: "malformed connection", main: "malformed-synthetic-pg-pass", wantError: true},
		{name: "unclosed keyword quote", main: "host=localhost password='synthetic-pg-pass", wantError: true},
		{name: "invalid runtime key", main: "postgresql://backup:synthetic-pg-pass@localhost/main?search_path%20-c%20log_statement=all", wantError: true},
		{name: "empty connection", main: " ", wantError: true},
		{name: "log dump fails", logs: logDSN, failure: "dump", wantError: true},
		{name: "log archive corrupt", logs: logDSN, failure: "validate", wantError: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			bin, backups, remote := filepath.Join(root, "bin"), filepath.Join(root, "backups"), filepath.Join(root, "remote")
			for _, dir := range []string{bin, backups, remote, filepath.Join(root, "data")} {
				require.NoError(t, os.MkdirAll(dir, 0o700))
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "data", "receipt"), []byte("uploaded receipt"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "backup.sh"), script, 0o700))
			oldLog := filepath.Join(backups, "adtyai-logdb-20000101-000000.dump")
			require.NoError(t, os.WriteFile(oldLog, []byte("old retained log"), 0o600))
			oldTime := time.Now().Add(-30 * 24 * time.Hour)
			require.NoError(t, os.Chtimes(oldLog, oldTime, oldTime))
			commands := map[string]string{
				"docker": `#!/usr/bin/env bash
printf '%s\n' "$@" >> "$BACKUP_TEST_ARGV"
while [ "$1" != exec ]; do shift; done
shift
[ "$1" != -T ] || shift
service=$1
shift
case "$service" in
  adtyai-api) exec "$@";;
  postgres) exec "$@";;
  *) exit 99;;
esac
`,
				"pg_dump": `#!/usr/bin/env bash
printf '%s\n' "$@" >> "$BACKUP_TEST_ARGV"
[ "$PGPASSWORD" = "$BACKUP_TEST_PASSWORD" ] || exit 75
case "$*" in
  *logs*)
    [ "$BACKUP_TEST_FAILURE" != dump ] || exit 71
    if [ "$BACKUP_TEST_FAILURE" = validate ]; then echo corrupt; else echo logs-payload; fi;;
  *main*) echo main-payload;;
  *) exit 72;;
esac
`,
				"pg_restore": `#!/usr/bin/env bash
case "$(cat)" in
  main-payload | logs-payload) exit 0;;
  *) exit 73;;
esac
`,
				"rsync": `#!/usr/bin/env bash
[ "$1" = -a ] || exit 74
shift
for source in "${@:1:$#-1}"; do cp "$source" "${!#}"; done
`,
			}
			for name, content := range commands {
				require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(content), 0o700))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("X-Adty-Flush-Token") != "synthetic-flush" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				_, _ = w.Write([]byte(`{"success":true,"data":{"flushed":true}}`))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "bash", filepath.Join(root, "backup.sh"))
			command.Dir = root
			connection, expectedPassword := mainDSN, password
			if fixture.main != "" {
				connection = fixture.main
			}
			if fixture.password != "" {
				expectedPassword = fixture.password
			}
			command.Env = append(os.Environ(),
				"PATH="+bin+":"+os.Getenv("PATH"), "DEPLOY_DIR="+root,
				"BACKUP_DIR="+backups, "BACKUP_REMOTE="+remote+"/", "KEEP_DAYS=14",
				"ADTY_FLUSH_TOKEN=synthetic-flush", "ADTY_FLUSH_URL="+server.URL,
				"SQL_DSN="+connection, "LOG_SQL_DSN="+fixture.logs,
				"POSTGRES_USER=backup", "POSTGRES_DB=main",
				"BACKUP_TEST_ARGV="+filepath.Join(root, "argv"), "BACKUP_TEST_FAILURE="+fixture.failure,
				"BACKUP_TEST_PASSWORD="+expectedPassword)
			output, runErr := command.CombinedOutput()
			require.NotContains(t, string(output), password)
			arguments, err := os.ReadFile(filepath.Join(root, "argv"))
			require.NoError(t, err)
			require.NotContains(t, string(arguments), password, "database passwords must not be in subprocess arguments")
			require.NotContains(t, string(output), expectedPassword)
			require.NotContains(t, string(arguments), expectedPassword)
			_, err = os.Stat(filepath.Join(root, "injected"))
			require.True(t, os.IsNotExist(err), "DSN contents must never execute shell commands")
			partials, err := filepath.Glob(filepath.Join(backups, "*.partial"))
			require.NoError(t, err)
			require.Empty(t, partials)
			remoteFiles, err := os.ReadDir(remote)
			require.NoError(t, err)
			if fixture.wantError {
				require.Error(t, runErr, "%s", output)
				require.Empty(t, remoteFiles, "an incomplete backup set must not be replicated")
				files, err := os.ReadDir(backups)
				require.NoError(t, err)
				require.Len(t, files, 1, "no incomplete set may be published or old backup removed")
				require.Equal(t, filepath.Base(oldLog), files[0].Name())
				return
			}
			require.NoError(t, runErr, "%s", output)
			if fixture.options != "" {
				require.Contains(t, string(arguments), fixture.options, "server runtime settings must reach pg_dump")
				require.NotContains(t, string(arguments), "statement_cache_capacity")
				require.NotContains(t, string(arguments), "description_cache_capacity")
				require.NotContains(t, string(arguments), "default_query_exec_mode")
			}
			_, err = os.Stat(oldLog)
			require.True(t, os.IsNotExist(err), "log database archives must use the configured retention")
			require.Len(t, remoteFiles, 2+fixture.logArchives)
			logs := 0
			for _, entry := range remoteFiles {
				local := filepath.Join(backups, entry.Name())
				copied, err := os.ReadFile(filepath.Join(remote, entry.Name()))
				require.NoError(t, err)
				original, err := os.ReadFile(local)
				require.NoError(t, err)
				require.Equal(t, original, copied)
				info, err := os.Stat(local)
				require.NoError(t, err)
				require.Zero(t, info.Mode().Perm()&0o077)
				if strings.HasPrefix(entry.Name(), "adtyai-logdb-") {
					logs++
					require.Equal(t, "logs-payload\n", string(original))
				}
			}
			require.Equal(t, fixture.logArchives, logs)
		})
	}
}
