#!/usr/bin/env bash
# 联调一次性：force 备份 + restore_drill（MinIO 假 R2 + 测试钥）。
# 不开启 ADTY_RESTORE_DRILL_TASK_ENABLED；生产钥/真 R2 等总裁确认。
# 用法：在仓库根配置好 env 后执行本脚本（可 source 联调 secrets 拿 SQL_DSN）。
#
# 用 go build + exec 二进制，才能把 Go 的 os.Exit(2)（缺 recipient/identity）原样传到 shell。
# 不要用 go run：go run 会把程序非 0 退出码收成 1。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"

# 本脚本故意不 export ADTY_RESTORE_DRILL_TASK_ENABLED=true
# 若已在环境里打开，Go 侧会 stderr 警告；定时仍由进程 env 决定。

BIN="$(mktemp "${TMPDIR:-/tmp}/adty-manual-backup-restore-drill.XXXXXX")"
# exec 前若 build 失败可清掉；exec 成功后 shell 被替换，残留 /tmp 可接受
trap 'rm -f "$BIN"' EXIT
echo "manual_backup_restore_drill: recipient=${ADTY_BACKUP_AGE_RECIPIENT:-<unset>} identity=${ADTY_RESTORE_DRILL_IDENTITY_FILE:-<unset>} r2=${ADTY_BACKUP_R2_ENDPOINT:-<unset>}"
go build -o "$BIN" ./scripts/run_manual_backup_restore_drill.go
trap - EXIT
exec "$BIN"
