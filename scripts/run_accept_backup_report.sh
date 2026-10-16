#!/usr/bin/env bash
# B12 backup + B13 daily report accept (cloud PG, fake R2/age)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_backup_report.go
