#!/usr/bin/env bash
# Run the production UI against a real, isolated PostgreSQL 17 + Redis backend.
# Requires ADTY_TEST_PG_DSN for a role allowed to CREATE DATABASE, plus native
# go, bun, node, python3, psql, redis-server and redis-cli. Install web dependencies
# and Playwright Chromium first. ADTY_CHROMIUM_PATH may select system Chromium.
set -euo pipefail

adty_browser_scenario=${ADTY_BROWSER_SCENARIO:-all}
case "$adty_browser_scenario" in
  all)
    # Cold bookmark checks rotate refresh cookies. Give each complete scenario
    # its own database and Redis so the default security limits remain active.
    ADTY_BROWSER_SCENARIO=deployment bash "$0"
    ADTY_BROWSER_SCENARIO=wallet bash "$0"
    ADTY_BROWSER_SCENARIO=site-status bash "$0"
    ADTY_BROWSER_SCENARIO=password-reset bash "$0"
    ADTY_BROWSER_SCENARIO=user-sites bash "$0"
    ADTY_BROWSER_SCENARIO=finance-actions bash "$0"
    ADTY_BROWSER_SCENARIO=site-brand bash "$0"
    ADTY_BROWSER_SCENARIO=notification-read bash "$0"
    ADTY_BROWSER_SCENARIO=notification-publish bash "$0"
    ADTY_BROWSER_SCENARIO=unified-orders bash "$0"
    ADTY_BROWSER_SCENARIO=order-trace bash "$0"
    ADTY_BROWSER_SCENARIO=withdraw-networks bash "$0"
    ADTY_BROWSER_SCENARIO=site-staff bash "$0"
    exit 0
    ;;
  deployment | wallet | site-status | password-reset | user-sites | finance-actions | site-brand | notification-read | notification-publish | unified-orders | order-trace | withdraw-networks | site-staff) ;;
  *) echo 'browser-business: unknown scenario' >&2; exit 2 ;;
esac

if [ "$adty_browser_scenario" = site-staff ] && [ -z "${ADTY_BROWSER_STAFF_WIDTH:-}" ]; then
  # Each viewport runs a full sensitive lifecycle. Isolate their security
  # rate-limit windows without changing the production limits.
  ADTY_BROWSER_STAFF_WIDTH=1440 bash "$0"
  ADTY_BROWSER_STAFF_WIDTH=390 bash "$0"
  exit 0
fi

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
adty_repo_root=$(pwd -P)
case "$adty_repo_root" in
  /mnt | /mnt/*)
    echo 'browser-business: run from a native Linux checkout, not /mnt' >&2
    exit 1
    ;;
esac
if [ -z "${ADTY_TEST_PG_DSN:-}" ]; then
  echo 'browser-business: ADTY_TEST_PG_DSN is required; PostgreSQL cannot be skipped' >&2
  exit 1
fi
for adty_tool in go bun node python3 psql redis-server redis-cli; do
  adty_tool_path=$(command -v "$adty_tool") || {
    echo "browser-business: missing $adty_tool" >&2
    exit 1
  }
  case "$(readlink -f "$adty_tool_path")" in
    /mnt/*)
      echo "browser-business: $adty_tool must be a native Linux tool" >&2
      exit 1
      ;;
  esac
done

adty_pg_version=$(psql "$ADTY_TEST_PG_DSN" -XAt -v ON_ERROR_STOP=1 -c 'SHOW server_version_num')
if ! [[ "$adty_pg_version" =~ ^[0-9]+$ ]] ||
  [ "$adty_pg_version" -lt 170000 ] || [ "$adty_pg_version" -ge 180000 ]; then
  echo "browser-business: PostgreSQL 17 is required (server_version_num=$adty_pg_version)" >&2
  exit 1
fi

adty_tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/adty-browser.XXXXXXXX")
adty_database="adty_browser_$(date +%s)_$$_${RANDOM}"
adty_created_database=0
adty_backend_pid=''
adty_redis_pid=''

adty_stop_process() {
  local adty_pid=$1
  [ -n "$adty_pid" ] || return 0
  kill "$adty_pid" 2>/dev/null || return 0
  for adty_attempt in {1..50}; do
    kill -0 "$adty_pid" 2>/dev/null || break
    sleep 0.1
  done
  if kill -0 "$adty_pid" 2>/dev/null; then
    kill -KILL "$adty_pid" 2>/dev/null || true
  fi
  wait "$adty_pid" 2>/dev/null || true
}

adty_cleanup() {
  local adty_exit_code=$?
  set +e
  adty_stop_process "$adty_backend_pid"
  adty_stop_process "$adty_redis_pid"
  if [ "$adty_exit_code" -ne 0 ]; then
    for adty_log in "$adty_tmpdir/frontend-build.log" "$adty_tmpdir/backend-build.log" "$adty_tmpdir/backend.log"; do
      if [ -f "$adty_log" ]; then
        tail -n 100 "$adty_log" >&2
      fi
    done
  fi
  if [ "$adty_created_database" -eq 1 ] &&
    [[ "$adty_database" =~ ^adty_browser_[0-9]+_[0-9]+_[0-9]+$ ]]; then
    psql "$ADTY_TEST_PG_DSN" -X -v ON_ERROR_STOP=1 \
      -c "DROP DATABASE \"$adty_database\" WITH (FORCE)" >/dev/null || adty_exit_code=1
  fi
  if [ -d "$adty_tmpdir" ] && [[ "${adty_tmpdir##*/}" == adty-browser.* ]]; then
    rm -rf -- "$adty_tmpdir"
  fi
  trap - EXIT
  exit "$adty_exit_code"
}
trap adty_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

psql "$ADTY_TEST_PG_DSN" -X -v ON_ERROR_STOP=1 \
  -c "CREATE DATABASE \"$adty_database\"" >/dev/null
adty_created_database=1
export ADTY_BROWSER_DATABASE="$adty_database"
adty_sql_dsn=$(python3 - <<'PY'
import os
from urllib.parse import parse_qsl, quote, urlencode, urlsplit
dsn = os.environ['ADTY_TEST_PG_DSN']
database = os.environ['ADTY_BROWSER_DATABASE']
if dsn.startswith(('postgres://', 'postgresql://')):
    url = urlsplit(dsn)
    query = [(key, value) for key, value in parse_qsl(url.query, keep_blank_values=True)
             if key.lower() not in ('dbname', 'database')]
    derived = url.scheme + '://' + url.netloc + '/' + quote(database, safe='')
    if query:
        derived += '?' + urlencode(query)
    print(derived)
else:
    print(dsn + ' dbname=' + database)
PY
)
adty_connected_database=$(psql "$adty_sql_dsn" -XAt -v ON_ERROR_STOP=1 -c 'SELECT current_database()')
if [ "$adty_connected_database" != "$adty_database" ]; then
  echo 'browser-business: derived DSN did not connect to the isolated database' >&2
  exit 1
fi
export ADTY_BROWSER_SQL_DSN="$adty_sql_dsn"

adty_reserve_port() {
  python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0))
    print(listener.getsockname()[1])
PY
}
adty_api_port=$(adty_reserve_port)
adty_redis_port=$(adty_reserve_port)
while [ "$adty_api_port" = "$adty_redis_port" ]; do
  adty_redis_port=$(adty_reserve_port)
done
mkdir -p "$adty_tmpdir/redis" "$adty_tmpdir/runtime"
redis-server --bind 127.0.0.1 --port "$adty_redis_port" \
  --save '' --appendonly no --dir "$adty_tmpdir/redis" \
  >"$adty_tmpdir/redis.log" 2>&1 &
adty_redis_pid=$!
adty_redis_ready=0
for adty_attempt in {1..50}; do
  if [ "$(redis-cli -h 127.0.0.1 -p "$adty_redis_port" ping 2>/dev/null)" = PONG ]; then
    adty_redis_ready=1
    break
  fi
  kill -0 "$adty_redis_pid" 2>/dev/null || break
  sleep 0.1
done
if [ "$adty_redis_ready" -ne 1 ]; then
  echo 'browser-business: isolated Redis did not start' >&2
  exit 1
fi

echo "browser-business: PostgreSQL $adty_pg_version, isolated database and Redis instance"
(cd web && bun run build) >"$adty_tmpdir/frontend-build.log" 2>&1
echo 'browser-business: production frontend build and eight-language prerender passed'
GOWORK=off go build -o "$adty_tmpdir/new-api" . >"$adty_tmpdir/backend-build.log" 2>&1
echo 'browser-business: production backend build passed'

# A fresh runtime allows the default ServerAddress host, localhost, before setup.
export ADTY_BROWSER_BASE_URL="http://localhost:$adty_api_port"
export ADTY_BROWSER_USERNAME=browserroot
export ADTY_BROWSER_PASSWORD
ADTY_BROWSER_PASSWORD=$(python3 -c 'import secrets; print("LocalGate!" + secrets.token_hex(16))')
adty_session_secret=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
adty_crypto_secret=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
# Start outside the checkout so an operator's .env, database or log files cannot
# be loaded. The compiled binary embeds the actual production web/dist bundle.
(
  cd "$adty_tmpdir/runtime"
  exec env HOST=127.0.0.1 PORT="$adty_api_port" GIN_MODE=release \
    SQL_DSN="$adty_sql_dsn" LOG_SQL_DSN="$adty_sql_dsn" \
    REDIS_CONN_STRING="redis://127.0.0.1:$adty_redis_port/0" \
    SESSION_SECRET="$adty_session_secret" CRYPTO_SECRET="$adty_crypto_secret" "$adty_tmpdir/new-api"
) >"$adty_tmpdir/backend.log" 2>&1 &
adty_backend_pid=$!
export ADTY_BROWSER_BACKEND_PID="$adty_backend_pid"

python3 - <<'PY'
import json
import os
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

base = os.environ['ADTY_BROWSER_BASE_URL']
deadline = time.monotonic() + 60
while True:
    try:
        with urlopen(base + '/api/setup', timeout=2) as response:
            setup = json.load(response)
        if not setup.get('success'):
            raise RuntimeError('Setup endpoint did not succeed')
        assert setup['data']['database_type'] == 'postgres', setup
        assert not setup['data']['status'] and not setup['data']['root_init'], 'Database is not empty'
        break
    except HTTPError as error:
        raise RuntimeError(f'Isolated setup endpoint returned HTTP {error.code}') from None
    except (URLError, TimeoutError):
        os.kill(int(os.environ['ADTY_BROWSER_BACKEND_PID']), 0)
        if time.monotonic() >= deadline:
            raise RuntimeError('Isolated backend did not become ready') from None
        time.sleep(0.2)

password = os.environ['ADTY_BROWSER_PASSWORD']
body = json.dumps({
    'username': os.environ['ADTY_BROWSER_USERNAME'],
    'password': password,
    'confirmPassword': password,
    'SelfUseModeEnabled': False,
    'DemoSiteEnabled': False,
}).encode()
request = Request(base + '/api/setup', data=body, headers={'Content-Type': 'application/json'})
with urlopen(request, timeout=10) as response:
    result = json.load(response)
assert result.get('success'), result.get('message')
print('browser-business: real PostgreSQL runtime and synthetic root initialized')
PY

(cd web && node "scripts/adty/browser-$adty_browser_scenario.mjs")
echo "browser-business: production $adty_browser_scenario checks passed"
