# 全局约定（每个任务都遵守）

抄自实施方案 WhiteRelay（文中称「本计划」）v2.5 的第三节，标「v2.5」的是这一版改过的地方。文中的任务号和执行层决定都指本计划，「设计稿」指本计划依据的设计文档；这两份文档都不在仓库里。和原文只有一处不同：仓库是公开的，所以本机路径改成了通用写法（「Windows 仓库」、`<旧项目目录>`、`~/adtyai/...` 这类），也去掉了本机的用户名。

**约定 A：代码放哪、怎么命名**
- 后端新文件以 `adty_` 开头（`model/adty_*.go`、`service/adty_*.go`、`controller/adty_*.go`、`middleware/adty_*.go`），配置放 `setting/adty_setting/`。从旧项目移植的文件沿用旧名（如 `service/bepusdt.go`、`model/referral.go`），方便对照。
- 新表名 `adty_*`；移植的返佣表沿用 `referral_*`。所有新表在 `model/adty_migrate.go` 的 `migrateAdtyTables` 里登记，只在主节点执行（上游迁移就只在主节点跑，model/main.go:215-222）。
- 新接口一律在 `router/adty-router.go` 的 `SetAdtyRouter`（router 包，可直接用未导出的 `handlePermissionRoute`）里注册。全新接口用 `/api/adty/...`；移植的沿用旧项目路径（`/api/user/bepusdt/*`、`/api/user/referral/*`、`/api/user/admin/referral/*`、`/api/user/admin/finance/*`、`/api/referral-assets/*path`）；易支付回调用 `/api/user/epay/notify/:channelId`（和上游的 `/api/user/epay/notify` 不冲突，gin 静态路由优先）。
- 每个接口都登记访问令牌规则（执行层决定 14）。删接口时同步删 `middleware/access_token_routes.go` 里的规则。
- 前端新功能放 `web/src/features/adty-*`；钱包里的新组件放 `web/src/features/wallet/adty/`；移植的沿用 `features/referral`、`features/admin-referral`、`features/recharge-audit`。
- 脚本放 `scripts/adty/`，工程文档放 `docs/adty/`。本计划、设计稿和调研材料不进仓库。
- 从旧项目取文件只读：在 WSL 的任务脚本里 `git -C <旧项目目录> show main:<路径>`（旧项目目录是旧项目在 Windows 上的本地仓库，WSL 里经 `/mnt/c/` 下的对应路径访问；实测能读，当前是 b3c2552f0），输出进日志再读。Windows 的 git 也能读，但 PowerShell 会把中文显示成乱码。不在旧项目里做任何修改。

**约定 B：改上游文件**
- 只加挂载点或做删除，不顺手重构。每改一个上游文件（包括上游的测试文件），在 `docs/adty/upstream-touchpoints.md` 记一行：文件、改了什么、为什么。
- 主要挂载点：`router/api-router.go`（`SetApiRouter` 末尾一行 `SetAdtyRouter(apiRouter)`）、`main.go`（`model.InitDB()` 后一行 PostgreSQL 检查，`authz.Init` 前一行开启后台权限，`go model.SyncOptions` 后一行 `service.StartAdty()`）、`model/main.go`（一行 `migrateAdtyTables`）、`middleware/auth.go`（一行后台权限检查）、`controller/user.go`（注册、ManageUser、UpdateUser、登录响应各一行钩子）、`controller/option.go`（一段分组校验）、`model/log.go`（日志投影、TPM 计数）、`model/topup.go`（一行拒绝）、几个中转路由文件（限流）、`i18n/i18n.go`（加载新文件和新语言）、`web/src/styles/theme.css`（主题）。其余见各任务。
- 前端语言包只增不删；后端上游 yaml 不动（执行层决定 13）。
- `web/src/routeTree.gen.ts` 由构建生成并已入库。增删路由后先跑 `bun run build` 重新生成，用 `git grep` 确认旧路由不在了，再跑检查，和改动一起提交。它带 `// @ts-nocheck`，类型检查发现不了里面的旧引用。冲突时也重新生成，不手工合。

**约定 C：钱**
- 计算一律用 `github.com/shopspring/decimal`（go.mod 已有），不用 float 直接加减和比较。新表金额列写 `type:decimal(20,8);not null;default:0`；decimal 读到 NULL 会报错，LEFT JOIN 查询要 COALESCE。
- 余额沿用上游额度整数：1 USD = `QuotaPerUnit`（默认 500000，common/constants.go:22）。入账额度 = (面额 + 加赠) × QuotaPerUnit，先 `.Floor()` 再转成整数。退款只退没花掉的本金：按用户当前余额扣回这笔订单还剩的额度，最多扣到 0，赠金不退，规则和算例见 [features.md](features.md) 的「充值退款」。
- 余额不能变负：每个钱包请求都要先预扣，余额不够就拒绝；结算补扣最多扣到 0，差额由平台承担（见 features.md 的「余额下限」）。新写的扣费代码要用 `model.ChargeUserQuotaUpTo`，不要用 `model.DecreaseUserQuota`。
- 显示：美元、人民币保留 2 位；USDT 按网关返回的位数原样显示。
- 支付商名：`bepusdt`、`adty_epay`。支付方式：`usdt.trc20`、`usdt.bep20`、`usdt.polygon`、`usdt.xlayer`、`alipay`、`wxpay`。
- 订单状态沿用上游 `top_ups.status` 的 `pending`、`success`、`expired`、`failed`（common/constants.go:275-278），新增 `canceled`、`refunded`。注意表名是 `top_ups`。
- 后台「货币显示」设为 USD（上线清单）。TOKENS 显示模式会改变金额含义（controller/topup.go:154-158），这时报价接口直接拒绝。

**约定 D：权限和密钥**
- 我们的后台接口一律用 `handlePermissionRoute(adminGroup, 方法, 路径, authz.AdtyXxx, 处理函数)` 注册，`adminGroup` 挂 `middleware.AdminAuth()`；权限清单见任务 2.6，文中写作「区域·动作」。只有超级管理员能做的（系统设置里的页面、易支付通道的新增和密钥、管理员任免）用 `middleware.RootAuth()`（middleware/auth.go:144），访问令牌规则登记为 `option:read` 或 `option:write`。
- 存进 options 的密钥，键名必须以 `secret`、`Secret`、`Key`、`Token` 或 `api_key` 结尾，`GET /api/option/` 才会过滤掉（controller/option.go:92-96）。易支付通道的商户密钥存在通道表的 `merchant_key` 列（列名用 merchant_key，不用笼统的 key），字段带 `json:"-"`。
- 仓库是公开的：密钥、证书、服务器地址都不进仓库。
- AGPL：不删 LICENSE、NOTICE、THIRD-PARTY-LICENSES.md 和上游源文件头（`bun run copyright:check` 会查），不自称官方。公开页脚不放上游署名、源码链接和客服邮箱。

**约定 E：文案和语言**
- 内容的语言键统一用 `zh-CN`、`zh-TW`、`en`、`fr`、`ja`、`ru`、`vi`。前端 i18next 用的是 `zhCN`、`zhTW`（i18n/config.ts:32-40），用 `web/src/lib/adty-lang.ts` 的 `toContentLang()` 转换，两套不要混用。
- 控制台接口的提示用 `common.ApiErrorT(c, "英文原文")`，译文放前端语言包，键就是英文原文。新键**先写进 `en.json`**（值同键），再写 `zh.json`。如果只写了 zh.json，`i18n:sync` 会把 zh 选成基准，把中文写进 en.json。
- 中转接口（/v1）的报错和邮件用 `i18n.T` / `i18n.Translate`，键常量写 `i18n/keys_adty.go`（`MsgAdtyXxx = "adty.xxx"`），译文写 `i18n/locales/adty.<语言>.yaml`（执行层决定 13）。7 种语言一次写齐（任务 9.1 起后端已有 7 种）。
- 改了上游 `t()` 原文的地方，按新文案加新键，旧键留着。
- 站名写 ADTY AI，7 种语言都不翻译。代码里引用 `common.AdtyBrandName` 和前端的 `ADTY_BRAND_NAME`，不要另写字面量（任务 8.9）。
- 对外名字（任务 8.9）：访客、调用 API 的人、上游服务商和浏览器开发者工具能看到的 New API 已经换成 ADTY AI 或 adtyai（后端常量 `common.AdtyBrandName`、`AdtyBrandSlug`、`AdtyBrandURL`）。品牌守卫测试 `common/adty_brand_guard_test.go` 和 `web/src/features/adty-guards/__tests__/brand.test.ts` 会报出合并上游时新带进来的旧写法。**下面这些不改**，守卫的允许名单也只放它们：
  - Go 模块路径和 import 路径（`github.com/QuantumNous/new-api/...`）；
  - 镜像里的二进制文件名 `/new-api`（Dockerfile）；
  - Redis 键和锁的名字（如 `new-api:wsmanager:...`、`new-api:channel_affinity:...`）、服务端日志、`--help` 的文字，这些都只在服务器上；
  - 渠道类型「New API」（type 60，指另一种上游）；
  - 模型元数据、插件市场、系统更新检查用到的上游地址（后台功能要用）；
  - 只有后台看得到的占位文字；
  - `features/chat/lib/send-to-fluent.ts` 里的容器 id（第三方插件的对接标识）；
  - 源码版权头、LICENSE、NOTICE、THIRD-PARTY-LICENSES.md、上游的 README.md（GitHub 优先显示 `.github/README.md`，那里写明本仓库是 QuantumNous/new-api 的修改版）；
  - 公开页脚和关于页空状态已经去掉上游署名和仓库链接，不要加回去。

**约定 F：测试**
- 先写测试并确认失败，再实现并确认通过，然后提交。
- 后端用 testify 的 require/assert。
- （v2.5）我们写的、要用数据库的测试一律连 PostgreSQL，统一用各包的 `adtyTestDB(t)`（文件 `adty_testhelper_test.go`，任务 2.1 起）：
  - 连接串读环境变量 `ADTY_TEST_PG_DSN`，没设就 `t.Skip` 并说明原因。从任务 2.1 起，check.sh 跑后端时要求它有值，CI 的 test 作业起 postgres:16 并设好它，所以门禁里这些测试不会被跳过。
  - 每个测试建一个随机名字的 schema，连接时用 `search_path` 指向它，测试结束删掉。`go test` 会同时跑多个包，各测试互不干扰。
  - model 和 service 包已经有 TestMain（model/task_cas_test.go:19、service/task_billing_test.go:30），`adtyTestDB` 在单个测试里换库、用完换回，写法参照 service/text_quota_test.go:78-82。换库时调 `common.SetDatabaseTypes(PostgreSQL, PostgreSQL)` 并重跑列名初始化：上游的 `initCol` 未导出（model/main.go:43），在 `model/adty_testsupport.go` 加一个导出包装；恢复时把数据库类型和列名一起还原。
- 上游自己的测试照旧用它们的内存 SQLite，我们改过的上游代码不能让它们失败（执行层决定 11 的例外）。钉住上游行为的特征测试（任务 1.1）也沿用上游包里现成的测试库。任务 1.6 的 `TestAdtyDeletedSchemaAbsent` 跑的是上游的完整迁移，也用内存 SQLite：删订阅时去掉的 SQLite 专用建表语句只在 SQLite 上执行，换成 PostgreSQL 就查不到它们回来没有；而且 P1 时 `adtyTestDB` 还没有。
- 前端测试放模块内的 `__tests__/`；新源文件带 AGPL 文件头（`bun run copyright`）。断言文件存在与否用 `node:fs`（`readFileSync`、`existsSync` 加 `import.meta.dirname`，上游 `theme-boot-script.test.ts:19-31` 就是这样写的），不用 `import.meta.glob`（类型检查不认）。
- 每个任务结束都跑 `scripts/adty/check.sh`（任务 0.3），不允许新增失败；`docs/adty/baseline.md` 里记录的基线失败不算。

**约定 G：提交**
- 提交信息：`feat(范围): …`、`fix`、`test`、`docs`、`ci`、`chore(delete): …`。删除功能单独提交，写清楚删了什么。
- 每个任务至少一个提交，一个提交只做一件事。

**约定 H：开发环境（v2.5：仓库在 Windows，WSL 只用来测试）**
- 仓库放在 Windows 上的一个目录里（下文的「Windows 仓库」），WSL 里经 `/mnt/c/` 下的对应路径访问同一个目录。改代码、git、推送都在 Windows 上做，用 Windows 的 git 和 gh（gh 已经用 adtyai 账号登录）。设计文档放在 Windows 上仓库以外的目录里，不进仓库。派出去的子代理同样遵守。
- 换行：克隆时设 `core.autocrlf=false`、`core.eol=lf`（只写进这个仓库自己的 .git/config），工作区的文件和仓库里的字节完全一样，都是 LF。Windows 上 git 的系统默认是 `autocrlf=true`，不改的话，没写明换行的文件（测试数据、Dockerfile、.txt）会变成 CRLF，到了 WSL 里测试会失败。编辑器和代理的文件工具都保持 LF；不要用 PowerShell 的 Out-File、Set-Content 写仓库文件。
- 提交身份只在命令里指定：`git -c user.name=adtyai -c user.email=342593550+adtyai@users.noreply.github.com commit …`，不改 git 配置（Windows 的全局身份是你的另一个账号）。
- WSL（实测）：WSL2，发行版 `Debian`（Debian 13），systemd 已开，16 核、15 GiB 内存。只用来装依赖、构建、测试；WSL 里只跑只读的 git 命令（`git log`、`git diff`），暂存、提交都在 Windows 上做。
- 测试副本：WSL 里的 `~/adtyai/mirror/main` 是仓库的副本。`node_modules`（1.6 GB）和 `web/dist` 只放在副本里，Windows 仓库里不放：从 WSL 读写 /mnt/c 很慢（[Microsoft 文档](https://learn.microsoft.com/windows/wsl/filesystems)）。每次运行前用 rsync 把 Windows 仓库同步过去：跳过 .git、node_modules、web/dist，Windows 上删掉的文件副本里也删。
- 怎么跑：命令写成 bash 脚本，`wsl.exe -d Debian -- bash -l ~/adtyai/bin/wrun.sh <任务名>`。它先同步副本、确认测试用的 PostgreSQL 已启动，再在副本目录里运行，输出写进日志文件。`-l` 让 bash 读 `~/.profile`，PATH 和环境变量都写在那里。check.sh 拒绝在 /mnt 下运行，防止依赖装进 Windows 仓库。
- 会改仓库文件的命令（`bun run build` 重新生成 `web/src/routeTree.gen.ts`、`bun run i18n:sync`、`bun run copyright`、`bun add` 改 package.json 和 bun.lock）在副本里跑完，用 `bash ~/adtyai/bin/adty-pull.sh <路径...>` 把这些文件拉回 Windows 仓库，再在 Windows 上提交。
- 只用 Linux 版的工具：WSL 的 PATH 末尾带着 Windows 的目录，里面有 Windows 版的 node、npm、git、gh。任务 0.1 装的 Linux 版排在它们前面；`check.sh` 开头检查 `git go bun node npm gh rg` 都不在 /mnt/c 下，否则直接退出。Windows 版的 node、npm、bun、go 不要在仓库里跑。
- 在 WSL 里起的服务，用 Windows 的浏览器打开 `http://localhost:<端口>`（WSL2 默认把 localhost 转发过来）。
- 网络（实测）：proxy.golang.org 和 sum.golang.org 连不上，Go 改用 goproxy.cn；校验和数据库也经它代理，GOSUMDB 不用改。github.com、npm 仓库、bun.sh、nodejs.org、Debian 源都能连。
- 下载慢或失败时走代理：Windows 上的代理是 127.0.0.1:10808（HTTP、SOCKS 都行），用 `bash ~/adtyai/bin/with-proxy.sh <命令>` 重跑。实测这个代理只监听 Windows 的 127.0.0.1，而 WSL 现在是 NAT 网络模式，WSL 里的 127.0.0.1 指 WSL 自己，所以 WSL 暂时用不了它。要用的话二选一：在代理软件里打开「允许局域网连接」，脚本会改走 Windows 主机在 WSL 网络里的地址；或者把 WSL 改成镜像网络模式（`%USERPROFILE%\.wslconfig` 里写 `[wsl2]` 和 `networkingMode=mirrored`，再 `wsl --shutdown`）。两种都改你机器上的设置，由你来定。
- WSL 实例在没有 wsl.exe 连着时会自动停；/tmp 是内存盘，停一次就清空。中间结果和日志放 `~/.cache/`，不放 /tmp。
- 不用 sudo：工具都装在用户目录。测试用的 PostgreSQL 是自己的实例：用系统里 PostgreSQL 17 的程序在 `~/adtyai/pg` 建立，只监听 Unix socket（端口 54329）；wrun.sh 运行前会确认它已启动，并设好 `ADTY_TEST_PG_DSN`。CI 和生产用 16，这些测试用到的事务、行锁和 `ON CONFLICT` 在两个版本里行为一样。Redis 相关测试用 miniredis。系统自带的 PostgreSQL、Redis 服务不用。
- Docker 没装：镜像在 GitHub Actions 里构建；做 10.1 时如果要在本地试跑 compose 再装（要 sudo）。
- 检查脚本 `bash scripts/adty/check.sh`（在副本里跑）：`--backend` 跑三个 Go 模块（根模块、relaykit、tokenkit）的 go vet、go build、go test；`--frontend` 跑 typecheck、lint、test、copyright:check；不带参数两个都跑。脚本开头 `set -euo pipefail`，任何一步失败就停。
- 单测：后端 `GOWORK=off go test ./model -run TestXxx -count=1`；前端在 `web` 目录 `bun run test -- <文件名>`；都写进任务脚本，经 wrun.sh 在副本里跑。
