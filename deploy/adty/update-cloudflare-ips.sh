#!/usr/bin/env bash
# 下载 Cloudflare 的网段，校验后更新 Nginx 的 set_real_ip_from 列表，再刷新 ufw 规则（任务 10.1）。
# 每周跑一次，要 root（cron 见 docs/adty/deploy.md）。
# 下载失败、校验不过或 nginx -t 不过：保留旧文件，不 reload，退出码非 0。
set -euo pipefail

CF_IPV4_URL="${CF_IPV4_URL:-https://www.cloudflare.com/ips-v4}"
CF_IPV6_URL="${CF_IPV6_URL:-https://www.cloudflare.com/ips-v6}"
# nginx.conf 里 include 的文件
NGINX_INCLUDE="${NGINX_INCLUDE:-/etc/nginx/cloudflare-ips.conf}"
# 一行一个网段的纯列表，交给 ufw-cloudflare.sh
CF_IP_LIST="${CF_IP_LIST:-/etc/adtyai/cloudflare-ips.txt}"
NGINX_BIN="${NGINX_BIN:-nginx}"
UFW_SCRIPT="${UFW_SCRIPT:-$(dirname "$(readlink -f "$0")")/ufw-cloudflare.sh}"
# 网段数量的下限，防止拿到一个只剩几行的残缺列表。Cloudflare 目前是 15 个 IPv4、7 个 IPv6。
MIN_IPV4="${MIN_IPV4:-5}"
MIN_IPV6="${MIN_IPV6:-3}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() {
    echo "update-cloudflare-ips: $*，保留旧文件" >&2
    exit 1
}

# 下载并整理成一行一个网段：去掉 \r 和首尾空白，丢掉空行和重复行。
# Cloudflare 的两个文件结尾都没有换行；awk 照样处理最后一行，而且每行输出都带换行。
fetch_list() {
    local url="$1" out="$2"
    curl -fsS --retry 3 --max-time 30 "$url" -o "$work/raw" || fail "下载 $url 失败"
    tr -d '\r' <"$work/raw" | awk '{ gsub(/^[ \t]+|[ \t]+$/, "") } $0 != "" && !seen[$0]++' >"$out"
}

# 检查每一行都是合法的 CIDR，打印第一个不合法的行；全部合法时不输出。
# 前缀长度设了下限（IPv4 至少 /8、IPv6 至少 /16），防止一行 0.0.0.0/0 让所有地址都能伪造 IP、绕过防火墙。
validate_list() {
    local family="$1" file="$2"
    awk -v family="$family" '
        function valid_v4(s,    parts, octets, i) {
            if (s !~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\/[0-9]+$/) return 0
            split(s, parts, "/")
            if (parts[2] ~ /^0/ || length(parts[2]) > 2 || parts[2] + 0 < 8 || parts[2] + 0 > 32) return 0
            split(parts[1], octets, ".")
            for (i = 1; i <= 4; i++) {
                if (octets[i] ~ /^0[0-9]/ || length(octets[i]) > 3 || octets[i] + 0 > 255) return 0
            }
            return 1
        }
        function valid_v6(s,    parts, addr, groups, n, i, compressed) {
            if (s !~ /^[0-9A-Fa-f:]+\/[0-9]+$/) return 0
            split(s, parts, "/")
            if (parts[2] ~ /^0/ || length(parts[2]) > 3 || parts[2] + 0 < 16 || parts[2] + 0 > 128) return 0
            addr = parts[1]
            # 单个冒号不能开头或结尾，只有 :: 可以
            if (addr ~ /^:[^:]/ || addr ~ /[^:]:$/) return 0
            compressed = gsub(/::/, "::", addr)
            if (compressed > 1 || addr ~ /:::/) return 0
            if (compressed == 1) {
                if (addr == "::") return 1
                sub(/::/, ":", addr)
                sub(/^:/, "", addr)
                sub(/:$/, "", addr)
            }
            n = split(addr, groups, ":")
            if (compressed == 0 && n != 8) return 0
            if (compressed == 1 && n > 7) return 0
            for (i = 1; i <= n; i++) {
                if (groups[i] !~ /^[0-9A-Fa-f]+$/ || length(groups[i]) > 4) return 0
            }
            return 1
        }
        {
            ok = (family == "4") ? valid_v4($0) : valid_v6($0)
            if (!ok) { print; exit }
        }
    ' "$file"
}

fetch_list "$CF_IPV4_URL" "$work/v4"
fetch_list "$CF_IPV6_URL" "$work/v6"

for family in 4 6; do
    bad="$(validate_list "$family" "$work/v$family")"
    [ -z "$bad" ] || fail "IPv$family 列表里有不合法的行：$bad"
done

count_v4="$(wc -l <"$work/v4")"
count_v6="$(wc -l <"$work/v6")"
[ "$count_v4" -ge "$MIN_IPV4" ] || fail "IPv4 只有 $count_v4 个网段，少于 $MIN_IPV4"
[ "$count_v6" -ge "$MIN_IPV6" ] || fail "IPv6 只有 $count_v6 个网段，少于 $MIN_IPV6"

cat "$work/v4" "$work/v6" >"$work/list"
{
    echo "# 由 update-cloudflare-ips.sh 生成，不要手工改。来源：$CF_IPV4_URL、$CF_IPV6_URL"
    awk '{ print "set_real_ip_from " $0 ";" }' "$work/list"
} >"$work/include"

# 先在同一目录写临时文件再 mv，替换是原子的
replace_file() {
    local src="$1" dest="$2"
    mkdir -p "$(dirname "$dest")"
    cp "$src" "$dest.new"
    chmod 644 "$dest.new"
    mv -f "$dest.new" "$dest"
}

if cmp -s "$work/include" "$NGINX_INCLUDE"; then
    echo "update-cloudflare-ips: 网段没有变化（IPv4 $count_v4 个，IPv6 $count_v6 个）"
else
    had_old=0
    if [ -f "$NGINX_INCLUDE" ]; then
        cp -p "$NGINX_INCLUDE" "$work/include.old"
        had_old=1
    fi
    replace_file "$work/include" "$NGINX_INCLUDE"
    if ! "$NGINX_BIN" -t -q; then
        if [ "$had_old" = 1 ]; then
            mv -f "$work/include.old" "$NGINX_INCLUDE"
        else
            rm -f "$NGINX_INCLUDE"
        fi
        fail "nginx -t 没有通过"
    fi
    "$NGINX_BIN" -s reload
    echo "update-cloudflare-ips: 已更新 Nginx（IPv4 $count_v4 个，IPv6 $count_v6 个）"
fi

replace_file "$work/list" "$CF_IP_LIST"
"$UFW_SCRIPT" "$CF_IP_LIST"
