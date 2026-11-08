# adtyai-api 工程文档

adtyai-api 是 [QuantumNous/new-api](https://github.com/QuantumNous/new-api) 的二开，起点是上游 v1.0.0-rc.47，以后持续合并上游的新版本。为了合并顺利，改动尽量放在新文件里，上游文件只加挂载点或做删除（[conventions.md](conventions.md) 的约定 B）。实施方案（代号 WhiteRelay）和设计稿不在仓库里，下文的任务号都指实施方案里的任务。

## 这个 fork 改了什么

实施方案 P0–P12 都已经做完（2026-11-01），只有任务 11.0（用真实的 Cloudflare、Resend 账号实测）留到上线前。每个功能的规则、后台入口和算例见 [features.md](features.md)。

- **删掉的功能**：上游的订阅、兑换码（连同它的「购买链接」设置 TopUpLink）、上游邀请奖励，前后端整条删掉，删除单独提交。删了什么记在 deleted-features.md；整个删掉的文件和目录另记在 deleted-paths.txt，合并上游的脚本（任务 1.7）和守卫测试（任务 1.6）读它，防止合并上游时把删掉的东西带回来。
- **上游的发布工作流**：docker-build、docker-image-branch、electron-build、release、sync-release-to-gitcode 五个 workflow 删掉（也在 deleted-paths.txt 里），镜像只由 `.github/workflows/adty-ghcr.yml` 构建，`ci.yml` 保留。本地的 712 个上游标签已经删掉（清单备份在仓库外的 `adtyai-api-upstream-tags-backup.txt`），`upstream` 远程设了 `tagopt --no-tags`，合并脚本把上游标签取到 `refs/upstream-tags/`，见 [upstream-merge.md](upstream-merge.md)。
- **充值**：USDT 走 BEpusdt（TRC20、BEP20、Polygon、X Layer），支付宝、微信走我们自己的易支付多通道。订单另记在新表 `adty_topup_orders`，只走我们自己的入账：面额、加赠和优惠码加赠一起入账、算佣金、写流水；晚到的成功回调照样入账。加赠档位、最低充值额等和钱有关的参数都在后台设置，没有内置值。上游的单通道易支付、Stripe、Creem、Waffo 后端和设置项保留、不配置，钱包页只显示我们的方式。
- **充值退款**：后台「充值对账」里的一个操作。只退没花掉的本金，赠金收回不退：按用户当前余额扣回这笔订单还剩的额度，余额最多扣到 0；按退掉的本金占本金的比例扣回佣金；分站订单里站长出的加赠只按收回的部分还给站长。真正把钱退给用户在线下做，后台显示应退多少。
- **余额下限**：不允许透支。每个钱包请求都先预扣，余额不够直接拒绝（403），上游收不到请求；结算时实际费用比预扣多，最多扣到 0，差额由平台承担并记进日志的 `admin_info.shortfall_quota`（只有管理员看得到）。上游原来余额够多时跳过预扣的「信任额度」对钱包不再生效。
- **推广返佣**：从旧项目移植（表 `referral_*`，接口 `/api/user/referral/*` 等），返佣比例沿用百分数，注册即有推广链接，只按充值面额计佣；订单里存返佣快照；退款时按退掉的本金比例扣回佣金。站长和分站客户不计邀请返佣。
- **后台分权**：普通管理员默认没有任何后台权限，由超级管理员逐项开启、随时可以关；系统设置、易支付通道的新增和密钥、管理员任免只有超级管理员能做。
- **站长分站（P11）**：用户可以开分站，客户付「主站价 × 站长倍率 k」，平台收「主站价 × 拿货系数 d」，站长赚差价，收益按消费记账、冻结后可以提现。每个分站有平台子域名，可以接自定义域名（Cloudflare for SaaS）和站长自己的发件域名（Resend）。主站后台能看各站明细、复核风控、关站和转移客户。
- **营销（P12）**：营销邮件的同意和退订、群发（Resend Broadcasts）、三条自动触发邮件、充值优惠码（平台出资或站长出资）、注册来源统计。
- **数据库**：部署目标为 PostgreSQL 17，缓存用 Redis。保留上游 SQLite 单测和 SQLite、MySQL、PostgreSQL 的兼容代码；ADTY 启动时主库或日志库不是 PostgreSQL 就直接退出。已有 PostgreSQL 16 数据目录须独立迁移，不能直接换镜像启动，见 [deploy.md](deploy.md)。
- **名字、Logo 和语言**：访客、调用 API 的人、上游服务商和浏览器开发者工具能看到的 New API，都换成 ADTY AI 或 adtyai；站名 ADTY AI 在 8 种语言（zh-CN、zh-TW、en、fr、ja、ko、ru、vi）里都不翻译。Logo 换成「层叠网关」一套（`web/public` 下的图标、manifest、分享图），原稿和生成脚本在仓库外的 `adtyai-logo` 目录（`source/build.py`）。前端和后端的提示都有 8 种语言，CI 检查漏翻。Go 模块路径不改；LICENSE、NOTICE、THIRD-PARTY-LICENSES.md、上游源文件头和页脚的上游署名都保留（AGPL）。
- **首页**：`/` 固定是英文，另外 7 种语言用 `/zh-cn`、`/ko` 这类网址；首页的标题、描述和正文写进网页源码，其余页面一律 noindex。
- **其他**：白色主题；按用户限 RPM、TPM、并发，开了 Redis 但计数不可用时拒绝而不放行；客户能看到的日志和任务不露渠道和倍率；人工调额留痕，且不能把余额调成负数；防邮箱枚举，验证码写不进共享存储时不发信；密码连续错误 8 次后锁定 15 分钟；主站发布了协议或隐私政策后，注册必须接受并留下系统日志；所有流式格式在等首字时都能发保活；分组有固定 ID 和多语言显示名；官方价每天从 models.dev 同步，价格页显示节省比例；接入文档、推广计划、多语言条款和退款政策；网页客服 tawk.to；公开的服务状态页。

## 和 AGENTS.md 不一致的地方

仓库根目录的 AGENTS.md 是上游写给 AI 代理的规范，我们不改它，方便合并上游。下面四点以本 fork 的约定为准：

1. **数据库**：ADTY 的部署和资金验收目标为真实 PostgreSQL 17；通过连接实例的 `server_version_num` 确认主版本。我们写的、要用数据库的测试连 PostgreSQL；上游自己的 SQLite 单测照旧保留，公共建表、迁移和业务代码继续保留 SQLite、MySQL、PostgreSQL 的兼容分支。本轮不要求 MySQL 生产环境验收，不代表可以删除或破坏 MySQL 兼容代码。
2. **对外的名字**：AGENTS.md 规定 New API 和 QuantumNous 的名称、标识「任何情况下不得修改」。本 fork 按执行层决定 21 和任务 8.9，把访客、调用方、上游服务商和浏览器能看到的名字换成了 ADTY AI 或 adtyai（cookie 名、错误类型、本地存储键、网页标题、Logo 等，改了哪些、哪些不改见 conventions.md 和任务 8.9）。LICENSE、NOTICE、THIRD-PARTY-LICENSES.md、源码版权头、页脚的上游署名、Go 模块路径 `github.com/QuantumNous/new-api` 都照旧保留。
3. **docs/adty/ 下的文档**：AGENTS.md 说没有用户明确要求时不要在 `docs/` 下新建文件。`docs/adty/` 下的文档都是按实施方案（约定 A：工程文档放 `docs/adty/`）或用户的要求建的，以后改功能时要同步更新这里的文档。
4. **后端语言**：后端有 8 种（任务 9.1，后补 ko）：en、zh-CN、zh-TW、fr、ja、ko、ru、vi。上游键和我们自己的 `i18n/locales/adty.<语言>.yaml` 都须一次写齐，根目录 AGENTS.md 的语言清单已同步。

## 各个文档的用途

- `README.md`：本文件，说明这个 fork 改了什么、各个文档的用途、和 AGENTS.md 不一致的地方。
- [features.md](features.md)：功能说明，给运营人员和接手的开发看。每个功能写做什么、规则、后台在哪里设置或操作、默认开还是关、相关的表和接口、已知的限制；涉及钱的都有算例。
- [conventions.md](conventions.md)：全局约定 A–H（代码放哪、怎么改上游文件、钱、权限和密钥、文案和语言、测试、提交、开发环境），每个任务都遵守。
- [upstream-touchpoints.md](upstream-touchpoints.md)：改过的上游文件。每改一个上游文件记一行：文件、改了什么、为什么（约定 B）；合并上游时对照它解决冲突。
- [deleted-features.md](deleted-features.md)：删掉的功能，按功能列出删掉的文件、路由、表和列、配置项、前端目录和提交，以及有意保留的东西和理由。
- [deleted-paths.txt](deleted-paths.txt)：整个删掉的文件和目录，一行一个。合并上游的脚本（任务 1.7）和守卫测试（任务 1.6）读它。
- [upstream-merge.md](upstream-merge.md)：合并上游新版本的流程（实施方案第七节），以及合并脚本 `scripts/adty/merge-upstream.sh` 怎么处理冲突、什么情况下不跑。
- [deploy.md](deploy.md)：生产部署（任务 10.1、11.11）：`deploy/adty/` 下的 compose、Nginx、Cloudflare 网段和防火墙脚本、备份脚本怎么用，Cloudflare 面板怎么设，分站的泛解析和 Cloudflare for SaaS，真实 IP 验证，升级和回滚。
- [launch-checklist.md](launch-checklist.md)：上线检查清单（任务 10.2、11.11）。上线前逐条打勾：环境变量、后台设置、支付和推广实测、分站和营销、浏览器和客户端回归、备份演练、SEO。
- [baseline.md](baseline.md)：上游基线（任务 0.3）。记录上游 v1.0.0-rc.47 原样不改时 `scripts/adty/check.sh` 每一步的结果；这里记下的上游失败不算新失败（约定 F）。

`docs/` 整个目录不进 Docker 镜像（`.dockerignore` 里的 `docs` 一行）。
