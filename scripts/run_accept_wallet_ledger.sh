#!/usr/bin/env bash
# A17/U16 wallet ledger accept (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_wallet_ledger.go
