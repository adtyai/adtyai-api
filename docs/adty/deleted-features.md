# 删掉的功能

## 签到奖励入口

- 删除个人页签到卡片、专用请求函数和响应类型，以及计费设置中的签到分区、默认值和表单。历史 `/system-settings/billing/checkin` 书签按现有未知分区规则回到额度设置。
- 删除 `web/src/features/profile/components/checkin-calendar-card.tsx` 和 `web/src/features/system-settings/general/checkin-settings-section.tsx`，不再受历史 `checkin_enabled` 或 `checkin_setting.*` 启用值影响。
- 保留 `GET/POST /api/user/checkin` 的永久 403、历史签到记录、`checkin_quota` 和退款来源保护；不改写或删除历史资金。
- 保留语言包旧键与历史数据库选项用于升级兼容；不再提供操作入口。
- 提交：`fix(ui): remove retired check-in controls and refresh read badges`。

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
