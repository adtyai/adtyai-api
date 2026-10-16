#!/usr/bin/env bash
# B-03 wallet split accept. TEMP DB ONLY: no secrets fallback; SQL_DSN, LOG_SQL_DSN and
# ADTY_B03_EXPECT_DB must be set explicitly (the Go side also checks current_database()).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."
: "${SQL_DSN:?set SQL_DSN to a temporary database}"
: "${LOG_SQL_DSN:?set LOG_SQL_DSN to the same temporary database}"
: "${ADTY_B03_EXPECT_DB:?set ADTY_B03_EXPECT_DB to the temporary database name}"
export BATCH_UPDATE_ENABLED=false REDIS_CONN_STRING=""
export ADTY_SECRET_KEY="${ADTY_SECRET_KEY:-adtyai-accept-test-secret-key-do-not-use-prod}"
go run ./scripts/accept_b03_wallet_split.go
