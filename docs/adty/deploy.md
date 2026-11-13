# 部署：Docker Compose、Nginx 和 Cloudflare

单台服务器：Cloudflare 小云朵 → 本机 Nginx（443）→ `127.0.0.1:3000` 上的 adtyai-api 容器。正式运行和验收只使用 PostgreSQL，不使用 SQLite 或 MySQL。不设最低版本或最高版本。建议版本为 PostgreSQL 17，compose 示例优先使用 17，但不把 16、18 或其他版本排除在验收之外。Redis 也在 compose 里，不对外开端口。已有数据目录升级主要版本时，必须另建目录迁移并核对账目，不能直接换镜像启动。文件都在 `deploy/adty/`（任务 10.1）：

| 文件 | 用途 |
| --- | --- |
| `docker-compose.yml` | adtyai-api、postgres、redis 三个服务，固定子网 172.30.0.0/24 |
| `.env.example` | 环境变量模板，复制成 `.env` 填密钥 |
| `nginx.conf` | Nginx 站点配置：真实 IP、WebSocket、流式、100m 请求体；另有一个接收分站域名的 server 块（第 11 节） |
| `update-cloudflare-ips.sh` | 下载 Cloudflare 网段，更新 Nginx 的 include 文件，再调 `ufw-cloudflare.sh` |
| `ufw-cloudflare.sh` | 443 只放行 Cloudflare 网段，保留 SSH |
| `backup.sh` | 每天 pg_dump 加打包 data 目录，本机留 14 天，同步到另一台机器 |

这份文档还没有在真实服务器上完整走过一遍。第一次部署时按顺序做，和实际情况不一致的地方改进本文。

## 1. 准备服务器

- 系统用 Debian 12 或 Ubuntu 24.04。装 Docker Engine 和 Compose 插件（按 Docker 官方文档装 `docker-ce`、`docker-compose-plugin`），再装 `nginx`（1.19.4 以上，`ssl_reject_handshake` 要用）、`ufw`、`curl`、`rsync`、`python3`、`libpq5`（后两项供备份解析 PostgreSQL 连接串）。
- 云服务商的安全组只开 22 和 443。80 不用开：Cloudflare 那边打开「始终使用 HTTPS」，回源只走 443。
- 部署目录是 `/opt/adtyai`，把仓库里 `deploy/adty/` 下的文件都放进去：

```bash
sudo mkdir -p /opt/adtyai && cd /opt/adtyai
# 从仓库拷 docker-compose.yml .env.example nginx.conf *.sh 到这里
sudo chmod +x *.sh
sudo cp .env.example .env && sudo chmod 600 .env
```

## 2. 填 .env

- 密码和密钥用 `openssl rand -hex 32` 生成。十六进制只有 0-9a-f，放进 `SQL_DSN`、`REDIS_CONN_STRING` 网址里不用转义。
- `SESSION_SECRET`、`CRYPTO_SECRET` 分开设，设了就不要再改：换了会让所有登录会话、推广 cookie 签名、防刷指纹和任务产物的签名失效。不设 `CRYPTO_SECRET` 时它等于 `SESSION_SECRET`，以后想换 `SESSION_SECRET` 就会连带出问题，所以一开始就分开。
- `.env` 另存一份到密码管理器。备份脚本不备份 `.env`，恢复时要用它。
- Cloudflare Token 和 Turnstile widget secret 的数据库加密要求显式设置至少 32 字节的 `CRYPTO_SECRET`（建议上述随机 64 位十六进制），不能使用临时生成或会话默认值。所有节点及恢复环境必须使用同一个 key。历史明文首次载入自动加密；key 错误或密文损坏会停用本节点此域凭证并记录通用错误。恢复原 key 后重载配置，或由 Root 重新录入有效凭证；不得把密文当令牌填回界面。外部令牌泄露时先在 Cloudflare 撤销，再由 Root 保存替代值；不要直接修改 `CRYPTO_SECRET`，它还用于其他签名。旧明文备份仍须按敏感文件保管，不能因新版本加密就公开。
- `LOG_SQL_DSN` 可选，留空时日志使用主库；设置时必须是 PostgreSQL，不限定主版本，且 API 容器和用于备份的 postgres 客户端容器均能访问它。备份按 API 容器实际的 `SQL_DSN` 和 `LOG_SQL_DSN` 导出，不假设连接是 compose 默认库。
- compose 里固定写好的：`SESSION_COOKIE_SECURE=true`（和 `SESSION_COOKIE_TRUSTED_URL` 必须同时设，否则启动失败）、`ERROR_LOG_ENABLED=true`（默认关，不开就不记错误日志）、`BATCH_UPDATE_ENABLED=true`、`TRUSTED_PROXIES=172.30.0.1`。

## 3. 选镜像标签，从 GHCR 拉取

- 生产环境只用 sha 标签，不用 `latest` 或 `main`。`.github/workflows/adty-ghcr.yml` 每次推送 `main` 都会推一个 `ghcr.io/adtyai/adtyai-api:sha-<提交的前 7 位>`。在 GitHub 仓库的 Packages → adtyai-api 里能看到所有标签；也可以在 `main` 上跑 `git rev-parse --short=7 HEAD` 得到。确认这次提交的 Actions 已经跑完、镜像推上去了。
- 把 `.env` 里的 `ADTY_IMAGE=ghcr.io/adtyai/adtyai-api:sha-xxxxxxx` 换成真实标签，比如 `sha-1a2b3c4`。
- 包是公开的：直接拉，不用登录。
- 包是私有的：用 GitHub 账号建一个只有 `read:packages` 权限的 personal access token (classic)（GitHub 的容器仓库只认 classic 令牌），在服务器上登录一次：

```bash
echo '<令牌>' | sudo docker login ghcr.io -u <GitHub 用户名> --password-stdin
```

## 4. 启动和确认网关

```bash
cd /opt/adtyai
sudo docker compose pull
sudo docker compose up -d
sudo docker compose ps          # 三个服务都是 healthy
curl -s http://127.0.0.1:3000/api/ready | head -c 200
sudo docker network inspect adtyai_default --format '{{range .IPAM.Config}}{{.Subnet}} {{.Gateway}}{{end}}'
```

- 最后一条应该输出 `172.30.0.0/24 172.30.0.1`。Nginx 连 `127.0.0.1:3000` 时，容器看到的来源就是这个网关，所以 `TRUSTED_PROXIES` 只写它。
- 如果 172.30.0.0/24 和机器上已有的网络冲突（`up` 时报 `Pool overlaps`），在 `docker-compose.yml` 里把 `subnet`、`gateway` 和 `TRUSTED_PROXIES` 一起改成另一个网段。
- `sudo docker compose logs adtyai-api` 里不应该有 `TRUSTED_PROXIES is unset` 的警告。
- 第一次启动后系统处于未初始化状态，谁先打开网页完成初始化向导，谁就是超级管理员。所以在配 Nginx 之前完成它：在自己电脑上 `ssh -L 3000:127.0.0.1:3000 <服务器>`，浏览器打开 `http://localhost:3000` 只建好超级管理员。其余后台设置等第 7 节验证通过后，用正式域名登录再按上线检查清单改（任务 10.2）：`SESSION_COOKIE_SECURE=true` 时刷新和退出登录只认 `SESSION_COOKIE_TRUSTED_URL` 里的地址。

## 5. Nginx、Cloudflare 网段和防火墙

1. 源服务器证书：Cloudflare 面板 → SSL/TLS → 源服务器 → 创建证书，主机名填 `adtyai.com` 和 `*.adtyai.com`。证书存成 `/etc/ssl/cloudflare-origin.pem`，私钥存成 `/etc/ssl/cloudflare-origin.key`，私钥 `chmod 600`。证书和私钥不进仓库。
2. 第一次跑网段脚本。这时 Nginx 还是装好时的默认配置、正在运行；脚本生成 `/etc/nginx/cloudflare-ips.conf` 和 `/etc/adtyai/cloudflare-ips.txt`，reload Nginx，再配 ufw：

```bash
sudo /opt/adtyai/update-cloudflare-ips.sh
sudo ufw status verbose        # 22 放行；443 只有带 adtyai-cloudflare 备注的规则
```

   - SSH 不是 22 端口时先 `export SSH_PORT=<端口>` 再跑，否则启用 ufw 后会连不上。
   - 脚本提示「还有对所有地址开放 443 的规则」时，用 `sudo ufw status numbered` 找到 `Nginx Full`、`443` 这类 `Anywhere` 规则删掉。
   - Docker 发布的端口不经过 ufw。compose 里只有 `127.0.0.1:3000`，外面连不上；不要再给 postgres、redis 加 `ports`。
3. 装站点配置：

```bash
sudo cp /opt/adtyai/nginx.conf /etc/nginx/conf.d/adtyai.conf
sudo nginx -t && sudo systemctl reload nginx
```

4. 定时任务，写进 `/etc/cron.d/adtyai`（`BACKUP_REMOTE` 见第 9 节）：

```
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
BACKUP_REMOTE=backup@backup-host:/srv/backup/adtyai/
# 每周一更新 Cloudflare 网段和防火墙
17 4 * * 1 root /opt/adtyai/update-cloudflare-ips.sh >>/var/log/adtyai-cloudflare.log 2>&1
# 每天备份
30 3 * * * root /opt/adtyai/backup.sh >>/var/log/adtyai-backup.log 2>&1
```

网段脚本的行为：从 https://www.cloudflare.com/ips-v4 和 https://www.cloudflare.com/ips-v6 下载（两个文件结尾都没有换行，脚本不会漏掉最后一行），每行都校验是合法的 IPv4 或 IPv6 网段，数量不能太少；然后替换 include 文件并 `nginx -t`，不通过就换回旧文件。下载失败、校验不过、`nginx -t` 不过时都保留旧文件、不 reload、退出码非 0，日志里能看到原因。网段没变时不 reload Nginx，但照样刷新一遍 ufw。

robots.txt 由网关按站点返回（任务 11.3）：主站和开了 SEO 的分站只屏蔽 `/api/`、`/v1/`、`/pay/`，没开 SEO 的分站整站不收录，Nginx 不用另配。sitemap 另外在 Nginx 上配，只放首页的 8 个网址。

## 6. Cloudflare 面板

- DNS：`adtyai.com`、`api.adtyai.com` 的记录开小云朵（已代理）。
- SSL/TLS：模式选「完全（严格）」，源站用上面的源服务器证书；打开「始终使用 HTTPS」。
- 缓存：不要给 `/api/*`、`/v1/*` 设「缓存所有内容」之类的缓存规则。Cloudflare 默认不缓存接口返回的 JSON，保持默认即可。
- 安全：不开 Bot Fight Mode，不开「Under Attack」模式。Claude Code、Codex 这类客户端过不了浏览器质询，请求会被拦。
- 网络：「伪 IPv4」保持关闭，否则 Cloudflare 会改写 `CF-Connecting-IP`；WebSockets 保持打开。

## 7. 验证真实 IP

两层信任：Nginx 只相信从 Cloudflare 网段来的 `CF-Connecting-IP`，再用覆盖的方式写 `X-Forwarded-For`；adtyai-api 只相信 compose 网关（Nginx）。

1. 在用户设置里打开「记录 IP」。
2. 手机关掉 Wi-Fi，用流量访问网站并登录，调一次接口。
3. `sudo tail /var/log/nginx/access.log` 和后台使用日志里记的应该是手机的 IP，不是 104.16.x.x、172.64.x.x 这类 Cloudflare 的地址，也不是 172.30.0.1。
4. 伪造测试：对已经决定的主站 Origin 发 `curl -H 'X-Forwarded-For: 1.2.3.4' https://panel.example.com/api/status`，访问日志里记的是你自己的 IP，不是 1.2.3.4。
5. 绕过 Cloudflare 测试：在别的机器上 `curl -k --max-time 10 https://<源站 IP>/`，应该超时（被 ufw 拦）。

## 8. 524 超时

- 经过小云朵的请求，源站 125 秒内没有返回任何内容，Cloudflare 就回 524。这个时间只有企业版能改；Nginx 的 `proxy_read_timeout 600s` 管不到这一层。
- 流式请求一直在输出，一般不受影响。推理模型等首字可能超过 125 秒：在后台「通用设置」打开 ping，间隔设 30 秒（上线检查清单）。
- 受影响的主要是非流式的长请求（长篇输出、图片生成）。接入文档里提示用户尽量用流式。如果这类请求多了，再单独开一个灰云（只做 DNS 解析）的 API 子域名，最好放在另一台机器上，不暴露主站 IP。
- 上传大小：Nginx 设了 `client_max_body_size 100m`；Cloudflare 免费版和专业版单个请求最大 100 MB。

## 9. 备份和恢复

`backup.sh` 从运行中的 API 读取数据库连接，用 PostgreSQL 的 `pg_dump --format=custom` 导出主库；配置了不同的 `LOG_SQL_DSN` 时，同一时间戳另存 `adtyai-logdb-<时间>.dump`。客户端主版本应能读取该库，升级主要版本时另建目录迁移，不能直接换镜像启动。日志连接为空或与主库连接完全相同时只导出一次；同一库的等价但不同写法会安全地多导出一份。每份归档经 `pg_restore --list` 校验，再打包 `/opt/adtyai/data`（返佣收款码、打款凭证等上传文件）。全部成功才发布文件、清理本机超过 14 天的三类归档并同步到 `BACKUP_REMOTE`；任何导出或校验失败都不发布和复制不完整文件组，也不清理旧备份。

数据库密码经 libpq 分离，只通过 stdin 和容器内 `PGPASSWORD` 传递，避免进入命令参数或日志。宿主机必须有 `python3` 和 `libpq5`。应用 pgx 的 `search_path` 等服务器参数会转成 PostgreSQL `options`，已有 `options` 保留；仅忽略 pgx 的语句/描述缓存容量和查询执行模式，这些不适用于 `pg_dump`。`database` 别名和 `servicefile` 会转换为 libpq 的对应配置。客户端证书、service 文件使用已有只读挂载路径；脚本拒绝含 `sslpassword`、`krbspn` 或显式协议版本限制（`min_protocol_version`/`max_protocol_version`）的连接，不静默降低认证或协议要求。脚本不会保存 `.env`、数据库角色或 TLS 证书，这些须独立安全保管。

各库的在线转储分别是一致快照，不能据此认定跨库及上传文件处于同一业务时刻。需要精确恢复点时，先阻断新业务写入、支付回调和后台任务并排空在途请求，保持 API 存活供刷盘和读取配置；备份成功后再停止 API。多节点须逐节点排空及刷盘。日常在线备份仍应配合数据库持续归档和明确的恢复点目标。

- `BACKUP_REMOTE` 是另一台机器的 rsync 目标。服务器上 root 的 SSH 公钥要先加到那台机器的备份账号里。没设时本机备份照做，但退出码非 0，日志里会提示。
- 同步不带 `--delete`：本机的备份被删或被改坏时，那边的旧备份还在。那台机器自己定期清理旧文件。
- 可以用环境变量改：`BACKUP_DIR`、`KEEP_DAYS`、`DEPLOY_DIR`、`DATA_DIR`。

恢复（上线前演练一次，最好在另一台机器上）：

```bash
cd /opt/adtyai
sudo docker compose stop adtyai-api
sudo docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --exit-on-error' \
  </var/backups/adtyai/adtyai-db-<时间>.dump
# 配置了独立日志库时，必须在启动 API 前恢复同时间戳的日志归档，见下文。
sudo tar -xzf /var/backups/adtyai/adtyai-data-<时间>.tar.gz -C /opt/adtyai   # 解出 /opt/adtyai/data
```

上述主库命令用于 compose 默认主库。自定义主库或独立日志库须连接各自的新建空库，不能把两份 dump 恢复到同一个库，也不能把下面命令直接用于已有业务表的库。保留原库，另建空库恢复及核对，再在停写状态下切换连接。使用版本 17 的客户端，在 root 专属、权限 600 的 `/root/adty-restore.pg_service.conf` 配置 `[adty-main]` 和 `[adty-log]` 的目标 host、port、dbname、user、password、sslmode；该文件不进入仓库或日志。确认两个目标均为空后分别执行：

```bash
sudo env PGSERVICEFILE=/root/adty-restore.pg_service.conf \
  /usr/lib/postgresql/17/bin/pg_restore --dbname=service=adty-main --no-owner --exit-on-error \
  </var/backups/adtyai/adtyai-db-<时间>.dump
sudo env PGSERVICEFILE=/root/adty-restore.pg_service.conf \
  /usr/lib/postgresql/17/bin/pg_restore --dbname=service=adty-log --no-owner --exit-on-error \
  </var/backups/adtyai/adtyai-logdb-<时间>.dump
```

恢复完成后核对用户活余额（quota 加 quota_pending）、订单、账本、消费日志、审计事件及上传文件，再执行 `sudo docker compose start adtyai-api` 启动全部 API 节点。Redis 只保留兼容投影，恢复时应失效本应用相关缓存，不能清空共享 Redis。

在新机器上恢复时，先放好 `.env`（同样的 `SESSION_SECRET`、`CRYPTO_SECRET`、`POSTGRES_PASSWORD`），`docker compose up -d postgres redis` 起好数据库，再按上面的步骤恢复。

## 10. 升级和回滚

应用版本通过 `.env` 里的 sha 标签选择。涉及额度持久化差额和令牌缓存协议的首次升级，必须先完成下面的停写切换，再启动新标签：

1. 阻止新的模型请求和管理写入，将支付回调切到可重试的维护入口，暂停会写入金额的定时任务。等待所有节点的请求、流、异步结算和退款事务结束；不能只停止负载均衡转发后立即杀进程。
2. 在每个旧节点仍存活时，逐节点调用 `POST /api/adty/internal/flush-batch`，使用该部署的 `X-Adty-Flush-Token`，确认 HTTP 和 JSON 都成功；等在途工作清零后再次刷盘。旧版未提交的进程内差额不能由新程序自动找回。节点已经丢失而差额未确认落库时，先对账恢复，不能继续切换或宣称余额完整。
3. 停止全部旧应用节点，保留数据库和 Redis。在停写状态下备份数据库、单独配置的日志库以及上传目录，记录旧标签和合成演练过的恢复步骤。所有节点必须一起切换；新旧金额或令牌缓存协议不支持滚动混跑。
4. 先以 `NODE_TYPE=master` 启动一个新节点完成前向迁移，分别检查主库和单独配置的日志库；slave 不执行这两类迁移。确认用户和令牌的 `quota_pending` 列为 bigint、非空、默认值为 0。迁移必须保留已提交的非零差额；启用 batch 的完整应用启动允许将差额按值精确一次折入原始额度，并保持活余额和令牌已用量一致，重复启动不能再次扣减。旧库额度列若来自 32 位结构，启动预检会拒绝，须先完成单独验证过的额度列迁移，不能跳过预检。迁移和刷盘出错时保持维护状态。
5. 所有节点统一到新标签后，检查用户活余额、令牌剩余/已用、账本和订单；确认没有待刷差额，再恢复定时任务、回调和业务写入。缓存失效只清理本应用的相关键，不能清空共享 Redis 实例。

以下是应用标签切换命令，不代替上述停写、排空、备份和账目核验：

```bash
cd /opt/adtyai
sudo ./backup.sh                                  # 升级前先备份
sudo sed -i 's|^ADTY_IMAGE=.*|ADTY_IMAGE=ghcr.io/adtyai/adtyai-api:sha-<新标签>|' .env
sudo docker compose pull adtyai-api
sudo docker compose up -d adtyai-api
sudo docker compose ps adtyai-api                 # 等到 healthy
```

- 回滚同样先停写、排空全部节点。新协议已经处理过业务时，不能直接启动不认识 `quota_pending` 的旧版本；先用新版本刷完差额并核对，再使用已在隔离环境验证过的兼容回滚方案。
- 新库已有写入后不能直接恢复升级前备份或切回旧数据卷，这会丢失新订单、余额和会话变更。数据库 16 → 17 使用独立新目录迁移，保留旧卷；应用迁移是前向迁移，不能仅凭“只加列”断言旧应用兼容。

## 11. 站长分站（P11）

分站有两种域名：平台子域名 `<子域名>.<分站根域名>`，和站长自己的自定义域名。两种都走同一台机器、同一个网关，网关按请求的 Host 查站点表识别是哪个站（任务 11.2），Nginx 只要把 Host 原样传过去。下面用 `sites.example.com` 代表分站根域名。

### Cloudflare

1. 把分站根域名放进 Cloudflare，和主站同一个账号。
2. DNS 加泛解析：`*.sites.example.com` 指向源站，开小云朵。根域名本身按需要加。
3. 在这个 zone 上开 Cloudflare for SaaS（SSL/TLS → 自定义主机名），建两条记录，都开小云朵：
   - 回源主机名，例如 `origin.sites.example.com`，指向源站。在「自定义主机名」页把它设为回源（fallback origin），状态要变成 active。
   - CNAME 目标，例如 `customers.sites.example.com`，CNAME 到回源主机名。站长的域名 CNAME 到它。
4. 后台「分站管理 → 设置」里填：Cloudflare API Token、Zone ID、Account ID、CNAME 目标、回源主机名、Turnstile widget（id、site key、secret）。Token 要的权限：这个 zone 的 `SSL and Certificates Write`，账号的 `Turnstile Sites Write`。这几项都填好之前，「自定义域名」开关打不开。
5. Turnstile：第一个 widget 的域名列表里登记主站域名和分站根域名（一个域名连带它的子域名一起授权，平台子域名不用逐个加）。自定义域名生效后，网关会自动登记进还有空位的 widget；免费版每个 widget 最多 10 个域名、每个账号最多 20 个 widget。

### 源服务器证书

主站域名和分站根域名各一张 Cloudflare Origin CA 证书：

- 主站：`adtyai.com`、`*.adtyai.com`，存成 `/etc/ssl/cloudflare-origin.pem` 和 `.key`（第 5 节）。
- 分站：`sites.example.com`、`*.sites.example.com`，存成 `/etc/ssl/cloudflare-origin-sites.pem` 和 `.key`，私钥 `chmod 600`。

自定义域名回源时，TLS 握手的 SNI 是回源主机名，所以分站这张证书就能用，不用给每个站长的域名单独发证书。

### Nginx

`nginx.conf` 末尾的第二个 `server` 块接收主站以外的所有 Host，配置和主站一样（真实 IP、透传、超时、100m 请求体），另外透传 `CF-IPCountry`（营销同意的默认值按它判断，任务 12.1）。

- `server_name` 写成正则 `~.`，匹配任意 Host。nginx 里单写 `_` 只是一个匹配不到的名字，收不到请求。
- 代价：带任意 SNI 的握手都会拿到分站证书，不再被第一个 `server` 块的 `ssl_reject_handshake` 拒掉；不带 SNI（用 IP 直连）的照样拒。443 只对 Cloudflare 网段开放（第 5 节的 ufw 规则），所以可以接受。这个块必须在 ufw 规则生效之后再加载。
- `proxy_set_header Host $host` 不能改，网关靠它识别站点。
- 改完 `sudo nginx -t && sudo systemctl reload nginx`。

### 其他

- `SESSION_COOKIE_TRUSTED_URL` 不用加分站的地址：会话 cookie 本来就按 Host 写，同站请求放行（任务 11.2 已说明）。
- 分站 Logo 存在 `/data/uploads/site-assets`，在 `./data` 卷里，跟着 `backup.sh` 每天的 data 打包一起备份。
- 后台的服务器地址（ServerAddress）仍填主站地址：支付回调、支付跳转都走主站（执行层决定 30）。
