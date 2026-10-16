#!/usr/bin/env bash
# settle accept via dual_wallet (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
echo "=== accept_dual_wallet (incl. settle no-negative) ==="
go run ./scripts/accept_dual_wallet.go
echo "=== DONE ==="
