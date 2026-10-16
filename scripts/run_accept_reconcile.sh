#!/usr/bin/env bash
# A16 daily reconcile / admin reconcile accept (cloud PG)
# 注意：脚本前后会清理 accept-recon-/ACCEPT-RECON- 自造数据，清理期间临时关闭
# 账本防篡改触发器；不能和联调写入（真实充值/消费/退款）同时跑，请单独执行。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
go run ./scripts/accept_reconcile.go
