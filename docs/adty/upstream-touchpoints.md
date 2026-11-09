# 改过的上游文件

每改一个上游文件（包括上游的测试文件），在下表加一行：文件、改了什么、为什么（约定 B）。同一个文件在几个任务里改过，就记几行。合并上游时，冲突对照这张表解决。

整个删掉的上游文件和目录不记在这里，记进 [deleted-paths.txt](deleted-paths.txt) 和 [deleted-features.md](deleted-features.md)；在上游文件里删掉一部分代码算改动，记在这里。

各列写法：

- 上游文件：从仓库根目录写起的路径。
- 改了什么：改在哪里、改了什么。位置尽量写函数名，行号会随上游变。
- 为什么：这样改的原因。
- 任务：实施方案里的任务号。

| 上游文件 | 改了什么 | 为什么 | 任务 |
| `router/api-router.go` | 上游单通道易支付下单和回调、Stripe/Creem/Waffo 下单和回调、签到、旧管理员补单改成 `middleware.AdtyClosedFeature`，在登录中间件之前拒绝 | 这些入口能绕过统一订单、加赠、佣金和余额流水入账；规范明确不做签到和未配置的卡支付 | 审查 v3 |
| `router/relay-router.go` | `/pg` 在 `Distribute` 之前挂 `AdtyUserLimit` | 演练场是可用的文本入口，必须和 `/v1` 共用按用户的 RPM/TPM/并发限制 | 审查 v3 |
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
| `model/quota_reserve.go` | 增加 `userQuotaDeltaFloorScript` 和 `cacheApplyManualQuotaDelta` | 人工调额写缓存前拒绝会把活余额减成负数的增量 | 审查 v3 |
| `model/user_quota_adjustment.go` | `AdjustUserQuota` 在写数据库前用活缓存做下限；缓存不可用时仍提交数据库变更 | 批量更新下缓存才是可花余额；缓存故障不能回滚已经决定的调额 | 审查 v3 |
| `router/relay-router.go` | `relayV1Router`、`/pg`、`/v1beta` 和 `/mj` 增加 `AdtyRequireCurrentSiteTerms` | 分站条款未接受时不能继续调用中转或演练场 | 审查 v3 |
| `router/api-router.go` | 令牌、日志、用量、任务和审计的自助路由在登录后增加 `AdtyRequireCurrentSiteTerms` | 条款升级后，未确认的分站客户不能继续管密钥或读自己的用量 | 审查 v3 |
| `router/adty-router.go` | 钱包充值、USDT 下单和易支付下单在登录后增加 `AdtyRequireCurrentSiteTerms` | 条款升级后，未确认的分站客户不能继续报价或下单 | 审查 v3 |
| `controller/midjourney.go` | 转发的 Midjourney 图片地址改成带过期签名的 `/mj/image/:id` | 只知道任务编号不能取到别人的图 | 审查 v3 |
| `relay/mjproxy_handler.go` | `RelayMidjourneyImage` 先验签和过期，再查任务和向上游取图；成功响应加 `Cache-Control: private, no-store` | 猜测任务编号不能取到图像；签名过期后也不能取 | 审查 v3 |
| `relay/mjproxy_handler_test.go` | 图片代理用例带上签名；另加未签名和过期签名不得取图 | 证明修复前未签名请求能拿到图，修复后不能 | 审查 v3 |
| `relay/channel/task/jsplugin/adaptor.go` | JSON 提交已经收到上游响应后，读体失败和 `parseSubmitResponse` 失败都设 `NoRetry` | 解析失败再换渠道会把已经受理的任务再提交一次 | 审查 v3 |
| `relay/relay_task.go` | 视频查询找不到适配器时不再把渠道编号写进错误信息 | 普通用户的错误响应不能带内部渠道编号 | 审查 v3 |
| `controller/option.go` | 通用设置接口拒绝 `adty_marketing_setting.*` | 不能绕过营销设置页，在缺发件地址和密钥时打开营销 | 审查 v3 |
| `controller/option.go` | 通用设置接口拒绝整个 `adty_referral_setting.*` | 不能绕过推广设置页，在没有默认比例时打开返佣 | 审查 v3 |
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
| `docker-compose.yml` | 示例密码改为必填环境变量，端口只绑 127.0.0.1，数据库镜像改为 PostgreSQL 17 | 根目录 compose 不能用公开示例密码和全网卡端口启动；已有 16 数据目录须独立迁移 | 审查 v3 / v4 |
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
| `web/src/features/wallet/__tests__/copy.test.tsx` | `RechargeFormCard` 不再传兑换码的 4 个属性 | 属性删了 | 1.4 |
| `web/src/features/wallet/api.ts` | 删 `redeemTopupCode` 和它的类型导入 | `POST /api/user/topup` 删了 | 1.4 |
| `web/src/features/wallet/components/recharge-form-card.tsx` | `RechargeFormCard` 删兑换码一节（含购买链接、合规提示）、加载骨架里的兑换码占位、5 个属性和用不到的 `Gift`、`ExternalLink`、`IconBadge` 导入；未开在线充值的提示去掉「使用兑换码」，换成新键 | 兑换码删了；旧键留在语言包里（约定 E） | 1.4 |
| `web/src/features/wallet/hooks/index.ts` | 删 `use-redemption` 的导出 | 文件删了 | 1.4 |
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
| `web/src/features/wallet/api.ts` | 删 `getAffiliateCode`、`transferAffiliateQuota` 和它们的类型导入 | `GET /api/user/aff`、`POST /api/user/aff_transfer` 删了 | 1.5 |
| `web/src/features/wallet/hooks/index.ts` | 删 `use-affiliate` 的导出 | 文件删了 | 1.5 |
| `web/src/features/wallet/index.tsx` | `Wallet` 删邀请奖励卡片、划转弹窗、`useAffiliate`、`handleTransfer` 和弹窗状态 | 邀请奖励删了 | 1.5 |
| `web/src/features/wallet/lib/index.ts` | 删 `affiliate` 的导出 | 文件删了 | 1.5 |
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
| `web/src/features/keys/components/{api-keys-mutate-drawer,api-key-group-cell}.tsx`、`web/src/features/usage-logs/components/common-logs-filter-bar.tsx`、`web/src/features/pricing/components/{model-details,pricing-sidebar}.tsx`、`web/src/features/playground/hooks/use-playground-options.ts`、`web/src/features/users/components/{users-mutate-drawer,users-table}.tsx`、`web/src/features/channels/components/{channels-table,drawers/channel-mutate-drawer,dialogs/tag-batch-edit-dialog}.tsx`、`web/src/features/profile/components/profile-header.tsx` | 分组下拉、筛选、提示里的 label 从 ID 换成显示名（后台用「显示名 · ID」）；值仍是 ID | 前台一律显示显示名 | 2.5 |
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
| `web/public/{favicon.ico,logo.png,adty-logo.svg,adty-og.png,adty-apple-touch-icon.png}` | 换成新 Logo（方案 H），logo.png 和 adty-logo.svg 用 48px 以下的简化版；新增 favicon.svg、favicon-16x16.png、favicon-32x32.png、android-chrome-192x192.png、android-chrome-512x512.png、maskable-512x512.png、site.webmanifest、safari-pinned-tab.svg、mstile-150x150.png | 新 Logo | 品牌 |
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
