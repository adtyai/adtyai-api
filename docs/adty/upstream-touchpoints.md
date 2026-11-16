# 改过的上游文件

每改一个上游文件（包括上游的测试文件），在下表加一行：文件、改了什么、为什么（约定 B）。同一个文件在几个任务里改过，就记几行。合并上游时，冲突对照这张表解决。

整个删掉的上游文件和目录不记在这里，记进 [deleted-paths.txt](deleted-paths.txt) 和 [deleted-features.md](deleted-features.md)；在上游文件里删掉一部分代码算改动，记在这里。

各列写法：

- 上游文件：从仓库根目录写起的路径。
- 改了什么：改在哪里、改了什么。位置尽量写函数名，行号会随上游变。
- 为什么：这样改的原因。
- 任务：实施方案里的任务号。

| 上游文件 | 改了什么 | 为什么 | 任务 |
| `model/user.go` | OAuth 身份与单 Root 索引调用统一条件唯一索引 helper | MySQL 不支持原部分索引语法，不能放弃数据库约束 | R-02 |
| `model/task_plugin.go` | 活动插件索引调用统一 helper；历史折叠查询引用保留字 key | MySQL 完整启动及单活动版本约束 | R-02 |
| `model/model_meta.go` | 未删除模型名索引调用统一 helper | 三库保留同名历史和活动名称唯一性 | R-02 |
| `model/migration_dialector.go` | MySQL CreateTable 默认 DYNAMIC，保留显式行格式和其他表选项 | 最低支持版本 COMPACT 的索引长度限制不能容纳现有 Passkey | R-02 |
| `router/api-router.go` | 上游单通道易支付下单和回调、Stripe/Creem/Waffo 下单和回调、签到、旧管理员补单改成 `middleware.AdtyClosedFeature`，在登录中间件之前拒绝 | 这些入口能绕过统一订单、加赠、佣金和余额流水入账；规范明确不做签到和未配置的卡支付 | 审查 v3 |
| `router/relay-router.go` | `/pg` 在 `Distribute` 之前挂 `AdtyUserLimit`（这个挂载后来随 Playground 删除一起撤掉了，`/pg` 现在 404，见「全平台验收 A/S」两条） | 演练场当时是可用的文本入口，必须和 `/v1` 共用按用户的 RPM/TPM/并发限制 | 审查 v3 |
| `controller/topup.go` | `GetTopUpInfo` 不再返回上游易支付、Stripe、Creem、Waffo 的开关和支付方式 | 钱包不能展示已经关掉的充值入口 | 审查 v3 |
| `controller/topup_creem.go` | `verifyCreemSignature`、`CreemWebhook`、下单请求和上游响应日志不再写入签名、请求体、卡号、邮箱、姓名和结账链接 | 支付回调和上游响应会把密钥和持卡人数据写进日志 | 审查 v3 |
| `controller/topup_stripe.go` | `StripeWebhook` 收到回调的日志不再写入签名和请求体 | 支付回调日志不能保留签名或报文体 | 审查 v3 |
| `controller/topup_waffo.go` | `WaffoWebhook` 收到回调、验签失败和解析失败的日志不再写入签名和请求体；下单被拒不再写完整响应 | 支付回调和上游响应会把密钥和持卡人数据写进日志 | 审查 v3 |
| `controller/topup_waffo_pancake.go` | `WaffoPancakeWebhook` 收到回调和验签失败的日志不再写入签名和请求体 | 支付回调日志不能保留签名或报文体 | 审查 v3 |
| `controller/token.go` | `GetTokenKey`、`GetTokenKeysBatch` 在返回明文密钥前要求 `api_key.read` 二次验证 | 会话或访问令牌不能在没有二次验证时读出 API 密钥 | 审查 v3 |
| `controller/token_test.go` | 密钥查看用例改为期望 403；PAT 改用带 `api_key:reveal` 的访问令牌；批量查看密钥在二次验证拒绝时不保存无界 ID 列表 | 跟上二次验证，并证明响应和审计都不含明文密钥 | 审查 v3 |
| `controller/oauth.go` | `findOrCreateOAuthUser` 在分站要求条款且未接受时返回 `OAuthSiteTermsError` | 分站客户注册必须先接受当前条款 | 审查 v3 |
| `controller/user_quota.go` | 空幂等键和过长幂等键都映射为 `Invalid idempotency key` | 调额不能在没有幂等键时入账 | 审查 v3 |
| `controller/user_manage_test.go` | 调额请求补上幂等键 | 空幂等键现在会被拒绝，原用例要带上键才能走到原来的分支 | 审查 v3 |
| `controller/user_manage_test.go` | `large_odd_difference` 改为 `negative_override_keeps_cache`：负覆盖被拒绝，余额和缓存保持原值 | 2026-11-06 选定的调额下限不允许写成负数；原用例期望成功写入 `-MaxWalletQuota`，和这条规则冲突，门禁因此失败 | 审查 v3 |
| `controller/user_manage_test.go` | 增加 `subtract_rejects_when_live_cache_would_go_negative` | 数据库余额够减、缓存已更低时，减额不能把活余额写成负数 | 审查 v3 |
| `model/quota_reserve.go` | 配额变更统一走 `persistUserQuotaDelta`、`reserveUserQuotaDB`：在事务内 `lockForUpdate` 读用户行，按 `userWalletBalance(user.Quota, user.QuotaPending)` 的活余额判定后写库，提交后再 `invalidateUserCache`；没有写缓存的配额增量 Lua 脚本 | 缓存只做失效不做增量，活余额下限以数据库为准，人工调额与扣费都不会把活余额减成负数 | 审查 v3 |
| `model/user_quota_adjustment.go` | `AdjustUserQuota` 在写数据库前用活缓存做下限；缓存不可用时仍提交数据库变更 | 批量更新下缓存才是可花余额；缓存故障不能回滚已经决定的调额 | 审查 v3 |
| `router/relay-router.go` | `relayV1Router`、`/pg`、`/v1beta` 和 `/mj` 增加 `AdtyRequireCurrentSiteTerms`（`/pg` 那一处后来随 Playground 删除撤掉了） | 分站条款未接受时不能继续调用中转或演练场 | 审查 v3 |
| `router/api-router.go` | 令牌、日志、用量、任务和审计的自助路由在登录后增加 `AdtyRequireCurrentSiteTerms` | 条款升级后，未确认的分站客户不能继续管密钥或读自己的用量 | 审查 v3 |
| `router/adty-router.go` | 钱包充值、USDT 下单和易支付下单在登录后增加 `AdtyRequireCurrentSiteTerms` | 条款升级后，未确认的分站客户不能继续报价或下单 | 审查 v3 |
| `controller/midjourney.go` | 转发的 Midjourney 图片地址改成带过期签名的 `/mj/image/:id` | 只知道任务编号不能取到别人的图 | 审查 v3 |
| `relay/mjproxy_handler.go` | `RelayMidjourneyImage` 先验签和过期，再查任务和向上游取图；成功响应加 `Cache-Control: private, no-store` | 猜测任务编号不能取到图像；签名过期后也不能取 | 审查 v3 |
| `relay/mjproxy_handler_test.go` | 图片代理用例带上签名；另加未签名和过期签名不得取图 | 证明修复前未签名请求能拿到图，修复后不能 | 审查 v3 |
| `relay/channel/task/jsplugin/adaptor.go` | JSON 提交已经收到上游响应后，读体失败和 `parseSubmitResponse` 失败都设 `NoRetry` | 解析失败再换渠道会把已经受理的任务再提交一次 | 审查 v3 |
| `relay/relay_task.go` | 视频查询找不到适配器时不再把渠道编号写进错误信息 | 普通用户的错误响应不能带内部渠道编号 | 审查 v3 |
| `controller/option.go` | 通用设置接口拒绝 `adty_marketing_setting.*` | 不能绕过营销设置页，在缺发件地址和密钥时打开营销 | 审查 v3 |
| `controller/option.go` | 通用设置接口拒绝整个 `adty_referral_setting.*` | 不能绕过推广设置页，在没有默认比例时打开返佣 | 审查 v3 |
| `service/referral_withdraw.go`、`web/src/features/referral/{lib/address.ts,components/withdraw-drawer.tsx,components/referral-tables.tsx}`、`web/src/features/admin-referral/components/withdrawal-panel.tsx` | USDT白名单补齐`XLAYER`，复用EVM地址校验并统一显示X Layer；保留人工审核/冻结/TxID流程 | 第四网络可选择、确认、提交及核对；不代替FIN-J-02独立站长收益账户 | FIN-J-03 |
| `service/referral_test.go`、`web/src/features/referral/__tests__/{address.test.ts,withdraw-drawer.test.tsx}`、`scripts/adty/browser-business.sh`、`web/scripts/adty/browser-withdraw-networks.mjs` | 四网络三库生命周期回归、前端网络边界与生产浏览器withdraw-networks场景 | 保护幂等冻结、取消/驳回/批准/标付及桌面/移动网络和收款资料 | FIN-J-03 |
| `setting/config/config.go` | 分层配置的非法布尔、整数和浮点不再静默跳过 | 非法配置不能显示成保存成功，也不能只写进数据库 | 审查 v3 |
| `model/option.go` | `UpdateOption` 在写库前校验分层配置字段 | 解析失败不能先把非法值写入 options 表 | 审查 v3 |
| `model/main.go` | `chooseDB` 把 PostgreSQL 关键字连接串认成 PostgreSQL；增加 `PingDBImmediate` | CI 和本地关键字 DSN 不能被当成 MySQL；就绪探针不能吃 10 秒缓存 | 审查 v3 |
| `model/adty_migrate.go` | `adtyDSNIsPostgreSQL` 改为与 `IsPostgresDSN` 相同 | 启动检查不能把合法的 PostgreSQL 关键字连接串拒绝掉 | 审查 v3 |
| `controller/misc.go` | 增加 `GetReady`，只返回是否就绪；`GetStatus` 增加当前可用的 `payment_methods` | 健康检查不能把始终成功的 `/api/status` 当成数据库已连通；首页不能写死未开启的支付方式 | 审查 v3 |
| `router/api-router.go` | 增加 `GET /api/ready` 和 `POST /api/system-task/:task_id/cancel` | 容器探针和日志清理取消要有真实入口 | 审查 v3 |
| `model/system_task.go` | 增加取消状态和 `RequestSystemTaskCancel` | 待执行的清理立即停止，正在执行的清理在下一批前停止 | 审查 v3 |
| `controller/system_task.go` | `CancelSystemTask` 写审计并返回取消后的任务 | 清理任务必须能从后台停下来 | 审查 v3 |
| `service/system_task.go` | `runLogCleanupTask` 在每批删除前检查取消 | 已经开始的清理不能在取消后继续删用量日志 | 审查 v3 |
| `middleware/cors.go` | 不再同时允许任意 Origin 和凭据 | 跨站浏览器不能被声明为可携带会话 cookie | 审查 v3 |
| `main.go` | pprof 只监听 127.0.0.1，并给 HTTP 服务加安全响应头 | 分析端口和登录页不能对所有网卡或任意站点开放 | 审查 v3 |
| `docker-compose.yml` | 示例密码改为必填环境变量，端口只绑 127.0.0.1，建议数据库镜像为 PostgreSQL 17 | 根目录 compose 不能用公开示例密码和全网卡端口启动；已有数据目录升级主要版本须另建目录迁移，不把其他主版本排除在验收之外 | 审查 v3 / v4 |
| `controller/system_task.go` | 清理目标落在 90 天保留期内时返回 400 | 未来时间戳不能启动一次删掉近期付款日志的任务 | 审查 v3 |
| `model/log.go` | `CountOldLog` 和 `DeleteOldLogBatch` 不删除充值、退款和管理员调额日志；关掉消费日志时仍写 `quota_data` | 用量清理不能误删付款记录；关日志不能让真实扣费失去汇总证据 | 审查 v3 |
| `middleware/adty_user_limit.go` | 计数存储出错时返回 503，不再放行请求 | Redis 故障不能放开按用户的 RPM、TPM 和并发限制 | 审查 v3 |
| `i18n/keys_adty.go` | 增加 `adty.limit.unavailable` | 限流存储不可用时给调用方一条可翻译的受控错误 | 审查 v3 |
| `service/security_verification.go` | 增加 `VerificationScopeAPIKeyRead`，并把它列入无上下文的二次验证范围 | 密钥查看使用独立的二次验证范围 | 审查 v3 |
| `service/access_token_scope.go` | `api_key.read` 对应访问令牌范围 `api_key:reveal` | 没有该范围的访问令牌不能取得查看密钥的验证 | 审查 v3 |
| --- | --- | --- | --- |
| `common/str.go` | 删 `NormalizeBillingPreference` | 删订阅后没有调用方（原来只有 `NewBillingSession` 用它规范扣费偏好） | 1.2 |
| `controller/access_token_audit_test.go` | `TestAuditRoleSnapshotSurvivesActorChanges`、`TestSecurityAndOperationEventsUseAuditTable` 删 `recordSubscriptionResetUserLogs` 的调用，审计条数 4→3、10→9、10→9 | 这个函数随 `controller/subscription.go` 删了；每次调用写一条审计 | 1.2 |
| `controller/audit.go` | `auditContentTemplates` 删 `subscription.plan_reset`、`subscription.user_plan_reset`、`subscription.user_quota_reset` | 写这三种审计的订阅管理接口删了 | 1.2 |
| `controller/payment_webhook_availability_test.go` | `TestEpayCallbacksVerifyBeforeSettlement` 删订阅易支付 notify、return 两个用例 | 回调路由和处理函数删了 | 1.2 |
| `controller/relay.go` | `executeTaskSubmissionWith` 删写 `task.PrivateData.SubscriptionId` 的一行 | 字段删了 | 1.2 |
| `controller/topup_creem.go` | `handleCheckoutCompleted` 删先按订阅订单完成（`CompleteSubscriptionOrder`）的分支和它的注释；`LockOrder`、`UnlockOrder` 保留 | 订阅订单删了；锁同时保护后面的充值入账 | 1.2 |
| `controller/topup_stripe.go` | `fulfillOrder` 删订阅订单分支（`CompleteSubscriptionOrder` 和它用的 payload），`sessionExpired` 删订阅订单过期分支（`ExpireSubscriptionOrder`）和注释；两处 `LockOrder`、`UnlockOrder` 保留 | 订阅订单删了；锁同时保护后面的充值入账和过期处理 | 1.2 |
| `controller/topup_waffo.go` | `webhookPayloadWithSubInfo` 删没人读的 `SubscriptionInfo` 字段和 `webhookSubscriptionInfo` 类型；讲订阅的注释改了两处（这个类型、`WaffoWebhook`），删了一处；类型名不改 | 订阅删了；类型名沿用上游 | 1.2 |
| `controller/topup_waffo_pancake.go` | 删 `CreateWaffoPancakeSubscriptionProduct`、`ListWaffoPancakeSubscriptionProductOptions` 和它们的请求类型；`WaffoPancakeWebhook` 删按订单号前缀 `WAFFO_PANCAKE_SUB-` 转到订阅订单的分支 | 订阅产品路由和订阅订单删了 | 1.2 |
| `main.go` | 删 `service.StartSubscriptionQuotaResetTask()` 和它的注释 | 订阅额度重置任务（`service/subscription_reset_task.go`）删了 | 1.2 |
| `middleware/access_token_routes.go` | `accessTokenRouteRules` 删 `/api/subscription` 的 19 条和 waffo-pancake 订阅产品的 2 条；option 一段随 gofmt 重新对齐 | 删接口时同步删规则（约定 A）；规则没有对应的路由时，上游测试 `TestAccessTokenRouteRulesCoverEveryDashboardRoute` 失败 | 1.2 |
| `middleware/audit.go` | `auditRouteActions` 删订阅管理员接口的 3 条 | 路由删了 | 1.2 |
| `model/main.go` | `migrateDB` 删 `migrateSubscriptionPlanPriceAmount` 的调用、AutoMigrate 里的 3 个订阅模型和 `subscription_plans` 的建表（SQLite 用裸 SQL，别的库用 AutoMigrate）；删 `sqliteColumnDef`、`ensureSubscriptionPlanTableSQLite`、`migrateSubscriptionPlanPriceAmount` | 不再建订阅表；SQLite 的裸 SQL 不引用模型类型，只删 AutoMigrate 的话表照样会建出来 | 1.2 |
| `model/option.go` | `InitOptionMap`、`updateOptionMap` 删 `WaffoSubscriptionReturnUrl` | 配置项随订阅删除 | 1.2 |
| `model/payment_method_guard_test.go` | 删订阅的 3 个辅助函数和 `TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider`、`TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider` | 被测函数随 `model/subscription.go` 删了 | 1.2 |
| `model/task.go` | `TaskPrivateData` 删 `SubscriptionId`，`Value` 的空值判断去掉它；`BillingSource` 的注释改成只有 wallet | 任务不再记订阅 ID | 1.2 |
| `model/task_cas_test.go` | `TestMain` 的迁移列表删 3 个订阅模型，`truncateTables` 删 `subscription_orders`、`subscription_plans`、`user_subscriptions` 的清表语句 | 模型删了 | 1.2 |
| `relay/common/relay_info.go` | `RelayInfo` 删 `SubscriptionId`、`SubscriptionPreConsumed`、`SubscriptionPostDelta`、`SubscriptionPlanId`、`SubscriptionPlanTitle`、`SubscriptionAmountTotal`、`SubscriptionAmountUsedAfterPreConsume`；`BillingSource`、`RequestId` 保留，`BillingSource` 的注释改成只有 wallet；后面几个字段随 gofmt 重新对齐 | 只有订阅计费用这些字段 | 1.2 |
| `router/access_token_scope_test.go` | `accessTokenExemptRoutes` 删订阅易支付回调的 4 条 | 路由删了；免检名单里的路由没注册时，这个测试失败 | 1.2 |
| `router/api-router.go` | `SetApiRouter` 删 `/api/subscription` 的路由组（用户、管理员、易支付回调），option 组删 waffo-pancake 订阅产品的 2 条 | 删订阅 | 1.2 |
| `service/billing.go` | 删常量 `BillingSourceSubscription`；`SettleBilling` 删订阅余额提醒的分支，只发钱包提醒；改两处注释 | 中转扣费只走钱包（业务链路 4.1） | 1.2 |
| `service/billing_session.go` | `NewBillingSession` 删扣费偏好的读取、`trySubscription` 和按偏好的分支，只建钱包会话，开头加一段：用户 ID 无效（`<= 0`）时报 `invalid userId`；`Settle`、`Refund`、`needsRefundLocked`、`preConsume` 的错误映射、`reserveFunding`、`rollbackFundingReserve`、`shouldTrust`、`syncRelayInfo` 删订阅分支；删 `extraReserved` 字段；改注释 | 中转扣费只走钱包（业务链路 4.1）；`extraReserved` 只在订阅退款和订阅日志里读；无效用户 ID 原来由默认路径的订阅检查（`model.HasActiveUserSubscription`）拒绝，`relay/relay_task_test.go` 里三个「停在预扣」的测试靠它；删掉后它们走进钱包查额度，在没有 Redis 客户端的测试里空指针崩溃 | 1.2 |
| `service/funding_source.go` | 删 `SubscriptionFunding` 和只有它用的 `refundWithRetry`；改注释 | 资金来源只剩钱包 | 1.2 |
| `service/log_info_generate.go` | `appendBillingInfo` 删 `billing_preference` 和订阅的日志字段，`billing_source` 保留 | 没有订阅计费了，扣费偏好也不再读 | 1.2 |
| `service/midjourney.go` | `PrepareMidjourneyTaskBilling` 删拒绝订阅计费的检查 | 没有订阅计费来源了 | 1.2 |
| `service/quota.go` | `postConsumeQuotaWithResult` 删订阅分支，只扣钱包；删 `checkAndSendSubscriptionQuotaNotify` | 结算只动钱包（业务链路 4.1） | 1.2 |
| `service/task_billing.go` | 删 `taskIsSubscription`，`taskAdjustFunding` 只调钱包；改三处注释 | 任务计费只动钱包（业务链路 4.1） | 1.2 |
| `service/task_billing_test.go` | `TestMain` 的迁移和 `truncate` 删 `user_subscriptions`；删 `seedSubscription`、`getSubscriptionUsed` 和 `TestPrepareMidjourneyTaskBillingRejectsSubscriptionBeforeCharge`、`TestRefundTaskQuota_Subscription`、`TestRecalculate_Subscription_NegativeDelta`；`makeTask` 不再写 `SubscriptionId`，最后一个参数留着不用；`TestRefundTaskQuota_FundingFailureKeepsAccountingAndPendingMarker` 改成钱包已到上限、退款加不回去 | 订阅删了；`makeTask` 在两个测试文件里有 40 处调用，参数留着这些调用就不用改；原用例用不存在的订阅制造资金退还失败 | 1.2 |
| `service/waffo_pancake.go` | 删 `ResolveWaffoPancakeSubscriptionTradeNo`、`CreateWaffoPancakeProductForPlan` | 订阅订单和订阅产品删了 | 1.2 |
| `setting/payment_waffo.go` | 删 `WaffoSubscriptionReturnUrl`；其余变量随 gofmt 重新对齐 | 配置项随订阅删除 | 1.2 |
| `web/src/features/keys/components/__tests__/api-key-listing.test.tsx` | 不限额令牌详情的断言改成新文案 `... Requests still require available wallet quota.` | 跟着 `api-key-quota-cell.tsx` 换键 | 1.3 |
| `web/src/features/keys/components/api-key-quota-cell.tsx` | 不限额令牌的说明去掉「或订阅额度」，换成新键 | 订阅删了，请求只受钱包余额约束；旧键留在语言包里（约定 E） | 1.3 |
| `web/src/features/system-settings/general/quota-settings-section.tsx` | 预扣阈值的说明去掉「订阅」，换成新键 | 订阅删了，只有异步任务总是预留额度 | 1.3 |
| `web/src/features/system-settings/integrations/payment-settings-section.tsx` | 合规确认的提示和对话框说明（两句）换成只说支付的新键 | 两句同时提到兑换码、订阅、邀请奖励，三项都要删（任务 1.3、1.4、1.5），这次直接改成最终文案 | 1.3 |
| `web/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx` | 「为什么只绑一个店铺和产品」的说明：店铺一句去掉订阅套餐产品，换成新键；删掉订阅套餐不用绑定产品的一条和注释里讲订阅的一句 | 订阅套餐和它的 Pancake 产品删了 | 1.3 |
| `web/src/features/system-settings/maintenance/config.ts` | 侧边栏模块默认值删 `admin.subscription` | 订阅页删了 | 1.3 |
| `web/src/features/system-settings/maintenance/sidebar-modules-section.tsx` | 侧边栏模块开关删「订阅管理」一项 | 订阅页删了 | 1.3 |
| `web/src/features/usage-logs/components/__tests__/cost-display.test.tsx` | 删订阅和钱包计费来源的用例（订阅扣费、钱包图标、图标居中、悬停显示来源、订阅加附加费）；大金额用例去掉 `billing_source` 和订阅图标断言；没有来源的用例去掉订阅文字的断言；删用不到的 `userEvent`、`waitFor` 导入 | 费用列不再显示计费来源图标 | 1.3 |
| `web/src/features/usage-logs/components/columns/common-logs-columns.tsx` | `useCommonLogsColumns` 删 `showBillingSource` 参数，费用列不再传给 `LogCostDisplay` | 计费来源图标删了 | 1.3 |
| `web/src/features/usage-logs/components/dialogs/details-dialog.tsx` | `DetailsDialog` 删 `isSubscription` 和「订阅计费」一节 | 日志不再写订阅字段（任务 1.2） | 1.3 |
| `web/src/features/usage-logs/components/log-cost-display.tsx` | `LogCostDisplay` 删 `showBillingSource` 属性、订阅扣费（`subscription_consumed`）和计费来源图标，只显示 `quota`；删用不到的图标导入和 `useTranslation` 调用 | 只有钱包一种来源，图标不再有区分意义 | 1.3 |
| `web/src/features/usage-logs/components/usage-logs-table.tsx` | `UsageLogsTable` 删按订阅决定是否显示计费来源的查询（调 `/api/subscription/admin/plans`、`/api/subscription/self`）和 `useAuthStore` | 后端接口删了（任务 1.2），请求会 404 | 1.3 |
| `web/src/features/usage-logs/lib/columns.ts` | `useColumnsByCategory` 删 `showBillingSource` 参数 | 计费来源图标删了 | 1.3 |
| `web/src/features/usage-logs/lib/format.ts` | 审计文案模板删 `subscription.*` 四条 | 订阅管理接口和它们的审计删了（任务 1.2） | 1.3 |
| `web/src/features/usage-logs/types.ts` | `LogOtherData` 删 `subscription_*` 字段 | 日志不再写这些键（任务 1.2） | 1.3 |
| `web/src/features/users/components/data-table-row-actions.tsx` | `DataTableRowActions` 删「管理订阅」菜单项、对话框和它的状态、`CreditCard` 图标导入 | 订阅对话框随 `features/subscriptions/` 删了 | 1.3 |
| `web/src/features/wallet/index.tsx` | `Wallet` 删订阅套餐卡片、`showSubscriptionPanel` 和它的回调；充值区固定单栏 `grid gap-4` | 订阅套餐卡片删了 | 1.3 |
| `web/src/hooks/use-sidebar-config.ts` | 默认值删 `admin.subscription`，`URL_TO_CONFIG_MAP` 删 `/subscriptions` | 订阅页删了；两处一起删，没映射的地址默认显示 | 1.3 |
| `web/src/hooks/use-sidebar-data.ts` | 管理区删「订阅」菜单和只在这里用的 `CreditCard` 图标导入 | 订阅页删了 | 1.3 |
| `web/src/i18n/locales/en.json`、`web/src/i18n/locales/zh.json` | 加 5 个新键（上面换掉的文案），旧键保留 | 改了 `t()` 原文就加新键，先 en 再 zh（约定 E）；其他 5 种语言的译文后来补上了，见下面 zh-TW 等 5 个语言包的登记 | 1.3 |
| `web/src/lib/legacy-route.test.ts` | 映射表删 `/console/subscription`，在未知地址的用例里加一条断言：它跳 `/dashboard` | 保护删除后的跳转 | 1.3 |
| `web/src/lib/legacy-route.ts` | `legacyConsoleRoutes` 删 `/console/subscription` | `/subscriptions` 页删了 | 1.3 |
| `web/src/routeTree.gen.ts` | `bun run build` 重新生成，去掉 `/_authenticated/subscriptions/` | 路由删了（约定 B） | 1.3 |
| `common/constants.go` | 删 `TopUpLink` 变量和 `RedemptionCodeStatus*` 三个常量 | 兑换码和它的购买链接删了（执行层决定 20） | 1.4 |
| `controller/audit.go` | `auditContentTemplates` 删 `redemption.create`、`redemption.delete_batch` | 写这两种审计的兑换码管理接口删了 | 1.4 |
| `controller/topup.go` | `GetTopUpInfo` 的返回删 `enable_redemption`、`topup_link` | 兑换码和购买链接删了 | 1.4 |
| `controller/user.go` | `generateDefaultSidebarConfig` 的管理员、超级管理员默认值删 `redemption`（gofmt 重新对齐）；删 `topUpRequest`、`topUpLocks`、`topUpCreateLock`、`topUpTryLock`、`getTopUpLock` 和 `TopUp`；删用不到的 `sync`、`logger`、`operation_setting` 导入 | 兑换码页和 `POST /api/user/topup` 删了；那几个锁只给兑换用 | 1.4 |
| `middleware/access_token_routes.go` | `accessTokenRouteRules` 删 `POST /api/user/topup` 和 `/api/redemption` 的 8 条 | 删接口时同步删规则（约定 A）；规则没有对应的路由时，上游测试 `TestAccessTokenRouteRulesCoverEveryDashboardRoute` 失败 | 1.4 |
| `middleware/audit.go` | `auditRouteActions` 删兑换码管理接口的 4 条 | 路由删了 | 1.4 |
| `model/errors.go` | 删 `ErrRedeemFailed` | 只有 `model/redemption.go` 用它 | 1.4 |
| `model/locking_test.go` | `TestLockForUpdateEmitsRowLock` 生成 SQL 用的模型由 `[]Redemption` 换成 `[]TopUp` | `Redemption` 删了；测试讲的是 `lockForUpdate`，和模型无关 | 1.4 |
| `model/main.go` | `migrateDB` 的 AutoMigrate 删 `&Redemption{}` | 不再建 `redemptions` 表 | 1.4 |
| `model/option.go` | `InitOptionMap`、`updateOptionMap` 删 `TopUpLink` | 购买链接随兑换码删除（执行层决定 20） | 1.4 |
| `model/user.go` | `generateDefaultSidebarConfigForRole`（用户侧边栏默认值）的管理员、超级管理员 `admin` 区删 `redemption`（gofmt 重新对齐） | 兑换码页删了 | 1.4 |
| `router/api-router.go` | `SetApiRouter` 删 `POST /api/user/topup` 和 `/api/redemption` 路由组；`GET /api/user/topup`（后台充值记录）保留 | 删兑换码 | 1.4 |
| `service/access_token_scope.go` | 访问令牌资源删 `redemption` | 它管的接口删了 | 1.4 |
| `service/access_token_scope.go` | wallet、billing 两项资源的描述去掉订阅、邀请奖励 | 功能删了，说明不能再提 | 1.2、1.5 |
| `controller/payment_compliance.go` | 未确认合规时的报错换成只说支付的新原文 | 兑换码、订阅、邀请奖励都删了 | 1.2、1.4、1.5 |
| `web/src/i18n/static-keys.ts` | 访问令牌资源描述换成上面的新原文 | 和后端保持一致 | 1.2、1.5 |
| `web/src/i18n/static-keys.ts` | 删「Subscription constants」「Subscription plans」「Subscription management」「User subscription management」四段 | 订阅页面删了；`STATIC_I18N_KEYS` 没有读取方，语言包里的键不动 | 1.3 |
| `controller/option.go` | `UpdateOption` 拒绝 `QuotaForInviter`、`QuotaForInvitee`、`TopUpLink`、`WaffoSubscriptionReturnUrl` | 这些配置项随功能删了，不让它们再写进 options 表、下次同步又载入 | 1.2、1.4、1.5 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ko,ru,vi}.json` | 加访问令牌与独立动作权限的描述和译文，旧键保留，韩语同步补齐 | 改了原文就加新键（约定 E），8 种语言都要有 | 1.2–1.5 / 审查 v4 |
| `web/src/components/__tests__/command-menu.test.tsx` | `adminEntries` 删 `Redemption Codes` | 侧边栏的兑换码菜单删了 | 1.4 |
| `web/src/features/system-settings/billing/index.tsx` | `defaultBillingSettings` 删 `TopUpLink` | 购买链接随兑换码删除（执行层决定 20） | 1.4 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 额度设置的 `defaultValues` 删 `TopUpLink` | 同上 | 1.4 |
| `web/src/features/system-settings/general/__tests__/pre-consume-settings.test.tsx` | `defaultValues` 删 `TopUpLink` | 表单不再有这个字段 | 1.4 |
| `web/src/features/system-settings/general/quota-settings-section.tsx` | `quotaSchema` 和表单删 `TopUpLink`（「充值链接」输入项） | 同上 | 1.4 |
| `web/src/features/system-settings/maintenance/config.ts` | 侧边栏模块默认值删 `admin.redemption` | 兑换码页删了 | 1.4 |
| `web/src/features/system-settings/maintenance/sidebar-modules-section.tsx` | 侧边栏模块开关删「兑换码」一项 | 兑换码页删了 | 1.4 |
| `web/src/features/system-settings/types.ts` | `BillingSettings` 删 `TopUpLink` | 购买链接删了 | 1.4 |
| `web/src/features/usage-logs/audit/__tests__/details.test.tsx` | 删兑换码审计的 `it.each` 和「批量删除的数量和请求的兑换码 ID 分开显示」用例 | 被测的兑换码审计文案删了 | 1.4 |
| `web/src/features/usage-logs/audit/lib/audit-details.ts` | 字段名删 `requested_redemption_ids` | 只有兑换码批量删除的审计写它 | 1.4 |
| `web/src/features/usage-logs/lib/format.ts` | 审计文案模板删 `redemption.*` 四条；`renderAuditContent` 删批量删除兑换码的分支 | 兑换码管理接口和它们的审计删了（任务 1.4 后端） | 1.4 |
| `web/src/features/wallet/index.tsx` | `Wallet` 删兑换码的状态、`useRedemption`、`handleRedeem` 和传给充值卡片的 5 个属性 | 兑换码删了 | 1.4 |
| `web/src/features/wallet/types.ts` | 删 `RedemptionResponse`、`RedemptionRequest`，`TopupInfo` 删 `topup_link`、`enable_redemption` | 后端不再返回这两个字段（任务 1.4 后端） | 1.4 |
| `web/src/hooks/use-sidebar-config.ts` | 默认值删 `admin.redemption`，`URL_TO_CONFIG_MAP` 删 `/redemption-codes` | 兑换码页删了；两处一起删，没映射的地址默认显示 | 1.4 |
| `web/src/hooks/use-sidebar-data.ts` | 管理区删「兑换码」菜单和只在这里用的 `Ticket` 图标导入 | 兑换码页删了 | 1.4 |
| `web/src/i18n/locales/en.json`、`web/src/i18n/locales/zh.json` | 加 1 个新键（上面换掉的未开在线充值提示），旧键保留 | 改了 `t()` 原文就加新键，先 en 再 zh（约定 E）；其他 5 种语言的译文后来补上了，见下面 zh-TW 等 5 个语言包的登记 | 1.4 |
| `web/src/i18n/static-keys.ts` | 删「Redemption codes」一段（兑换码页的状态、提示和校验文案）和访问令牌权限里兑换码的 3 个键 | 页面和访问令牌资源删了。这一段里的 `Used`、`Copied to clipboard`、`An unexpected error occurred` 别处还在用，但都写成了 `t('...')` 字面量或常量；`Disabled`、`Expired` 在文件前面另有登记。`STATIC_I18N_KEYS` 没有读取方，语言包里的键不动 | 1.4 |
| `web/src/lib/legacy-route.test.ts` | 映射表删 `/console/redemption`，在未知地址的用例里加一条断言：它跳 `/dashboard` | 保护删除后的跳转 | 1.4 |
| `web/src/lib/legacy-route.ts` | `legacyConsoleRoutes` 删 `/console/redemption` | `/redemption-codes` 页删了 | 1.4 |
| `web/src/routeTree.gen.ts` | `bun run build` 重新生成，去掉 `/_authenticated/redemption-codes/` | 路由删了（约定 B） | 1.4 |
| `common/constants.go` | 删 `QuotaForInviter`、`QuotaForInvitee` | 邀请奖励删了 | 1.5 |
| `controller/oauth.go` | `oauthStateRequest` 删 `Aff`，`oauthFlowPayload` 删 `AffiliateCode`；`GenerateOAuthCode` 删 `aff` 的两条检查；`findOrCreateOAuthUser` 删 `affiliateCode` 形参和查邀请人，`InsertWithTx`、`FinalizeOAuthUserCreation` 传 0 | OAuth 注册不再带邀请码；其余参数检查不变 | 1.5 |
| `controller/option.go` | `UpdateOption` 删 `QuotaForInviter`、`QuotaForInvitee` 的合规检查（`switch` 只剩 `default`）；删只给它用的 `isPositiveOptionValue` | 配置项删了 | 1.5 |
| `controller/user.go` | `Register` 删按 `aff_code` 查邀请人和 `InviterId`，改成 `cleanUser.Insert(0)`；删 `TransferAffQuotaRequest`、`TransferAffQuota`、`GetAffCode`；`buildSelfUserData` 删 `aff_*`、`inviter_id` | 邀请奖励删了；`cleanUser` 和 `Insert` 调用保留（6.2、7.3 要在它前后改） | 1.5 |
| `middleware/access_token_routes.go` | `accessTokenRouteRules` 删 `GET /api/user/aff`、`POST /api/user/aff_transfer` | 删接口时同步删规则（约定 A） | 1.5 |
| `model/main.go` | `userQuotaColumns` 删 `aff_quota`、`aff_history` | 列删了 | 1.5 |
| `model/option.go` | `InitOptionMap`、`updateOptionMap` 删 `QuotaForInviter`、`QuotaForInvitee` | 配置项删了 | 1.5 |
| `model/user.go` | `User` 删 `AffCode`、`AffCount`、`AffQuota`、`AffHistoryQuota`、`InviterId`（gofmt 重新对齐）；`GetSelfUserById` 的 Select 删这五列；删 `GetUserIdByAffCode`、`inviteUser`、`TransferAffQuotaToQuota`；`Insert`、`InsertWithTx` 不再生成邀请码；`finishInsert`、`FinalizeOAuthUserCreation` 删邀请奖励；`Insert`、`FinishInsert`、`InsertWithTx`、`FinalizeOAuthUserCreation` 的邀请人形参改成 `_ int`，`finishInsert` 去掉形参；`UpdateWithTx` 的 Omit 删 `aff_*`；删用不到的 `operation_setting` 导入 | 新库不建这些列；Select 不删会让 `GET /api/user/self` 报 SQL 错误；保留形参少改调用方 | 1.5 |
| `router/api-router.go` | `SetApiRouter` 删 `GET /api/user/aff`、`POST /api/user/aff_transfer` | 删邀请奖励 | 1.5 |
| `controller/access_token_audit_test.go` | 4 个 `model.User` 夹具删 `AffCode`；`releasedAuditUser`（模拟旧版 users 表）和它的建行不动 | 字段删了；升级测试要保留旧表的 aff 列 | 1.5 |
| `controller/auth_flow_test.go` | 删 `TestGenerateOAuthCodeCarriesAffiliateInLoginFlow`；夹具删 `AffCode` | 被测的邀请码传递删了 | 1.5 |
| `controller/admin_user_verification_test.go`、`controller/billing_option_test.go`、`controller/relay_task_plugin_test.go`、`controller/security_account_test.go`、`controller/task_generic_test.go`、`controller/telegram_test.go`、`controller/token_test.go`、`controller/user_manage_test.go`、`middleware/auth_test.go`、`model/external_identity_claim_test.go`、`model/quota_reserve_test.go`、`model/user_pagination_test.go`、`model/user_session_test.go`、`service/user_notify_test.go` | 用户夹具删 `AffCode` | 字段删了。原来填它是为了避开 `aff_code` 唯一索引的空串冲突，列没了就不需要 | 1.5 |
| `model/user_update_test.go` | `TestUserUpdateDoesNotOverwriteConcurrentAccountingOrTokenChanges`、`TestRevokeUserAccessTokenOnlyClearsLegacyToken` 删读写 `aff_*` 列的部分和断言（额度、令牌部分保留）；夹具删 `AffCode` | 列删了 | 1.5 |
| `web/src/features/auth/api.ts` | `createOAuthAuthorization` 不再读邀请码、不再发 `aff`；删 `./lib/storage` 导入 | 后端 `POST /api/oauth/state` 不再收 `aff`（任务 1.5 后端） | 1.5 |
| `web/src/features/auth/index.ts` | 删 `getAffiliateCode`、`saveAffiliateCode` 的导出 | `lib/storage.ts` 删了 | 1.5 |
| `web/src/features/auth/sign-up/components/sign-up-form.tsx` | `SignUpForm` 删保存 `?aff=` 的 effect 和注册请求里的 `aff_code`；删 `lib/storage` 导入 | 注册不再带邀请码 | 1.5 |
| `web/src/features/auth/types.ts` | `RegisterPayload` 删 `aff_code` | 同上 | 1.5 |
| `web/src/features/profile/__tests__/settings.test.tsx`、`web/src/features/security/__tests__/page.test.tsx`、`web/src/features/security/components/__tests__/privacy-card.test.tsx` | 用户夹具删 `aff_count`、`aff_quota`、`aff_history_quota` | `UserProfile` 删了这些字段 | 1.5 |
| `web/src/features/profile/types.ts` | `UserProfile` 删 `aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`invite_user_id` | 后端不再返回 | 1.5 |
| `web/src/features/system-settings/billing/index.tsx` | `defaultBillingSettings` 删 `QuotaForInviter`、`QuotaForInvitee` | 配置项删了（任务 1.5 后端） | 1.5 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 额度设置的 `defaultValues` 删 `QuotaForInviter`、`QuotaForInvitee`，不再传 `complianceConfirmed` | 同上；这个属性只给邀请奖励的提示用 | 1.5 |
| `web/src/features/system-settings/general/__tests__/pre-consume-settings.test.tsx` | `defaultValues` 删 `QuotaForInviter`、`QuotaForInvitee` | 表单不再有这两个字段 | 1.5 |
| `web/src/features/system-settings/general/quota-settings-section.tsx` | `quotaSchema` 和表单删「邀请人奖励」「被邀请人奖励」；删 `complianceConfirmed` 属性、它控制的合规提示和用不到的 `Alert` 导入 | 邀请奖励删了 | 1.5 |
| `web/src/features/system-settings/types.ts` | `BillingSettings` 删 `QuotaForInviter`、`QuotaForInvitee` | 同上 | 1.5 |
| `web/src/features/usage-logs/components/dialogs/user-info-dialog.tsx` | `UserInfoDialog` 删邀请码、邀请人数、邀请额度；说明文字去掉「邀请详情」，换成新键 | 后端不再返回这些字段；旧键留在语言包里（约定 E） | 1.5 |
| `web/src/features/usage-logs/types.ts` | `UserInfo` 删 `aff_code`、`aff_count`、`aff_quota` | 同上 | 1.5 |
| `web/src/features/users/components/__tests__/quota-display.test.tsx` | 用户夹具删 `aff_count`、`aff_history_quota`、`inviter_id` 和 `renderUsersList` 的 `emptyInvitation` 参数；删邀请信息的断言（`Inviter ID: 42`、`Invited 2 users`、`No Inviter`）和时间列用例里检查邀请列空值的 `—`；第一个用例的名称去掉「邀请信息分两行」 | 「邀请信息」列删了；`—` 只由这一列显示 | 1.5 |
| `web/src/features/users/components/users-columns.tsx` | 删「邀请信息」列（`invite_info`）和变成未使用的 `formatQuota` 导入 | 后端不再返回邀请字段 | 1.5 |
| `web/src/features/users/types.ts` | `userSchema` 删 `aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`inviter_id` | 同上 | 1.5 |
| `web/src/features/wallet/index.tsx` | `Wallet` 删邀请奖励卡片、划转弹窗、`useAffiliate`、`handleTransfer` 和弹窗状态 | 邀请奖励删了 | 1.5 |
| `web/src/features/wallet/types.ts` | 删 `AffiliateCodeResponse`、`AffiliateTransferResponse`、`AffiliateTransferRequest`；`UserWalletData` 删 `aff_quota`、`aff_history_quota`、`aff_count` | 接口和字段删了 | 1.5 |
| `web/src/i18n/locales/en.json`、`web/src/i18n/locales/zh.json` | 加 1 个新键（上面换掉的用户信息弹窗说明），旧键保留 | 改了 `t()` 原文就加新键，先 en 再 zh（约定 E）；其他 5 种语言的译文后来补上了，见下面 zh-TW 等 5 个语言包的登记 | 1.5 |
| `web/src/routes/__root.tsx` | `RootComponent` 删保存 `?aff=` 的 effect 和 `lib/storage` 导入 | 不再记录邀请码 | 1.5 |
| `web/src/lib/frontend-cache.ts` | `PRESERVED_LOCAL_STORAGE_KEYS` 删 `'aff'` | 不再记录邀请码，清缓存时不用留这个键 | 1.5 |
| `web/src/stores/auth-store.ts` | `AuthUser` 删 `aff_code`、`aff_count`、`aff_quota`、`aff_history_quota`、`inviter_id` | 后端不再返回 | 1.5 |
| `main.go` | `InitResources` 在 `model.InitDB()` 之前调用 `model.CheckAdtyPostgreSQL()` | 按 `SQL_DSN`、`LOG_SQL_DSN` 判断，不是 PostgreSQL 就退出并写明是哪一个；要在连库和迁移之前，`LOG_SQL_DSN` 的库类型在 `InitLogDB` 之前还不知道 | 2.1 |
| `main.go` | `main` 在 `go model.SyncOptions(...)` 之后调用 `service.StartAdty()` | 设置载入之后由主节点写分组种子数据、清掉上游占位键 `edit_this`，再启动二开的后台任务 | 2.1 |
| `model/main.go` | `migrateDB` 末尾调用 `MigrateAdtyTables` | 只在主节点迁移里建二开的表 | 2.1 |
| `router/api-router.go` | `SetApiRouter` 末尾调用 `SetAdtyRouter` | 分组路由挂在 `/api` | 2.1 |
| `controller/audit.go` | `auditContentTemplates` 加 `adty.group.create`、`adty.group.update` | 审计正文不只剩动作名 | 2.1 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加 `Group not found`、`The English group name is required` | 分组管理接口的报错是英文原文，由前端翻译（约定 E） | 2.1 |
| `controller/option.go` | `UpdateOption` 在第二个 `switch` 之前调用 `service.ValidateAdtyGroupOption` | 分组类配置里的 ID 必须在分组表里，`auto` 只能出现在允许的位置，`GroupRatio` 不能漏掉还在的分组 | 2.2 |
| `web/src/features/system-settings/models/group-ratio-form.tsx` | 删 JSON 编辑模式（切换按钮、`editMode`、JSON 分支和只给它用的导入） | 分组 ID 只能在表格里改，保存时有校验 | 2.2 |
| `web/src/features/system-settings/models/__tests__/group-settings.test.tsx` | 「在可视和 JSON 之间切换保留修改」的用例换成「没有 JSON 编辑器」 | JSON 模式删了 | 2.2 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加分组校验的 3 条报错 | 后端返回英文原文，前端翻译 | 2.2 |
| `web/src/features/system-settings/models/group-ratio-visual-editor.tsx` | 分组定价表移到新文件 `group-pricing-table.tsx`（ID 只读、显示名在多语言对话框里改、不显示充值倍率、显示引用数、迁移和删除走接口）；各页签的分组下拉和名字改用 `useGroupDisplay().adminLabel`；删掉随之不用的导入 | 分组 ID 化（任务 2.3、2.4） | 2.3 |
| `web/src/features/system-settings/models/group-special-usable-editor.tsx` | 分组下拉和规则标题改用 `adminLabel` | 管理员看到「显示名 · ID」 | 2.3 |
| `web/src/components/auto-group-order-item.tsx` | 加可选的 `label` 属性，不传时仍显示分组 ID | 自动分组列表显示「显示名 · ID」；密钥页的调用不变 | 2.3 |
| `web/src/features/system-settings/models/__tests__/group-settings.test.tsx` | 夹具加 QueryClient 和分组接口的 mock；新增分组、搜索的用例改成新行为（接口返回的 ID、按显示名搜、ID 只读、引用数、删除按钮的可用条件）；空状态用例不返回分组 | 定价表改成按分组表工作 | 2.3、2.4 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加 `adty.group.migrate`、`adty.group.delete`（前端另加 create、update） | 审计正文能显示 | 2.4 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加分组表、名称对话框、迁移和删除的文案 | 新界面文案 | 2.3、2.4 |
| `web/src/routes/__root.tsx` | `RootComponent` 挂 `<GroupDisplaySync />` | 进入应用时加载分组显示名，换会话时重新加载 | 2.5 |
| `web/src/components/group-badge.tsx` | `getGroupLabel` 的分组名改用 `useGroupDisplay().name`，颜色仍按 ID | 前台一律显示显示名 | 2.5 |
| `web/src/features/users/components/users-columns.tsx`、`web/src/features/channels/components/{channels-columns,channel-card}.tsx`、`web/src/features/channels/components/dialogs/edit-tag-dialog.tsx`、`web/src/features/models/components/model-connections.tsx` | `GroupBadge` 换成 `AdminGroupBadge` | 后台显示「显示名 · ID」 | 2.5 |
| `web/src/features/keys/components/{api-keys-mutate-drawer,api-key-group-cell}.tsx`、`web/src/features/usage-logs/components/common-logs-filter-bar.tsx`、`web/src/features/pricing/components/{model-details,pricing-sidebar}.tsx`、`web/src/features/users/components/{users-mutate-drawer,users-table}.tsx`、`web/src/features/channels/components/{channels-table,drawers/channel-mutate-drawer,dialogs/tag-batch-edit-dialog}.tsx`、`web/src/features/profile/components/profile-header.tsx` | 分组下拉、筛选、提示里的 label 从 ID 换成显示名（后台用「显示名 · ID」）；值仍是 ID | 前台一律显示显示名 | 2.5 |
| `web/src/features/system-settings/models/__tests__/group-pricing-decimals.test.tsx` | 夹具加 QueryClient 和分组接口 mock；删掉充值倍率那一组用例；按只读 ID 找行 | 定价表不再有充值倍率列，名字不再是输入框 | 2.3 |
| `web/src/features/usage-logs/components/__tests__/group-filter.test.tsx`、`web/src/features/users/components/__tests__/group-filter.test.tsx` | 各加一个用例：选项显示显示名（后台是「显示名 · ID」），按 ID 搜索；每个用例后清空显示名 store | 钉住显示名 | 2.5 |
| `main.go` | `InitResources` 在 `authz.Init` 之前调用 `middleware.EnableAdtyAdminPermissions()` | 登记二开的权限项、清空内置管理员的默认权限、打开后台路由检查；`TestMainEnablesAdtyAdminPermissions` 钉住顺序 | 2.6 |
| `middleware/auth.go` | `authHelper` 在写入身份之后、`beginAdminAudit` 之前加 `adtyAdminRouteAllowed`；`SetupContextForToken` 里 `sk-xxx-渠道ID` 指定渠道还要渠道·查看 | 普通管理员默认没有后台权限，按上游的访问令牌分类对应权限；被拒的请求不写兜底审计 | 2.6 |
| `controller/user.go` | `ManageUser` 解码后、`add_quota` 提前返回之前调 `adtyCheckManageAction`；`UpdateUser` 在 `canManageTargetRole` 之后调 `adtyCheckUserUpdate`，事务前记权限、`ReloadPolicy` 之后写差异审计 | 动作和分组按权限细分；权限变化留痕，事务回滚时不写 | 2.6 |
| `controller/task.go` | `taskArtifactLookup` 读别人的任务产物还要日志·查看 | 这里只按角色放行，路由检查覆盖不到 | 2.6 |
| `service/user_notify.go` | `notifyAdmins` 的 `receives` 多传用户 ID；渠道通知和上游模型更新通知只发给有渠道·查看的管理员 | 同上 | 2.6 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加 `adty.admin_permissions.update` | 权限变化审计能显示 | 2.6 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加二开权限项的名称和说明 | 权限矩阵显示 | 2.6 |
| `web/src/components/layout/types.ts` | 菜单项加 `requiredPermission` | 后台菜单按权限显示 | 2.7 |
| `web/src/hooks/use-sidebar-view.ts` | 根导航按 `requiredPermission` 过滤，过滤后没有菜单项的分组整个去掉；hook 取整个 user | 同上 | 2.7 |
| `web/src/hooks/use-sidebar-data.ts` | 渠道、模型、用户三个后台菜单标上渠道·查看、模型·查看、用户·查看 | 同上 | 2.7 |
| `web/src/routes/_authenticated/{users/index,channels/index,models/index,models/$section}.tsx` | `beforeLoad` 从「至少管理员」改成要对应的查看权限；模型页的 deployments 分区只给超级管理员 | 和后端一致；部署接口只给超级管理员 | 2.7 |
| `web/src/features/usage-logs/components/usage-logs-provider.tsx` | `resolveLogsViewAccess` 加 `canViewAll` 参数（默认只有超级管理员为真），`useLogsViewScope` 传 `canViewAllLogs(user)` | 没有日志·查看的管理员只看本人数据，也不请求全站接口 | 2.7 |
| `web/src/features/dashboard/index.tsx`、`web/src/features/dashboard/components/flow/flow-charts.tsx`、`web/src/features/dashboard/components/models/{log-stat-cards,models-filter-dialog}.tsx` | 「能不能看全站数据」改成 `canViewAllLogs(user)` | 同上 | 2.7 |
| `web/src/features/dashboard/components/overview/overview-dashboard.tsx` | 「渠道」快捷入口改成要渠道·查看 | 同上 | 2.7 |
| `web/src/features/users/components/data-table-row-actions.tsx` | 启用、禁用、绑定、重置 2FA 和 Passkey 要用户·编辑，删除要用户·删除，提升、降级只给超级管理员 | 按钮按权限显示 | 2.7 |
| `web/src/features/users/components/users-mutate-drawer.tsx` | 分组字段要用户·改分组，调余额要用户·调余额；权限矩阵上方放预设按钮，说明文案换成新键 | 同上 | 2.7 |
| `web/src/components/__tests__/command-menu.test.tsx`、`web/src/features/usage-logs/__tests__/access.test.ts`、`web/src/features/usage-logs/components/__tests__/{export-dialog,group-filter}.test.tsx`、`web/src/features/users/components/__tests__/step-up-verification.test.tsx` | 管理员夹具补上对应权限（或超级管理员操作者）；command-menu 加「没有权限的管理员看不到后台页」，access 加「没有日志权限时只看本人」 | 后台页面和全站数据按权限放行 | 2.7 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加预设按钮和权限说明的文案、`Support` | 新界面文案 | 2.7 |
| `controller/option.go` | `UpdateOption` 第一个 `switch` 加 `adty_topup_setting.bonus_rates` 的校验 | 值写错时 config 会静默保留旧值，而 OptionMap 已是新字符串 | 3.1 |
| `model/topup.go` | `ManualCompleteTopUp` 锁行读出订单之后，支付商是 `bepusdt`、`adty_epay` 就拒绝 | 我们的订单只能走充值对账入账（面额加赠、返佣、流水） | 3.2 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加补单、退款、orphan 补入账和处理 | 审计正文能显示 | 3.5 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加充值报价、订单、对账、退款的报错和日志文案 | 后端返回英文原文，前端翻译 | 3.1–3.5 |
| `web/src/features/wallet/index.tsx` | 整页换成余额统计卡 + 待付款提示 + 新充值卡 + 充值记录（`features/wallet/adty/`）；`initialShowHistory` 改成滚动到充值记录 | 新的充值底座（任务 3.4） | 3.4 |
| `web/src/hooks/use-sidebar-config.ts`、`web/src/features/system-settings/maintenance/{config.ts,sidebar-modules-section.tsx}` | 后台加 `recharge_audit`、`payment_channels`、`admin_referral` 三个模块和对应地址 | 新菜单能在侧边栏设置里开关（任务 3.5、5.4、6.7） | 3.5 |
| `web/src/hooks/use-sidebar-data.ts` | 后台加「充值对账」，要充值财务·查看 | 同上 | 3.5 |
| `web/src/routeTree.gen.ts` | `bun run build` 重新生成，加 `/_authenticated/recharge-audit/` | 新路由（约定 B） | 3.5 |
| `scripts/adty/baseline-lint.txt` | 删 `creem-products-section.tsx` 一行 | 文件随 3.4 删除 | 3.4 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加钱包新充值卡、充值记录、充值对账的文案 | 新界面文案 | 3.4、3.5 |
| `i18n/i18n.go` | `Init` 的文件列表加上 `adtyLocaleFiles`（`i18n/adty_locales.go`） | 二开的后端提示放在 `adty.<语言>.yaml`，上游 keys.go 和三个 yaml 不动（执行层决定 13） | 5.3 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加支付通道的新建、修改、删除、启停、测试、自动维护 | 审计正文能显示 | 5.3、5.4 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加支付通道后台的报错 | 后端返回英文原文 | 5.4 |
| `web/src/features/system-settings/integrations/payment-settings-section.tsx` | 「充值折扣」表单字段换成「充值加赠」（`AdtyBonusRatesEditor`），保存到 `adty_topup_setting.bonus_rates`；删掉随之不用的模式切换状态 | 我们用加赠不用折扣（任务 4.3） | 4.3 |
| `web/src/features/system-settings/billing/{section-registry.tsx,index.tsx}`、`web/src/features/system-settings/types.ts` | 加 USDT（BEpusdt）分区和 `adty_bepusdt.*`、`adty_topup_setting.*` 的默认值和类型；支付分区传 `BonusRates` | 否则 `getOptionValue` 读不到这些键 | 4.3 |
| `web/src/hooks/use-sidebar-data.ts` | 后台加「支付通道」，要支付通道·查看 | 任务 5.4 | 5.4 |
| `web/src/routeTree.gen.ts` | 重新生成，加 `/_authenticated/payment-channels/` | 新路由 | 5.4 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加 USDT、易支付支付窗、USDT 设置、加赠编辑器、支付通道后台的文案 | 新界面文案 | 4.3–5.5 |
| `controller/user.go` | `Register` 查出 `insertedUser` 后、生成默认令牌前调 `service.AdtyAfterRegister`；`writeLoginResponse` 在 `recordLoginAudit` 后调 `service.AdtyRecordLoginFingerprint` | 注册即推广员、按 cookie 绑定、记指纹（任务 6.2、6.5）；登录各路径都汇到这个出口 | 6.2、6.5 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加推广设置、合作伙伴审核、推广员调整、提现审核和打款 | 审计正文能显示 | 6.7 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加推广返佣和提现的报错文案 | 后端返回英文原文 | 6.1–6.4 |
| `web/src/routes/__root.tsx` | `RootComponent` 里调 `useReferralCapture()`：写设备 cookie `adty_did`，任何页面带 `?ref=` 时记点击并去掉这个参数 | 推广落地和自邀检查（任务 6.2、6.5） | 6.2、6.5 |
| `web/src/hooks/use-sidebar-data.ts` | 个人组加「推广返佣」；后台加「推广管理」，要推广返佣·查看，旁边显示待处理数 | 任务 6.6、6.7 | 6.6、6.7 |
| `web/src/hooks/use-sidebar-config.ts` | personal 加 `referral` 模块、`/referral` 的映射 | 侧边栏开关能管到新菜单 | 6.6 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | 个人组的断言加上「推广返佣」 | 个人组从三项变成四项 | 6.6 |
| `web/src/routeTree.gen.ts` | 重新生成，加 `/r/$code`、`/_authenticated/referral/`、`/_authenticated/admin-referral/` | 新路由 | 6.2、6.6、6.7 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 加「支付宝和微信支付」分区（订单有效期、换通道时限、补查、维护阈值、探活） | 约定 I：参数都能在后台改 | 3.6 |
| `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` | 加推广返佣页、提现抽屉、推广管理的文案 | 新界面文案 | 6.6、6.7 |
| `model/log.go` | `formatUserLogs` 和 `StreamUserLogs` 改调 `AdtyProjectUserLog`（`model/adty_log_projection.go`） | 客户日志清掉渠道、上游请求 ID，other 只留用量类白名单（任务 7.1） | 7.1 |
| `relay/relay_task.go` | `TaskModel2Dto` 不再填 ChannelId | 视频任务查询接口直接返回它给 API 令牌（任务 7.1） | 7.1 |
| `controller/task.go` | `tasksToDto` 管理员补 ChannelId，非管理员清掉 `Properties.UpstreamModelName` | 任务 7.1 | 7.1 |
| `controller/midjourney.go` | `GetUserMidjourney` 返回前清零 ChannelId | 任务 7.1 | 7.1 |
| `controller/log.go` | `GetLogsSelfStat` 的 channel 参数只给管理员 | 任务 7.1 | 7.1 |
| `controller/relay.go` | `presentTaskSubmission` 不再设 `X-New-Api-Other-Ratios` | 把计价倍率发给调用方（任务 7.1） | 7.1 |
| `model/log_format_test.go`、`controller/log_export_test.go`、`controller/relay_error_log_test.go`、`controller/relay_task_plugin_test.go` | 断言改成客户看不到渠道、价格和倍率头 | 任务 7.1 改了这些行为 | 7.1 |
| `web/src/features/usage-logs/{lib/utils.ts,components/dialogs/details-dialog.tsx,components/columns/common-logs-columns.tsx}` | `isPerCallBilling` 改成看 `billing_mode === 'per_call'` 或 model_price；没有价格时只显示「按次」 | 客户日志没有 model_price（任务 7.1） | 7.1 |
| `controller/user.go` | `ManageRequest` 加 `Reason`、`IdempotencyKey` | 人工调额留痕（任务 7.2） | 7.2 |
| `controller/user_quota.go` | 改调 `model.AdjustUserQuotaWithLedger`（原因必填、幂等、写余额流水）；原因只进审计，写进用户日志的 params 去掉原因；重放时不再写审计和用户日志 | 任务 7.2 | 7.2 |
| `controller/user_manage_test.go`、`router/adty_admin_permission_test.go` | 调额请求带上 reason；测试库建 `adty_balance_ledgers`；审计 params 的预期加上 reason | 原因必填后上游测试要带原因 | 7.2 |
| `web/src/features/users/{types.ts,components/user-quota-dialog.tsx,components/users-mutate-drawer.tsx}` | 调额对话框加必填原因和幂等键；用户抽屉加「余额流水」 | 任务 7.2 | 7.2 |
| `controller/misc.go` | `SendEmailVerification` 遇到已注册邮箱时调 `service.AdtySendRegisteredReminder` 并照常回成功；验证码和找回密码改用 `service.AdtySendEmailAsync` 异步发信 | 防邮箱枚举（任务 7.3） | 7.3 |
| `controller/user.go` | `Register` 里邮箱已被占用的两处报错改成「验证码错误或已过期」 | 任务 7.3 | 7.3 |
| `router/relay-router.go` | /v1、/v1beta 在 `ModelRequestRateLimit` 之后加 `middleware.AdtyUserLimit()`；MJ 的提交类路由加 `AdtyUserLimit(true)`，查询类不加 | 按用户限制 RPM、TPM、并发（任务 7.4） | 7.4 |
| `router/task-plugin-protocol-router.go`、`router/video-router.go`、`router/plugin-router.go`、`router/task-router.go`、`controller/responses_websocket.go` | 同样在上游限流之后加 `AdtyUserLimit()`（Codex 的 /v1/responses、图片、视频、插件、任务提交、Responses WebSocket 的每个 response.create） | 任务 7.4 | 7.4 |
| `model/log.go` | `RecordConsumeLog` 在 `LogConsumeEnabled` 判断之前调 `AdtyOnConsumeTokens` | 关掉消费日志时 TPM 也要累计；渠道测试（没有 token_id）不算 | 7.4 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加 `adty.user.limits` | 审计正文能显示 | 7.4 |
| `web/src/features/system-settings/{security/section-registry.tsx,security/index.tsx,types.ts}` | 安全设置加「按用户限流」分区和 `adty_limit_setting.*` | 任务 7.4 | 7.4 |
| `web/src/features/users/components/users-mutate-drawer.tsx` | 用户抽屉加单个用户的 RPM、TPM、并发 | 任务 7.4 | 7.4 |
| `controller/relay.go` | 保活的开启条件从「Claude 格式的流式请求」放宽到「除 Realtime 以外的所有流式请求」 | 推理模型首字慢时 Cloudflare 会 524（任务 7.5） | 7.5 |
| `controller/audit.go`、`web/src/features/usage-logs/lib/format.ts` | 审计模板加 `adty.official_price.update`、`adty.official_price.sync` | 审计正文能显示 | 8.4 |
| `i18n/i18n.go` | 加 `LangFr`、`LangJa`、`LangKo`、`LangRu`、`LangVi`；`Init` 加载 fr、ja、ko、ru、vi 五个 yaml，预建 localizer 改成遍历 `SupportedLanguages()`；`normalizeLang` 认这些语言及带地区的写法；`SupportedLanguages` 包含这 5 种 | 后端补齐 8 种语言 | 9.1 |
| `i18n/i18n_test.go` | `TestLanguageFromUserSetting` 的 fr、ja 改为期望 fr、ja，另加 de、ko 回退 en；`TestBackendMessageLanguage` 的无译文语言改用 de-DE | fr、ja 现在有译文 | 9.1 |
| `i18n/locales/{fr,ja,ru,vi}.yaml` | 新增上游键的 4 种译文；以后合并上游时新键要补进来 | 任务 9.1 | 9.1 |
| `web/src/styles/theme.css` | 浅色、暗色变量换成黑白取值；新增 `--canvas`、`--*-soft`、`--font-mono` 并映射到 `@theme`；`--radius` 0.5rem，`--radius-xl` 改为 ×1.5；`--font-ui-sans` 改为 Inter 加中文兜底 | 白色主题（设计稿 §6） | 8.1 |
| `web/src/styles/index.css` | 加 `@fontsource-variable/inter`、`jetbrains-mono` 两行 import | 字体自托管 | 8.1 |
| `web/src/components/layout/components/authenticated-layout.tsx` | SidebarInset 加 `bg-canvas` | 控制台灰画布 | 8.1 |
| `web/src/components/ui/table.tsx` | 行高 h-15 改 h-11；表头文字改 `text-muted-foreground` | 行高 44px | 8.1 |
| `web/src/lib/theme-customization.ts` | default 预设色块改成 #0A0A0A、#FFFFFF | 外观预览和白色主题一致 | 8.1 |
| `web/package.json`、`web/bun.lock` | 加 `@fontsource-variable/inter`、`jetbrains-mono` 5.3.0（OFL-1.1） | 字体自托管 | 8.1 |
| `web/src/components/layout/components/app-header.tsx` | LanguageSwitcher 前加 `<BalanceChip />` | 顶栏余额和充值按钮 | 8.2 |
| `web/src/hooks/use-top-nav-links.ts` | 末尾加 affiliate 导航（/affiliate，`affiliate:false` 时隐藏） | 顶栏推广计划链接 | 8.2 |
| `web/src/hooks/use-sidebar-data.ts` | 分组标题改为 Console、Chat、Account、Administration，chat 组移到 general 后；推广管理移到用户之后；分组 id 不变 | 侧边栏按控制台草案排序 | 8.2 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | 加分组顺序和标题、管理组顺序两条测试 | 钉住 8.2 的顺序 | 8.2 |
| `web/src/components/layout/components/footer.tsx` | 加 `AdtyFooterColumns`，两个分支都渲染；`ProjectAttribution` 未改 | 页脚链接列和源码链接 | 8.2 |
| `web/src/components/layout/components/public-layout.tsx` | 末尾渲染 `<Footer />` | 所有公开页都有署名和源码链接 | 8.2 |
| `web/src/features/home/index.tsx` | 删默认分支的 `<Footer />` | PublicLayout 已带页脚 | 8.2 |
| `web/src/features/auth/auth-layout.tsx` | Logo 和骨架 rounded-full 改 rounded-lg | 方形 Logo 不被裁成圆 | 8.2 |
| `web/src/features/pricing/hooks/use-filters.ts` | 加参数 availableGroups；没选分组时默认选本人分组（未登录选 default），不可选时回退「全部」；「全部」显式存入状态 | 价格页默认分组 | 8.5 |
| `web/src/features/pricing/components/{pricing-columns.tsx,pricing-table.tsx,model-card-grid.tsx,model-card.tsx}` | 加官方价、节省两列和卡片上的「省 X%」；分组和可用分组取交集（卡片不传参数时不过滤） | 官方价、节省、不露隐藏分组 ID | 8.5 |
| `web/src/features/pricing/index.tsx` | 取官方价、页头显示更新时间、删掉写死的蓝紫渐变 | 任务 8.5 | 8.5 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 加「官方价」分区 | 后台官方价表 | 8.5 |
| `web/rsbuild.config.ts` | `tools.rspack.module.rules` 加一条 `.md?raw` → asset/source | 接入文档用 `?raw` 导入 Markdown，不加构建失败 | 8.6 |
| `web/src/features/legal/{api.ts,legal-document.tsx,user-agreement.tsx,privacy-policy.tsx,index.ts}` | 改调 `/api/adty/legal/:doc?lang=`，queryKey 带语言；导出 RefundPolicy | 条款按语言取，回退 本语言 → en → 上游 legal.* | 8.6 |
| `web/src/features/auth/components/{legal-consent.tsx,terms-footer.tsx}` | `refund_policy_enabled` 为真时加退款政策链接 | 注册、登录页显示退款政策 | 8.6 |
| `web/src/features/system-settings/{content/section-registry.tsx,content/index.tsx,types.ts}` | 加多语言条款分区和 `adty_legal_setting.*` 默认值、类型 | 后台编辑多语言条款 | 8.6 |
| `web/src/features/adty-groups/components/group-text-dialog.tsx` | 语言标签名改用 `adty-lang.ts` 的 `CONTENT_LANG_LABELS` | 和条款编辑器共用 | 8.6 |
| `common/constants.go` | `SystemName` 默认值改为 `AdtyBrandName` | 对外站名 | 8.9 |
| `main.go` | 去掉 `middleware.Version()`；崩溃响应改为固定文案、类型 `adtyai_panic`；统计脚本注释去掉 QuantumNous | 不发版本号头，不泄露 panic 详情 | 8.9 |
| `middleware/recover.go` | 崩溃响应同上 | 不泄露 panic 详情 | 8.9 |
| `middleware/utils.go`、`relaykit/types/error.go`、`controller/{billing.go,plugin_protocol.go,plugin_protocol_image.go,relay.go}`、`service/error.go` | 报错类型 `new_api_error` → `adtyai_error`（常量名不变） | 调用方能看到的报错类型 | 8.9 |
| `service/auth_session.go`、`web/src/lib/session-hint.ts` | cookie 名改为 `adtyai_refresh`、`adtyai_has_session` | 浏览器能看到；上线后已登录用户要重新登录一次 | 8.9 |
| `service/auth_token.go` | JWT 签发方 `adtyai`、受众 `adtyai-dashboard` | 浏览器能看到 | 8.9 |
| `service/codex_models.go` | User-Agent 改为 `AdtyBrandSlug` | 上游服务商能看到 | 8.9 |
| `relay/channel/openai/adaptor.go` | OpenRouter 的 HTTP-Referer、X-OpenRouter-Title 改为 `AdtyBrandURL`、`AdtyBrandName` | OpenRouter 公开排行 | 8.9 |
| `setting/operation_setting/general_setting.go` | DocsLink 默认值改为空 | 顶栏「文档」指向 /docs | 8.9 |
| `middleware/{auth_test.go,distributor_test.go,model_rate_limit_test.go}` | JWT 签发方和受众、报错类型的期望值 | 跟着改名 | 8.9 |
| `web/index.html` | 标题、描述换成 ADTY AI；主题键改为 `adtyai:`；加 icon 和 apple-touch-icon | 访客和搜索引擎能看到 | 8.9 |
| `web/public/favicon.ico`、`web/public/logo.png` | 换成从 `adty-logo.svg` 渲染的图 | 站点图标 | 8.9 |
| `web/src/lib/{theme-storage.ts,frontend-cache.ts,auth-session-sync.ts,auth-session.ts}` | 键改为 `adtyai:` 前缀；frontend-cache 保留 `adty.pending-payment` | 浏览器存储键 | 8.9 |
| `web/src/lib/constants.ts`、`components/layout/components/system-brand.tsx`、`features/system-settings/site/index.tsx` | 默认站名用 `ADTY_BRAND_NAME` | 对外站名 | 8.9 |
| `web/src/components/layout/components/footer.tsx` | 默认站名用 `ADTY_BRAND_NAME`；口号换新键；删掉演示站模式下指向上游文档的默认栏目（ProjectAttribution 不动） | 对外站名，不链到上游文档 | 8.9 |
| `web/src/features/pricing/components/model-details-api.tsx` | 示例变量改为 `ADTYAI_API_KEY` | 价格页能看到 | 8.9 |
| `web/src/features/errors/general-error.tsx` | 反馈按钮改成 `/api/adty/site` 的 support_email，没配置就隐藏 | 不指向上游 issues | 8.9 |
| `web/src/features/chat/lib/chat-links.ts` | 导入客户端时的 id、platform 改为 adtyai，名称改为 ADTY AI | 客户端里能看到 | 8.9 |
| `web/src/context/__tests__/theme-preferences.test.tsx`、`web/src/features/errors/__tests__/general-error.test.tsx` | 存储键；mock `/api/adty/site`，加反馈按钮用例 | 跟着改名 | 8.9 |
| `model/user.go` | Username 的 validate 标签加 `excludes=@` | 用户名不能含 @，登录框按用户名或邮箱查时不会撞上别人（执行层决定 16） | 8.7 |
| `controller/oauth.go` | 第三方登录建号时，提供方给的用户名含 @ 就改用自动生成的名字 | 这条路径不走 Validate | 8.7 |
| `web/src/features/auth/auth-layout.tsx` | 改成左右两栏：左边品牌区（lg 以下隐藏），右边表单 | 登录注册页布局 | 8.7 |
| `web/src/features/auth/constants.ts`、`web/src/features/users/lib/user-form.ts` | 用户名规则（必填、最长 20、不能含 @），注册表单和后台用户表单共用 | 前端提前提示，规则和后端一致 | 8.7 |
| `web/src/i18n/static-keys.ts` | 登记两条用户名校验提示 | schema 里的文案不会被 t() 扫描到 | 8.7 |
| `web/src/features/dashboard/components/overview/{summary-cards.tsx,overview-dashboard.tsx}`、`web/src/features/dashboard/hooks/use-dashboard-config.tsx` | 卡片改为本月消费、今日请求、今日 tokens；Runway 按 7 天日均算；常驻快速接入卡 | 控制台概览 | 8.7 |
| `web/src/features/keys/components/dialogs/cc-switch-dialog.tsx` | 新增可选 prop endpoint，不传时行为不变 | 快速接入卡用站点的 Base URL 导入 | 8.7 |
| `web/src/features/dashboard/components/overview/__tests__/setup-guide.test.tsx` | 补 `/api/adty/site` 的 mock；标题列表加 Quick start | 概览多了一张常驻卡 | 8.7 |
| `web/src/routes/__root.tsx` | RootComponent 里 Toaster 旁挂 `<TawkWidget />` | 网页客服 tawk.to，配置了两个 id 才加载（任务 8.8） | 8.8 |
| `model/user.go` | `User` 加 `SiteId`（`site_id` 列，带索引，默认 0）；`UpdateWithTx` 的 Omit 加 `site_id` | 分站客户归属（执行层决定 29），只能由分站服务改，用户改资料和管理员编辑都改不动 | 11.1 |
| `web/src/features/home/index.tsx` | 默认分支换成 `<AdtyLanding />`，删掉旧区块的 import；加载自定义首页内容时直接显示落地页，不再显示「Loading...」 | 新首页 | 8.3 |
| `web/src/i18n/config.ts` | 末尾调 `bindDocumentLang(i18n)` | `<html lang>` 跟随界面语言 | 9.2 |
| `web/src/styles/theme.css` | `.dark` 前加 `:root:lang(ja):not([data-theme-font='sans'])` 字体栈 | 日文页面汉字用日文字形 | 9.2 |
| `web/src/i18n/locales/{zh,zh-TW,fr,ru,vi}.json` | 改 7 个已有键的值（Model Square→模型价格/模型價格；fr、ru、vi 补 4 个仍是英文的值） | 设计稿叫法；漏翻检查 | 9.2 |
| `router/web-router.go` | `ServeFrontendFiles` 前插 `AdtySEOHomePages(assets.IndexPage, assets.BuildFS)` | 首页 8 个语言网址写 SEO 头和预渲染正文 | 9.3 |
| `web/index.html` | 标题、描述移进 `<!--adty-head-->…<!--/adty-head-->`，里面默认带 noindex | Go 替换首页头部；其余页面不进搜索 | 9.3 |
| `web/src/main.tsx` | 系统品牌初始化的 `apply` 开头：`<html>` 带 `data-adty-seo` 时不改标题 | 保留服务端写的首页标题 | 9.3 |
| `web/src/routes/index.tsx` | 加 beforeLoad（调 `prepareRootHome`） | `/` 按记住的语言或登录用户语言跳转 | 9.3 |
| `web/src/components/language-switcher.tsx` | `changeLanguage` 之后调 `adtyAfterLanguageChange(code)` | 在首页切换语言时换网址 | 9.3 |
| `web/src/routes/__root.tsx` | RootComponent 里调 `useAdtyPrerenderCleanup()` | 不在首页时删掉预渲染块 | 9.3 |
| `web/src/lib/frontend-cache.ts` | `PRESERVED_LOCAL_STORAGE_KEYS` 加 `adtyai:lang` | 缓存版本变了也保留首页语言 | 9.3 |
| `web/package.json` | build 改成 `rsbuild build && bun scripts/adty/prerender-home.tsx` | 构建时预渲染首页 | 9.3 |
| `web/src/test-setup.ts` | 浏览器 shim 包进 `if (typeof window !== 'undefined')` | 预渲染测试在 node 环境跑，没有 window | 9.3 |
| `web/src/features/system-settings/site/{index.tsx,section-registry.tsx}`、`types.ts` | 加 `adty_seo_setting.*` 默认值、类型和「Home Page SEO」分区 | 首页 SEO 后台设置 | 9.3 |
| `web/src/hooks/use-sidebar-data.ts` | admin 组 Marketing 之后加「Reseller sites」（/site-admin/sites，SITE_READ） | 分站管理入口 | 11.9 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | admin 组顺序的断言加上 Reseller sites | 新菜单项 | 11.9 |
| `web/src/features/auth/sign-up/components/sign-up-form.tsx` | 加营销同意框 `<MarketingConsentCheckbox>`，注册请求带 `marketing_consent`、`lang` | 注册时记录营销同意 | 12.1 |
| `web/src/features/auth/api.ts`、`web/src/features/auth/types.ts` | `register` 把 `marketing_consent`、`lang` 放进 query，不放进请求体；`RegisterPayload` 加这两个字段 | 上游把请求体直接解进 model.User | 12.1 |
| `web/src/features/profile/index.tsx` | 语言卡片下挂 `<MarketingPreferencesCard />` | 个人设置里随时改营销同意 | 12.1 |
| `web/src/hooks/use-sidebar-config.ts`、`web/src/features/system-settings/maintenance/{config.ts,sidebar-modules-section.tsx}` | 后台加 `marketing` 模块和 `/marketing` 地址 | 营销菜单能在侧边栏设置里开关 | 12.1 |
| `web/src/hooks/use-sidebar-data.ts` | 后台菜单加「Marketing」（`Megaphone` 图标，需要 adty_marketing·read） | 营销后台入口 | 12.1 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | 后台菜单顺序期望里加 `Marketing` | 新菜单项 | 12.1 |
| `web/src/routes/__root.tsx` | `RootComponent` 在 `useReferralCapture()` 后挂 `useSignupSourceCapture()` | 首次访问写 `adty_src` cookie（UTM、落地页、来源主机名） | 12.5 |
| `controller/topup_bepusdt.go` | `bepusdtPayRequest` 加 `promo_code`；`AdtyQuoteFor` 多传优惠码参数 | 下单时校验并快照优惠码 | 12.4 |
| `web/src/features/wallet/index.tsx` | `handlePay` 多传 `promoCode`，写进 `ActivePayment` | 充值卡的优惠码传给支付弹窗 | 12.4 |
| `middleware/turnstile-check.go` | `TurnstileCheck` 在调 siteverify 之前加一段：gin 上下文有 `adty_site`（11.2 写入）且这个站的 widget 有 secret 时，改调 `adtyTurnstileCheckSite`（用分站 secret，并比对返回的 hostname）；上游那次调用的地址改成包级变量 `adtyTurnstileSiteverifyURL`（值不变） | 分站按站点选 Turnstile secret | 11.8 |
| `main.go` | 在 I18n 中间件之后加 `server.Use(middleware.AdtySiteContext())` | 按 Host 识别分站 | 11.2 |
| `controller/user.go` | `Register` 写 `site_id`，closing 的站拒绝注册；`setupLogin`、`setupLoginAtAuthVersion` 开头各一行站点检查 | 分站注册归属、site_login_only | 11.2 |
| `controller/login_verification.go` | `completeVerifiedLoginResponse` 再查一次站点，不符就撤掉刚建的会话 | 防止在别的站完成二次验证 | 11.2 |
| `controller/misc.go` | `GetStatus` 末尾一行 `service.AdtyOverrideStatus` | 分站站名、Logo、地址，关掉第三方登录和 Passkey | 11.2 |
| `controller/option.go` | `adty_reseller_setting.*` 拒绝走通用接口 | 防止绕过金额参数和依赖项的校验 | 11.1 |
| `controller/audit.go` | 加 `adty.site.*` 审计模板 | 分站管理的审计内容 | 11.1 |
| `model/user.go` | `ToBaseUser` 带上 `SiteId` | 用户缓存要有所属站点 | 11.4 |
| `model/user_cache.go` | `UserBase` 加 `SiteId`，缓存版本 2→3 | 旧缓存失效 | 11.4 |
| `types/price_data.go` | `GroupRatioInfo` 加 `SiteMarkup` | 记下乘进去的 k | 11.4 |
| `relay/helper/price.go` | `HandleGroupRatio` return 前乘 k（渠道测试跳过） | 分站价覆盖倍率、按次、阶梯、图片、任务提交 | 11.4 |
| `service/quota.go` | `PreWssConsumeQuota` 算出分组倍率后乘 k | Realtime 预扣 | 11.4 |
| `service/task_billing.go` | 按 token 重算的分组倍率改用快照；`taskBillingOther` 写 k、d 到 admin_info | 不重复乘 k，任务收益记账 | 11.4、11.5 |
| `model/task.go` | `TaskBillingContext` 加 `SiteMarkup`、`SiteCostFactor`、`SiteId` | 任务快照 | 11.4、11.5 |
| `controller/relay.go` | 建 BillingContext 后一行 `AdtyStampTaskSiteBilling` | 写任务快照 | 11.4 |
| `controller/pricing.go` | `GetPricing` 一行 `AdtyApplySitePricing` | 价格页按分站倍率 | 11.4 |
| `controller/group.go` | `/api/user/self/groups` 的 ratio 改调 `AdtyUserGroupRatio` | 分组列表按分站价 | 11.4 |
| `model/log.go` | `RecordConsumeLog`、`RecordTaskBillingLog` 在消费日志开关前各一行钩子 | k、d 写进 admin_info，站长收益记账 | 11.4、11.5 |
| `controller/topup_bepusdt.go` | 报价改调 `AdtyQuoteForSite`（带站点和优惠码） | 分站客户用站点的加赠档位 | 11.5 |
| `router/adty_admin_permission_test.go` | 资源列表加 `adty_site` | 新权限资源 | 11.1 |
| `service/security_verification.go` | BindVerificationOperation、securityVerificationPolicy 的 default 分支和 GetVerificationRequirements 的「密码登录已关闭」分支各挂一处，识别 site.owner.manage（实现在 service/adty_site_verification.go） | 站长改价、改加赠、送试用、删域名要重新验证身份，proof 绑定操作和目标 | 11.6 |
| `web/src/features/auth/secure-verification/types.ts` | SecurityProofScope 和 VerificationOperation 加 site.owner.manage 及其 {action, target} 上下文 | 前端站长中心申请二次验证时有类型 | 11.6 |
| `common/email.go` | 加 `SendEmailFrom(from, subject, receiver, content)`，`SendEmail` 转调它并传空；Resend 用整个发件地址，SMTP 只换显示名（`From:` 头的显示名改用 `mime.QEncoding` 编码） | 分站系统邮件的站名和发件域名 | 11.3 |
| `controller/misc.go` | `SendEmailVerification` 的站名和发件人用 `AdtySiteForRequest`；`SendPasswordResetEmail` 的链接、站名、发件人用 `AdtySiteForUser(user.Id)`，不再用 `ServerAddress` | 邮件链接按用户所属站点，防 Host header injection | 11.3 |
| `service/account_security.go` | `NotifyAccountSecurityChange` 的站名和发件人按收件人所属站点（`AdtySiteForEmail`） | 分站客户收到分站站名 | 11.3 |
| `service/email_binding.go` | `sendEmailBindingCodes` 的站名和发件人按 `state.Authorization.UserID` 所属站点 | 同上 | 11.3 |
| `service/user_notify.go` | `sendEmailNotify` 多一个站点参数：发件人按用户所属站点，标题里的平台站名换成分站站名 | 同上 | 11.3 |
| `service/quota.go` | 余额提醒邮件的充值链接用 `AdtySiteBaseURL(AdtySiteForUser(uid))`；`quotaNotifyContent` 多一个 `baseURL` 参数 | 链接指向用户所属站点 | 11.3 |
| `router/main.go` | `SetPluginRouter` 之后加一行 `SetAdtyWebRoutes(router)`（`/pay/result/:token`、`/robots.txt`） | 支付跳回、按站点的 robots.txt | 11.3 |
| `controller/user.go` | buildSelfUserData 加 adty_site_owner（调 service.AdtyIsSiteOwner，分站功能关着时不查库） | 侧边栏「站长中心」只对站长显示 | 11.6 |
| `web/src/stores/auth-store.ts` | AuthUser 加可选字段 adty_site_owner | 同上 | 11.6 |
| `web/src/hooks/use-sidebar-data.ts` | personal 组在 Referral 后面插入「Site owner center」，只在 adty_site_owner 为 true 时出现 | 站长中心入口 | 11.6 |
| `web/src/hooks/use-sidebar-config.ts` | 默认模块加 personal.site_owner，URL 映射加 /site-owner/overview | 管理员可以在侧边栏模块里关掉它 | 11.6 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | 新增「site owner center entry」三个用例，sidebarFor 多一个 siteOwner 参数 | 站长才看到菜单、模块开关能关 | 11.6 |
| `web/src/features/legal/api.ts` | LegalDoc 加 site-agreement、site-customer-terms | 站长中心显示开站协议 | 11.6 |
| `web/src/features/auth/secure-verification/types.ts` | SiteOwnerAction 再加 'sender.delete' | 删除发件域名要重新验证 | 11.6 |
| `web/index.html` | favicon.ico 那行后加 SVG 和 16/32 PNG 图标、manifest、Safari mask-icon、Windows 磁贴两行 meta（都在 adty-head 标记之外，首页 SEO 替换不会动它们） | 新 Logo 全套图标 | 品牌 |
| `web/public/{favicon.ico,logo.png,adty-logo.svg,adty-og.png,adty-apple-touch-icon.png}` | 换成新 Logo（方案 H），logo.png 和 adty-logo.svg 用 48px 以下的简化版；新增 favicon.svg、favicon-16x16.png、favicon-32x32.png、android-chrome-192x192.png、android-chrome-512x512.png、maskable-512x512.png、site.webmanifest、safari-pinned-tab.svg、mstile-150x150.png | 新 Logo；其中的 `adty-logo.svg`、`favicon.svg`、`safari-pinned-tab.svg` 后来在 `2df2e4c` 换成 PNG 时删掉了 | 品牌 |
| `model/quota_reserve.go` | 加 `userQuotaChargeUpToScript`、`cacheChargeUserQuotaUpTo`、`chargeUserQuotaUpToDB`、`ChargeUserQuotaUpTo`：最多扣到 0 的原子扣减，返回实扣额。Redis 命中走 Lua，落库走 `persistUserQuotaDelta`、失败补偿缓存；Redis 关、异常或水合失败时事务内 `lockForUpdate` 读余额扣 `min(余额, 扣额)`，批量开着也直写不进队列 | 结算不透支，余额下限 | 余额下限 |
| `model/quota_reserve_test.go` | 加 `ChargeUserQuotaUpTo` 的用例：SQLite 和 PostgreSQL 各跑一遍 Redis 开关 × BatchUpdateEnabled 开关 × 扣足、刚好扣完、扣到 0、余额已为 0 和已为负；负数扣额报错；PostgreSQL 并发 20 笔不透支 | 钉住最多扣到 0 的语义，上游测试仍在 SQLite 上跑，要证明两种库都对 | 余额下限 |
| `relay/common/relay_info.go` | 计费字段旁加 `BillingShortfall int` | 本请求所有扣费点累计的平台承担差额，写日志 admin_info | 余额下限 |
| `service/funding_source.go` | `FundingSource.Settle` 改成返回 `(charged int, err error)`；`WalletFunding.Settle` 正差额改调 `model.ChargeUserQuotaUpTo` | 结算补扣最多扣到 0，调用方要知道实扣 | 余额下限 |
| `service/billing_session.go` | `Settle` 用实扣算差额累计进 `relayInfo.BillingShortfall`；`reserveFunding` 去掉 `requireAvailableQuota` 参数，补预扣一律 `PreConsume`（条件扣减，不够就 403），删掉「余额可为负」的注释；`shouldTrust` 钱包来源一律返回 false，`TrustQuotaUSD` 保留但对钱包不再生效 | 请求前余额不够就拒绝，不再记欠费，不再有信任旁路 | 余额下限 |
| `service/billing.go` | `SettleBilling` 两条路径（会话、旧 `PostConsumeQuota`）结算后调 `adtyNoteBillingShortfall` 把新增差额记进 gin 上下文 | 消费日志钩子从上下文取差额写 admin_info | 余额下限 |
| `service/quota.go` | `postConsumeQuotaResult` 加 `Shortfall`；`postConsumeQuotaWithResult` 正额改调 `model.ChargeUserQuotaUpTo` 并累计 `BillingShortfall`；`PreWssConsumeQuota` 扣费后记差额到上下文 | Realtime、违规扣费、MJ、旧结算路径都经过它，统一不透支 | 余额下限 |
| `service/violation_fee.go` | `ChargeViolationFeeIfNeeded` 扣费后记差额到上下文 | 违规扣费日志写 `shortfall_quota` | 余额下限 |
| `service/midjourney.go` | `SettleMidjourneyTaskBilling` 把 `task.Quota` 减去 `Shortfall` 后再落库 | 任务 quota 是退款依据，记实扣，退款不退平台没收到的钱 | 余额下限 |
| `service/task_billing.go` | `taskAdjustFunding` 返回差额，正差额改调 `model.ChargeUserQuotaUpTo`；`RecalculateTaskQuota` 的 `task.Quota` 记实扣（actual − 差额），差额 > 0 时写进日志 `admin_info.shortfall_quota` | 任务差额结算不透支；退款按实扣；站长收益按实扣 | 余额下限 |
| `service/tiered_settle.go` | `PrepareTieredBillingForSelectedGroup` 补预扣失败时原样返回 `*types.NewAPIError`（额度不足 403），不再包成 `ErrorCodeUpdateDataError` | 补预扣现在会因余额不够被拒，错误码要对 | 余额下限 |
| `service/tiered_settle_test.go` | `TestPrepareTieredBillingForSelectedGroupTopUpArrearsAllowsNegativeBalance` 改名 `...TopUpRejectsInsufficientBalance`，改成钉住：补预扣余额不够返回 403、余额不变；结算补扣扣到 0、差额 10k 记 `BillingShortfall` | 原测试钉住欠费（余额 −30k、−10k），和余额下限相反 | 余额下限 |
| `controller/billing_option_test.go` | `TestPreConsumePolicyDatabaseMatrix` 里 `above threshold`、`fractional threshold`、`unlimited token` 三条的 `wantHeld` 从 0 改成 1500，加注释 | 原用例钉住信任旁路不预扣；余额下限规则下钱包请求一律预扣 | 余额下限 |
| `setting/operation_setting/payment_setting.go` | `IsPaymentComplianceConfirmed()` 直接返回 true | adtyai 不需要支付合规确认，支付默认就能用（2026-11-01 用户要求） | 取消合规确认 |
| `web/src/features/system-settings/integrations/payment-settings-section.tsx` | 删掉合规确认的提示、确认对话框、没确认时把设置变灰的那层，以及只给它们用的状态、请求、常量、`complianceDefaults` 属性和导入 | 同上 | 取消合规确认 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 不再给支付设置分区传 `complianceDefaults` | 同上 | 取消合规确认 |
| `router/adty_bepusdt_test.go`、`router/adty_topup_test.go`、`service/adty_topup_quote_test.go` | 「没确认合规时拒绝」改成「不确认也能充值」；BEpusdt 那条去掉验证锁的前半段并改名 | 同上（这几个是我们自己的测试，记在这里方便对照） | 取消合规确认 |
| `web/src/components/layout/components/footer.tsx` | 删掉 `ProjectAttribution`、产品、客服、源码列；法律链接留在版权行 | 公开页不再显示上游署名、源码链接和客服邮箱 | 公开页脚 |
| `web/src/components/layout/components/public-layout.tsx` | 去掉页脚必须保留源码链接的注释 | 同上 | 公开页脚 |
| `web/src/features/about/index.tsx` | 空状态只留「未设置关于内容」和图标，去掉上游仓库、作者和许可证链接 | 后台没填关于内容时不再列出上游作者 | 公开页脚 |
| `web/src/features/system-settings/billing/section-registry.tsx` | USDT、易支付不再单独成计费分区，参数改从支付网关分区传入 | 后台充值设置收到同一个入口；旧地址跳到对应页签 | 本次修改 |
| `web/src/features/system-settings/hooks/use-update-option.ts` | 保存成功后把 `system-options` 标成过期，但不立刻重新拉取当前设置表单 | 一次保存多个字段时，中途刷新会覆盖还没保存的草稿 | 本次修改 |
| `model/billing_quota.go` | 读未刷库的用户额度差额时持有队列锁，读完即放，不包住数据库事务 | 队列刚刷进数据库时不能把行和差额拆开读；锁序反过来会和批量刷新死锁 | 本次修改 |
| `model/user_quota_adjustment.go` | 调额下限按数据库余额加上还没刷库的差额判断，队列锁不包住事务 | 不能在队列刚刷库时把活余额调成负数，也不能卡住并发调额 | 本次修改 |
| `setting/ratio_setting/group_ratio.go` | JSON 编解码改走 `common` 的编解码包装，不再直接用 `encoding/json` | 根模块的 JSON 编解码必须走统一编解码 | 本次修改 |
| `middleware/auth.go` | `TokenAuth` 和 `TokenAuthReadOnly` 验证用户后核对实际 Host 与 Key 用户的站点归属 | 主站、各分站和未知 Host 之间不能跨站使用 API Key，包含只读接口；错误使用八种后端语言 | 全平台验收 B/P |
| `controller/responses_websocket.go` | 每条 `response.create` 重新鉴权时固定使用已认证握手的实际 Host | 内部合成请求不能改变站点身份，分站长连接继续按本站身份鉴权 | 全平台验收 B/P |
| `middleware/model-rate-limit.go`、`middleware/model_rate_limit_test.go` | Redis 启用但客户端为空且存在次数限制时拒绝请求，保留未限流/关闭限流的正常请求；补真实中间件五项回归 | 防止成功次数及总次数检查空指针，依赖异常不能放行受限请求 | 全平台验收 R/S |
| `web/src/components/layout/components/{nav-group.tsx,chat-presets-item.tsx}` | 子菜单默认折叠，移除当前路由自动展开；既有 Collapsible 保留用户选择和子项高亮 | 最新菜单规则，减少初始占用空间 | 全平台验收 A |
| `router/{relay-router.go,web-router.go}`、`middleware/{distributor.go,access_token_routes.go}` | 删除 Playground 会话中转路由、专用分组解析和令牌规则；退役 `/pg` 路径不再回退 HTML 成功页 | 最新产品范围删除 Playground | 全平台验收 A/S |
| `router/main.go`、`pkg/jsplugin/routing.go` | 外置前端模式也在插件分发前拒绝 `/pg`；插件路由保留 `/pg` 命名空间，不能重新接管退役接口 | 防止部署模式或插件重新暴露旧接口 | 全平台验收 A/S |
| `model/user.go`、`controller/user.go`、`web/src/hooks/{use-sidebar-data.ts,use-sidebar-config.ts}`、`web/src/features/{profile/components/sidebar-modules-card.tsx,system-settings/maintenance/config.ts,system-settings/maintenance/sidebar-modules-section.tsx}` | 移除 Playground 菜单和默认开关，管理员旧配置不再呈现该开关 | 历史设置不能重新暴露退役入口 | 全平台验收 A |
| `web/src/features/dashboard/components/overview/overview-dashboard.tsx`、`web/src/lib/legacy-route.ts`、`web/src/routeTree.gen.ts` | 首次请求快捷入口指向已有 API 文档；旧书签回仪表盘；构建重新生成路由树 | 清除页面、路由及旧链接残留 | 全平台验收 A |
| `web/src/features/dashboard/components/overview/{overview-dashboard.tsx,__tests__/setup-guide.test.tsx}` | 文档入口使用八语已存在的 `Documentation` 键并补可见文案回归 | 避免未登记的英文键在其他语言界面直接显示 | 全平台验收 A/Q |

| `controller/option.go` | 拒绝退休营销配置，保留充值优惠码布尔键 | 最新产品范围排除独立营销，保留充值与系统通知 | 营销删除 |
| `router/main.go` | 在插件分发前拒绝退休营销 API | 插件不能恢复已删除的接口 | 营销删除 |
| `router/web-router.go` | 内部和外置前端模式统一返回退休路径 JSON 404 | 旧 API 不能回退 HTML 或重定向 | 营销删除 |
| `router/plugin_router_test.go` | 覆盖插件和外置前端的营销精确及子路径 | 验证删除后的路由边界 | 营销删除 |
| `service/access_token_scope.go` | 删除退休营销资源的令牌规则 | 旧权限不能授权新资金写入 | 营销删除 |
| `service/authz/action_migration.go` | 删除营销动作迁移登记 | 权限目录不再包含营销功能 | 营销删除 |
| `service/authz/resources_adty.go` | 删除营销权限资源 | 权限目录不再包含营销功能 | 营销删除 |
| `service/authz/authz_test.go` | 用保留资源验证 scoped 旧策略迁移 | 保留通用权限迁移回归 | 营销删除 |
| `web/src/features/auth/api.ts` | 注册请求不再携带营销同意及来源 | 不再采集营销数据 | 营销删除 |
| `web/src/features/auth/types.ts` | 删除营销同意请求字段 | 与注册请求实现一致 | 营销删除 |
| `web/src/features/auth/sign-up/components/sign-up-form.tsx` | 删除营销同意控件 | 不再采集营销数据 | 营销删除 |
| `web/src/routes/__root.tsx` | 删除注册来源捕获 | 不再采集营销数据 | 营销删除 |
| `web/src/features/profile/index.tsx` | 移除个人营销偏好卡 | 最新产品范围排除独立营销，保留充值与系统通知 | 营销删除 |
| `web/src/features/system-settings/billing/index.tsx` | 挂载优惠码计费分区 | 在既有 Root 系统设置保留充值配置 | 营销删除 |
| `web/src/features/system-settings/billing/section-registry.tsx` | 登记优惠码分区和原开关值 | 在既有 Root 系统设置保留充值配置 | 营销删除 |
| `web/src/features/system-settings/types.ts` | 登记优惠码分区类型 | 在既有 Root 系统设置保留充值配置 | 营销删除 |
| `web/src/features/system-settings/maintenance/config.ts` | 删除默认营销模块 | 不再显示退休功能 | 营销删除 |
| `web/src/features/system-settings/maintenance/sidebar-modules-section.tsx` | 删除营销模块设置项 | 历史开关不能恢复菜单 | 营销删除 |
| `web/src/hooks/use-sidebar-config.ts` | 忽略历史营销侧栏开关 | 历史开关不能恢复菜单 | 营销删除 |
| `web/src/hooks/use-sidebar-data.ts` | 删除营销菜单及图标 | 不再显示退休功能 | 营销删除 |
| `web/src/hooks/__tests__/sidebar-config.test.tsx` | 更新默认侧栏断言 | 保留真实菜单配置回归 | 营销删除 |
| `web/src/routeTree.gen.ts` | 实际生产构建移除营销路由树 | 最新产品范围排除独立营销，保留充值与系统通知 | 营销删除 |
| `controller/option.go` | 隐藏并拒绝退休 model_deployment 配置 | 最新产品范围排除模型部署管理 | R-03 |
| `middleware/access_token_routes.go` | 删除部署接口令牌规则 | 最新产品范围排除模型部署管理 | R-03 |
| `middleware/audit.go` | 删除部署写操作审计登记，保留历史显示 | 最新产品范围排除模型部署管理 | R-03 |
| `middleware/testdata/adty_admin_routes.txt` | 移除退休部署路由快照 | 最新产品范围排除模型部署管理 | R-03 |
| `router/api-router.go` | 移除全部部署与容器管理路由 | 最新产品范围排除模型部署管理 | R-03 |
| `router/plugin_router_test.go` | 覆盖部署精确/子路径的插件和外置前端边界 | 最新产品范围排除模型部署管理 | R-03 |
| `router/web-router.go` | 在插件与外置前端前拒绝退休部署路径 | 最新产品范围排除模型部署管理 | R-03 |
| `service/access_token_scope.go` | 移除部署令牌资源 | 最新产品范围排除模型部署管理 | R-03 |
| `service/access_token_action_scope_test.go` | 验证Root不能新授予退休部署权限 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/channels/components/channels-columns.tsx` | 移除历史io.net来源的部署快捷入口 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/api.ts` | 删除部署API调用，保留模型与厂商操作 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/constants.ts` | 移除部署状态选项 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/index.tsx` | 移除部署分区及创建抽屉 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/lib/query-keys.ts` | 移除部署查询键 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/section-registry.tsx` | 移除部署导航登记 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/models/types.ts` | 移除部署专用类型 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/system-settings/models/index.tsx` | 移除io.net配置默认值 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/system-settings/models/section-registry.tsx` | 移除模型部署设置分区 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/features/system-settings/types.ts` | 移除部署配置类型 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/hooks/use-sidebar-config.ts` | 移除部署路径映射 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/lib/legacy-route.test.ts` | 验证退休部署书签回退 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/lib/legacy-route.ts` | 旧部署页面和设置回退保留的模型页 | 最新产品范围排除模型部署管理 | R-03 |
| `web/src/routes/_authenticated/models/$section.tsx` | 移除部署专用搜索参数和访问分支 | 最新产品范围排除模型部署管理 | R-03 |
| `.github/workflows/ci.yml` | 为真实备份脚本边界测试安装 python3/libpq5 | PostgreSQL 官方连接解析及密码保护不可跳过 | R-04 |
| `model/option.go` | 配置存储边界加密分站secret；加载失败清空此域凭证并继续其他设置，批量写锁序固定 | Cloudflare Token 加密存储、历史升级及失败关闭 | R-05 |
| `service/referral_test.go` | 用真实登录指纹落库值尝试解密配置凭据 | 防止业务HMAC输出与凭据加密key重合 | R-05 |
| `relay/channel/openai/relay_realtime.go` | 串行保护共享用量/音频配置，监听请求取消，限制双向帧大小，关闭时有界收敛并等待读协程，失败预扣段只累计一次，兼容缺失 response 的完成事件 | 防止 Realtime 用量丢失/重复累计、panic 及连接资源泄漏 | G-01 |
| `relay/channel/openai/relay_realtime_test.go` | 用真实双向 WebSocket 覆盖取消、写失败、双向超限、并发精确用量与真实隔离 PostgreSQL 余额不足 | 验证 Realtime 生命周期及失败预扣计量回归 | G-01 |
| `model/channel_cache.go`、`model/channel_satisfy.go`、`model/ability.go`、`model/pricing.go` | 配置提交后失效快照，失败时从数据库核验路由、状态和高级配置；generation 拒绝旧重载，定时故障保留有效快照 | 防止删除或禁用后继续使用旧渠道 | D-CACHE-01 |
| `model/channel.go`、`model/adty_group.go`、`model/model_meta.go` | 状态及分组/模型写入使用统一刷新，多 Key 轮询在缓存回退和恢复期间保留游标 | 配置撤销与轮询恢复一致性 | D-CACHE-01 |
| `controller/channel.go`、`controller/channel_priority.go`、`controller/channel_upstream_update.go`、`controller/task_plugin.go`、`service/codex_credential_refresh.go`、`service/codex_credential_refresh_task.go` | 配置变更统一调用 RefreshChannelCache；批量解绑、模型和凭据分页更新在部分提交后失败也刷新 | 覆盖管理接口及后台写入的缓存生效边界 | D-CACHE-01 |
| `controller/channel_cache_refresh_test.go`、`model/channel_cache_failure_test.go`、`service/codex_credential_refresh_test.go` | 真实数据库覆盖删除/禁用/改模型、查询失败、旧重载、单 Key 状态、轮询恢复及批量部分提交 | SQLite/MySQL/PostgreSQL 矩阵与 race 回归 | D-CACHE-01 |
| `controller/adty_site_admin.go`、`router/adty_site_test.go` | closed 动作在关站服务调用前要求 Root；真实 PG 会话/PAT 验证即时/定时及主站/分站目标，保留委派暂停和恢复 | 防止普通管理员通过分站调整权限迁移客户 | FIN-I-05 |
| `web/src/features/adty-site-admin/components/site-detail-sheet.tsx`、`close-site-dialog.tsx` 及现有 `__tests__/site-admin.test.tsx` | 仅 Root 展示关站入口，身份变化时移除弹窗并在排队提交前重新检查 | 前后端权限一致，保留现有 Dialog/Button 组件 | FIN-I-05 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-site-status.mjs` | 新增独立 PostgreSQL/Redis 生产浏览器场景，验证桌面/移动委派管理、直接请求拒绝及 Root 页面关站与审计 | 避免长钱包场景耗尽默认验证限流，不放宽生产限制 | FIN-I-05 |
| `model/adty_site.go`、`model/adty_site_user_test.go` | 迁移按 ID 锁用户和 Token，同事务软撤销旧 Key 并递增认证版本，提交后发布用户版本与清理缓存；真实三库验证回滚及 MySQL/PG 创建先后顺序 | 永久撤销旧秘密，保留历史事实，阻止恢复旧用户 hash 后误用原站点归属；浏览器需重新登录 | FIN-I-06 凭证子步骤 |
| `router/adty_site_context_test.go`、`service/referral_test.go` | 主站/分站及即时/定时迁移验证模型、只读用量和日志鉴权，真实 Redis 恢复旧 hash/generation；共享关站 fixture 补 Token 表 | 保护新 Key 与其他客户凭证，覆盖真实关站调用路径；资金及在途结清仍未关闭 | FIN-I-06 凭证子步骤 |
| `controller/misc.go`、`router/adty_site_brand_test.go` | 按用户所属主站/分站生成 fragment 重置链接，回归含加号邮箱 | 避免新邮件凭证进入首次文档 HTTP 请求 | AUTH-RESET-01 |
| `web/index.html`、`web/src/routes/(auth)/user/reset.tsx` | 资源加载前捕获新旧链接并清 URL，路由接收临时内存凭证后删除读取器，复用现有重置表单 | 清 URL 后仍能提交；刷新不恢复凭证，不写持久存储 | AUTH-RESET-01 |
| `middleware/security_headers.go`、`middleware/auth_origin_test.go`、`deploy/adty/nginx.conf` | 重置入口含尾斜杠使用 no-referrer；代理日志去掉旧 query 并遮蔽重置 Referer | 防止同源资源泄露凭证，保留其他入口既有策略；仓外代理与历史日志另验 | AUTH-RESET-01 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-password-reset.mjs` | 独立 PostgreSQL/Redis/生产 Chromium 验证新旧链接、桌面/移动、提交参数、清 URL 和刷新失效 | 真实后端拒绝合成凭证；不将该浏览器场景宣称为成功重置全链或整体 ASVS 合规 | AUTH-RESET-01 |
| `web/src/context/__tests__/theme-boot-script.test.ts` | 按 HTML 顺序执行所有无属性 inline classic scripts，保留原 light/dark 断言 | 新重置 bootstrap 在主题脚本之前运行，测试不能再假定主题是第一个脚本 | AUTH-RESET-01 |
| `web/src/components/ai-elements/code-block.tsx`、`__tests__/code-block-copy.test.tsx` | 复制反馈只保留最新一次计时，卸载清理并忽略已离开页面的异步回执 | 完整测试实际捕获卸载后定时更新；现有 CopyButton/useCopyToClipboard 不提供原 Error 回调契约，保留本组件 API | 本批门禁附带修复 |
| `web/src/features/profile/`、`web/src/features/system-settings/billing/`、`system-settings/types.ts` | 删除签到卡、设置分区、默认值和专用 API/类型/组件；真实 Profile 对旧启用状态不请求不展示，旧分区回退额度设置 | 后端永久403及历史签到资金保护保留；删除路径同步登记 | UI-CHECKIN-01 |
| `web/src/hooks/use-notifications.ts`、`__tests__/notifications.test.tsx` | 公告未读计算订阅已读 key 数组；ID 和历史内容 key 的多个挂载消费者即时刷新 | 仅修复响应性，不能代替账号隔离或服务端回执 | N-03 |
| `web/src/i18n/locales/ko.json` | Pancake 的 Store 使用韩语 스토어 | 与相邻商店文案一致，保留翻译键 | I18N-01 |
| `model/user.go`、`model/user_pagination_test.go`、`model/pancake_refund_test.go` | 用户列表/搜索的 count 和分页共用站点约束，提供受角色上限约束的站点 ID 集合；修正旧分页 fixture 的软删除残留 | 主站 0 与全部用户语义明确，保留角色和软删除边界 | M-01 |
| `controller/user.go`、`service/adty_site_resolve.go`、`controller/adty_user_projection_test.go` | 批量投影站点名到列表/搜索/详情；严格校验 site_id；只输出可见站点 ID/名称，三库 HTTP 回归 | 避免 N+1、旧品牌缓存及泄露站点配置 | M-01 |
| `router/api-router.go`、`middleware/access_token_routes.go`、`router/adty_admin_permission_test.go` | 新增用户站点选项路由，复用用户读取授权，验证 session/PAT 授予、撤销和错误 scope | ADTY 模式使用 adty_user:read，用户筛选不替代授权 | M-01 |
| `web/src/features/users/`、`web/src/routes/_authenticated/users/index.tsx` | 保留站点字段，复用 DataTablePage/useTableUrlState 单选筛选及详情 Drawer，显示名称和 ID，过期详情请求不覆盖当前用户 | URL 恢复、切站重置分页、主站 0、清除和加载错误重试 | M-01 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-user-sites.mjs` | 独立生产浏览器场景验证桌面/移动列表、筛选、详情和刷新；SQL 仅准备隔离客户归属和品牌 fixture | 验证用户查询链，不冒称品牌保存、客户注册或全平台验收 | M-01 |
| `service/adty_payment_registry.go`、`controller/admin_finance.go` | 从注册服务输出六支付商能力和各订单适用动作 | 保留缺扩展、终态和权限边界；BEpusdt在FIN-F-02接官方Info查询后通过同一能力注册表提供补查 | M-04、FIN-F-02 |
| `controller/adty_topup_admin.go`、`router/adty-router.go`、`middleware/audit.go`、`router/adty_topup_test.go` | 新增有写权限和 PAT scope 保护的主动补查，8 秒超时、当前状态回读、安全错误和审计；覆盖失败、金额不符、撤权及重复幂等 | 查询可能入账，不能由财务只读权限调用；缺扩展订单安全拒绝 | M-04 |
| `service/adty_epay_client.go`、`service/adty_epay_health.go`、`service/adty_epay_test.go`、`controller/admin_finance_test.go` | Epay 实现注册查询能力，HTTPS/订单/商户/方法/网关号绑定，禁止查询重定向；六服务动作和筛选真实三库回归 | 复用统一金额校验及入账，不改变 FIN-F-01 场景或 FIN-G-01 来源资金规则 | M-04 |
| `web/src/features/recharge-audit/`、`web/src/i18n/locales/` | 后端能力驱动筛选及行操作，复用 ConfirmDialog/Alert 显示补查错误和未支付/成功结果，八语同步 | 撤权立即移除操作弹窗，退款明确线下退钱 | M-04 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-finance-actions.mjs` | 独立生产浏览器桌面/移动操作六服务，真实补单、退款 API 与数据库流水核对 | SQL 只准备未付款 fixture，无真实商户支付或外部退款；查询成功另由真实业务的外部 HTTP 边界测试验证 | M-04 |
| `service/adty_site_mail.go`、`adty_site_resolve.go`、`router/adty_seo.go` | 沿用品牌 JSON 增加纯文本标题、独立上传图标及严格六位十六进制主色，公开 status 和 SEO 按当前站点读取 | 兼容旧品牌；清空恢复默认；无 schema 变更，保留默认 HTML 与版权 | BRAND-01 |
| `service/adty_site_assets.go`、`router/adty_site_logo_test.go` | 读取白名单补 WebP，真实上传后读取并核对内容与 nosniff | 修复上传成功却 404；新上传仍拒绝 SVG，保留历史资源读取 | BRAND-01 |
| `controller/adty_site_test.go`、`router/adty_site_brand_test.go` | 真实三库保存/回读/status/非法输入与他站保护，验证标题 HTML 转义 | 沿用 owner 授权、缓存失效和 SEO 策略 | BRAND-01 |
| `web/src/features/adty-site-owner/`、`web/src/i18n/locales/` | 在既有 BrandCard 复用 Input/Button/图片上传，增加标题、主色、独立图标并失效 status；八语同步 | 无新增通用控件或依赖，保存失败保留草稿 | BRAND-01 |
| `web/src/lib/dom-utils.ts`、`status-query.ts`、`stores/system-config-store.ts`、`hooks/use-system-config.ts`、`components/theme-switch.tsx`、`main.tsx` | 冷启动、status 更新及主题切换统一应用品牌，独立图标不再被 Logo 预加载覆盖，清空移除颜色覆盖 | 前景按亮度保证对比，保留服务端 SEO 标题 | BRAND-01 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-site-brand.mjs` | 独立 PG17/Redis 生产桌面/移动验证真实设置、PNG/WebP 图片、公开 Host 品牌、刷新、他站隔离及清空 | SQL 仅查证，业务操作均走真实页面/API | BRAND-01 |
| `model/adty_notification.go`、`model/adty_migrate.go`、`service/adty_notification.go` | 增加 user/site/read-version 复合主键回执，组装当前 Host 可见内容并生成 SHA256 版本键 | 幂等保留首次读取时间；仅接受当前仍可见的版本 | N-02 |
| `controller/adty_notification.go`、`router/adty-router.go`、`router/adty_site_brand_test.go` | 注册通知收件箱 GET 与已读 POST，沿用用户会话及 `profile:read`/`profile:write` PAT scope | 不接受调用者指定账号或站点；真实三库测试新库、升级、重复写入、隔离和授权 | N-02 |
| `web/src/lib/api.ts`、`web/src/hooks/use-notifications.ts`、`web/src/stores/notification-store.ts`、`web/src/components/notification-popover.tsx` | 登录账号走服务端回执，匿名保持本地回执；账号/Host 进入 query key，窗口重新聚焦及轮询刷新 | 不迁移来源不明的历史匿名回执；读取失败显示重试且不清未读 | N-02 |
| `web/src/hooks/__tests__/notifications.test.tsx`、`web/scripts/adty/browser-notification-read.mjs` | 覆盖账号切换、独立 QueryClient、内容编辑、保存失败、恢复重试及桌面/移动真实浏览器流程 | 生产 PG17/Redis SQL 核对 A 有 4 条、B 有 0 条，pageerrors=0 | N-02 |
| `model/adty_notification.go`、`adty_migrate.go`、`service/adty_notification.go` | 发布实体和版本键唯一索引；Root 全用户/全部分站/指定单站，站长仅本站发布域；分页及全量未读计数 | 发布人快照、首次发布时间不随编辑变化，条件版本更新防覆盖；撤回保留历史；关联业务仅纯文本说明 | N-01 部分 |
| `controller/adty_notification.go`、`router/adty-router.go` | Root 与站长管理 GET/POST/PUT/DELETE，沿用 RootAuth/UserAuth 与明确 PAT scopes；收件按 Host 和 SQL 归属判定 | 不信任调用者传入发布人/发布域/账号/站点覆盖；站长不能修改 Root 或他站发布；冲突 409 | N-01 部分 |
| `web/src/features/adty-notifications/`、现有公告设置与站长本站设置 | 共用管理表单/列表，复用 Dialog、ConfirmDialog、StaticDataTable、PageSwitcher、Field 与错误/空/加载状态；RHF+Zod 和草稿保护 | 已检索现有封装，无新增通用控件或依赖；普通管理员无 Root 发布入口，失败保留草稿，撤回需确认 | N-01 部分 |
| `web/src/hooks/use-notifications.ts`、`lib/api.ts`、`components/notification-popover.tsx`、两个 Header | 无限分页收件、批次回执、全量未读角标及发布元数据；维持匿名和账号/Host 隔离 | 仅当前可见页签的已加载内容标读，旧版本/撤回内容不能写新回执；业务收件筛选及员工待办仍待后续 | N-01 部分 |
| `controller/admin_finance.go`、`router/adty-router.go`及既有测试 | 统一订单投影、冻结站点/类型筛选和站点选项；新增站长订单/汇总只读端点 | 真实三库验证迁站不改历史、主站0、未知null和失效参数；站长及PAT范围强制隔离，未增加资金写入口 | M-02 |
| `web/src/features/recharge-audit/`、`adty-site-owner/`、`hooks/use-sidebar-data.ts`、八语 | 既有订单组件复用为主后台和本站只读视图，增加站点/ID/类型/完成时间及筛选，菜单改订单管理 | 查询缓存包含账号/模式/站点，站长隐藏财务操作和全局异常面板；复用StaticDataTable/NativeSelect及状态组件，无新通用控件 | M-02 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-unified-orders.mjs` | 生产桌面/移动验证全局筛选和站长列表/越权拒绝 | 独立PG17/Redis，SQL仅准备历史订单和已迁站买家fixture，不操作真实资金 | M-02 |
| `model/adty_order_payment_event.go`、`adty_topup_order.go`、`adty_migrate.go`及六服务回调/查询接线 | 追加安全支付接收和查询结果；成功/重放随入账事务写入，不存载荷、凭据或供应商错误 | 真实三库fresh/upgrade/重复迁移及事务失败回滚；订单归属保持冻结，重放不重复入账 | M-03 |
| `controller/adty_order_trace.go`、`admin_finance.go`、`router/adty-router.go` | 财务/站长详情及各分区独立分页，只查询明确关联本订单的数据 | 沿用财务读取与站长wallet:read权限；冻结归属、人工凭证及后台原因脱敏，站长无主站佣金；未关联日汇总不伪造归属 | M-03 |
| `web/src/features/recharge-audit/`、八语及静态键 | 点击订单号打开既有Dialog，组合订单快照、退款结果和StaticDataTable追溯分区 | 复用加载/错误/重试及NativeSelect；账号或权限变化关闭旧详情；明确历史接收记录不完整和线下退款 | M-03 |
| `web/scripts/adty/browser-order-trace*.mjs`、`browser-unified-orders.mjs`、`scripts/adty/browser-business.sh` | 在原订单验收后追加真实签名通知、重放、人工补单、退款、分页和脱敏流程 | 隔离PG17/Redis及合成密钥；仅准备未付款订单fixture，真实业务API入账/退款，不操作真实商户资金 | M-03 |
| `model/adty_site_staff.go`、`user.go`、`user_cache.go`、`user_auth_cache.go`、`adty_site.go` | 独立员工成员与永久账号标记；主库三库迁移；缓存 schema 5 完整传播标记，旧 schema 局部更新失效 | 不挪用管理员角色或客户关系；不走注册赠金；禁用/删除不转换为客户 | O-01 部分 |
| `service/adty_site_staff.go`、`adty_site_owner.go`、`adty_site_verification.go`、`security_verification.go` | 本站站长 CRUD、四种默认只读角色、独立客户密码/禁用授权、逐请求 SQL 授权、事务审计和授权版本递增；客户查询排除员工 | 员工操作单次证明绑定 scope/动作/目标，事务重验站点/客户；禁止管理员、站长/历史站长和员工目标，关闭中拒绝写；审计失败回滚 | O-01 部分 |
| `controller/adty_site_staff.go`、`misc.go`、`middleware/adty_site_staff.go`、`auth.go`、`router/adty-router.go`、`controller/user.go` | session-only 员工/管理 API，员工白名单默认拒绝；客户重置申请复用既有邮件，客户自行设密；禁用立即撤销会话 | 邮箱遮罩，无客户 Key/原始日志；无可写客户字段，不输出密码或重置链接；申请不等于送达，客户重置保留 MFA；独立限流 | O-01 部分 |
| `web/src/features/adty-site-staff/`、`adty-site-owner/`、认证路由/导航及八语 | 组合既有 Dialog/ConfirmDialog/StaticDataTable/PageSwitcher/Field 与 RHF+Zod；员工安全操作复用 SecureVerificationDialog/hook，撤权关闭弹窗 | 复用业务封装，无新增通用控件/依赖；默认角色不授予写权，取消不写、失败可重试；资金规则待确认，整项仍未关闭 | O-01 部分 |
| `scripts/adty/browser-business.sh`、`web/scripts/adty/browser-site-staff.mjs`及既有后端/导航测试 | 独立 PG17/Redis 桌面/移动创建、登录、脱敏、撤权/改密/删除及客户安全操作；仅 loopback SMTP 模拟外部投递，真实客户选择密码；三库事务/race | 合成账号及资金，无真实邮件/支付，验证本批范围；既有员工主体 fresh/upgrade/重复迁移证据保留，不宣称全平台验收 | O-01 部分 |
| `service/adty_bepusdt_provider.go`、`adty_hosted_recovery.go`及既有BEpusdt/财务能力测试、浏览器脚本 | 官方Info的HTTPS查询、冻结事实绑定、手动及后台恢复，共用原入账/唯一流水及财务授权 | 官方v03413/BEpusdt的78db081b版本；不跟随重定向，不伪造hash，不绕过网关指纹规则；无原始事实时保留人工对账 | FIN-F-02 |
| `model/adty_payment_scene.go`、`adty_migrate.go`、`adty_topup_order.go` | 版本化动态场景、幂等默认配置、订单冻结名称/标识/版本及私有非密钥快照 | 配置→通道→原返佣用户→最终用户锁序，保留现有资金算法及历史订单；真实三库迁移验证 | FIN-F-01 |
| `service/adty_payment_scene.go`、`adty_epay_pick.go`及六商户创建汇合点 | Root CAS配置和同事务审计，SQL身份/站点/金额/币种/方式检查；易支付报价与降级先筛授权通道 | 原生和统一下单入口共同复核；省略场景不绕过停用，旧单可信回调继续幂等入账 | FIN-F-01 |
| `controller/adty_payment_scene.go`、`admin_finance.go`、`router/adty-router.go` | Root GET/PUT配置与option:read/write PAT范围、409竞争；订单详情公开最小冻结字段 | 不公开策略快照或密钥；用户与财务原授权边界保留 | FIN-F-01 |
| `web/src/features/adty-payment-scenes/`、`wallet/adty/`、`recharge-audit/`、支付General页及八语 | 复用Dialog/ConfirmDialog/MultiSelect/Field、RHF+Zod及草稿保护，Root管理、动态场景和冻结信息展示 | 无新通用组件/依赖；取消不写、冲突保留草稿、普通admin不可管理 | FIN-F-01 |
| `web/scripts/adty/browser-payment-scenes.mjs`、`scripts/adty/browser-business.sh` | 隔离PG17/Redis生产桌面/手机配置→报价→合成订单→停用→签名回调与重放 | Loopback商户HTTP边界，SQL只读取证据，不操作真实资金 | FIN-F-01 |
| `model/adty_site_ownership.go`、`adty_site.go`、`adty_migrate.go`及计费/充值/Task/Midjourney模型 | 追加交接版本记录和财务站长快照；日汇总唯一键增加站长，原索引在新约束建立后移除 | 三库fresh/upgrade和重复迁移保留金额；原预扣优先，历史零快照沿首任记录解析 | FIN-I-07 |
| `service/adty_site_ownership.go`、`adty_site_earning.go`、`adty_site_settlement_task.go`、`adty_site_stats.go`及异步恢复链 | Root事务CAS交接、员工暂停/授权清空、协议重接受、会话版本撤销；结算/退款/强制追回按原归属 | 用户/affiliate锁序与开站一致，审计失败全部回滚；不迁客户、不挪历史余额，独立账户仍待FIN-J-02 | FIN-I-07 |
| `controller/adty_site_admin.go`、`router/adty-router.go`、`web/src/features/adty-site-admin/`及八语 | Root专用POST、option:write PAT、版本冲突；详情搜索和交接确认、错误草稿及撤权保护 | 复用现有searchUsers、Combobox、Dialog、ConfirmDialog、Field、RHF+Zod和草稿保护；检索未发现适用共享用户选择器，无新增通用控件/依赖 | FIN-I-07 |
| `web/scripts/adty/browser-site-ownership.mjs`、`scripts/adty/browser-business.sh`及集中后端回归 | 隔离PG17/Redis的1440/390真实UI交接、协议、旧权限、迟到到账/退款重放、分任结算和追回 | SQL仅准备代表性历史订单/待结夹具；真实API处理合成资金，未操作真实商户 | FIN-I-07 |
| `AGENTS.md`、`web/AGENTS.md`、根目录各语言 `README*.md` | 同步 Root 站长交接、历史资金归属、支付场景、员工授权及构建版本说明，链接现有工程文档 | 保留原有标题、段落、表格及上游署名；版本由既有工作流生成，不改包版本或发布标签 | 2026-11-10 收尾 |
| `relay/mjproxy_handler.go` | `pinMidjourneyImageURL` 在 IP 字面量短路前重复执行 URL、私网和端口校验 | 公网图片重定向到回环地址时，原短路不复核，图片代理会读取私网响应 | OBS-ROOT-006 |
| `model/option.go` | `QuotaPerUnit` 只接受 500000；写入前和加载时都拒绝其他值，内存不改写 | 1 USD = 500000 额度是固定尺度，后台选项不能把它改成其他比例 | OBS-FIN-011 |
| `model/billing_operation.go` | 预扣前若令牌绑定团队，改扣团队共享余额并跳过个人钱包 | 团队请求不能改扣成员或负责人个人账户 | A.24.7 |
| `service/billing_session.go` | 团队协议价低于供货价时拒绝预扣 | 分站团队最终价不能低于供货价 | A.24.7 |
| `model/user.go` | 删除用户事务里调用团队关闭钩子 | 负责人注销时关闭团队，余额保留 | A.24.7 |
| `service/http_client.go` | 原请求带 `mj-api-secret` 时，跨源重定向直接拒绝 | Go 会转发非标准密钥头；307/308 不能把渠道密钥送到另一个源 | OBS-ROOT-018 |
| `AGENTS.md`、`web/AGENTS.md`、根目录各语言 `README*.md` | 站长不可更换；正式语言改为简体中文和英文；补上兑换码、团队、通知机器人、工单、翻译和分组健康的开发边界 | 旧交接和八种正式语言句子与已确认规则冲突；保留原标题、上游署名和模块路径 | A.20 / DEC-011 |
| `AGENTS.md`、`web/AGENTS.md`、根目录各语言 `README*.md`、`docs/adty/README.md` | 版本说明补齐其他分支和拉取请求的生成规则，并写明仓库内 `VERSION` 保持为空 | 与 `adty-ghcr.yml` 的实际写法一致；不改 `package.json` 包版本 | 版本约定 |
| `pkg/perf_metrics/metrics.go` | `RecordRelayResult` 在分类后调用 `GroupHealthObserver` | 分组健康按最终用户请求统计一次，观察者不能改路由 | A.23 |
| `service/security_verification.go` | 二次验证增加 `adty.notice.configure`，只允许 Root | Bot Token 和翻译凭据变更要重新验证，证明绑定目标和动作 | A.24.10 |
| `controller/misc.go` | `GetStatus` 里 `announcements` 字段的数据源从 `console_setting.GetAnnouncements()` 换成 `service.AdtyPublicAnnouncementsForRequest(c)`，返回形状不变 | 公告并入新表后，`/api/status` 仍要服务未登录访客（这条路由没有认证中间件），所以数据源换、形状不换；按 Host 决定站点可见性，分站不会看到主站公告 | 公告并入 |
| `web/src/features/system-settings/content/announcements-section.tsx` | 删掉直接编辑 `console_setting.announcements` 的旧表格和它的新增/编辑/删除对话框，只留公开面板开关和 `NotificationManager` | 新旧两套都能发公告、读者只有一份：同一件事两个来源。公告统一由发布记录承载，旧 option 保留为回滚源 | 公告并入 |
| `docker-compose.yml` | app 服务的 environment 增加 `ADTY_TELEGRAM_ALLOW_NETWORK=1` | 不设这一条时出站 Telegram 完全关闭，通知与公告同步一条都发不出去，而外部只看到「没有消息要发」；开发用 compose 故意不设，避免本机测试往真实频道发消息 | 公告 TG 同步 |
| `service/text_quota.go` | `calculateTextQuotaSummary` 在 `PriceData.UsePrice` 且 `hasBillableUsage()` 为假时补一条估算 usage（`PromptTokens`/`TotalTokens` 取 `max(GetEstimatePromptTokens(), 1)`），不改 `:397` 那个归零判据 | 按次计价的金额只由 `ModelPrice × QuotaPerUnit × GroupRatio` 决定，与 token 无关；零 token usage 不能整单免单。给判据再加一个分支会让 tiered 与 legacy 两处继续分叉，补估算让同一个判据自然为真 | ISS-R4-0085 |
| `relay/channel/ollama/stream.go` | 累积每个已发出 delta 的文本；扫描结束后 `TotalTokens == 0` 时回退 `service.ResponseText2Usage`；中流解码错误在「已交付过内容」时 `break` 而不再整单退款 | 只有拿到上游 `done` 帧才给 usage 赋值，连接提前 EOF 时账单为 0，客户白拿已交付的生成内容 | ISS-R4-0072 |
| `relay/channel/zhipu/relay-zhipu.go` | 无 `meta:` 帧时回退 `service.ResponseText2Usage`，不再返回 typed-nil；扫描协程加 `done` 通道，handler 提前返回时发送侧退出 | `var usage *dto.Usage` 装进 `any` 是 typed nil，`usage == nil` 判空失效，调用方断言后解引用即 panic；客户端断开时原实现的发送会永久阻塞 | ISS-R4-0070 |
| `relay/compatible_handler.go` | 新增 `usageOrEmpty(any) *dto.Usage`，`PostTextConsumeQuota` / `PostAudioConsumeQuota` 经 `settleUsage` 取值 | 兜底而非修复：真修复在渠道侧返回值。这层的意义是「任何一个渠道再交回 typed-nil 也不会 panic」，所以它故意吃掉边界而不是再报错 | ISS-R4-0070 |
| `relay/channel/zhipu_4v/image.go` | 新增 `zhipuImagePayloadCount`（对象计 1 张、数组取 `max(url 数, b64 数)`，与 `openaiImageResponseCount` 同规则）；重建响应后调 `info.UpdateImageCount(...)` | 按张计价的请求必须按实际交付计费：上游少产或被过滤时不能按请求预留的 `n` 收 | ISS-R4-0037 |
| `relay/channel/replicate/adaptor.go` | `ConvertImageRequest` 在合并完 `extra` / `extra_fields` **之后**用 `request.ImageCount(false)` 重写 `inputPayload["num_outputs"]`；`DoResponse` 按 `replicateImagePayloadCount` 计费 | 直通字段能绕过 `dto.MaxImageN` 把上游工作量放大到模型上限，而计费只认顶层 `n`——写在前、被覆盖在后，顺序就是根因 | ISS-R4-0118 |
| `controller/user.go` | ① 管理端用户视图不再内嵌 `*model.User`，改成显式字段白名单 DTO；`setting` 由 `adminUserSettingOf` 逐字段重建，推送配置整组剔除，保留 `DeletedAt`；② 改密校验原口令前查登录锁、`ErrCurrentPasswordInvalid` 时计一次失败、成功时清计数 | ① 只读管理员用一次列表请求就能导出全站用户的 `webhook_secret`/`gotify_token` 明文；`DeletedAt` 是功能字段（前端 `isUserDeleted` 的唯一依据），不是敏感字段，不能跟着凭据一起砍；② 改密校验原口令是第三个绕过登录锁的口令入口 | ISS-R4-0034、ISS-R4-0038 |
| `controller/secure_verification.go` | `/api/verify` 的 password 分支在「口令错误」与「已锁定」两种失败上都写 `user.security_verify` 审计；锁定返回 `SECURITY_VERIFICATION_LOCKED` 而不是 500 | 二次验证的口令旁路不能既不受登录锁定约束、又失败无留痕——那会让 2FA 的有效尝试次数变成不限 | ISS-R4-0038 |
| `service/security_verification.go` | password 分支接入 `LoginAccountKey` / `LoginLocked` / `RecordLoginFailure` / `ClearLoginFailures`，与登录共用同一失败计数和 15 分钟固定窗口；新增 `ErrVerificationLocked` | 同一账号的 2FA 口令猜测不该比登录口令猜测更便宜。计数键必须用解析出的 userID，否则用户名与邮箱两种写法各记一份，额度翻倍 | ISS-R4-0038 |
| `model/migration_dialector.go` | PostgreSQL 的 `MigrateColumnUnique` 不再把 `LOCK TABLE ?` 交给 GORM 渲染：在事务内 `SELECT current_schema()` 取字面量，拼成 `LOCK TABLE "schema"."table"`，标识符内部引号转义 | 旧写法传 `clause.Table`，GORM 内联成**不带 schema** 的 `LOCK TABLE "t"`——不是语法错，是锁靠 `search_path` 解析，与同一分支里带 schema 的目录查询可能不是同一个关系；而且换一个 var 形态（string / `clause.Expr`）就真的 42601。**这条是加固，不是修一个会崩启动的 bug** | ISS-R4-0129 |
| `model/migration_dialector_test.go` | 新增 PG `MigrateColumnUnique` 用例：断言 LOCK 语句是带 schema 的双引号字面量、外键命名的约束形态也能走通 | 证明这条分支真实进入，并且锁的目标与目录查询读到的是同一个关系 | ISS-R4-0129 |
| `model/external_identity_claim.go` | 站点回填只在目标站尚未占用同一 `(provider, subject)` 时搬迁 claim；关站造成的冲突降级为 `SysError` 告警并保留 claim 的历史站点；legacy import 的同形冲突同样降级 | 关站把同一 Telegram 身份的两个账号搬到同一站后，回填会撞 `(provider, site_id, subject)` 唯一索引，让下一次启动 fatal。**部分修复**：启动链第三步 `EnsureBuiltinOAuthSubjectIndexes` 仍会 fatal，见该条 | ISS-R4-0124 |
| `model/external_identity_claim_test.go` | 新增「关站后重启」用例，覆盖回填与 legacy import 两步 | 证明 23505 缺陷真实存在（反向验证红）且修复承重 | ISS-R4-0124 |
| `web/src/i18n/locales/en.json`、`web/src/i18n/locales/zh.json` | 新增 5 条文案：审批弹窗的两个标题、原因必填提示、说明段、输入占位符 | 后端把 `reason` 变成硬要求后前端必须跟上，否则 Root 点「通过」必然 400——finding 会从「缺一个字段」变成「按钮点了没反应」。文案必须两边同时加 | ISS-R4-0035 |
| `service/adty_remaining_gaps.go` | `ReviewAdtyStaffBalanceRequest` 把 `reason` 写进 ledger 与 `staff.balance.review` 审计的 `Reason`；审计在同一事务内写 `adty_site_audits`，官方 `audit_logs` 走 `LOG_DB`、失败只 `SysError` | 审批人写的原因必须落库，否则「为什么动钱」这一列永远空；`adty_site_audits` 同事务保证「钱动了至少有一条本地痕迹」，官方审计刻意不阻塞资金操作——日志库故障不能回滚一次已成立的余额调整 | ISS-R4-0035 |
| `controller/misc.go` | `GetStatus` 删 `github_oauth`、`github_client_id`、`discord_oauth`、`discord_client_id`、`linuxdo_oauth`、`linuxdo_client_id`、`linuxdo_minimum_trust_level`、`wechat_login`、`wechat_qrcode`、`oidc_enabled`、`oidc_client_id`、`oidc_authorization_endpoint`、`oidc_display_name` 十三个字段，并删掉末尾调 `oauth.GetEnabledCustomProviders()` 拼 `custom_oauth_providers` 的整段和 `oauth` 导入 | `/api/status` 不再对外报已删的登录方式；合上游时上游会把这些键加回来，要连同 `AdtyOverrideStatus` 之前那一段再删一次，前端也不该再读（见下面 `web/src/features/auth/types.ts` 那条） | 86536ce |
| `controller/option.go` | `UpdateOption` 的开关校验 `switch` 删 `GitHubOAuthEnabled`、`discord.enabled`、`oidc.enabled`、`LinuxDOOAuthEnabled`、`WeChatAuthEnabled` 五个 case，只剩邮箱域名、Turnstile 等 | 没有对应的登录方式，就不该再拦「先填 Client ID 才能打开」；上游新增 provider 的 case 要一起删，否则合并后这些键会被重新拦住或写进 options | 86536ce |
| `controller/oauth.go` | `oauthFlowPayload` 删 `OIDC`；`GenerateOAuthCode` 删 OIDC flow 初始化、`refreshCustomOAuthProvider` 调用和 `authorization_url` 分支；`HandleOAuth` 删 `refreshCustomOAuthProvider` 和 OIDC context 注入，取 token、取用户信息失败一律走 `writeSecurityOperationError`（原来的 Telegram 特例变成默认）；`findOrCreateOAuthUser`、`handleOAuthLogin` 去掉 `*oauth.OAuthToken` 形参和 `*service.LegacyGitHubMigration` 返回值，建号只剩 `user.InsertWithTx` 一条路；删 `handleOAuthError`、`recordLegacyGitHubBindingAudit`、`OAuthLegacyBindingNotConfirmedError`、`customOAuthRowExists` | 六种登录一起删，`/api/oauth/:provider` 只剩退役的 Telegram 占位（`IsEnabled()` 恒 false），整条链 fail closed；这是最容易冲突的文件——上游新增 provider 的专用 flow、legacy 绑定重写、可翻译报错都要按「不做第三方登录」再删一遍 | 86536ce |
| `controller/login_verification.go` | `VerifyLogin`、`LoginPasskeyFinish` 解构 `service.VerifyLoginCodeOnSite`、`CompleteLoginVerificationOnSite` 时少接一个返回值；`completeVerifiedLoginResponse` 去掉 `migration` 形参和「登录后重写 GitHub 绑定 → 发安全通知 + 记 `user.binding_bind` 审计」那段；删 `i18n` 导入 | 2FA/Passkey 登录链本身不动，只是不再顺带写 GitHub 绑定；上游改这几个签名会同时撞 `controller/user.go` 和 `controller/oauth.go` | 86536ce |
| `controller/user.go` | `loginMethodFromContext` 删 `/api/oauth/wechat` 一支；`setupLogin` 去掉 `migration *service.LegacyGitHubMigration` 形参（`Login` 改成两参），`service.StartLoginVerification` 同步少传一个参数 | legacy GitHub 绑定重写随 GitHub 登录删了；上游再动 `setupLogin` 按两参版本合，别把 `migration` 形参带回来 | 86536ce |
| `main.go` | `InitResources` 删 `oauth.LoadCustomProviders()` 调用和 `oauth` 导入 | 自定义 OAuth provider 不再从数据库载入；上游会把这行加回来，但 `oauth/generic.go` 和 `model/custom_oauth_provider.go` 已删，原样合会编译失败 | 86536ce |
| `model/main.go` | `migrateDB` 的 AutoMigrate 删 `&CustomOAuthProvider{}`、`&UserOAuthBinding{}`，删 `EnsureUserOAuthBindingSiteSubjectIndex` 调用；`EnsureBuiltinOAuthSubjectIndexes` 保留（`users` 表历史 `github_id` 等列还在） | 两张表的数据保留，但不新建、不建索引；合上游时别跟着把这两个模型和那个索引调用加回去，模型类型已随文件删除 | 86536ce |
| `model/option.go` | `InitOptionMap` 删 `GitHubOAuthEnabled`、`LinuxDOOAuthEnabled`、`WeChatAuthEnabled` 三个开关和 `GitHubClientId`、`GitHubClientSecret`、`WeChatServerAddress`、`WeChatServerToken`、`WeChatAccountQRCodeImageURL` 五个空默认值；`updateOptionMap` 删这些开关、`GitHubClientId`/`Secret`、`LinuxDOClientId`/`Secret`/`LinuxDOMinimumTrustLevel`、`WeChatServerAddress`/`Token`/`AccountQRCodeImageURL` 的赋值分支 | 这些键不再进 `OptionMap`，库里残留的 option 行下次同步也不会载入内存；合上游时上游新增的 OAuth 键不要加回 `OptionMap` | 86536ce |
| `model/user.go` | 删 `userBindColumns` 白名单和 `UpdateUserBindColumn`、`FillUserByGitHubId`/`DiscordId`/`OidcId`/`WeChatId`/`LinuxDOId`、`AssignBuiltinProviderIDsWithTx`、`IsWeChatIdAlreadyTaken`/`IsGitHubIdAlreadyTaken`/`IsDiscordIdAlreadyTaken`/`IsOidcIdAlreadyTaken`/`IsLinuxDOIdAlreadyTaken`；`ClearBinding` 的 `bindingColumnMap` 只留 `email`、`telegram`；`deleteUserAuthenticationData` 末尾改成 `return nil`，不再删 `user_oauth_bindings` | 列还在（不写删列迁移），但读写这些列的函数随 provider 删了；`ClearBinding` 只剩两项，上游新增绑定类型时要先判断我们还有没有对应登录方式；合并上游时这些 helper 会回来，按「没有 provider 就没有调用方」再删 | 86536ce |
| `model/account_security.go` | 删 `AccountLoginMethods`、`UnbindUserOAuthForSession`、`UpdateUserBindColumnForSessionWithTx`、`UpdateUserOAuthBindingForSessionWithTx`、`MigrateLegacyGitHubBindingWithTx`，文件只剩 `ChangeUserPassword` | 解绑、改绑和 legacy 重写都只服务已删的 provider；上游会让这个文件整段回来，按「只剩改密」再删 | 86536ce |
| `oauth/provider.go` | 删 `VerifiedEmailProvider` 接口（provider 自报已验证邮箱，登录时拿来匹配本地账号） | 没有实现方了；合上游时这个接口会回来，但没有 provider 也没人调 | 86536ce |
| `oauth/registry.go` | 删 `customProviderSlugs`、`customProviderConflicts` 两个 map 和 `RegisterCustom`、`HasCustomProviderConflict`、`GetEnabledCustomProviders`、`IsCustomProvider`、`LoadCustomProviders`、`ReloadCustomProviders`、`RegisterOrUpdateCustomProvider`、`UnregisterCustomProvider`；`Unregister` 只删 `providers` | 注册表只剩 `Register`、`GetProvider`、`GetAllProviders`、`IsProviderRegistered`（Telegram 占位仍注册）；上游这段会回来，但它依赖已删的 `oauth/generic.go` 和 `model/custom_oauth_provider.go`，不能原样合 | 86536ce |
| `oauth/types.go` | `OAuthToken` 只留 JSON 六个字段，删 `ClientID`、`Subject`、`VerifiedEmail`、`UserInfoEndpoint`；删 `OAuthError`、`NewOAuthError`、`NewOAuthErrorWithRaw`、`AccessDeniedError` 和 `msgInvalidCode` 等五条可翻译报错常量 | 这些是 OIDC 和通用 OAuth 的上下文与报错用的；上游给 `OAuthToken` 加字段时没人填值，保持删 | 86536ce |
| `router/api-router.go` | 删 `GET /api/oauth/wechat`、`POST /api/oauth/wechat/bind`、自助的 `GET /api/user/oauth/bindings` 和 `DELETE /api/user/oauth/bindings/:provider_id`、管理员的 `GET /api/user/:id/oauth/bindings` 和 `DELETE /api/user/:id/oauth/bindings/:provider_id`、整个 `/api/custom-oauth-provider` Root 组六条；`/api/oauth/:provider` 保留，注释改成只剩退役的 Telegram 占位 | handler 随 `controller/wechat.go`、`controller/custom_oauth.go` 一起删了；合上游时这些路由会回来，要连令牌规则、审计登记和前端一起判断，只删一半会编译失败或留死代码 | 86536ce |
| `service/account_security.go` | 删 `UnbindAccountOAuth`（拼 `AccountLoginMethods` 快照后调 `model.UnbindUserOAuthForSession`），`AccountMailLanguage` 等保留 | 解绑入口随路由删了；合上游时这个函数会回来，但依赖的两个类型都已删 | 86536ce |
| `service/login_verification.go` | 删 `LegacyGitHubMigration` 类型、`loginFlowPayload` 的 `PendingGitHubID`/`PendingGitHubLegacyID`、`StartLoginVerification` 的 `migration` 形参和「不需要二次验证就不能带重写」的分支；`VerifyLoginCode`、`VerifyLoginCodeOnSite`、`CompleteLoginVerification`、`CompleteLoginVerificationOnSite`、`completeLoginVerification` 去掉 `*LegacyGitHubMigration` 返回值和事务内的 `MigrateLegacyGitHubBindingWithTx` | 一次登录不再顺带改写 GitHub 绑定；上游改这些签名会在三处 controller 一起冲突（`controller/login_verification.go`、`controller/user.go`、`controller/oauth.go`） | 86536ce |
| `service/security_verification.go` | `BindVerificationOperation` 删 `wechat` 上下文的 case；`verificationOAuthProviders` 删自定义 provider 绑定表查询和 `github_id`、`discord_id`、`oidc_id`、`linux_do_id` 四个分支，只认 `telegram_id` | 二次验证的「用已绑定账号」不能用已删的 provider；上游会把这些分支加回来，而对应列已没有写入路径 | 86536ce |
| `router/access_token_scope_test.go` | `accessTokenExemptRoutes` 删 `GET /api/oauth/wechat` | 路由删了；免检名单里留着没有对应路由的条目，上游 `TestAccessTokenRouteRulesCoverEveryDashboardRoute` 会失败 | 86536ce |
| `controller/auth_flow_test.go`、`controller/oauth_site_flow_test.go`、`controller/oauth_site_identity_test.go`、`model/oauth_site_subject_test.go` | 删只测 GitHub、OIDC、通用 provider 的用例和 `legacyGitHubOAuthProvider` 夹具（legacy 绑定重写、OIDC 分站回调、自定义 provider 跨站 subject、内置列跨站解绑）；`setupOAuthSiteFixture` 换成不依赖 provider 的 `setupOAuthCallbackSiteFixture` | 被测的 provider 和绑定重写都删了；上游让这些用例回来时会引用已不存在的 `oauth.GenericOAuthProvider`、`MigrateLegacyGitHubBindingWithTx` | 86536ce |
| `controller/security_account_test.go`、`model/adty_conditional_index_test.go` | 删 `TestSecurityAccountUnbindPreservesUsableLoginMethod`；条件唯一索引的并发用例不再走 `UpdateUserBindColumnForSessionWithTx`，改成直接 `Update` 那一列，`UserSession` 夹具跟着去掉 | 两个被测函数随解绑、改绑一起删了；索引本身（`github_id` 等列上的条件唯一索引）没动，测试仍钉住唯一冲突 | 86536ce |
| `controller/admin_user_verification_test.go`、`controller/oauth_site_register_test.go`、`controller/security_enrollment_test.go`、`controller/user_manage_test.go`、`controller/access_token_audit_test.go`、`model/user_authentication_test.go`、`model/user_delete_identity_test.go`、`model/task_cas_test.go` | 测试建表和夹具去掉 `model.UserOAuthBinding`（`user_oauth_bindings` 表）、自定义 provider 解绑用例和已删的 `IsWeChatIdAlreadyTaken` 断言（改成直接数库里的 `wechat_id`）；`TestAdminUserBindingProofRejectsWrongBindingType` 取代原来「内置和自定义绑定分开校验」的用例 | 模型和路由删了，建表会失败、用例也打不到 handler | 86536ce |
| `controller/auth_session_test.go`、`controller/login_backup_retention_test.go`、`controller/login_totp_replay_test.go`、`controller/passkey_test.go`、`controller/model_list_test.go` | `setupLogin`、`service.StartLoginVerification` 的调用去掉 `nil` 的 `migration` 实参 | 跟着函数签名改，行为不变 | 86536ce |
| `controller/ratio_sync_test.go` | `portOf` 从删掉的 `controller/custom_oauth_discovery_test.go` 搬到文件末尾，加 `strings` 导入 | 只是挪位置：删自定义 OAuth discovery 测试时别把这个 helper 一起删掉，`TestFetchUpstreamRatios*` 还在用 | 86536ce |
| `web/src/features/auth/api.ts`、`web/src/features/auth/index.ts`、`web/src/features/auth/types.ts` | `createOAuthAuthorization`、`createOAuthFlow`、`githubOAuthStart`、`wechatLoginByCode`、`telegramLogin` 和它们的导出删掉；`SystemStatus` 删 `github_*`、`discord_*`、`oidc_*`、`linuxdo_*`、`telegram_oauth*`、`wechat_*` 字段（`telegram_bot_name` 保留） | 没有登录入口就不需要这些请求和后端字段；合上游时 `SystemStatus` 的字段会回来，前端又会按它们渲染已删的按钮 | 86536ce |
| `web/src/features/auth/sign-in/components/user-auth-form.tsx`、`web/src/features/auth/sign-up/components/sign-up-form.tsx` | 两个表单删 `<OAuthProviders />` 区块、微信扫码对话框（二维码、验证码、`wechatLoginByCode` 提交流程）和随之不用的状态与 `Dialog`、`Label`、`Input` 导入；注册表单也不再读 `oauth_register_enabled`、`wechat_qrcode` | 登录页和注册页只剩账号密码与 Passkey；上游会把这套组件加回来，但它依赖已删的 `oauth-providers.tsx`、`lib/oauth.ts` | 86536ce |
| `web/src/features/auth/secure-verification/{api.ts,components/secure-verification-dialog.tsx}` | 删 `verifyOAuth`；`withDeviceAvailability` 过滤掉 `oauth` 方法，`verify()` 遇到 `oauth` 直接抛 `OAuth verification is unavailable`；对话框删 OAuth 页签和「用已绑定账号继续」按钮 | 二次验证不能再用第三方账号作凭证；上游新增的 `oauth` 方法要一起处理，否则前端会显示一个点了必失败的页签 | 86536ce |
| `web/src/features/security/components/account-bindings.tsx`、`web/src/features/profile/api.ts`、`web/src/features/users/api.ts`、`web/src/features/users/components/dialogs/user-binding-dialog.tsx`、`web/src/hooks/use-user-display.ts` | 账号绑定页只剩邮箱一项（删 GitHub、Discord、OIDC、Telegram、LinuxDO、微信和自定义 provider 的绑定与解绑流程）；`profile/api.ts` 删 `bindWeChat`、`startTelegramBind`、`getSelfOAuthBindings`、`unbindCustomOAuth`；`users/api.ts` 删自定义绑定那对管理员接口；用户绑定弹窗改走 `getUser` + `adminClearUserBinding`，只列内置列；`useUserDisplay` 的副标题只留邮箱和用户名 | 解绑接口和 provider 列表都删了；上游让绑定页回来时会请求已删的 `/api/user/oauth/bindings` | 86536ce |
| `web/src/features/system-settings/auth/{index.tsx,section-registry.tsx}`、`web/src/features/system-settings/types.ts` | `defaultAuthSettings` 和 `AuthSettings` 删 25 个 OAuth 键；分区注册表删 `oauth`、`custom-oauth` 两个分区，只剩基础登录、机器人防护、Passkey、邮箱等 | 后台不再有这些开关，写了也不进 `OptionMap`（见上面 `model/option.go` 那条）；合上游时这两个分区会回来，要连着删 | 86536ce |
| `web/src/assets/brand-icons/index.ts` | 删 `IconGithub`、`IconDiscord`、`IconLinuxDo`、`IconWeChat` 的导出（对应的图标文件同批删除，`IconTelegram` 保留） | 上游会把这些导出加回来，但文件已删，会直接构建失败 | 86536ce |
| `web/src/routeTree.gen.ts` | `bun run build` 重新生成，去掉 `/(auth)/oauth` 和 `/oauth/$provider` | 两个路由文件删了（约定 B）；上游重新生成时会带回 `/oauth/:provider` 的条目 | 86536ce |
| `web/src/features/auth/api.test.ts`、`web/src/features/auth/secure-verification/__tests__/{verification-api.test.ts,verification-flow.test.tsx}`、`web/src/features/auth/sign-up/__tests__/username-rules.test.tsx`、`web/src/features/security/__tests__/{enrollment.test.tsx,page.test.tsx}` | 删 Telegram、OIDC 登录、`useOAuthLogin`、`consumeOAuthLoginRedirect` 和 `oauth` 二次验证的用例，其余夹具去掉 provider 字段 | 被测的入口删了；上游这些用例会回来，它们引用已删的 `hooks/use-oauth-login` 和 `lib/oauth*` | 86536ce |
| `web/public/`（22 个品牌资产：删 `adty-logo.svg`、`favicon.svg`、`safari-pinned-tab.svg`，新增 `adty-logo.png`、`adty-logo-480.png`、`adty-logo-960.png`、`adty-logo-dark.png`、`adty-mark.png`、`logo-dark.png`、`favicon-48x48.png`、`favicon-128x128.png`、`favicon-256x256.png` 等）、`web/index.html`、`web/package.json`、`web/bun.lock`、`web/src/features/adty-home/`、`web/src/styles/{theme.css,index.css}`、`web/src/lib/{chart-theme.ts,colors.ts,square-icon.ts}`，以及 `web/src` 其余改样式的文件（这个提交共动 301 个 `web/src` 文件） | 一行汇总，不逐条登记：① 品牌资产从 SVG 换 PNG（`web/public/`，`web/index.html` 改图标 link，去掉 mask-icon 和 SVG favicon）；② 首页区块重组，`adty-home/index.tsx` 从 Hero、PaymentStrip、PopularModels、Steps、ClientGuides、SellingPoints、Faq 改成 Hero、MigrationSection、EndpointsSection、AffiliateBanner、ClosingCta 五段（旧组件文件还在，但不再渲染）；③ 图表五级色从灰阶换成品牌主色阶（`theme.css` 的 `--chart-1..3` 是 `#0070ec`、`#4fa3ff`、`#8fc2ff`，`--chart-4/5` 仍灰，暗色另有取值），新增 `web/src/lib/chart-theme.ts` 统一取色，`colors.ts` 的装饰色按深浅落到 brand；④ 字体加 `@fontsource-variable/plus-jakarta-sans`，`--font-ui-sans` 改成 Plus Jakarta Sans 优先，卡片不再 hover 上浮；⑤ 其余大量组件（`components/ui`、`features/system-settings`、`features/channels`、`features/dashboard` 等）跟着换成 `--brand*`、`--ink*`、`--line*` 变量 | 301 个文件不能逐条写，这一行只保证「改过」有案可查；合上游时按目录逐个 `git diff 2df2e4c -- <目录>` 对照，别把已经下线的旧首页组件挂回 `adty-home/index.tsx`，也别把 `theme.css` 的灰阶图表色和 `#0a0a0a` 前景色合回来 | 2df2e4c |
| `.github/workflows/adty-ghcr.yml` | 后端作业超时改为 45 分钟；安装备份测试依赖；正式翻译检查说明改为简体中文和英文 | PostgreSQL 全量集成测试超过默认单包 10 分钟，备份检查需要 Python 和 libpq；语言说明与实际可加载语言一致 | Actions 43766575490 |
| `.github/workflows/ci.yml` | 后端作业超时改为 45 分钟；前端显式使用 `web/.node-version` | PR 和 push 使用同一套测试时间预算及 Node 版本 | Actions 43766575490 |
| `go.mod` | Go 补丁版本及 Heroku 提示升级为 1.26.11 | 修复漏洞扫描发现的可达标准库漏洞 | Actions 43766575490 |
| `Dockerfile` | Go 构建阶段升级为 1.26.11，并固定对应镜像 digest | 发布二进制与 CI 使用同一已修补 Go 版本 | Actions 43766575490 |
| `Dockerfile.dev` | Go 构建阶段升级为 1.26.11，并固定对应镜像 digest | 开发镜像同样使用已修补 Go 版本 | Actions 43766575490 |
| `makefile` | 三个 Go 模块的测试使用可覆盖的 `GO_TEST_TIMEOUT`，默认 30 分钟 | 真实 PostgreSQL 集成测试超过 Go 默认单包 10 分钟；保留全部测试 | Actions 43766575490 |
| `common/adty_deploy_constraints_test.go` | 插件验证顺序断言查找拆分后的前端作业名 | CI 已拆分前后端，旧作业名让静态检查误报 | Actions 43766575490 |
| `controller/adty_action_proof_test.go` | Root 证明夹具使用现有的 `root_exempt` 方法 | 与现行 Root 证明策略一致，保留权限重查及删除凭证断言 | Actions 43766575490 |
| `controller/adty_email_enum_test.go` | 密码重置提交结果测试补齐审计表和可触发发布失败的登录方式 | 用例必须进入待验证的查询失败分支，保留提交和失败计数检查 | Actions 43766575490 |
| `controller/adty_hosted_checkout_test.go` | 退役支付下单与公开回调断言拒绝且不入账；历史订单直接准备夹具，并继续验证 SDK 签名和冻结事实 | 已关闭支付方式不能为了旧用例重新开放；保留金额、身份、重放和零入账约束 | Actions 43766575490 |
| `controller/billing_option_test.go` | 预扣策略夹具补齐资金批次、分摊和挂账表 | 当前结算路径会访问这些表，不能因夹具缺表误判策略失败 | Actions 43766575490 |
| `controller/plugin_native_e2e_test.go` | 原生插件结算夹具补齐资金批次、分摊和挂账表 | 实际扣费路径需要完整资金模型 | Actions 43766575490 |
| `controller/relay_task_plugin_test.go` | 即时任务结算夹具补齐资金操作、批次、分摊和挂账表 | 保留真实结算及幂等断言，避免旧夹具缺表 | Actions 43766575490 |
| `controller/responses_websocket_test.go` | WebSocket 夹具补齐密钥限额、资金批次、分摊和挂账表 | 每次请求重查授权、限流及结算需要这些现有表 | Actions 43766575490 |
| `controller/security_account_test.go` | MFA 变更夹具同步增加认证版本，并确认旧会话失效 | 模拟正式因子注册的版本轮换，保留撤权、审计和删除拒绝断言 | Actions 43766575490 |
| `controller/security_enrollment_test.go` | Passkey 流程使用普通用户允许的操作；Root 渠道密钥证明使用既有 `root_exempt`，撤销 Root 后拒绝消费 | 跟随现行验证协议，保留范围绑定、重放拒绝、权限重查和日志脱敏检查 | Actions 43766575490 |
| `controller/user_manage_test.go` | Root 权限证明改为 `root_exempt`；补齐审计及资金相关夹具表 | 与现有权限策略及同事务记录路径一致 | Actions 43766575490 |
| `middleware/distributor_test.go` | 传统中文偏好按正式简体中文回退断言 | 产品只开放简体中文和英文，保留语言包不代表可加载 | Actions 43766575490 |
| `middleware/task_plugin_origin_task_test.go` | 来源任务渠道固定夹具补齐模型上架表 | 渠道选择现在需要模型可用性数据，保留固定渠道优先级断言 | Actions 43766575490 |
| `relay/mjproxy_handler_test.go` | 补齐资金相关表，并为 Midjourney 计费显式设置模型价格 | 默认价格目录不再自带该测试模型，不能依赖隐式价格 | Actions 43766575490 |
| `relay/relay_task_test.go` | 任务退款夹具补齐资金批次、分摊和挂账表 | 退款与恢复走现有资金路径，原夹具不完整 | Actions 43766575490 |
| `relay/websocket_settle_test.go` | WebSocket 结算恢复夹具补齐资金批次、分摊和挂账表 | 保留结算失败待恢复及重试后的资金断言 | Actions 43766575490 |
| `scripts/adty/check.sh` | 后端测试超时与 make 对齐为默认 30 分钟；语言检查提示改为两种正式语言 | 本地与 CI 使用相同测试预算和语言边界 | Actions 43766575490 |
| `service/adty_site_earning_test.go` | 风险退款断言按既有分站收益规则计算，并确认不写主站返佣账户 | 客户邀请佣金已独立，不能继续期望旧的重复主站返佣 | Actions 43766575490 |
| `service/adty_site_finance_test.go` | SQLite 员工操作用例使用独立审计库文件 | 独立审计连接不能争用业务事务写锁；保留并发撤权与回滚断言 | Actions 43766575490 |
| `service/adty_site_audit_visibility_test.go` | 更新夹具注释，说明同文件 SQLite 自锁是保留的故障注入场景 | 通用返佣夹具已分库；专用用例仍须覆盖审计自锁与缺表时的可定位错误 | Actions 43766575490 |
| `service/adty_site_ownership_test.go` | SQLite 归属并发夹具使用独立审计库文件，并在清理时恢复连接 | 避免业务事务锁阻塞独立审计写入，保留所有权串行化与失败回滚断言 | Actions 43766575490 |
| `service/adty_team_pay_test.go` | 废弃 Stripe 下单断言不可用、无 checkout 且无订单写入 | 跟随现行关闭策略，不能重新启用退役支付入口 | Actions 43766575490 |
| `service/adty_wallet_billing_test.go` | 固定音频价格同时初始化精确金额与分组配置 | 实际计费以精确价格及请求捕获的分组为准，保留原金额断言 | Actions 43766575490 |
| `service/openrouter_cache_create_tokens_test.go` | 缓存计费夹具使用当前默认模型 `claude-sonnet-4-6` | 保留成本反推语义，避免依赖已退出默认目录的模型 | Actions 43766575490 |
| `service/referral_commission.go` | 延迟计佣始终读取已完成退款比例；连续退款按原面额和比例快照计算累计应留佣金，用计佣、扣回及追偿账本只补扣差额；同步缩减已追回现金的可分配金额 | 部分人工退款后订单仍为 success，不能全额多发佣金；后续退款不能重复套累计比例、漏扣或重新提现已追回现金 | Actions 43766575490 |
| `service/referral_test.go` | 锁序夹具补齐资金表，SQLite 共用夹具分离业务与审计库文件；零金额退款断言拒绝且佣金不变；补实际完成退款后延迟计佣、连续部分退款、重放、结算及冻结/已付/混合提现到全额退款回归 | 独立审计写入不能争用业务事务的 SQLite 写锁；PostgreSQL 验证累计佣金、差额追偿、提现分配及无关佣金隔离，保留锁序、幂等及资金断言 | Actions 43766575490 |
| `web/scripts/adty/browser-smoke.mjs` | 生产冒烟检查当前首页标题和注册链接，直接启动 Rsbuild 预览进程 | 首页已改版；退出时应能终止实际预览进程并关闭输出流 | Actions 43766575490 |
| `web/AGENTS.md` | 翻译要求改为实际可加载的中英语言包，说明语义英文显示值与中文回退规则 | 避免继续要求未加载的六份旧语言包，与运行时和 CI 门禁一致 | Actions 43766575490 |
| `web/scripts/adty/i18n-check.mjs` | 检查中英键并集、非空文本和中文回退，允许语义化英文显示值；去掉首尾空白后同时拒绝英文显示值和原始键 | 两种正式语言与运行时一致；英文显示值不必等于内部语义键，中文不能借原始键或空白绕过检查 | Actions 43766575490 |
| `web/src/components/layout/components/__tests__/top-nav-identity.test.tsx` | 焦点变更等待 React 提交后再检查键盘菜单 | 保留身份菜单及焦点行为断言，避免异步竞态 | Actions 43766575490 |
| `web/src/features/admin-referral/__tests__/admin-referral.test.tsx` | 等待真实状态徽标渲染并限定重复空状态的查询范围 | 避免请求尚未完成或其他区域文字造成误判 | Actions 43766575490 |
| `web/src/features/adty-notifications/components/notification-editor.tsx` | 发布按钮文案使用普通分支计算 | 消除新增嵌套三元 lint 错误，保留原文案和行为 | Actions 43766575490 |
| `web/src/features/adty-notifications/components/notification-manager.tsx` | 通知状态徽标使用明确分支 | 消除新增嵌套三元 lint 错误，复用既有状态徽标 | Actions 43766575490 |
| `web/src/features/adty-site-owner/__tests__/query-errors.test.tsx` | 错误断言与当前正式语言文案一致 | 跟随语言归一化，保留请求失败显示检查 | Actions 43766575490 |
| `web/src/features/adty-site-owner/components/stat-grid.tsx` | 尾部占位格使用固定 key | 消除数组下标 key 的新增 lint 错误，布局不变 | Actions 43766575490 |
| `web/src/features/adty-support/__tests__/tawk-widget.test.tsx` | 等待客服脚本注入，并按当前语言回退断言 | 异步配置完成后才可检查注入结果 | Actions 43766575490 |
| `web/src/features/channels/components/__tests__/priority-order.test.tsx` | 等待渠道行加载后再执行排序断言 | 保留排序契约，避免空表阶段提前断言 | Actions 43766575490 |
| `web/src/features/security/__tests__/account-security.test.tsx` | 删除账号夹具按现有流程提供独立邮箱确认码和显式验证方法 | 保留单次删除证明、取消及迟到响应保护 | Actions 43766575490 |
| `web/src/features/system-settings/components/settings-page-context.tsx` | 共享 context 与 hooks 移至独立 hook 模块，只导出组件 | 满足 React 热更新 lint 规则，组件仍共享同一个 context | Actions 43766575490 |
| `web/src/features/system-settings/components/settings-section.tsx` | hooks 从新模块导入 | 与 context 拆分保持一致 | Actions 43766575490 |
| `web/src/features/system-settings/hooks/use-settings-page-context.ts` | 承载原设置页 context 和两个 hooks | 组件文件只导出组件，保留默认值和公开行为 | Actions 43766575490 |
| `web/src/features/system-settings/maintenance/__tests__/data-retention.test.tsx` | 等待保留期数据加载后再验证 | 保留清理边界断言，避免初始默认值竞态 | Actions 43766575490 |
| `web/src/features/task-plugins/__tests__/marketplace-install-dialog.test.tsx` | 测试结束清理全局 toast，等待新安装结果可见 | 防止前一个用例通知堆积影响当前安装断言 | Actions 43766575490 |
| `web/src/features/usage-logs/components/__tests__/mobile-filter.test.tsx` | 适配正式语言归一化及当前筛选文案 | 保留移动筛选行为检查 | Actions 43766575490 |
| `web/src/features/users/components/__tests__/username-rules.test.tsx` | 密码夹具满足当前最小长度 | 用例继续验证用户名规则，不在无关密码校验处停止 | Actions 43766575490 |
| `web/src/i18n/__tests__/translation-check.test.ts` | 子进程验证正式语言、语义英文、缺失中英翻译、原始键/带空白英文回退及技术名词空译文拒绝 | 防止翻译检查缩小语言范围或允许字面量时漏掉实际错误 | Actions 43766575490 |
| `i18n/i18n.go` | 只加载 `zh-CN.yaml`、`en.yaml`；删除 `LangZhTW`、`LangFr`、`LangJa`、`LangRu`、`LangVi`、`LangKo`、`LoadableLanguages`、`TranslateLoadable`、`canonicalLoadableLang`、`loadableLocalizer` | 六种非正式语言的 yaml 已删除，这些符号只为读取它们而存在 | 语言清理 2026-11-16 |
| `i18n/i18n_test.go` | `TestLocalesRenderEveryKey` 只检查两种正式语言，用 `localizeOfficial` 判断每个键能否在本语言包里渲染 | `TranslateLoadable` 已删除；`Translate` 会回退到另一种正式语言，不能发现单个语言包的渲染错误 | 语言清理 2026-11-16 |
| `AGENTS.md`、`web/AGENTS.md` | 把六种语言包“留在仓库里”的说法改为已删除并登记在删除清单 | 规范要和仓库实际内容一致 | 语言清理 2026-11-16 |
| `web/src/i18n/languages.test.ts`、`web/src/features/usage-logs/audit/__tests__/viewer.test.tsx`、`web/src/features/auth/secure-verification/__tests__/verification-flow.test.tsx` | 不再导入六种语言包；审计记录和邮箱验证码标签用例只保留简体中文和英文 | 被导入的语言包已删除 | 语言清理 2026-11-16 |
| `web/src/features/channels/components/__tests__/model-mapping-batch-dialog.test.tsx`、`web/src/features/models/__tests__/model-listing.test.tsx` | 换行和截断用例改用简体中文包或英文原文，不再借用法语包 | 法语包已删除；断言的是样式类，与语言无关 | 语言清理 2026-11-16 |
| `web/package.json`、`web/bun.lock` | 移除 `ai`、`auto-skeleton-react`、`input-otp`、`nanoid`、`react-resizable-panels`、`sse.js`、`stream-markdown-parser`、`tokenlens`、`use-stick-to-bottom`、`@xyflow/react`、`embla-carousel-react` | 引用它们的文件已全部删除（见 deleted-features.md） | 前端死代码清理 2026-11-16 |
| `web/src/styles/index.css` | 删除 `.auto-skeleton-fade` 规则 | 只给 `auto-skeleton-react` 用，组件和依赖已删除 | 前端死代码清理 2026-11-16 |
| `THIRD-PARTY-LICENSES.md` | 去掉上面 11 个依赖的行 | 清单只列 `web/package.json` 的直接依赖 | 前端死代码清理 2026-11-16 |
| `web/AGENTS.md` | 去掉关于 `static-keys.ts` 的说明，更新日志加一条 | 文件已删除 | 前端死代码清理 2026-11-16 |
| `router/api-router.go` | 删除 `POST /api/oauth/state`、`GET /api/oauth/:provider`、三条 Telegram 旧路由和 `oauth` 包的空导入 | OAuth 框架整体删除，只留邮箱绑定 | 第三方登录框架删除 2026-11-16 |
| `middleware/access_token_routes.go` | 删除上面三条路由的访问令牌规则 | 路由已删除 | 第三方登录框架删除 2026-11-16 |
| `controller/secure_verification.go` | 删除 Telegram OAuth、外部身份已被认领、OAuth 账号不匹配的错误码分支 | 这些错误只会由已删的 OAuth 流程产生 | 第三方登录框架删除 2026-11-16 |
| `service/security_verification.go` | 删除验证方法 `oauth`、范围 `account.binding.unbind`、`VerificationOAuthProvider`、`oauth_providers`；`account.binding.bind` 只接受 email | 第三方账号不再能当二次验证方法 | 第三方登录框架删除 2026-11-16 |
| `service/access_token_scope.go` | 删除 `account.binding.unbind` 的范围映射 | 范围已删除 | 第三方登录框架删除 2026-11-16 |
| `service/adty_site_resolve.go` | 删除只给 OAuth 回调用的 `AdtyOAuthSiteTermsContextKey`，条款版本只读查询参数 | OAuth 回调已删除 | 第三方登录框架删除 2026-11-16 |
| `middleware/adty_site_context.go` | 只改注释：`/api/oauth` 下只剩邮箱绑定 | 前缀规则不变 | 第三方登录框架删除 2026-11-16 |
| `controller/auth_flow_test.go`、`controller/security_account_test.go`、`controller/security_enrollment_test.go`、`controller/adty_username_test.go`、`controller/telegram_test.go`、`service/security_verification_email_code_test.go` | 删除只测 OAuth 登录、绑定、二次验证的用例和测试 provider；方法清单断言去掉 `oauth`，无可用因素时期望方法不在清单里 | 被测功能已删除；其余安全断言保留 | 第三方登录框架删除 2026-11-16 |
| `router/adty_deleted_routes_test.go`、`router/access_token_scope_test.go`、`router/adty_site_context_test.go` | 被删路由加入不得复活清单；免检名单去掉 Telegram 旧路由；站点前缀用例改用邮箱绑定路由 | 跟上路由变化，继续钉住前缀规则 | 第三方登录框架删除 2026-11-16 |
| `router/api-router.go` | 删除四家支付的下单和回调、签到、上游单通道易支付回调、旧管理员补单（原来都挂 `AdtyClosedFeature`），以及 `/topup/info`、`/topup/self`、`/amount`、`/option/payment_compliance`、`/option/waffo-pancake/*` | 功能删除，403 占位也不再需要 | 四家托管支付等删除 2026-11-16 |
| `controller/topup.go` | 删除上游单通道易支付下单、回调、充值信息、用户充值记录、`/amount`、管理员补单和 `paymentError`；只留订单锁和 `GetAllTopUps` | 入口早已关闭，函数没有调用方 | 四家托管支付等删除 2026-11-16 |
| `model/topup.go` | 删除四家支付和上游单通道易支付的入账、退款、管理员补单、用户充值记录查询，以及四家的 provider/method 常量和只给它们用的辅助函数 | 没有调用方；项目未上线，没有这些支付商的历史订单 | 四家托管支付等删除 2026-11-16 |
| `model/option.go` | 删除 `Stripe*`/`Creem*`/`Waffo*` 选项的注册与读写分支、Pancake 环境校验，以及拒绝写入这些键和 `PayMethods` 托管类型的保护 | 设置已删除 | 四家托管支付等删除 2026-11-16 |
| `model/checkin.go` | 只留 `Checkin` 表结构和 `CheckinRecord`，删除签到、统计和查询函数 | 签到删除；表继续迁移，保留历史行 | 四家托管支付等删除 2026-11-16 |
| `controller/misc.go` | `/api/status` 去掉 `stripe_unit_price` 和 `checkin_enabled` | 功能删除 | 四家托管支付等删除 2026-11-16 |
| `controller/option.go` | 删除支付合规字段的写入保护；`adty_marketing_setting.*` 整组拒绝写入 | 合规确认和优惠码开关删除 | 四家托管支付等删除 2026-11-16 |
| `setting/operation_setting/payment_setting.go` | 删除支付合规确认字段、条款版本和 `IsPaymentComplianceConfirmed` | 合规确认删除 | 四家托管支付等删除 2026-11-16 |
| `middleware/access_token_routes.go`、`middleware/audit.go`、`middleware/testdata/adty_admin_routes.txt` | 删掉被删路由的访问令牌规则、审计登记和后台路由分类 | 跟上路由删除 | 四家托管支付等删除 2026-11-16 |
| `go.mod`、`go.sum`、`THIRD-PARTY-LICENSES.md` | `go mod tidy` 去掉四家支付 SDK、`randstr` 和第三方登录留下的 `go-oidc`、`oauth2`、`moego` | 依赖没有使用方 | 四家托管支付等删除 2026-11-16 |
| `service/referral_withdraw.go` | 只改注释：不再提已删的 `creditTopUpQuotaWithPrincipalBatch` | 函数已删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/components/react-icon-by-name.tsx` | 改成固定的四个支付图标（支付宝、微信、USDT、通用卡片），不再按名字动态加载 react-icons 图标包 | 图标选择器只保留这几种 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/system-settings/integrations/payment-settings-section.tsx`、`payment-method-dialog.tsx`、`payment-methods-visual-editor.tsx` | 删掉 Stripe、Creem、Waffo、Waffo Pancake 的表单、标签和类型；图标输入改成下拉选择 | 四家支付删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/system-settings/billing/index.tsx`、`billing/section-registry.tsx`、`types.ts`、`api.ts` | 删掉四家支付、支付合规和优惠码开关的默认值、类型、接口和“优惠码”分区 | 功能删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/wallet/index.tsx`、`wallet/types.ts` | 订单恢复只认 USDT 和易支付；类型文件只留钱包余额相关的两个类型 | 托管支付和旧版上游钱包接口删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/errors/maintenance-error.tsx` | 去掉指向 `/status` 的按钮 | 公开状态页删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/usage-logs/lib/format.ts`、`web/src/features/auth/secure-verification/types.ts` | 删掉 `option.payment_compliance` 审计动作名和站长二次验证动作 `promo` | 功能删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/i18n/locales/en.json`、`zh.json` | 新增支付方式类型说明、图标选择器的文案 | 图标改成下拉选择；旧键按约定 B 保留 | 四家托管支付等删除 2026-11-16 |
| 测试：`controller/admin_finance_test.go`、`model/payment_method_guard_test.go`、`model/payment_orphan_resolution_test.go`、`model/topup_money_exact_test.go`、`model/user_pagination_test.go`、`router/access_token_scope_test.go`、`service/bepusdt_test.go`、`web/src/features/security/__tests__/page.test.tsx`、`web/src/features/system-settings/integrations/__tests__/payment-provider.test.tsx` | 删掉只测已删代码的用例；拿四家当“某个支付商”的用例改用 BEpusdt 或易支付；从 Pancake 测试文件里挪出无关的 `TestAdminUserListHidesSameOrHigherRole` | 跟上实现删除 | 四家托管支付等删除 2026-11-16 |
| `web/src/features/wallet/index.tsx`、`web/src/i18n/locales/en.json`、`zh.json` | 发起支付时带上客户选的易支付通道；新增通道显示名称、换支付方式的文案 | 钱包按易支付通道平铺，失败让客户自己换 | 易支付通道平铺 2026-11-16 |
| `controller/admin_finance.go` | 充值对账和站长订单列表多返回 `method_label`（下单时的易支付显示名称） | 多个易支付商户时对账要分清 | 易支付通道平铺 2026-11-16 |
| `setting/operation_setting/payment_setting_old.go` | 删除 `PayAddress`、`EpayId`、`EpayKey`、`PayMethods` 和 `UpdatePayMethodsByJsonString`、`PayMethods2JsonString`、`ContainsPayMethod`；只留回调地址、单价、最低充值、汇率 | 只剩一次性迁移在读，迁移已删除 | 旧单通道易支付设置删除 2026-11-16 |
| `model/option.go` | 删除 `PayAddress`、`EpayId`、`EpayKey`、`PayMethods` 的注册、校验和读写分支 | 设置已删除 | 旧单通道易支付设置删除 2026-11-16 |
| `main.go` | 启动时不再调用 `MigrateLegacyAdtyEpayChannel` | 迁移已删除 | 旧单通道易支付设置删除 2026-11-16 |
| `web/src/features/system-settings/integrations/payment-settings-section.tsx`、`billing/index.tsx`、`billing/section-registry.tsx`、`types.ts`、`integrations/__tests__/payment-provider.test.tsx` | 删掉旧易支付网关地址、商户号、密钥和「支付方式」字段；测试改用回调地址 | 旧设置删除 | 旧单通道易支付设置删除 2026-11-16 |
| `web/package.json`、`web/bun.lock`、`THIRD-PARTY-LICENSES.md` | 删除 `react-icons` | 不再有使用方 | 旧单通道易支付设置删除 2026-11-16 |
| `web/src/i18n/locales/en.json`、`zh.json` | 把放在 `translation` 外面、前端从不加载的 37 个词条挪回去（其中 18 个原来缺失），整份按两格缩进重排；新增通知状态专用的 `Withdrawn notification` | 这些文字在中文界面一直显示英文；通知的「Withdrawn」和推广提现共用一个键，中文显示成「已提现」 | 旧单通道易支付设置删除 2026-11-16 |
| `web/src/features/adty-notifications/components/notification-manager.tsx`、`__tests__/publishing.test.tsx` | 已撤回状态改用 `Withdrawn notification` 键 | 和推广提现的「已提现」分开 | 旧单通道易支付设置删除 2026-11-16 |
| `web/src/features/recharge-audit/__tests__/provider-actions.test.tsx` | 假数据里的 Stripe、Creem、Waffo 换成 BEpusdt 和易支付 | 四家托管支付已删除 | 旧单通道易支付设置删除 2026-11-16 |
| `web/src/features/wallet/index.tsx` | 发起支付时不再带优惠码 | 优惠码删除 | 下单流程优惠码删除 2026-11-16 |
| `AGENTS.md` | 优惠码一条改成「已删除，升级库旧表不读不写」 | 跟上删除 | 下单流程优惠码删除 2026-11-16 |
