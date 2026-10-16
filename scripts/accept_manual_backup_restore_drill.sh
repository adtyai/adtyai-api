#!/usr/bin/env bash
# 验收联调 force 脚本入口：缺 ADTY_BACKUP_AGE_RECIPIENT 时经 .sh 必须 exit 2
# （go build + exec；证明不再被 go run 收成 1）。不跑真备份/MinIO。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"

# secrets / 外层 env 可能带了 recipient；本用例强制清掉，保证走 preflight exit 2
unset ADTY_BACKUP_AGE_RECIPIENT || true
unset ADTY_RESTORE_DRILL_IDENTITY_FILE || true

set +e
env -u ADTY_BACKUP_AGE_RECIPIENT -u ADTY_RESTORE_DRILL_IDENTITY_FILE \
  "${SCRIPT_DIR}/run_manual_backup_restore_drill.sh"
code=$?
set -e

if [[ "$code" -ne 2 ]]; then
  echo "FAIL: expected exit 2 for missing ADTY_BACKUP_AGE_RECIPIENT via .sh, got ${code}" >&2
  exit 1
fi
echo "OK: missing recipient → exit 2 (via .sh go build+exec)"
echo "accept_manual_backup_restore_drill EXIT=0"
