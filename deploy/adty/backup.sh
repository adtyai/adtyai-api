#!/usr/bin/env bash
# 每天备份 PostgreSQL（pg_dump）和 /data 卷（返佣收款码、打款凭证等上传文件），本机保留 14 天，再同步到另一台机器（任务 10.1）。
# 要 root（读 data 目录、执行 docker）。cron 和恢复方法见 docs/adty/deploy.md。
#   BACKUP_REMOTE  另一台机器上的 rsync 目标，比如 backup@10.0.0.2:/srv/backup/adtyai/。必须设：没有异地备份时本机备份照做，但退出码非 0。
#   DEPLOY_DIR     docker-compose.yml 和 data 目录所在的目录，默认是本脚本所在的目录。
#   BACKUP_DIR     本机备份目录，默认 /var/backups/adtyai。
#   KEEP_DAYS      本机保留天数，默认 14。
set -euo pipefail

DEPLOY_DIR="${DEPLOY_DIR:-$(dirname "$(readlink -f "$0")")}"
DATA_DIR="${DATA_DIR:-$DEPLOY_DIR/data}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/adtyai}"
KEEP_DAYS="${KEEP_DAYS:-14}"
BACKUP_REMOTE="${BACKUP_REMOTE:-}"

case "$KEEP_DAYS" in
    '' | *[!0-9]* | 0)
        echo "backup: KEEP_DAYS 必须是正整数" >&2
        exit 1
        ;;
esac

stamp="$(date +%Y%m%d-%H%M%S)"
dump="$BACKUP_DIR/adtyai-db-$stamp.dump"
archive="$BACKUP_DIR/adtyai-data-$stamp.tar.gz"

# 备份里有用户数据和渠道密钥，只给 root 读
umask 077
mkdir -p "$BACKUP_DIR"
trap 'rm -f "$dump.partial" "$archive.partial"' EXIT

# 批量额度还在内存队列时，转储会丢掉尚未落库的差额。先让 API 刷盘。
flush_url="${ADTY_FLUSH_URL:-http://127.0.0.1:3000/api/adty/internal/flush-batch}"
if [ -z "${ADTY_FLUSH_TOKEN:-}" ]; then
    echo "backup: 没有设 ADTY_FLUSH_TOKEN，不能在批量额度未刷盘时备份" >&2
    exit 1
fi
curl -fsS -X POST -H "X-Adty-Flush-Token: ${ADTY_FLUSH_TOKEN}" "$flush_url" >/dev/null

# 自定义格式，用 pg_restore 恢复。用户名和库名取容器里的 POSTGRES_USER、POSTGRES_DB。
docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T postgres \
    sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' >"$dump.partial"
# 读一遍目录，确认备份文件完整
docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T postgres pg_restore --list >/dev/null <"$dump.partial"
mv "$dump.partial" "$dump"

tar -czf "$archive.partial" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
mv "$archive.partial" "$archive"

echo "backup: $dump（$(du -h "$dump" | cut -f1)）、$archive（$(du -h "$archive" | cut -f1)）"

# 只删本脚本生成的、超过保留天数的文件。-mtime +N 是满 N+1 天，所以用 KEEP_DAYS-1。
find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'adtyai-db-*.dump' -o -name 'adtyai-data-*.tar.gz' \) \
    -mtime +"$((KEEP_DAYS - 1))" -print -delete

if [ -z "$BACKUP_REMOTE" ]; then
    echo "backup: 没有设 BACKUP_REMOTE，这次没有异地备份" >&2
    exit 1
fi
# 不带 --delete：本机的备份被删或被改坏时，另一台机器上的旧备份还在。那边的保留期在那台机器上自己清理。
rsync -a "$dump" "$archive" "$BACKUP_REMOTE"
echo "backup: 已同步到 $BACKUP_REMOTE"
