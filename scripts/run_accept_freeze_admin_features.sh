#!/usr/bin/env bash
# Freeze acceptance: enterprise-pricing admin HTTP 403 + runtime Match off;
# balance-adjustments admin APIs → 403.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
"${SCRIPT_DIR}/run_accept_enterprise_pricing.sh"
"${SCRIPT_DIR}/run_accept_balance_adjust.sh"
echo "ALL freeze admin features PASSED"
