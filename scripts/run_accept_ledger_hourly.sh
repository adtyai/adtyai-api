#!/usr/bin/env bash
# ledger hourly rollup + partition/trigger acceptance (cloud PG; source _accept_env.sh)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_ledger_hourly.go
