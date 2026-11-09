#!/usr/bin/env bash
# 每天备份 PostgreSQL（pg_dump）和 /data 卷（返佣收款码、打款凭证等上传文件），本机保留 14 天，再同步到另一台机器（任务 10.1）。
# 要 root（读 data 目录、执行 docker）、宿主机 python3/libpq5。cron 和恢复方法见 docs/adty/deploy.md。
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
log_dump="$BACKUP_DIR/adtyai-logdb-$stamp.dump"

# 备份里有用户数据和渠道密钥，只给 root 读
umask 077
mkdir -p "$BACKUP_DIR"
trap 'rm -f "$dump.partial" "$log_dump.partial" "$archive.partial"' EXIT

# 先让运行中的 API 把持久化待刷额度折入主余额；失败不得继续备份。
flush_url="${ADTY_FLUSH_URL:-http://127.0.0.1:3000/api/adty/internal/flush-batch}"
if [ "${ADTY_FLUSH_TOKEN+x}" != x ]; then
    if ! token_output="$(docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T adtyai-api \
        sh -c 'printenv ADTY_FLUSH_TOKEN || exit; printf "."' 2>/dev/null)"; then
        echo "backup: cannot read ADTY_FLUSH_TOKEN from running API container" >&2
        exit 1
    fi
    ADTY_FLUSH_TOKEN="${token_output%.}"
    ADTY_FLUSH_TOKEN="${ADTY_FLUSH_TOKEN%$'\n'}"
fi
case "$ADTY_FLUSH_TOKEN" in
    ''|*$'\r'*|*$'\n'*)
        echo "backup: ADTY_FLUSH_TOKEN must be a non-empty single line" >&2
        exit 1
        ;;
esac
printf 'X-Adty-Flush-Token: %s\n' "$ADTY_FLUSH_TOKEN" |
    curl -fsS -X POST --header @- "$flush_url" >/dev/null

# 从实际运行的 API 读取连接，不能假设主库/日志库就是 compose 的默认库。
sql_dsn=$(docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T adtyai-api \
    sh -c 'printf "%s" "$SQL_DSN"')
log_sql_dsn=$(docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T adtyai-api \
    sh -c 'printf "%s" "${LOG_SQL_DSN:-}"')
if [ -z "$sql_dsn" ]; then
    echo 'backup: running API has no SQL_DSN' >&2
    exit 1
fi

dump_database() {
    local connection=$1 destination=$2
    # 兼容应用 pgx 的客户端/服务器参数，再由 libpq 验证连接并分离密码。
    # Python 输出带 shell 引号的脚本，只经 stdin 传入，不放进进程参数。
    python3 - 3<<<"$connection" <<'PY' | docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T postgres sh -s >"$destination.partial"
import ctypes
import ctypes.util
import os
import re
import shlex
import sys
from urllib.parse import parse_qsl

class Option(ctypes.Structure):
    _fields_ = [(name, ctypes.c_char_p) for name in ("keyword", "envvar", "compiled", "val", "label", "dispchar")] + [("dispsize", ctypes.c_int)]

def parse_libpq(raw):
    error = ctypes.c_void_p()
    options = pq.PQconninfoParse(raw.encode(), ctypes.byref(error))
    if not options:
        if error.value:
            pq.PQfreemem(error)
        raise ValueError()
    fields = {}
    try:
        index = 0
        while options[index].keyword:
            option = options[index]
            if option.val is not None:
                fields[option.keyword.decode()] = option.val.decode()
            index += 1
    finally:
        pq.PQconninfoFree(options)
    return fields

def keyword_dsn(fields):
    if any("\x00" in value for value in fields.values()):
        raise ValueError()
    return " ".join(key + "='" + value.replace("\\", "\\\\").replace("'", "\\'") + "'" for key, value in fields.items())

try:
    library = ctypes.util.find_library("pq")
    if not library:
        raise ValueError()
    pq = ctypes.CDLL(library)
    pq.PQconninfoParse.argtypes = [ctypes.c_char_p, ctypes.POINTER(ctypes.c_void_p)]
    pq.PQconninfoParse.restype = ctypes.POINTER(Option)
    pq.PQconninfoFree.argtypes = [ctypes.POINTER(Option)]
    pq.PQfreemem.argtypes = [ctypes.c_void_p]
    pq.PQconndefaults.restype = ctypes.POINTER(Option)
    raw = os.fdopen(3, "rb").read().decode().removesuffix("\n")
    if not raw.strip() or "\x00" in raw:
        raise ValueError()
    if raw.startswith(("postgres://", "postgresql://")):
        base, _, query = raw.partition("?")
        fields = parse_libpq(base)
        if re.search(r"%(?![0-9a-fA-F]{2})", query):
            raise ValueError()
        # pgx uses the first value of a repeated URL parameter.
        for key, value in reversed(parse_qsl(query, keep_blank_values=True, errors="strict")):
            fields["dbname" if key == "database" else key] = value
    else:
        fields = {}
        # pgx only unescapes doubled backslashes and escaped apostrophes.
        token = re.compile(r"""\s*([^\s=]+)\s*=\s*('(?:\\.|[^'\\])*'|(?!')(?:\\.|[^\s\\])*)""", re.ASCII | re.DOTALL)
        remaining = raw
        while remaining.strip():
            match = token.match(remaining)
            if not match:
                raise ValueError()
            key = match[1]
            value = match[2]
            if value.startswith("'"):
                value = value[1:-1]
            fields["dbname" if key == "database" else key] = value.replace("\\\\", "\\").replace("\\'", "'")
            remaining = remaining[match.end():]

    defaults = pq.PQconndefaults()
    if not defaults:
        raise ValueError()
    connection_keys = set()
    try:
        index = 0
        while defaults[index].keyword:
            connection_keys.add(defaults[index].keyword.decode())
            index += 1
    finally:
        pq.PQconninfoFree(defaults)
    # PG17 clients support this even when Debian's host parser is libpq15/16.
    connection_keys.add("sslnegotiation")
    for key in ("statement_cache_capacity", "description_cache_capacity", "default_query_exec_mode"):
        fields.pop(key, None)
    servicefile = fields.pop("servicefile", None)
    runtime_options = []
    for key in list(fields):
        if key in connection_keys:
            continue
        # These pgx connection options have no equivalent in libpq 17.
        if key in ("min_protocol_version", "max_protocol_version", "krbspn") or not re.fullmatch(r"[A-Za-z_][A-Za-z_0-9]*(?:\.[A-Za-z_][A-Za-z_0-9]*)*", key):
            raise ValueError()
        value = fields.pop(key)
        # PostgreSQL splits options on whitespace; backslashes protect literal characters.
        escaped = re.sub(r"([\\\s])", r"\\\1", key + "=" + value, flags=re.ASCII)
        runtime_options.append("-c " + escaped)
    if runtime_options:
        fields["options"] = " ".join(filter(None, (fields.get("options", ""), *runtime_options)))
    negotiation = fields.pop("sslnegotiation", None)
    if negotiation not in (None, "postgres", "direct"):
        raise ValueError()
    if negotiation == "direct" and fields.get("sslmode", "prefer") == "prefer":
        fields["sslmode"] = "require"
    fields = parse_libpq(keyword_dsn(fields))
    if negotiation is not None:
        fields["sslnegotiation"] = negotiation
    password = fields.pop("password", "")
    if fields.get("sslpassword"):
        raise ValueError()
    if servicefile is not None:
        if "\x00" in servicefile:
            raise ValueError()
        print("export PGSERVICEFILE=" + shlex.quote(servicefile))
    print("export PGPASSWORD=" + shlex.quote(password))
    print("exec pg_dump --format=custom --dbname=" + shlex.quote(keyword_dsn(fields)))
except Exception:
    print("backup: PostgreSQL connection parsing failed; python3 and libpq are required", file=sys.stderr)
    raise SystemExit(1)
PY
    docker compose -f "$DEPLOY_DIR/docker-compose.yml" exec -T postgres pg_restore --list <"$destination.partial" >/dev/null
}

dump_database "$sql_dsn" "$dump"
backup_files=("$dump" "$archive")
if [ -n "$log_sql_dsn" ] && [ "$log_sql_dsn" != "$sql_dsn" ]; then
    dump_database "$log_sql_dsn" "$log_dump"
    backup_files+=("$log_dump")
fi

tar -czf "$archive.partial" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
# 导出和校验全部成功后，才发布文件、清理旧备份或同步。
for backup_file in "${backup_files[@]}"; do
    mv "$backup_file.partial" "$backup_file"
    echo "backup: $backup_file（$(du -h "$backup_file" | cut -f1)）"
done

# 只删本脚本生成的、超过保留天数的文件。-mtime +N 是满 N+1 天，所以用 KEEP_DAYS-1。
find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'adtyai-db-*.dump' -o -name 'adtyai-logdb-*.dump' -o -name 'adtyai-data-*.tar.gz' \) \
    -mtime +"$((KEEP_DAYS - 1))" -print -delete

if [ -z "$BACKUP_REMOTE" ]; then
    echo "backup: 没有设 BACKUP_REMOTE，这次没有异地备份" >&2
    exit 1
fi
# 不带 --delete：本机的备份被删或被改坏时，另一台机器上的旧备份还在。那边的保留期在那台机器上自己清理。
rsync -a "${backup_files[@]}" "$BACKUP_REMOTE"
echo "backup: 已同步到 $BACKUP_REMOTE"
