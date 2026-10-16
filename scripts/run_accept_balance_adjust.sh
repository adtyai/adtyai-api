#!/usr/bin/env bash
# A13 balance-adjust acceptance (cloud PG). Cases include (39) other-approve missing confirm, (40) reject missing confirm.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_balance_adjust.go
