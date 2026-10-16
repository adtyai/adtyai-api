#!/usr/bin/env bash
# B12 restore_drill skeleton accept (cloud PG, fake R2, no age private key)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
# 可选：ADTY_ACCEPT_AGE_IDENTITY_FILE=<运维测试钥路径> 用测试钥跑真链；未设则临时 age-keygen；无 age 则 skip
go run ./scripts/accept_restore_drill.go
