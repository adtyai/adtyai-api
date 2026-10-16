#!/usr/bin/env bash
# subscription direct-pay aff commission accept (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_subscription_aff.go
