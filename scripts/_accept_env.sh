#!/usr/bin/env bash
# Shared accept-runner env for cloud迁云口径.
# Source from scripts/run_accept_*.sh after set -euo pipefail.
# - REPO_ROOT via SCRIPT_DIR (no I5 / WSL hardcode)
# - SQL_DSN from env, or ADTYAI_SECRETS_FILE / 联调-secrets.env, or ADTYAI_PG_* defaults
# Never embed DB passwords in committed scripts.

_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${_SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

_SECRETS="${ADTYAI_SECRETS_FILE:-/workspace/adty-mock/联调-secrets.env}"
if [[ -z "${SQL_DSN:-}" && -f "$_SECRETS" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$_SECRETS"
  set +a
fi

if [[ -z "${SQL_DSN:-}" ]]; then
  _host="${ADTYAI_PG_HOST:-127.0.0.1}"
  _port="${ADTYAI_PG_PORT:-5432}"
  _user="${ADTYAI_PG_USER:-adtyai}"
  _db="${ADTYAI_PG_DB:-adtyai}"
  _pass="${ADTYAI_PG_PASSWORD:-${PGPASSWORD:-}}"
  if [[ -z "$_pass" ]]; then
    echo "accept_env: SQL_DSN unset and no ADTYAI_PG_PASSWORD/PGPASSWORD; set SQL_DSN or provide ${_SECRETS}" >&2
    exit 1
  fi
  SQL_DSN="postgres://${_user}:${_pass}@${_host}:${_port}/${_db}?sslmode=disable"
fi

export SQL_DSN
export LOG_SQL_DSN="${LOG_SQL_DSN:-$SQL_DSN}"
export BATCH_UPDATE_ENABLED="${BATCH_UPDATE_ENABLED:-false}"
export REDIS_CONN_STRING="${REDIS_CONN_STRING:-}"
export ADTY_SECRET_KEY="${ADTY_SECRET_KEY:-adtyai-accept-test-secret-key-do-not-use-prod}"

# Accepts must serialize against the shared PostgreSQL used by all run_accept_*.sh /
# accept_manual_*.sh / run_manual_*.sh that source this file. Parallel accepts are
# unsupported (tuple concurrently updated, missing probe summary, reconcile seed
# pollution). Hold an exclusive flock for the lifetime of this shell process.
# Nested source (e.g. accept_manual → run_manual) skips re-lock via ADTY_ACCEPT_LOCK_HELD
# so the child does not deadlock waiting on the parent.
if [[ -z "${ADTY_ACCEPT_LOCK_HELD:-}" ]]; then
  _ACCEPT_LOCK="${ADTY_ACCEPT_LOCK:-/tmp/adtyai-accept.lock}"
  exec 200>"${_ACCEPT_LOCK}"
  flock -x 200
  export ADTY_ACCEPT_LOCK_HELD=1
fi
