#!/usr/bin/env bash
# A27 redeem → AdjustQuota accept (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
if grep -nE 'creditTopUpQuota|increaseUserQuotaTx|syncCreditUserQuotaCache' model/redemption.go; then
  echo "FAIL A27: model/redemption.go must credit via AdjustQuota only" >&2
  exit 1
fi
go run ./scripts/accept_redeem_adjust.go
