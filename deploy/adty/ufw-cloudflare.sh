#!/usr/bin/env bash
# 让 443 只对 Cloudflare 的网段开放，保留 SSH（任务 10.1）。要 root。
# 用法：ufw-cloudflare.sh [网段列表文件]，列表一行一个网段，默认是 update-cloudflare-ips.sh 写的 /etc/adtyai/cloudflare-ips.txt。
# 只增删带「adtyai-cloudflare」备注的规则，别的规则不动。重复运行结果一样。
set -euo pipefail

CF_IP_LIST="${1:-${CF_IP_LIST:-/etc/adtyai/cloudflare-ips.txt}}"
SSH_PORT="${SSH_PORT:-22}"
TAG="adtyai-cloudflare"

[ -s "$CF_IP_LIST" ] || {
    echo "ufw-cloudflare: 网段列表 $CF_IP_LIST 不存在或是空的，先跑 update-cloudflare-ips.sh" >&2
    exit 1
}
mapfile -t wanted < <(awk 'NF { print $1 }' "$CF_IP_LIST")
[ "${#wanted[@]}" -gt 0 ] || {
    echo "ufw-cloudflare: 网段列表 $CF_IP_LIST 里没有网段" >&2
    exit 1
}
declare -A is_wanted=()
for cidr in "${wanted[@]}"; do
    is_wanted["$cidr"]=1
done

# 先放行 SSH 再启用 ufw，不会把自己锁在外面
ufw allow "$SSH_PORT/tcp" comment "ssh"
ufw default deny incoming
ufw default allow outgoing

for cidr in "${wanted[@]}"; do
    ufw allow proto tcp from "$cidr" to any port 443 comment "$TAG" >/dev/null
done

# Cloudflare 撤掉的网段：删掉我们以前加的规则。ufw show added 的行形如
#   ufw allow proto tcp from 173.245.48.0/20 to any port 443 comment 'adtyai-cloudflare'
while read -r cidr; do
    [ -n "$cidr" ] || continue
    if [ -z "${is_wanted[$cidr]:-}" ]; then
        ufw delete allow proto tcp from "$cidr" to any port 443 >/dev/null
        echo "ufw-cloudflare: 删除已不在列表里的网段 $cidr"
    fi
done < <(ufw show added | awk -v tag="'$TAG'" '$0 ~ tag && $2 == "allow" { for (i = 1; i < NF; i++) if ($i == "from") print $(i + 1) }')

ufw --force enable >/dev/null

# 别的规则如果对所有地址开了 443（比如 ufw allow 'Nginx Full'、ufw allow 443），只放行 Cloudflare 就没有意义了。
# 这类规则不是本脚本加的，只提醒，不删。
status="$(ufw status)"
open_to_all="$(printf '%s\n' "$status" | grep -E '(^|[^0-9])443([^0-9]|$)|Nginx (Full|HTTPS)' | grep -v "$TAG" | grep 'Anywhere' || true)"
if [ -n "$open_to_all" ]; then
    echo "ufw-cloudflare: 警告：还有对所有地址开放 443 的规则，请用 ufw status numbered 查看并删掉" >&2
fi

echo "ufw-cloudflare: 443 只放行 ${#wanted[@]} 个 Cloudflare 网段，SSH 端口 $SSH_PORT 保持放行"
