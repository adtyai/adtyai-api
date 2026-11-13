# 分站系统规范实施记录

依据：工作区 `ADTYAI_分站系统开发规范.docx`，2026-11-09。

## 架构与差异审查

当前是 Go/Gin/GORM 单体网关与 React/TypeScript 前端，复用模型、渠道、路由及计费引擎。主数据库和日志库按现有工程约定使用 PostgreSQL 17；保留 SQLite/MySQL 兼容路径。新增能力继续放入 `adty_*` 文件，避免复制中转引擎。按用户指定的当前主开发分支增量实施，保留工作区原有改动，不创建长期开发分支。

| 规范模块 | 现有实现 | 实施差异与后续模块 |
|---|---|---|
| 创建与生命周期 | 申请/邀请码/管理员开站、暂停、关站迁移 | 规范只允许 Root 创建；需收口旧入口、补分项暂停与关闭财务检查 |
| 等级与价格 | 旧月度消费档位、d/k 加成、消费/任务价格快照 | 新增动态等级基础；综合评分、保证金资格、考核预警仍需实现；规范售价为供货成本 × (1 + 加价)，不同于当前主站价 × k |
| 三钱包 | 本批此前新增 operating/earnings/margin 与不可改写流水 | 目前仅为资金底座，未完成真实充值/消费联动、资金覆盖、冻结与透支授权，不能视为完整钱包 |
| 客户隔离 | users.site_id、站长查询带站点条件、转站时间边界 | 同名客户账户需调整全局唯一约束及所有身份查询；Key/订单/返佣/缓存需继续审查 |
| 邀请与提现 | 主站 referral 佣金账户和提现流程 | 分站邀请佣金单独冻结收益和已授权保证金，待覆盖不可提；现金提现由该站站长审核，打款完成后才释放。不能套用旧主站账户或 `Referral.CreateWithdrawal` |
| 支付与订单 | 充值扩展订单有 site_id；可信回调、补单、退款事务及钩子 | 需接入经营钱包、原币支付快照、财务调整单，保持业务幂等 |
| 域名与品牌 | Host 缓存、Cloudflare SaaS、自定义域名、邮件、Logo | 需按 s{site_id}.adtyai.top 自动生成、严格未知 Host/Key 站点校验、Token 加密、基础域名切换 |
| 人员与通知 | 主站 Casbin 后台分权、站长协议及公告 | 分站员工、站内角色和操作边界、通知已读记录仍需新增 |
| 报表 | 日收益汇总、站长概览、Root 管理页 | 需完整日结单，含三钱包、佣金、退款、提现、财务调整和差异 |

## 数据库与资金设计

- 已新增表：`adty_site_tiers`、`adty_site_wallets`、`adty_site_wallet_ledgers`；站点增加等级标识、管理模式及生效属性。钱包唯一键为 `(site_id, kind)`，流水幂等键全局唯一。
- 钱包余额和流水在同一事务更新，使用项目共享行锁。历史流水不更新/删除，纠错走反向流水与未来财务调整单。
- 真实充值进入经营钱包；赠送不得增加经营钱包。消费扣经营钱包并按价格快照计算收益；主动赠送/返佣按收益钱包优先、Root 授权保证金其次覆盖。经营钱包不得提供站长充值、提现或内部划转接口。
- 资金接入前需补冻结/授权/待追偿实体、来源批次和调整单，不把负余额直接等同于规范要求的授权透支。
- 三钱包记账币种尚待确认。此前新增底座使用 USD，规范默认保证金以人民币描述。确认前不新增币种换算，不接真实支付资金流，不将人民币档位当成美元写入。

## 隔离、域名与权限设计

- 所有业务记录和查询显式追溯 site_id；站长站点由登录身份服务端解析，不能通过请求参数切换。Root 跨站查询仍必须指定有效站点。
- 本批钱包读取延用当前站长资格和协议校验；Root 钱包读取只经 RootAuth，PAT 同时要求 option:read；站长 PAT 要求 wallet:read。
- 不开放任意钱包增减/划转接口。后续资金接口绑定真实业务来源、请求幂等键、授权和审计。
- Cloudflare 延用当前 SaaS 接入代码，外部凭据、DNS/SSL 与四种 USDT 实际打款能力需真实账号验证；不得用模拟返回代替生产验收。

## 本批实施计划

### Task 1：钱包查询与权限

- [x] 扩展 `router/adty_site_owner_test.go`：本站余额/流水、钱包类型筛选、分页、篡改 site_id、非站长/协议校验、Root 专属读取和 PAT 最小权限；先运行确认缺失路由返回 404。
- [x] 在 `service/adty_site_finance.go` 添加 `ListAdtySiteWallets(siteID)` 和 `ListAdtySiteWalletLedger(siteID, kind, offset, limit)`；零记录返回空列表，旧站未初始化钱包只投影零余额，不伪造历史。
- [x] 新增 `controller/adty_site_wallet.go`，注册站长 `/wallets`、`/wallet-ledgers` 与 Root `/sites/:id/wallets`、`/sites/:id/wallet-ledgers`；不提供写入接口。
- [x] PostgreSQL 集成测试通过，检查数据库查询与接口响应均不泄露其他站点或幂等键。

### Task 2：基础资金与等级回归

- [x] 在现有 `service/adty_site_finance_test.go` 添加失败用例：同日迟到收益的独立结算、未足额保证金分批补入、手动等级不受旧任务覆盖、新等级缓存立即失效。
- [x] 修正结算批次幂等边界与保证金补入条件；修正新旧等级任务边界、切换等级时的遗留待生效值及缓存失效。
- [x] 保持主站原有计费/订单行为不变，针对变更运行 PostgreSQL 回归与 SQLite 迁移测试。

### Task 3：验证与交付记录

- [x] gofmt、git diff --check；初轮 `GOWORK=off go test ./... -count=1`（含 PostgreSQL 测试库）通过，最终代码在独立测试实例上通过 `bash scripts/adty/check.sh --backend`。
- [x] 独立复查本批 diff 并修正发现的问题，更新 features.md 的实际范围和差异；不声称整套规范已完成。
- [ ] 真实 MySQL 新建/升级和资金并发矩阵，当前缺少实例；SQLite 完整资金边界矩阵及正式版本完整升级演练也未完成，不按三库兼容/生产迁移交付。

## 后续分批顺序与验收边界

先完成钱包资金覆盖与真实充值/消费，再接分站返佣和余额来源批次、独立提现/退款调整单，随后员工/通知与域名收口，最后综合等级考核及全站联调。每批都需要前后端、账本、权限、迁移和自动化用例，不能以本批只读接口替代最终交付。

主站风险包括：全局用户名唯一约束变更、旧 d/k 价格语义、已结算佣金与新收益钱包重复可支取、旧申请入口、Key 跨 Host 调用以及历史金额币种。相关切换必须有兼容迁移及回滚路径，禁止直接重写历史订单和流水。

尚需确认：钱包记账币种与汇率来源；Cloudflare 真实 SaaS 权限、加密主密钥管理及四种 USDT 人工打款能力。当前沿用工作区 main，不擅自切分支。Root 考核、扣划、透支、手续费及各期限只由配置决定，不擅自填值。

## 执行记录

- 既有基础已包含动态等级和三钱包模型、钱包变动事务、收益结算投影及 Root 等级配置 API；本批在此基础继续。
- Windows PATH 无 Go，改用现有 WSL Debian Go；发现本机 PostgreSQL 17.11，创建独立 `codex_adty_site_verify_20261109` 测试库，测试逐例使用独立 schema 并清理。
- 当前工作树有大量先前改动，不自动提交、暂存、重置或迁移这些修改。
- Task 1：PostgreSQL `go test ./router -run WalletReadAPIs -count=1`，先因 404 失败，补接口后通过；未初始化旧站的查询没有创建钱包。
- Task 2：PostgreSQL `go test ./service -run TestAdtySite -count=1`，先复现同日批次只记一笔、保证金补入被拒、手动等级被覆盖和缓存旧值，修正后通过；追加并发重放/扣账验证与 SQLite 多次迁移验证通过。
- 代码实现过程另发现 GORM 在复用结构体回读 NULL 列时保留旧指针；等级更新后改用空结构体回读，保证返回与数据库一致。
- 数值边界回归：钱包严格遵守 decimal(28,8) 的精度和范围，拒绝数据库会舍入或溢出的输入及余额；等级各项按各自 decimal 列验证，避免正倍率被舍入为零。先复现 PostgreSQL 静默舍入/溢出，再加校验后测试通过。
- 独立审查提出两项 Important：等级定义变更未同步当前站点，以及等级数值精度不匹配。均已补失败测试后修正：现有自动/手动等级站点在同一事务同步倍率、上限及保证金要求，提交后失效缓存；变更要求不视为保证金到账。
- 代表性升级测试在 SQLite 与 PostgreSQL 上先创建旧站点结构和数据，再重复迁移两次，保留站点 ID、站长、域名、原倍率及唯一约束；入账后再次迁移仍保留余额。
- 最终补充两项并发回归，均先确认失败后修正：等级分配读取事务外旧定义会覆盖新倍率/保证金要求，甚至分配已停用等级；并发重放同一笔全额扣款时，成功后的请求会误报余额不足。现改为锁定站点后读取等级定义，取得钱包锁后用当前读复查幂等记录。
- 最终验证改用 WSL Linux 副本 `~/adtyai/mirror/main` 和独立 PostgreSQL 数据目录 `~/.cache/adty-site-verify-20261109/pgdata`，Unix socket 在该目录的 `socket` 下，端口 55439，仅本机 socket 连接，不接正式服务；测试库 `codex_adty_site_verify`。实际 `server_version_num=170011`，PostgreSQL 17.11；纯 Go SQLite 驱动实际返回 SQLite 3.50.4。工具为 Linux Go 1.27.3。
- 验证结果：初轮根模块 `GOWORK=off go test ./... -count=1 -json` 为 50 个包通过、35 个包无测试、0 失败。最终补充并发修正后的 `go test ./service -run TestAdtySite -count=1` 通过；`bash scripts/adty/check.sh --backend` 退出 0，根模块 49 个测试包、relaykit 9 个测试包、tokenkit 1 个测试包通过，三个模块的 vet/build 也通过。该脚本按工程约定排除依赖前端构建产物的 main 包，本批未改前端且未重新运行前端门禁。最终日志：`~/.cache/adty-site-verify-20261109/backend-gate.log`。
- `gofmt -d` 无差异；`git diff --check` 通过，新增文件额外检查无行尾空格且保留末尾换行。仅有工作区原有 `web/bun.lock` 的 LF/CRLF 提示，未为此改写锁文件。本批未提交、推送或部署。
- MySQL 验证尚未完成：当前 WSL 未发现 MySQL 服务端或 Docker，无法执行真实 MySQL 新建/升级、事务和并发矩阵。SQLite 仅完成本批基础变动与迁移用例，不等于所有资金边界都已跨库验证；不得据此声明三数据库完整兼容或生产迁移已验收。
- Ruling：币种确认前只做资金底座和只读查询，不增加换算或实际经营钱包入账；旧账户与新钱包并存期间不新增支取入口，防止同一收益重复支取。代价是完整资金闭环和前端尚未交付，不能按完整分站系统上线。

最终后端验证命令（先启动上述独立测试实例，在 Linux 副本执行）：

```bash
export ADTY_TEST_PG_DSN="postgresql:///codex_adty_site_verify?host=$HOME/.cache/adty-site-verify-20261109/socket&port=55439"
export GOWORK=off
go test ./service -run TestAdtySite -count=1
bash scripts/adty/check.sh --backend
```
