# 删掉的功能

## 签到奖励入口

- 删除个人页签到卡片、专用请求函数和响应类型，以及计费设置中的签到分区、默认值和表单。历史 `/system-settings/billing/checkin` 书签按现有未知分区规则回到额度设置。
- 删除 `web/src/features/profile/components/checkin-calendar-card.tsx` 和 `web/src/features/system-settings/general/checkin-settings-section.tsx`，不再受历史 `checkin_enabled` 或 `checkin_setting.*` 启用值影响。
- 保留 `GET/POST /api/user/checkin` 的永久 403、历史签到记录、`checkin_quota` 和退款来源保护；不改写或删除历史资金。
- 保留语言包旧键与历史数据库选项用于升级兼容；不再提供操作入口。
- 提交：`fix(ui): remove retired check-in controls and refresh read badges`。

## Stripe / Creem / Waffo 与遗留单通道易支付入账入口

- 保留下列路由的永久 403（`This feature is not available`），在鉴权和入账之前拒绝，避免合并时静默恢复旧处理函数；不改写或删除历史充值行、回调日志和设置。
  - `POST /api/stripe/webhook`、`POST /api/creem/webhook`、`POST /api/waffo/webhook`、`POST /api/waffo-pancake/webhook/:env`
  - `POST /api/user/stripe/pay`、`POST /api/user/stripe/amount`
  - `POST /api/user/creem/pay`
  - `POST /api/user/waffo/pay`、`POST /api/user/waffo/amount`
  - `POST /api/user/waffo-pancake/pay`、`POST /api/user/waffo-pancake/amount`
  - 遗留 `GET /api/user/epay/notify` 和 `POST /api/user/epay/notify`（无 `:channelId`，`controller.EpayNotify`）
  - 仅用于退役托管目录的 `POST /api/option/waffo-pancake/catalog|pair|save`
- `GET /api/user/topup/info` 的 `enable_stripe_topup`、`enable_creem_topup`、`enable_waffo_topup`、`enable_waffo_pancake_topup` 固定为 false，不再跟随现场 Stripe/Creem/Waffo 设置。官方 `GetAdtyTopUpInfo` / `AdtyPaymentScenesForUser` 不再向客户展示这些托管方式。
- 保留官方资金路径：`POST /api/user/pay`（`RequestAdtyEpayPay`，仍需登录）、`GET/POST /api/user/epay/notify/:channelId`（`AdtyEpayNotify`）、`/api/adty/topup/*` 报价/下单/取消/查询，以及官方 USDT/易支付通道和已有管理员退款/补单。
- 保留历史订单的管理员查询、补单和退款能力；不把退役处理函数改造成新的 ADTY 批次入账入口。
- 提交：`Close retired Stripe, Creem, Waffo, and legacy epay notify payment entries (ISS-R1-0021).`。

## Playground（最新全平台方案）

- 删除用户菜单、页面、路由、个人和管理员侧栏开关、仪表盘旧快捷链接，以及仅由该功能使用的前端实现和测试。首次请求快捷入口改用已有 `/docs` 页面。
- 删除 `POST /pg/chat/completions`、访问令牌规则、会话中转控制器和专用请求 DTO；Web 回退对 `/pg` 及子路径返回 JSON 404。旧 `/console/playground` 使用现有未知旧链接规则跳到仪表盘，`/playground` 显示前端 404。
- 历史 `chat.playground` 配置读取时丢弃，不能重新显示开关；既有数据库设置无需改写。
- 保留真实计费与任务中转仍引用的 `IsPlayground` 内部兼容标志、旧命名空间的 Host 分类和插件保留路径，以及语言包旧键；这些均不注册用户功能。整个删除路径见 [deleted-paths.txt](deleted-paths.txt)。

删掉的上游功能，一个功能一节。删除任务把删掉的东西逐条写在对应的小节下面，写成列表项；整个删掉的文件和目录同时写进 [deleted-paths.txt](deleted-paths.txt)（约定 B）。P1 删订阅、兑换码、上游邀请奖励，下面先留好这三节；以后再删别的功能，照同样的格式加一节。

各小节写法：

- 文件、前端目录：从仓库根目录写起的路径，目录以 `/` 结尾。
- 路由：方法加路径，如 `GET /api/...`。
- 表和列：表名，或者表名加列名。
- 配置项：配置的键名。
- 提交：删除所在的提交。登记和删除在同一个提交里，那时还没有 SHA，先写提交标题。
- 保留名单和理由：看起来相关、但有意留下的东西，以及为什么留。

## 站长自助申请分站

- 删除 `web/src/features/referral/components/site-apply-card.tsx` 及推广页引用，推广页不再显示申请表单。
- 保留 `POST /api/adty/site/apply` 的拒绝处理，所有请求均返回403，不再创建站点或使用邀请码。
- 保留历史申请数据、Root审批和内部开站服务，用于历史兼容及Root创建；用户不能通过旧入口自行开站。
- 提交：`chore(delete): remove reseller self-service application UI`。

## 订阅

### 文件

- `controller/subscription.go`
- `controller/subscription_payment_creem.go`
- `controller/subscription_payment_epay.go`
- `controller/subscription_payment_stripe.go`
- `controller/subscription_payment_waffo_pancake.go`
- `model/subscription.go`
- `model/subscription_auth_test.go`
- `model/subscription_reset_test.go`
- `service/subscription_reset_task.go`（订阅额度的定时重置和到期处理，原来由 `main.go` 启动）

### 路由

访问令牌规则（`middleware/access_token_routes.go`）和管理员操作的审计登记（`middleware/audit.go`）随路由一起删了；四条易支付回调也从上游测试的免检名单（`router/access_token_scope_test.go`）里删了。

- 用户（`middleware.UserAuth`）：
  - `GET /api/subscription/plans`
  - `GET /api/subscription/self`
  - `PUT /api/subscription/self/preference`
  - `POST /api/subscription/balance/pay`
  - `POST /api/subscription/epay/pay`
  - `POST /api/subscription/stripe/pay`
  - `POST /api/subscription/creem/pay`
  - `POST /api/subscription/waffo-pancake/pay`
- 管理员（`middleware.AdminAuth`）：
  - `GET /api/subscription/admin/plans`
  - `POST /api/subscription/admin/plans`
  - `PUT /api/subscription/admin/plans/:id`
  - `PATCH /api/subscription/admin/plans/:id`
  - `POST /api/subscription/admin/bind`
  - `POST /api/subscription/admin/plans/:id/subscriptions/reset`
  - `GET /api/subscription/admin/users/:id/subscriptions`
  - `POST /api/subscription/admin/users/:id/subscriptions`
  - `POST /api/subscription/admin/users/:id/subscriptions/reset`
  - `POST /api/subscription/admin/user_subscriptions/:id/invalidate`
  - `DELETE /api/subscription/admin/user_subscriptions/:id`
- 易支付回调（不登录）：
  - `GET /api/subscription/epay/notify`
  - `POST /api/subscription/epay/notify`
  - `GET /api/subscription/epay/return`
  - `POST /api/subscription/epay/return`
- Waffo Pancake 订阅产品（`middleware.RootAuth`）：
  - `POST /api/option/waffo-pancake/subscription-product`
  - `GET /api/option/waffo-pancake/subscription-product-options`

### 表和列

迁移（`model/main.go` 的 `migrateDB`）不再建下面四张表，SQLite 专用的 `subscription_plans` 建表语句和 `subscription_plans.price_amount` 的类型迁移也删了。已经存在的表不会自动删除。

- `subscription_plans`
- `subscription_orders`
- `user_subscriptions`
- `subscription_pre_consume_records`

列里 JSON 的键（不是列，记在这里方便对照）：

- `tasks.private_data` 的 `subscription_id`（`model.TaskPrivateData.SubscriptionId`）。
- `logs.other` 的 `billing_preference`、`subscription_id`、`subscription_pre_consumed`、`subscription_post_delta`、`subscription_plan_id`、`subscription_plan_title`、`subscription_total`、`subscription_used`、`subscription_remain`、`subscription_consumed`、`wallet_quota_deducted`：消费日志不再写这些键（`service/log_info_generate.go` 的 `appendBillingInfo`），`billing_source` 照常写。

### 配置项

- `WaffoSubscriptionReturnUrl`（`setting.WaffoSubscriptionReturnUrl`；`model/option.go` 的 `InitOptionMap`、`updateOptionMap`）

这一节和兑换码、邀请奖励两节删掉的配置项，`PUT /api/option/` 都返回 400，不再写进 options 表（`controller/option.go` 的 `UpdateOption`，测试 `TestUpdateOptionRejectsDeletedFeatureKeys`）。

### 前端目录

- `web/src/features/subscriptions/`（订阅套餐管理页、用户订阅对话框、购买对话框、订阅接口调用）
- `web/src/routes/_authenticated/subscriptions/`（页面 `/subscriptions`；`web/src/routeTree.gen.ts` 已重新生成）
- `web/src/features/wallet/components/subscription-plans-card.tsx`（钱包页的订阅套餐卡片）
- `web/src/features/usage-logs/lib/billing-source.ts` 和它的测试 `web/src/features/usage-logs/lib/__tests__/billing-source.test.ts`、`web/src/features/usage-logs/components/__tests__/billing-source-visibility.test.tsx`（按有没有订阅决定使用日志费用列显不显示计费来源图标）

页面入口和前端功能（上游文件里的改动见 [upstream-touchpoints.md](upstream-touchpoints.md)）：

- 侧边栏「订阅」菜单（`/subscriptions`）和侧边栏模块开关 `admin.subscription`（默认值、地址映射、系统设置里的开关项）。已经存在的 `SidebarModulesAdmin` 配置里如果还有 `subscription` 键，前端不再读它。
- 后台用户行的「管理订阅」菜单和对话框。
- 使用日志：费用列的计费来源图标（订阅、钱包），订阅日志按 `subscription_consumed` 显示扣费，详情里的「订阅计费」一节，审计操作 `subscription.*` 的文案模板。进入使用日志页时不再请求 `/api/subscription/admin/plans`、`/api/subscription/self`。
- 旧地址 `/console/subscription` 不再跳 `/subscriptions`，按其他未知的 `/console/*` 地址跳 `/dashboard`。

### 提交

- `aa19f2ed7` `chore(delete): remove subscription plans (backend)`（任务 1.2，后端）
- `11138b37a` `chore(delete): remove subscription plans (frontend)`（任务 1.3，前端）

### 保留名单和理由

- `GET /dashboard/billing/subscription`、`GET /v1/dashboard/billing/subscription`（`router/dashboard.go`）和处理函数 `controller/billing.go` 的 `GetSubscription`：OpenAI 兼容的余额查询，客户端会调，和订阅套餐无关。
- `controller/channel-billing.go`、`controller/channel_billing_sub2api_test.go`：查上游渠道的余额（OpenAI 的 `/v1/dashboard/billing/subscription`、Sub2API 的订阅分组），和本站的订阅套餐无关。
- `constant/channel.go` 的渠道类型名 `ChatGPT Subscription (Codex)`：上游渠道的显示名。
- `pkg/wsmanager`：Redis 的发布订阅，和订阅套餐无关。
- `pkg/jsplugin/routing.go` 前端保留路径里的 `/subscriptions`：插件不能占用这些路径。留着它，插件就挂不到原来的订阅页地址上，合并上游时也少一处冲突。
- `service/quota.go` 里 `quotaReminderDue` 的注释和 `service/user_notify_test.go` 的注释：上游注释顺带提到订阅余额，函数和测试讲的是钱包的余额提醒。
- `relay/common/billing.go` 里 `BillingSettler` 接口注释的「钱包/订阅」：只是注释，这个文件别处不用改，不为一句注释多改一个上游文件。
- `controller/topup_waffo.go` 的类型名 `webhookPayloadWithSubInfo`：里面的订阅字段删了，类型名沿用上游。
- 合规提示和 `service/access_token_scope.go` 里 wallet、billing 两项资源的描述：原文提到订阅、兑换码、邀请奖励，已换成不提这些功能的新原文，en 等 7 个语言包加了新键，旧键留着（约定 B、E）。合规提示现在是 `Payment features are locked until the root administrator confirms the compliance terms.`。
- `i18n/keys.go` 的 `MsgQuotaSubscriptionInsufficient`、`MsgNotifySubscriptionQuotaLowTitle` 和 `i18n/locales/*.yaml` 里对应的键：上游测试 `TestLocalesRenderEveryKey` 要求 keys.go 和每个 yaml 的键完全相同，删功能也不删键（执行层决定 13）。
- `common/log_text.zh-CN.json` 里订阅日志的中文：按英文原文查的译文表，没人用的条目不起作用。不改，合并上游时少一处冲突。
- `relaykit/dto/user_settings.go` 的 `UserSetting.BillingPreference`（用户设置里的 `billing_preference`）：上游 6 个测试文件在用，字段保留。读取它的两处（`service/billing_session.go` 的 `NewBillingSession`、`service/log_info_generate.go` 的 `appendBillingInfo`）删了，它现在不起作用。
- 计费来源：常量 `BillingSourceWallet`、`RelayInfo.BillingSource`、`TaskPrivateData.BillingSource` 和日志里的 `billing_source` 保留，现在只会是 `wallet` 或空。
- Stripe、Creem、Waffo Pancake 的支付回调（`POST /api/stripe/webhook`、`POST /api/creem/webhook`、`POST /api/waffo-pancake/webhook/:env`）：只删了里面处理订阅订单的分支，充值入账照旧。Creem、Stripe 回调里的 `LockOrder`/`UnlockOrder` 写在订阅分支前面，同时保护后面的充值入账，保留。
- `docs/openapi/api.json`：上游的接口文档，不改。它没有收录订阅接口（搜不到 `subscription` 和「订阅」），删订阅不用动它。
- `README.md`、`README.en.md`、`pkg/billingexpr/expr.md` 里提到订阅的句子：上游文档，不改。
- `service/adty_wallet_billing_test.go`：任务 1.1 的特征测试，注释里提到订阅。删订阅时这个文件不改，测试照样通过。
- 前端 `web/src/features/channels/constants.ts` 和 `web/src/i18n/static-keys.ts` 里的 `ChatGPT Subscription (Codex)`、`Access Codex using ChatGPT subscription credentials`：Codex 渠道类型的显示名和说明，和订阅套餐无关。
- `web/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx` 里 Waffo Pancake 的商户介绍（`Start collecting payments globally ...`）：Waffo 自己的产品介绍，提到的订阅是它的收款能力。
- `web/src/i18n/locales/*.json` 里订阅相关的键和换掉的旧键：前端语言包只增不删（约定 B）。`web/src/i18n/static-keys.ts` 不是语言包，它的订阅几段（订阅常量、订阅套餐、订阅管理、用户订阅管理）已经删掉，见 upstream-touchpoints.md。
- `web/src/lib/legacy-route.test.ts` 里 `/console/subscription` 的断言：保护删除后旧地址跳 `/dashboard`。

## 兑换码

### 文件

- `controller/redemption.go`
- `controller/redemption_batch_test.go`
- `model/redemption.go`
- `model/redemption_test.go`

### 路由

访问令牌规则（`middleware/access_token_routes.go`）、管理员操作的审计登记（`middleware/audit.go`）和访问令牌的 `redemption` 资源（`service/access_token_scope.go`）随路由一起删了。

- 用户（`middleware.UserAuth`）：
  - `POST /api/user/topup`（用兑换码充值，处理函数 `controller.TopUp`）。`GET /api/user/topup` 是后台的充值记录（`controller.GetAllTopUps`），保留。
- 管理员（`middleware.AdminAuth`）：
  - `GET /api/redemption/`
  - `GET /api/redemption/search`
  - `GET /api/redemption/:id`
  - `POST /api/redemption/`
  - `POST /api/redemption/batch`
  - `PUT /api/redemption/`
  - `DELETE /api/redemption/invalid`
  - `DELETE /api/redemption/:id`

接口返回里删掉的字段：`GET /api/user/topup/info` 不再返回 `enable_redemption` 和 `topup_link`；用户的侧边栏默认配置（`controller/user.go` 的 `generateDefaultSidebarConfig`、`model/user.go` 的 `generateDefaultSidebarConfigForRole`）的 `admin` 区不再有 `redemption`。

### 表和列

- `redemptions`：迁移（`model/main.go` 的 `migrateDB`）不再建这张表。已经存在的表不会自动删除。

### 配置项

- `TopUpLink`（`common.TopUpLink`；`model/option.go` 的 `InitOptionMap`、`updateOptionMap`）：兑换码区域的「购买链接」，随兑换码一起删（执行层决定 20）。数据库 options 表里已有的 `TopUpLink` 行不再读取。

### 前端目录

- `web/src/features/redemption-codes/`（兑换码管理页、新建和编辑抽屉、导出、批量删除、兑换码接口调用）
- `web/src/routes/_authenticated/redemption-codes/`（页面 `/redemption-codes`；`web/src/routeTree.gen.ts` 已重新生成）
- `web/src/features/wallet/hooks/use-redemption.ts`（钱包页兑换兑换码，调 `POST /api/user/topup`）

页面入口和前端功能（上游文件里的改动见 [upstream-touchpoints.md](upstream-touchpoints.md)）：

- 钱包充值卡片里的「有兑换码？」一节（输入框、兑换按钮、购买链接 `topup_link`、未确认合规时的提示）和它的加载骨架。
- 系统设置「额度设置」里的「充值链接」（`TopUpLink`）输入项。
- 侧边栏「兑换码」菜单（`/redemption-codes`）和侧边栏模块开关 `admin.redemption`（默认值、地址映射、系统设置里的开关项）。已经存在的 `SidebarModulesAdmin` 配置里如果还有 `redemption` 键，前端不再读它。
- 使用日志的审计显示：`redemption.*` 的文案模板、批量删除兑换码的专门文案和 `requested_redemption_ids` 字段名。
- 访问令牌权限里「兑换码」一项的文案（`web/src/i18n/static-keys.ts`），随后端的 `redemption` 资源删了。
- 旧地址 `/console/redemption` 不再跳 `/redemption-codes`，按其他未知的 `/console/*` 地址跳 `/dashboard`。

### 提交

- `2dbcf5e3e` `chore(delete): remove redemption codes (backend)`（任务 1.4，后端）
- `3a5db26b5` `chore(delete): remove redemption codes (frontend)`（任务 1.4，前端）

### 保留名单和理由

- `requirePaymentCompliance`（`controller/payment_compliance.go`）：充值入口的合规检查，P3 还要用。
- `service/codex_wham_usage.go` 的 `redeem_request_id`：Codex 用量接口的请求字段，和兑换码无关。
- `pkg/jsplugin/routing.go` 前端保留路径里的 `/redemption-codes`：插件不能占用这些路径。留着它，插件就挂不到原来的兑换码页地址上，合并上游时也少一处冲突。
- `service/quota.go` 的局部变量 `topUpLink`：余额提醒邮件里的钱包地址（`/wallet`），和 `TopUpLink` 设置无关。
- `common/log_text.zh-CN.json` 里兑换码日志的中文（`failed to insert redemption: %s`、`failed to redeem key ...`、`redemption`、`redemption failed: %s`）：按英文原文查的译文表，没人用的条目不起作用。不改，合并上游时少一处冲突。
- `model/locking_test.go` 的 `TestLockForUpdateEmitsRowLock`：原来借 `[]Redemption` 当模型生成 SQL，换成 `[]TopUp`，测试保留。
- `docs/openapi/api.json`、`docs/openapi/relay.json`：上游的接口文档，收录了 `/api/redemption/*` 和 `Redemption` 结构，不改。`api.json` 里的 `/api/user/topup` 只收录了 GET，删兑换码不影响它。
- `docs/translation-glossary.md`、`docs/translation-glossary.fr.md`、`docs/translation-glossary.ru.md` 里的「兑换码」词条：上游的翻译术语表，不改。
- `scripts/adty/baseline-lint.txt`、`docs/adty/baseline.md` 里 `src/features/redemption-codes/` 的条目：lint 基线，文件删了以后条目不起作用（ratchet 只报「可以更新清单」）；基线等合并上游时用 `ratchet.mjs --update` 统一重写，这次不改。
- `web/src/features/channels/components/dialogs/codex-usage-dialog.tsx` 里的 `redeemed`、`redeemed_at`、`Redeemed at`：Codex 额度券的状态和时间，和兑换码无关。
- `web/src/i18n/locales/*.json` 里兑换码相关的键和换掉的旧键（`Online topup is not enabled. Please use redemption code or contact administrator.`）：前端语言包只增不删（约定 B）。
- `web/src/lib/legacy-route.test.ts` 里 `/console/redemption` 的断言：保护删除后旧地址跳 `/dashboard`。

## 上游邀请奖励

### 文件

- 后端没有整个删掉的文件，删的都是上游文件里的一部分（见 [upstream-touchpoints.md](upstream-touchpoints.md)）：`model.GetUserIdByAffCode`、`inviteUser`、`(*User).TransferAffQuotaToQuota`，`controller.TransferAffQuotaRequest`、`TransferAffQuota`、`GetAffCode`，注册（`controller.Register`）和 OAuth 建号（`controller.findOrCreateOAuthUser`）里按邀请码查邀请人、给双方发奖励的逻辑。
- `Insert`、`FinishInsert`、`InsertWithTx`、`FinalizeOAuthUserCreation` 的第一个 `int` 形参（原来的邀请人 ID）保留成 `_ int`，调用方不用改；传什么都不再起作用。

### 路由

访问令牌规则（`middleware/access_token_routes.go`）随路由一起删了。

- 用户（`middleware.UserAuth`）：
  - `GET /api/user/aff`（取自己的邀请码，没有就生成，处理函数 `controller.GetAffCode`）。删掉以后这个地址落到管理员的 `GET /api/user/:id`，普通用户得到 403。
  - `POST /api/user/aff_transfer`（把邀请额度转进余额，处理函数 `controller.TransferAffQuota`，限流键 `aff-transfer`）

接口里删掉的字段：

- `POST /api/user/register` 的请求不再读 `aff_code`（JSON 里带了也忽略）。
- `POST /api/oauth/state` 的请求不再读 `aff`；原来 `aff` 超过 32 个字符、或者非登录意图带 `aff` 会报「参数无效」，这两条检查随字段一起删了，其余的提供方、意图、`scope`/`context` 检查不变。OAuth 流程的 payload 不再有 `affiliate_code`。
- `GET /api/user/self` 和登录、刷新返回的用户（`controller.buildSelfUserData`）不再有 `aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`inviter_id`。后台的用户列表、用户详情直接返回 `model.User`，这些字段随结构体字段一起没了。

### 表和列

- `users` 的 `aff_code`（唯一索引）、`aff_count`、`aff_quota`、`aff_history`、`inviter_id`（索引）：`model.User` 删了这五个字段，新库不再建这些列。已经存在的列不会自动删除（没有写 DropColumn）。
- `model/main.go` 的 `userQuotaColumns`（启动时检查 64 位额度列）去掉 `aff_quota`、`aff_history`。

### 配置项

- `QuotaForInviter`、`QuotaForInvitee`（`common.QuotaForInviter`、`common.QuotaForInvitee`；`model/option.go` 的 `InitOptionMap`、`updateOptionMap`）：邀请人、被邀请人的奖励额度。`controller/option.go` 的 `UpdateOption` 里「未确认合规时不许设成正数」的检查和只给它用的 `isPositiveOptionValue` 一起删了。数据库 options 表里已有的这两行不再读取。`QuotaForNewUser`（新用户赠送额度）保留。

### 前端目录

- `web/src/features/wallet/components/affiliate-rewards-card.tsx`（钱包页的邀请奖励卡片：邀请链接、待转、累计、邀请人数）
- `web/src/features/wallet/components/dialogs/transfer-dialog.tsx`（把邀请额度转进余额的弹窗）
- `web/src/features/wallet/hooks/use-affiliate.ts`（取邀请码、复制邀请链接、转额度，调 `GET /api/user/aff`、`POST /api/user/aff_transfer`）
- `web/src/features/wallet/lib/affiliate.ts`（生成 `/sign-up?aff=<邀请码>` 链接）
- `web/src/features/auth/lib/storage.ts`（把 `?aff=` 存进 localStorage 的 `aff` 键再读出来；删掉邀请码函数后只剩一个没人用的常量，整个删了）

页面入口和前端功能（上游文件里的改动见 [upstream-touchpoints.md](upstream-touchpoints.md)）：

- 任何页面地址里的 `?aff=` 不再保存（`routes/__root.tsx`、注册页 `sign-up-form.tsx`）；注册不再带 `aff_code`，OAuth 登录发起时不再带 `aff`。
- 钱包页的邀请奖励卡片和划转弹窗。
- 系统设置「额度设置」里的「邀请人奖励」「被邀请人奖励」（`QuotaForInviter`、`QuotaForInvitee`）和只为它们存在的「未确认合规时邀请奖励不能非零」提示（`complianceConfirmed` 属性）。
- 用户管理列表的「邀请信息」列（`invite_info`）。
- 使用日志里用户信息弹窗的邀请码、邀请人数、邀请额度；弹窗说明去掉「邀请详情」，换成新键。
- 各处用户类型里的 `aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`inviter_id`、`invite_user_id`。

P6 移植旧项目前端时，不要把 `?aff=` 和 `features/auth/lib/storage.ts` 带回来。

### 提交

- `b059dbac4` `chore(delete): remove upstream invite rewards (backend)`（任务 1.5，后端）
- `2d52bd02b` `chore(delete): remove upstream invite rewards (frontend)`（任务 1.5，前端）

### 保留名单和理由

- `controller/access_token_audit_test.go` 的 `releasedAuditUser`：模拟上一个正式版的 `users` 表，用来测升级。它的 `aff_*`、`inviter_id` 列和建行时的 `AffCode` 保留，升级测试才能证明旧库带着这些列也能启动。
- `model/auth_flow_test.go` 的 `Payload: {"affiliate_code":"invite"}`：测试里只把它当一段不透明的 payload 文本，检查流程的绑定和一次性消费，和邀请码无关。
- `docs/openapi/api.json`：上游的接口文档，收录了 `/api/user/aff_transfer` 和 `aff_code` 字段，不改。
- `web/src/i18n/locales/*.json` 里邀请奖励相关的键和换掉的旧键（`View detailed information about this user including balance, usage statistics, and invitation details.`）：前端语言包只增不删（约定 B）。
- 「每日签到」的 `quota rewards`、`Check-in Rewards` 文案：签到奖励，不是上游邀请奖励。

## 上游钱包充值卡（任务 3.4）

钱包页换成 `web/src/features/wallet/adty/` 的新充值卡、待付款提示和充值记录，上游的充值卡、确认弹窗、账单弹窗、Creem 产品区和它们的 hooks 没有引用了（`bun run knip --production` 列出），一起删掉，路径见 [deleted-paths.txt](deleted-paths.txt)。

- 上游后端的单通道易支付、Stripe、Creem、Waffo 接口和设置项保留不动、不配置（执行层决定 8）。
- 账单弹窗删了以后，上游「全站充值记录」和「补单」按钮没有入口了，后台只用「充值对账」（任务 3.5）。
- 保留：`lib/payment.ts`（系统设置两个支付分区用 `getPaymentErrorTranslation`）及其测试、`lib/format.ts`（Creem 产品编辑器用）、`types.ts`（`CreemProduct`）、`constants.ts`、`components/wallet-stats-card.tsx`。
- 路由 `/wallet?show_history=true`（上游 Waffo 的返回地址）改成滚动到新的充值记录。

## 上游充值折扣编辑器（任务 4.3）

后台「支付设置」里的「充值折扣」换成「充值加赠」，`amount-discount-dialog.tsx`、`amount-discount-visual-editor.tsx` 没有引用了，删掉。上游配置项 `payment_setting.amount_discount` 保留（后端上游代码还读它，我们的报价不用它）。

## 上游默认首页（任务 8.3）

没配自定义首页时，首页换成 `web/src/features/adty-home/` 的落地页（`AdtyLanding`）。上游默认首页的各区块和只给它们用的组件、常量、图标映射没有引用了，删掉，路径见 [deleted-paths.txt](deleted-paths.txt)：

- `web/src/features/home/components/`：`sections/{cta,features,hero,how-it-works,stats}.tsx`、`connection-line.tsx`、`feature-item.tsx`、`gateway-card.tsx`、`hero-buttons.tsx`、`hero-terminal-demo.tsx`、`icon-card.tsx`、`scrolling-icons.tsx`、`stat-item.tsx`、`index.ts`
- `web/src/features/home/constants.ts`
- `web/src/features/home/lib/icon-mapper.tsx`

页面上的变化：加载自定义首页内容时不再显示「Loading...」，直接显示落地页；后台自定义首页（iframe、HTML、Markdown）的分支不变。

- 提交：`feat(home): new landing page`
- 保留：`features/home/{index.tsx,api.ts,hooks/,types.ts}`，自定义首页分支还在用。`web/src/i18n/locales/*.json` 里旧首页的键照约定 B 保留。`scripts/adty/baseline-lint.txt` 里这几个文件的基线行随文件删掉；`docs/adty/baseline.md` 是基线当时的记录，不改。

## 上游的发布工作流

上游的 5 个 GitHub Actions 工作流删掉，镜像只由 `adty-ghcr.yml` 构建，`ci.yml` 照常保留。这几个工作流推标签或推 main 就会触发，会用上游的仓库名、Docker Hub 账号、GitCode 同步和桌面端签名去发布，在我们的仓库里不该跑。路径见 [deleted-paths.txt](deleted-paths.txt)：

- `.github/workflows/docker-build.yml`
- `.github/workflows/docker-image-branch.yml`
- `.github/workflows/electron-build.yml`
- `.github/workflows/release.yml`
- `.github/workflows/sync-release-to-gitcode.yml`

- 提交：`chore(delete): remove upstream release workflows`
- 合并上游时，`scripts/adty/merge-upstream.sh` 按删除清单把它们再删掉。

## 支付合规确认（2026-11-01 用户要求去掉）

上游要求超级管理员在「系统设置 → 支付设置」里确认一次合规条款，没确认之前所有充值入口都返回「Payment features are locked until the root administrator confirms the compliance terms.」，支付设置整块变灰不能改。adtyai 不需要这一步，支付默认就能用。

- 后端：`setting/operation_setting/payment_setting.go` 的 `IsPaymentComplianceConfirmed()` 直接返回 true。调用它的检查都自动放行，这些文件没改：`controller/payment_compliance.go` 的 `requirePaymentCompliance`、`controller/topup.go`、`controller/adty_topup.go`、`controller/adty_epay.go`、`controller/topup_bepusdt.go`、`controller/payment_webhook_availability.go`、`service/adty_topup_quote.go`。
- 前端：`payment-settings-section.tsx` 删掉「需要合规确认 / 已确认合规」提示、确认对话框、没确认时把设置变灰的那层，以及只给它们用的状态、请求、`CURRENT_COMPLIANCE_TERMS_VERSION` 常量、`complianceDefaults` 属性和导入；`billing/section-registry.tsx` 不再传 `complianceDefaults`；钱包充值卡（`wallet/adty/components/adty-recharge-card.tsx`）删掉没确认时显示「Top-up is not open yet」的分支。
- 保留：确认接口 `POST /api/option/payment_compliance`（`ConfirmPaymentCompliance`）、设置项 `payment_setting.compliance_*`、`/api/adty/topup/info` 返回的 `compliance_confirmed`（现在恒为 true）、钱包类型里的这个字段、语言包里的旧键（约定 E）。它们都不再起作用。
- 测试：`router/adty_topup_test.go`、`service/adty_topup_quote_test.go` 和前端 `adty-recharge-card.test.tsx` 原来测「没确认时拒绝」，改成测「不确认也能充值」；`router/adty_bepusdt_test.go` 的 `TestBEpusdtPayRequiresComplianceAndHandlesGatewayFailure` 改名为 `TestBEpusdtPayHandlesGatewayFailure`，删掉验证锁本身的前半段（「不确认也能下单」已由 `TestAdtyWalletTopUpAPIs` 覆盖）。上游的 `controller/payment_webhook_availability_test.go` 只是在准备阶段设成已确认，没断言锁，不用改。
- 提交：`chore(delete): drop the payment compliance confirmation`

## 独立营销中心（最新产品范围）

删除营销 UI、权限、注册同意和来源采集、个人偏好、联系人同步、群发和自动营销任务及旧接口；路径见 deleted-paths.txt。旧 API 在插件分发和外置前端重定向前返回 JSON 404。

删除营销中心时曾保留付费充值优惠码和站长优惠码，历史 `promo_enabled`、审计事件名和旧表仍不删除。当前规则已经改变：优惠码报价和写入拒绝新使用，历史行不重算。充值额度只走兑换码。系统公告、系统邮件、站点经营通知和发件域名任务仍保留。新库不创建七张营销专属表。

## 模型部署管理（最新产品范围）

删除 io.net 部署、硬件/地区/报价、实例与容器管理 API、SDK、前端部署分区、对话框及系统设置。旧 `/api/deployments` 精确路径和子路径在插件分发与外置前端重定向前返回 JSON 404；Root、普通管理员和旧 PAT 均不能操作。`model_deployment.*` 不再经通用配置 API 返回或接受写入，旧持久化值和历史审计不删除。

旧 `/console/deployment` 回退模型元数据，旧设置书签回退全局模型设置；渠道列表移除 io.net 部署快捷入口。保留模型/厂商元数据、正常渠道、中转协议、云供应商区域字段、历史渠道数据及审计事件显示；语言包旧键和上游赞助署名保留。完整删除路径见 deleted-paths.txt。

## Telegram OAuth 登录与二次验证

约定 I 写的是「Telegram 只有两个通知机器人……绑定不复用 Telegram OAuth，也不能登录、找回密码或充当二次验证」，但上游把这条链完整实现成了可用功能：Root 填 `telegram.client_id` / `telegram.client_secret`，再 `PUT /api/option/ TelegramOAuthEnabled=true`，`POST /api/oauth/state` 就能拿到 `authorization_url`，回调直接发会话。凭据还明文存进 options 表。本提交按「隔离或退役」的要求把它退役。

- 删除 `setting/system_setting/telegram.go`（`ClientID`/`ClientSecret` 的明文存储，路径见 [deleted-paths.txt](deleted-paths.txt)）。
- `oauth/telegram.go` 只保留退役说明和错误值：`TelegramProvider` 仍注册（占住 `telegram` 这个名字，防止被自定义 OAuth Provider 顶替），但 `IsEnabled()` 恒为 false，换 token / 取用户信息直接返回 `ErrTelegramOAuthNotConfigured`；`TelegramOAuthFlow`、`NewTelegramOAuthFlow` 和 PKCE 流程删除。
- `controller/oauth.go` 的 `/api/oauth/state` 对 `provider=telegram` 直接返回 `TELEGRAM_OAUTH_NOT_CONFIGURED`；回调里读 `flowPayload.Telegram` 的分支、绑定分支和 `authorization_url` 分支一并删除。
- 配置项：`TelegramOAuthEnabled`、`telegram.client_id`、`telegram.client_secret` 不再出现在 `/api/status`、`/api/option/`，`PUT /api/option/` 写入返回 400；`model.InitOptionMap` 启动时删掉历史 `telegram.client_id`/`client_secret` 行（这是凭据，不是要留的兼容配置）。
- 保留：`GET /api/oauth/telegram/login`、`POST /api/oauth/telegram/bind/start`、`GET /api/oauth/telegram/bind/:flow_token` 的永久 410（`TELEGRAM_LEGACY_AUTH_REMOVED`）；`telegram_id` 列和历史绑定数据；用户通知 / 管理通知两个机器人及其 `telegram:*` 加密 Token（`model/adty_telegram.go`）；语言包旧键。
- 测试：`controller/telegram_test.go` 由「登录/绑定/二次验证成功」改成「登录被拒、开关与凭据不可写、历史凭据被清理、旧路由仍是 410」；`controller/security_enrollment_test.go` 和 `controller/auth_flow_test.go` 里依赖 Telegram 登录成功的用例删除。
- 提交：`ISS-R4-0121`

## 第三方登录（GitHub / Discord / LinuxDO / OIDC / 自定义 OAuth / WeChat）

站点要开多个分站，每个第三方登录都要在对应平台按分站域名逐个登记回调，维护成本过高。老板决定一次性下掉六种登录方式，只保留账号密码、邮箱验证和 Passkey。整个删掉的文件见 [deleted-paths.txt](deleted-paths.txt)。

### 文件（后端）

- `oauth/github.go`、`oauth/discord.go`、`oauth/linuxdo.go`、`oauth/oidc.go`、`oauth/generic.go`（通用自定义 OAuth provider）、`oauth/outbound.go`（换 token、取用户信息的出站请求），以及只测它们的 `oauth/canonical_redirect_test.go`、`oauth/linuxdo_redirect_test.go`、`oauth/oidc_test.go`、`oauth/oidc_flow_test.go`
- `setting/system_setting/oidc.go`、`setting/system_setting/discord.go` 和 `setting/system_setting/oidc_test.go`：OIDC / Discord 的开关、显示名和凭据
- `controller/custom_oauth.go`（自定义 provider 的增删改查、discovery、绑定和解绑）及 `custom_oauth_delete_test.go`、`custom_oauth_discovery_test.go`、`custom_oauth_slug_test.go`、`custom_oauth_unbind_test.go`
- `controller/wechat.go`（微信扫码登录和绑定）及 `controller/wechat_site_register_test.go`
- `model/custom_oauth_provider.go`、`model/user_oauth_binding.go`：两张表的模型
- `controller/misc_oidc_test.go`、`controller/oidc_oauth_test.go`、`model/builtin_oauth_subject_test.go`、`model/oauth_binding_site_index_order_test.go`：只测这些 provider 的用例，随实现一起删

### 路由

删掉的注册：

- `GET /api/oauth/wechat`、`POST /api/oauth/wechat/bind`
- `/api/custom-oauth-provider/` 整个组（`middleware.RootAuth`）：`POST /discovery`、`GET /`、`GET /:id`、`POST /`、`PUT /:id`、`DELETE /:id`
- `GET /api/user/oauth/bindings`、`DELETE /api/user/oauth/bindings/:provider_id`（自己的绑定，登录用户组）
- `GET /api/user/:id/oauth/bindings`、`DELETE /api/user/:id/oauth/bindings/:provider_id`（管理员代查代解绑）

`GET /api/oauth/:provider` 这条统一回调壳保留，但六个 provider 都不再注册，`/api/oauth/github`、`/api/oauth/discord`、`/api/oauth/oidc`、`/api/oauth/linuxdo` 现在都返回 400 `Unknown OAuth provider`（`controller.HandleOAuth` 在 `oauth.GetProvider` 里查不到就直接拒绝）。`router/adty_deleted_routes_test.go` 盯着上面这些路由不许回来，同时断言壳路由、`POST /api/oauth/state`、`/api/oauth/email/bind*` 和 Telegram 旧路由还在。

### 表和列

- `custom_oauth_providers`、`user_oauth_bindings`：`model/main.go` 的 `migrateDB` 不再 AutoMigrate 这两张表，`EnsureUserOAuthBindingSiteSubjectIndex` 也删了。已经存在的表和里面的历史数据不删（没有写 DropTable / DropColumn），只是不再读写。
- `users` 的 `github_id`、`discord_id`、`oidc_id`、`wechat_id`、`linux_do_id`：列和历史数据保留，`model.User` 上的这些字段和 `builtinOAuthSubjectColumns` 的唯一性检查也保留；没有写删列迁移，只是不再有登录流程会写它们。

### 配置项

- `GitHubOAuthEnabled`、`GitHubClientId`、`GitHubClientSecret`
- `LinuxDOOAuthEnabled`、`LinuxDOClientId`、`LinuxDOClientSecret`、`LinuxDOMinimumTrustLevel`
- `WeChatAuthEnabled`、`WeChatServerAddress`、`WeChatServerToken`、`WeChatAccountQRCodeImageURL`
- `discord.*`（`enabled`、`client_id`、`client_secret`）、`oidc.*`（`enabled`、`display_name`、`client_id`、`client_secret`、`well_known`、`authorization_endpoint`、`token_endpoint`、`user_info_endpoint`）

`GET /api/status` 不再返回这些键（`github_oauth`、`github_client_id`、`discord_oauth`、`discord_client_id`、`linuxdo_oauth`、`linuxdo_client_id`、`linuxdo_minimum_trust_level`、`wechat_login`、`wechat_qrcode`），`model/option.go` 的 `InitOptionMap` / `updateOptionMap` 和 `controller/option.go` 里「没填 Client ID 不许打开」的校验一并删了；`common.GitHub*` / `LinuxDO*` / `WeChat*` 变量随后在 `b1d1049` 删掉（那时已经没人读）。数据库 options 表里的历史行保留，只是不再读取。

### 前端

- `web/src/lib/oauth.ts`（发起登录、轮询、换票）、`web/src/routes/(auth)/oauth.tsx`、`web/src/routes/oauth/$provider.tsx`（OAuth 落地页；`web/src/routeTree.gen.ts` 已重新生成）
- `web/src/features/auth/lib/oauth.ts`、`oauth-callback-mode.ts`、`oauth-handoff.ts`、`oauth-popup.ts`、`oauth-bind-window.ts` 及各自测试，`web/src/features/auth/hooks/use-oauth-login.ts` 和它的测试
- `web/src/features/auth/components/oauth-providers.tsx`、`oauth-callback-screen.tsx`
- `web/src/features/system-settings/auth/custom-oauth/`（自定义 OAuth 管理页：provider 表格、表单、preset、discovery、访问策略模板、接口调用）和 `oauth-section.tsx`、`oauth-callback-url.ts`
- 品牌图标 `web/src/assets/brand-icons/icon-github.tsx`、`icon-discord.tsx`、`icon-linuxdo.tsx`、`icon-wechat.tsx`（`index.ts` 的导出一起去掉）
- `web/src/features/security/components/dialogs/wechat-bind-dialog.tsx`，以及用户绑定对话框里的第三方部分
- `web/src/features/auth/lib/telegram-login.ts` 和它的测试：前端的 Telegram 登录入口，同一批删掉

登录页、注册页只剩账号密码（注册带邮箱验证），安全页的账号绑定卡片只留邮箱、Passkey 和 2FA。前端语言包旧键按约定 B 保留。

### 影响

曾只用第三方登录、从来没设过密码的账号登录不了，只能用登录页的「忘记密码」走 `GET /api/reset_password` 发邮件，重设密码后再登。

### 提交

- `86536ce` `feat(auth)!: retire all third-party sign-in, leaving password and email only`
- 整个删掉的文件和目录：[deleted-paths.txt](deleted-paths.txt)

### 保留名单和理由

- `oauth/telegram.go`：退役占位 provider，`IsEnabled()` 恒为 false，换 token / 取用户信息直接返回错误；注册它只为占住 `telegram` 这个名字，免得被自定义 OAuth Provider 顶替。登录链和二次验证的退役见上一节。
- `model/adty_telegram.go` 的绑定和两个通知机器人及它们的加密 Token：通知通道，和登录无关，一字未动。
- `GET /api/oauth/:provider` 回调壳、`POST /api/oauth/state`、`POST /api/oauth/email/bind/start`、`/api/oauth/email/bind/resend`、`/api/oauth/email/bind`：邮箱绑定和验证还在用三条 `/api/oauth/email/bind*`；壳和 state 现在没有启用的 provider 可走，除 `provider=telegram` 报 `TelegramConfigurationError()` 外一律 400 `Invalid parameters`。
- Telegram 三个旧路由的永久 410（`TELEGRAM_LEGACY_AUTH_REMOVED`）：旧客户端和旧书签不该在合上游后复活。
- `controller/oauth.go` 里绑定意图（`auth_flow` 的 `Intent == bind`）的分支：目前没有 provider 能走到，留着是为了不把通用回调和 state 的壳拆散。
- `service/adty_site_resolve.go`：分站状态里仍把 `github_oauth`、`discord_oauth`、`linuxdo_oauth`、`telegram_oauth`、`wechat_login`、`oidc_enabled` 置 false，删掉 `custom_oauth_providers` 键，把 `wechat_qrcode` 置空——前端历史版本读这些键时得到「关」，不会报错。
- `common.WeChatAccountQRCodeImageURL`：只剩一个空值，给上面那条归一用。
- `users` 的五个历史绑定列和历史数据（见上面「表和列」）：没有删列迁移，旧库照常启动。
- `router/adty_deleted_routes_test.go`：守卫测试，这些路由跟着上游回来会失败。
- `web/src/i18n/locales/*.json` 里第三方登录相关的键：前端语言包只增不删（约定 B）。

## 六种非正式语言的资源（2026-11-16）

正式语言只有简体中文和英文。繁体中文、法语、日语、韩语、俄语、越南语的资源原来留在仓库里但不参与产品，现在删除。

- 删除前端语言包 `web/src/i18n/locales/` 下的 `zh-TW.json`、`fr.json`、`ja.json`、`ko.json`、`ru.json`、`vi.json`，后端 `i18n/locales/` 下这六种语言的 yaml 和 `adty.*.yaml`，文档站 `web/src/features/adty-docs/content/` 下这六种语言的目录，以及 `docs/translation-glossary.fr.md`、`docs/translation-glossary.ru.md`。
- `i18n/i18n.go` 只加载 `zh-CN.yaml` 和 `en.yaml`，删除只为读取这些语言包而存在的 `LoadableLanguages`、`TranslateLoadable`、`canonicalLoadableLang`、`loadableLocalizer` 和六个语言常量。文档站 `content/index.ts` 只登记两种语言的页面。
- 保留语言码归一化：中文变体归简体中文，其余归英文，所以带旧语言码的请求、Cookie 和用户偏好照常工作；`toIntlLocale` 对旧界面码的映射也保留。
- 保留分组名称、法律页面和首页 SEO 文本里按内容语言码存的键（`model.AdtyContentLangs`、前端 `LEGACY_CONTENT_LANGS`），以及首页旧语言路径（`/fr` 等）到正式语言的处理。
- 保留根目录的 `README.fr.md`、`README.ja.md`、`README.zh_TW.md`，它们是上游项目说明。
- 这些路径登记在 [deleted-paths.txt](deleted-paths.txt)，合并上游时不带回来。

## 没有任何引用的前端文件（2026-11-16）

按 knip 的结果删除 103 个文件，删除后类型检查、生产构建和全部前端测试通过。这些文件没有任何页面、组件或测试引用。

- `web/src/components/ai-elements/`：聊天界面组件，Playground 删除后只有 `code-block.tsx` 还在用；除它和它的两个测试外全部删除，包括只被自己的测试引用的 `prompt-input.tsx`、`response*.ts(x)`。
- `web/src/components/ui/` 下没有引用的基础组件：`aspect-ratio`、`breadcrumb`、`carousel`、`context-menu`、`direction`、`hover-card`、`input-otp`、`kbd`、`menubar`、`navigation-menu`、`pagination`、`resizable`、`slider`。
- `web/src/assets/` 下没有引用的品牌图标和 Logo 组件，`web/src/components/layout/components/` 下的 `glow`、`logo`、`mockup`、`navbar`、`section`，以及 `animate-in-view`、`auto-skeleton`、`coming-soon`、`model-group-selector.tsx`、`lib/use-controllable-state.ts`。
- `web/src/features/adty-home/components/` 下首页没有渲染的区块：`client-guides`、`faq`、`landing-section`、`payment-strip`、`popular-models`、`price-compare-card`、`price-text`、`selling-points`、`steps`。
- 没有引用的汇总导出文件：`features/auth/index.ts`、`features/pricing/hooks/index.ts`、`features/pricing/lib/index.ts`、`features/rankings/lib/index.ts`、`features/usage-logs/lib/index.ts`，以及 `features/adty-promo/lib/dates.ts`。
- `web/src/i18n/static-keys.ts`：人工登记的文案清单，没有读取方，不参与提取或校验。
- 移除不再使用的依赖：`ai`、`auto-skeleton-react`、`input-otp`、`nanoid`、`react-resizable-panels`、`sse.js`、`stream-markdown-parser`、`tokenlens`、`use-stick-to-bottom`、`@xyflow/react`、`embla-carousel-react`，并从 `THIRD-PARTY-LICENSES.md` 去掉对应行。`web/src/styles/index.css` 去掉只给 `auto-skeleton-react` 用的样式。
- 保留只被测试引用的文件（如 `risk-acknowledgement-dialog.tsx`、`theme-quick-switcher.tsx`）和未使用的导出，另行判断。
- 这些路径登记在 [deleted-paths.txt](deleted-paths.txt)。以后要用其中的基础组件时，从上游或组件库重新加入并从清单里去掉对应路径。

## 第三方登录框架（2026-11-16）

六种第三方登录删掉以后，OAuth 框架只剩一个永远不可用的 Telegram 占位。这次把框架本身也删掉。

### 文件

- `oauth/`（provider 接口、注册表、Telegram 占位、token 类型）
- `controller/oauth.go`（`GenerateOAuthCode`、`HandleOAuth` 和登录、绑定、二次验证三种回调分支，`findOrCreateOAuthUser`）、`controller/telegram.go`（Telegram 旧路由的 410）
- `service/oauth_verification.go`（用已绑定的第三方账号做二次验证）
- `controller/oauth_site_flow_test.go`、`controller/oauth_site_identity_test.go`、`controller/oauth_site_register_test.go`：测 OAuth 登录把账号归到发起登录的分站，随实现一起删

### 路由

- `POST /api/oauth/state`、`GET /api/oauth/:provider`
- `GET /api/oauth/telegram/login`、`POST /api/oauth/telegram/bind/start`、`GET /api/oauth/telegram/bind/:flow_token`（原来固定返回 410）
- 访问令牌规则里对应的条目一起删除。`router/adty_deleted_routes_test.go` 盯着它们不许回来。

### 二次验证

- 删除验证方法 `oauth` 和验证范围 `account.binding.unbind`（按 `provider_id` 解绑第三方账号，已经没有入口）。`account.binding.bind` 只接受 `provider=email`。
- `/api/verify/methods` 不再返回 `oauth_providers`；重要操作的方法清单是密码（有密码时）、邮箱验证码（有邮箱时）、已登记的 TOTP 和 Passkey。
- 原来没有密码的账号会多列一个“已绑定的第三方账号”方法。现在所有建号入口（注册、管理员建号、团队注册、员工账号、初始化）都要求密码，不会再产生这种账号；历史上只用第三方登录建的账号可以走“忘记密码”重设密码，有邮箱时也能用邮箱验证码。
- 前端二次验证去掉 `oauth` 方法、`oauth_providers` 字段和只为它存在的错误码映射（`TELEGRAM_OAUTH_*`、`TELEGRAM_ACCOUNT_NOT_BOUND`、`TELEGRAM_LEGACY_AUTH_REMOVED`、`OAUTH_ACCOUNT_MISMATCH`）。

### 保留名单和理由

- `/api/oauth/email/bind/start|resend` 和 `/api/oauth/email/bind`：邮箱绑定，路径沿用旧前缀；站点上下文中间件对 `/api/oauth` 前缀的处理（分站限流、自定义域名拒绝）不变，`TestCustomDomainRejectsOAuthPath` 改用邮箱绑定路由钉住。
- `users` 的 `github_id`、`discord_id`、`oidc_id`、`wechat_id`、`linux_do_id`、`telegram_id` 列、`external_identity_claims` 表和它们的唯一索引：历史数据，启动迁移照旧，不写删列。
- `Telegram*` 选项的写入拒绝和启动时清理历史 `telegram.client_*` 凭据（`TestTelegramOAuthOptionsCannotBeWritten`、`TestTelegramOAuthLegacyCredentialsArePurged`）。
- 前端 `loginMethodLabel` 对 `oauth`、`oauth:github` 等的显示：历史会话和审计日志里还有这些值。
- `service/codex_oauth.go`：Codex 渠道的凭据刷新，和用户登录无关。
- 两份语言包里的旧文案：按约定 B 只增不删。

## 四家托管支付、签到、支付合规确认、优惠码后台和公开状态页（2026-11-16）

项目还没上线，没有这些功能的历史订单和数据。Stripe、Creem、Waffo、Waffo Pancake 决定不再接回，签到、支付合规确认、优惠码管理和公开状态页之前已经关掉或只剩空壳，这次连同代码一起删除。

### 文件（后端）

- 四家托管支付：`controller/topup_stripe.go`、`controller/topup_creem.go`、`controller/topup_waffo.go`、`controller/topup_waffo_pancake.go`、`controller/adty_hosted_payment.go`、`controller/adty_hosted_query.go`、`controller/adty_hosted_registry.go`、`controller/payment_webhook_availability.go`、`service/waffo_pancake.go`、`setting/payment_stripe.go`、`setting/payment_creem.go`、`setting/payment_waffo.go`、`setting/payment_waffo_pancake.go`、`constant/waffo_pay_method.go`
- 签到：`controller/checkin.go`、`setting/operation_setting/checkin_setting.go`；`model/checkin.go` 只留表结构
- 支付合规确认：`controller/payment_compliance.go`
- 永久 403 中间件 `middleware/adty_closed_features.go`：挂在上面这些入口上，入口删掉后没有用处了
- 优惠码管理：`controller/adty_promo.go`、`setting/adty_setting/promo.go`
- 部分删除：`model/topup.go` 里上游单通道易支付、四家支付和管理员补单的入账与退款函数（`RechargeEpay`、`Recharge`、`RechargeCreem`、`RechargeWaffo`、`RechargeWaffoPancake`、`RefundWaffo`、`RefundWaffoPancake`、`ManualCompleteTopUp`）、用户充值记录查询（`GetUserTopUps`、`SearchUserTopUps`）和四家的 provider 常量；`controller/topup.go` 里上游单通道易支付的下单、回调、充值信息和管理员补单（路由在本次之前已经关闭）；`model/adty_topup_order.go`、`model/option.go`、`service/adty_payment_scene.go` 里拒绝四家的写入保护；`service/adty_team_pay.go` 的团队 Stripe 充值
- 依赖：`go mod tidy` 移除 `stripe-go`、`waffo-go`、`waffo-pancake-sdk-go`、`randstr`，以及第三方登录框架删除后留下没用的 `go-oidc`、`golang.org/x/oauth2`、`moego`；`THIRD-PARTY-LICENSES.md` 同步删行
- 只测这些代码的测试文件随实现删除；混在别的测试里的用例改成用 BEpusdt 或易支付，或者删掉对应用例。管理员查单测试改用可控的易支付查单结果

### 路由

- `POST /api/stripe/webhook`、`POST /api/creem/webhook`、`POST /api/waffo/webhook`、`POST /api/waffo-pancake/webhook/:env`
- `POST /api/user/stripe/pay|amount`、`POST /api/user/creem/pay`、`POST /api/user/waffo/pay|amount`、`POST /api/user/waffo-pancake/pay|amount`、`POST /api/adty/topup/hosted/pay`
- `POST /api/option/waffo-pancake/catalog|pair|save`
- `GET`/`POST /api/user/checkin`
- `GET`/`POST /api/user/epay/notify`（上游单通道回调；正式回调是 `/api/user/epay/notify/:channelId`）、`GET /api/user/topup/info`、`GET /api/user/topup/self`、`POST /api/user/amount`、`POST /api/user/topup/complete`
- `POST /api/option/payment_compliance`
- `GET`/`POST /api/adty/admin/promo-codes`、`PUT /api/adty/admin/promo-codes/:id`、`GET`/`POST /api/adty/site/promo-codes`、`PUT`/`DELETE /api/adty/site/promo-codes/:id`
- 访问令牌规则、审计登记、`middleware/testdata/adty_admin_routes.txt` 和 `docs/openapi/api.json` 里对应的条目一起删除。`router/adty_deleted_routes_test.go` 盯着它们不许回来。

### 配置项

- `Stripe*`、`Creem*`、`Waffo*`、`WaffoPancake*`、`checkin_setting.*`、`payment_setting.compliance_*`、`adty_marketing_setting.promo_enabled` 不再注册。数据库里的旧行留着，启动加载时忽略；`adty_marketing_setting.*` 整组不能再通过设置接口写入。
- `/api/status` 不再返回 `stripe_unit_price` 和 `checkin_enabled`，`/api/adty/topup/info` 不再返回 `compliance_confirmed`。

### 二次验证

- 删除站长二次验证动作 `promo`（只给优惠码管理用）。

### 前端

- 支付设置只剩通用、Epay、USDT 三个标签：删掉隐藏的 Stripe、Creem、Waffo、Waffo Pancake 标签和表单字段、Creem 商品编辑器、Waffo 设置、Pancake 绑定流程和“托管支付已退役”提示。
- 支付方式图标改成下拉选择，只提供支付宝、微信、USDT 和通用卡片四个图标；`components/react-icon-by-name.tsx` 不再按名字动态加载 30 个 react-icons 图标包。支付方式类型去掉 `stripe` 和 `waffo_pancake`。
- 钱包去掉托管支付弹窗和 `payWithHosted`，订单恢复只认 USDT 和易支付；删掉只给旧版上游钱包用的 `wallet/lib/payment.ts`、`wallet/lib/format.ts`、`wallet/constants.ts` 和 `wallet/types.ts` 里的旧接口类型。
- 支付场景不再在前端过滤四家（`adty-payment-scenes/retired-providers.ts`），后端已经不认这些支付商。
- 优惠码：系统设置的“优惠码”分区、站长中心的“优惠码”标签和 `features/adty-promo/` 删除。
- 公开状态页：`routes/status.tsx` 和 `features/adty-status/` 删除，维护页去掉指向它的“了解更多”按钮。
- `public/` 下没用到的 `pay-card.png`、`pay-apple.png`、`pay-google.png`、`waffo-logo-dark.svg`、`waffo-logo-light.svg` 删除。

### 保留名单和理由

- `users.checkin_quota` 列、`checkins` 表的迁移和退款时把签到赠送排除在本金之外的逻辑（`TestCheckinQuotaIsNotRefundedAsPrincipal`）：签到赠送的历史额度仍在钱包里时，退款不能把它当本金退。
- `users.stripe_customer` 列：上游表结构，启动迁移照旧，不写删列。
- 下单链路里的优惠码字段和拒绝逻辑（`AdtyPromoInput`、`ErrAdtyPromoRetired`、`PromoCodeUserRateLimit`）以及 `adty_promo_codes`、`adty_promo_redemptions` 表：随后在「下单流程里的优惠码」一节删除。
- 上游单通道易支付的设置（`PayAddress`、`EpayId`、`EpayKey`、`PayMethods`）和 `MigrateLegacyAdtyEpayChannel`：随后在下一节删除。
- `/api/uptime/status` 和 Uptime Kuma 设置：后台仪表盘还在用。
- 两份语言包里的旧文案：按约定 B 只增不删。

## 上游单通道易支付的旧设置和「支付方式」编辑器（2026-11-16）

钱包和团队充值的支付按钮早就来自易支付多通道和 USDT 链，上游单通道易支付的设置只剩启动时的一次性迁移在读：把旧的网关地址、商户号、密钥和「支付方式」列表转成一个叫「Legacy Epay」的通道。项目没上线，没有旧配置要迁；留着反而有隐患——在这几个旧输入框里填了值，下次重启会自动多出一个通道。

### 文件

- `model/adty_epay_legacy_migration.go` 和它的测试（`MigrateLegacyAdtyEpayChannel`，`main.go` 不再调用）
- `web/src/features/system-settings/integrations/payment-method-dialog.tsx`、`payment-methods-visual-editor.tsx`（「支付方式」可视化编辑器和弹窗）
- `web/src/components/react-icon-by-name.tsx`：只给上面的编辑器显示图标
- 部分删除：`setting/operation_setting/payment_setting_old.go` 的 `PayAddress`、`EpayId`、`EpayKey`、`PayMethods` 和三个读写函数；`model/option.go` 里这几个键的注册、校验和读写分支；支付设置「通用」页签的「支付方式」字段和「Epay」页签的网关地址、商户号、密钥三个输入框

### 依赖

- `react-icons` 不再有使用方，从 `web/package.json`、`bun.lock` 和 `THIRD-PARTY-LICENSES.md` 删除。

### 保留名单和理由

- 回调地址 `CustomCallbackAddress`（Epay 页签里的「回调地址」）：多通道易支付和 USDT 下单时拼回调地址要用。
- `Price`（人民币单价）、`MinTopUp`（最低充值）、`USDExchangeRate`：报价和显示还在用。
- 数据库里这几个键的旧行：启动加载时忽略，不写删除。
- 两份语言包里的旧文案：按约定 B 只增不删。

## 下单流程里的优惠码（2026-11-16）

优惠码早已退役，报价只要带码就拒绝；管理面板和接口上一批已删。项目没上线，没有用过优惠码的订单，剩下的下单流程部分一起删掉。

### 文件

- `model/adty_promo_code.go`（优惠码和核销表、资格判断、核销、入账前的资格锁 `LockAdtyPromoEligibilityTx`）
- `service/adty_promo.go`（`AdtyPromoInput`、`ErrAdtyPromo*`、`AdtyUserSiteID`）和它的测试
- `middleware/adty_promo_rate_limit.go`（只对带码报价按用户限速）和它的测试
- `router/adty_promo_test.go` 改名为 `router/adty_marketing_removed_test.go`，只留营销接口 404 的用例

### 改动

- 报价 `AdtyQuoteFor` / `AdtyQuoteForSite` 去掉优惠码参数；`/api/adty/topup/quote`、`/api/adty/epay/pay`、`/api/user/bepusdt/pay` 不再读 `promo_code`（带了也忽略），报价结果不再有 `promo_code_id`、`promo_bonus_usd`。
- `adty_topup_orders` 去掉 `promo_code_id`、`promo_bonus_usd`、`promo_funder` 三个字段；入账不再核销优惠码，也不再先锁同指纹用户（那把锁只为「首单优惠」的资格判断）。
- 站长出资的加赠只剩支付方式加赠：站长收益、赠金预留、资金异常检测里「站长出资优惠码」的部分删除。
- 订单详情去掉「历史优惠码赠金」分区（`section=bonuses`）和 `promo_bonus_usd`；前端订单详情、钱包金额和充值记录不再加优惠赠金。
- 新库不再创建 `adty_promo_codes`、`adty_promo_redemptions`。

### 保留名单和理由

- 升级库里的 `adty_promo_codes`、`adty_promo_redemptions` 表和 `adty_topup_orders` 的三个旧列：不写删表删列，代码不再读写（`TestMarketingRemovalDatabaseMatrix` 盯着升级时旧行不变）。
- 两份语言包里的旧文案：按约定 B 只增不删。

## 二开代码里的死函数（2026-11-16）

用 `deadcode` 从 `main` 出发找不可达的函数，再对照上游 v1.0.0-rc.47 的文件清单，只处理二开自己的文件；上游文件里的不可达函数不动，免得以后合并上游时到处冲突。

### 删除

- 连测试都不调用的 16 个函数：`ParseOfficialPlanHeadings`、`lockUserQuotaTx`、`dropIndexIfExists`、`adtyManualRefundCapRemaining`、`migrateAdtyReferralTables`、`seedAdtyPaymentScenes`、`RecordAdtyRefundProof`、`AdtyTicketStatusIsLive`、`adtyDuplicateEventTrades`、`initGroupHealthThreshold`、`siteCommissionWithdrawnOutstanding`、`AdtySiteBonusRates`、`EvaluateAdtySiteTier`、`AdtySiteForEmail`、`adtyTaskRecalcGroupRatio`，以及测试夹具里没人调用的 `fundSettleFixture.settle`。
- `service/adty_remaining_gaps.go` 里 65 个未导出函数：文件开头写明它们是「剩余缺口的草稿，不是已上线的接口或功能」，只有测试在调用。对应测试里只测这些草稿的四个用例整个删掉，混合用例只删草稿那几行；邀请绑定的用例改用正式的 `BindSiteInviteFromIP`。
- 只为测试存在的探针：`AdtyGroupHealthDoesNotReroute`（读令牌分组）、`AdtyTranslationCallsDoNotDebit`（读余额）。
- `ValidateAdtySiteSVG` 和它的白名单：Logo 上传早就只收 PNG、JPEG、WebP。
- `service/adty_fund_fifo.go`（`ConsumeFundFIFO`）和它的测试：生产按数据库里的资金批次扣，不用这个内存模型。
- 随之没人用的变量和常量：`officialHeading`、`adtySvixTolerance`、`errAdtySvixInvalid`、`adtyTelegramBound`、`ErrAdtySiteLogoUnsafe`。

### 保留名单和理由

- 带 `ForTest` 的钩子、`model/adty_testsupport.go`、`model/adty_pg_acceptance.go`、`internal/pgtest`、`common/logsinktest`：跨包共用的测试工具，本来就只给测试用。
- 被大量测试拿来造数据的服务函数（如 `OpenAdtyMarginCycle`、`ApplyAdtySiteWalletChange`、`CreateAdtyTopUp`、`JoinAdtyTeam`）：删了要重写大量测试，收益小。
- `ApplyAdtySite`、`NewAdtySiteInviteCode`、`SaveAdtySiteInviteCode`：自助开站退役记录（`RETIRE-site-self-apply`）里写明历史申请和邀请码数据保留，测试用它们造历史数据。
- `internal/adtygov`：治理记录，只由测试核对。
- 上游文件里的 79 个只被测试调用、190 个连测试都不调用的函数：不动。

## 后台邀请码接口（2026-11-16）

自助开站和邀请码开站已经退役，后台的邀请码接口只剩列表和三个永远返回 403 的写入口，也没有界面。

- 删除 `GET`/`POST /api/adty/admin/reseller/invite-codes`、`PUT`/`DELETE /api/adty/admin/reseller/invite-codes/:id` 和对应的处理函数、访问令牌规则；`router/adty_deleted_routes_test.go` 盯着它们不许回来。
- 删除只给它们用的权限动作 `adty_site:settings_create`、`adty_site:settings_delete`；旧的 `adty_site:settings` 迁移时只展开成 `settings_read`。`settings_read`、`settings` 的说明不再提邀请码。
- 保留：`adty_site_invite_codes` 表和历史行、`ApplyAdtySite` 与生成邀请码的服务函数（自助开站退役记录要求保留历史数据，测试用它们造历史数据）。
