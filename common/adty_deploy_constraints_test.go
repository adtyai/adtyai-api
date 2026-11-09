package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDeploymentConstraintsAreInTheTestedPackages(t *testing.T) {
	mainSource, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), `127.0.0.1:8005`) {
		t.Fatal("pprof listen address is not loopback")
	}
	for _, name := range []string{"../docker-compose.yml", "../docker-compose.dev.yml"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "123456") {
			t.Fatalf("%s publishes an example secret", name)
		}
	}
}

func TestPluginValidationEntrypointsRejectInvalidPlugins(t *testing.T) {
	realBun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("plugin entrypoint integration requires Bun")
	}
	realNode, err := exec.LookPath("node")
	if err != nil {
		t.Skip("plugin entrypoint integration requires Node")
	}
	modules, err := filepath.Abs("../web/node_modules")
	require.NoError(t, err)
	for _, tool := range []string{"oxlint/bin/oxlint", "oxfmt/bin/oxfmt"} {
		if _, err := os.Stat(filepath.Join(modules, tool)); os.IsNotExist(err) {
			t.Skip("plugin entrypoint integration requires installed frontend dependencies")
		} else {
			require.NoError(t, err)
		}
	}
	for _, entry := range []string{"check", "build-web"} {
		for _, fixture := range []struct{ name, code string }{
			{"valid", "export const probe = 1;\n"},
			{"lint", "export var probe = 1;\n"},
			{"format", "export const probe=1;\n"},
		} {
			t.Run(entry+"/"+fixture.name, func(t *testing.T) {
				root := t.TempDir()
				for _, dir := range []string{"scripts/adty", "web", "plugins/tasks/probe", "bin"} {
					require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
				}
				for _, name := range []string{"scripts/adty/check.sh", "makefile", "VERSION", "web/package.json", "plugins/.oxlintrc.json", "plugins/.oxfmtrc.json"} {
					body, readErr := os.ReadFile(filepath.Join("..", name))
					require.NoError(t, readErr)
					require.NoError(t, os.WriteFile(filepath.Join(root, name), body, 0o600))
				}
				require.NoError(t, os.Symlink(modules, filepath.Join(root, "web/node_modules")))
				require.NoError(t, os.WriteFile(filepath.Join(root, "plugins/tasks/probe/plugin.js"), []byte(fixture.code), 0o600))
				trace := filepath.Join(root, "trace")
				bun := `#!/usr/bin/env bash
printf 'bun %s\n' "$*" >> "$GOV_TRACE"
if [ "${1:-}" = --version ]; then exec "$REAL_BUN" "$@"; fi
if [ "${1:-}" = run ]; then
  case "${2:-}" in lint:plugins|format:plugins:check) exec "$REAL_BUN" "$@";; esac
fi
exit 0
`
				node := `#!/usr/bin/env bash
case "${1:-}" in *scripts/adty/ratchet.mjs|*scripts/adty/i18n-check.mjs) exit 0;; esac
exec "$REAL_NODE" "$@"
`
				require.NoError(t, os.WriteFile(filepath.Join(root, "bin/bun"), []byte(bun), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(root, "bin/node"), []byte(node), 0o700))
				for _, tool := range []string{"git", "go", "npm", "gh", "rg"} {
					require.NoError(t, os.WriteFile(filepath.Join(root, "bin", tool), []byte("#!/usr/bin/env bash\necho fixture-version\n"), 0o700))
				}
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, "make", "build-web")
				if entry == "check" {
					command = exec.CommandContext(ctx, "bash", "scripts/adty/check.sh", "--frontend")
				}
				command.Dir = root
				command.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "REAL_BUN="+realBun, "REAL_NODE="+realNode, "GOV_TRACE="+trace)
				output, runErr := command.CombinedOutput()
				if fixture.name == "valid" {
					require.NoError(t, runErr, "%s", output)
				} else {
					require.Error(t, runErr, "invalid plugin bypassed %s", entry)
				}
				calls, readErr := os.ReadFile(trace)
				require.NoError(t, readErr)
				require.Contains(t, string(calls), "bun run lint:plugins")
				if fixture.name != "lint" {
					require.Contains(t, string(calls), "bun run format:plugins:check")
				}
				if fixture.name != "valid" {
					require.NotContains(t, string(calls), "bun run build")
					require.NotContains(t, string(calls), "bun run typecheck")
					require.NotContains(t, string(calls), "bun run test")
				}
			})
		}
	}
}

func TestPluginValidationCIRunsAfterInstallBeforeTypecheck(t *testing.T) {
	for _, fixture := range []struct{ file, job string }{{"ci.yml", "frontend"}, {"adty-ghcr.yml", "test"}} {
		t.Run(fixture.file, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("../.github/workflows", fixture.file))
			require.NoError(t, err)
			var workflow struct {
				Jobs map[string]struct {
					Defaults struct {
						Run struct {
							WorkingDirectory string `yaml:"working-directory"`
						}
					}
					Steps []struct {
						Run              string
						WorkingDirectory string `yaml:"working-directory"`
					}
				}
			}
			require.NoError(t, yaml.Unmarshal(body, &workflow))
			job, ok := workflow.Jobs[fixture.job]
			require.True(t, ok)
			install, gate, typecheck := -1, -1, -1
			for index, step := range job.Steps {
				switch strings.TrimSpace(step.Run) {
				case "bun install --frozen-lockfile":
					install = index
				case "bun check -b":
					typecheck = index
				case "make check-plugins":
					gate = index
					cwd := step.WorkingDirectory
					if cwd == "" {
						cwd = job.Defaults.Run.WorkingDirectory
					}
					require.True(t, cwd == "" || cwd == ".", "plugin gate must run from the repository root")
				}
			}
			require.GreaterOrEqual(t, install, 0)
			require.Greater(t, gate, install)
			require.Greater(t, typecheck, gate)
		})
	}
}

func TestFlushTokenIsRequiredByProductionConfiguration(t *testing.T) {
	body, err := os.ReadFile("../deploy/adty/docker-compose.yml")
	require.NoError(t, err)
	var compose struct {
		Services map[string]struct{ Environment map[string]string }
	}
	require.NoError(t, yaml.Unmarshal(body, &compose))
	require.Equal(t, "${ADTY_FLUSH_TOKEN:?set ADTY_FLUSH_TOKEN in .env}", compose.Services["adtyai-api"].Environment["ADTY_FLUSH_TOKEN"])
	example, err := os.ReadFile("../deploy/adty/.env.example")
	require.NoError(t, err)
	var tokenLines []string
	for _, line := range strings.Split(string(example), "\n") {
		if strings.HasPrefix(line, "ADTY_FLUSH_TOKEN=") {
			tokenLines = append(tokenLines, strings.TrimSpace(line))
		}
	}
	require.Equal(t, []string{"ADTY_FLUSH_TOKEN="}, tokenLines)
}

func TestBackupFlushAuthorizationDoesNotLeakOrFallbackExplicitTokens(t *testing.T) {
	const secret = "adty-gate-synthetic-secret"
	realCurl, err := exec.LookPath("curl")
	require.NoError(t, err)
	script, err := os.ReadFile("../deploy/adty/backup.sh")
	require.NoError(t, err)
	for _, fixture := range []struct {
		name                               string
		host                               string
		hostSet                            bool
		container                          string
		containerMissing, containerFailure bool
		reads, requests                    int
		dump                               bool
	}{
		{name: "host valid", hostSet: true, host: secret, dump: true, requests: 1},
		{name: "host wrong", hostSet: true, host: secret + "-wrong", container: secret, requests: 1},
		{name: "host explicitly empty", hostSet: true, container: secret},
		{name: "host line feed", hostSet: true, host: secret + "\nInjected: yes", container: secret},
		{name: "host carriage return", hostSet: true, host: secret + "\r", container: secret},
		{name: "container valid", container: secret, reads: 1, requests: 1, dump: true},
		{name: "container empty", reads: 1},
		{name: "container missing", containerMissing: true, reads: 1},
		{name: "container unavailable", containerFailure: true, reads: 1},
		{name: "container trailing line feed", container: secret + "\n", reads: 1},
		{name: "container carriage return", container: secret + "\r", reads: 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, "bin"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "backup.sh"), script, 0o600))
			actions, argv := filepath.Join(root, "actions"), filepath.Join(root, "argv")
			docker := `#!/usr/bin/env bash
printf '%s\n' "$@" >> "$GOV_ARGV"
case "$*" in
  *adtyai-api*)
    if [[ "$*" == *ADTY_FLUSH_TOKEN* ]]; then
      echo read >> "$GOV_ACTIONS"
      [ "$GOV_CONTAINER_FAILURE" = 0 ] || exit 55
    fi
    while [ "$1" != adtyai-api ]; do shift; done
    shift
    if [ "$GOV_CONTAINER_MISSING" = 1 ]; then unset ADTY_FLUSH_TOKEN; else export ADTY_FLUSH_TOKEN="$GOV_CONTAINER_TOKEN"; fi
    exec "$@";;
  *postgres*) echo dump >> "$GOV_ACTIONS"; exit 67;;
esac
exit 99
`
			curl := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" >> \"$GOV_ARGV\"\nexec \"$REAL_CURL\" \"$@\"\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "bin/docker"), []byte(docker), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "bin/curl"), []byte(curl), 0o700))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || r.Header.Get("X-Adty-Flush-Token") != secret {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			env := make([]string, 0, len(os.Environ())+12)
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "ADTY_FLUSH_TOKEN=") {
					env = append(env, value)
				}
			}
			env = append(env, "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "DEPLOY_DIR="+root, "BACKUP_DIR="+filepath.Join(root, "backups"), "ADTY_FLUSH_URL="+server.URL, "REAL_CURL="+realCurl, "GOV_ARGV="+argv, "GOV_ACTIONS="+actions, "GOV_CONTAINER_TOKEN="+fixture.container, "GOV_CONTAINER_MISSING=0", "GOV_CONTAINER_FAILURE=0", "SQL_DSN=postgresql://localhost/synthetic", "LOG_SQL_DSN=")
			if fixture.hostSet {
				env = append(env, "ADTY_FLUSH_TOKEN="+fixture.host)
			}
			if fixture.containerMissing {
				env = append(env, "GOV_CONTAINER_MISSING=1")
			}
			if fixture.containerFailure {
				env = append(env, "GOV_CONTAINER_FAILURE=1")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "bash", filepath.Join(root, "backup.sh"))
			command.Env = env
			output, runErr := command.CombinedOutput()
			if strings.Contains(string(output), secret) {
				t.Fatal("flush secret leaked in output")
			}
			arguments, readErr := os.ReadFile(argv)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			if strings.Contains(string(arguments), secret) {
				t.Fatal("flush secret leaked in command arguments")
			}
			calls, readErr := os.ReadFile(actions)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			require.Equal(t, fixture.reads, strings.Count(string(calls), "read\n"))
			require.Equal(t, fixture.requests, int(requests.Load()))
			require.Equal(t, fixture.dump, strings.Contains(string(calls), "dump\n"))
			require.Error(t, runErr)
			if fixture.dump {
				var exit *exec.ExitError
				require.ErrorAs(t, runErr, &exit)
				require.Equal(t, 67, exit.ExitCode(), "authorized request must reach only the stubbed dump boundary")
			}
		})
	}
}
