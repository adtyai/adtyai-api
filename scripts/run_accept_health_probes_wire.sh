#!/usr/bin/env bash
# A44 probes wire (epay/bepusdt/chatwoot) + A36 err redact accept (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_health_probes_wire.go
