# 功能说明

给运营人员和以后接手的开发看：每个功能做什么、规则、后台在哪里设置或操作、默认开还是关、相关的表和接口、已知的限制。内容按代码写（main，2026-11-08），每节末尾是对应的实施方案任务号；代码和方案不一致的地方单独写出来。

几个通用的说明：

- **额度和美元**：余额沿用上游的额度整数，1 美元 = `QuotaPerUnit` 额度（默认 500000）。下文的算例都用美元写，括号里是额度。
- **后台设置**：我们的设置都存在 options 表里，键是「模块名.键」，例如 `adty_topup_setting.bonus_rates`。和钱有关的参数没有内置默认值，没设之前对应功能不开（约定 I）。
- **后台权限**：后台接口按「区域·动作」授权，见「分组和后台权限」一节。下文写「需要 财务·退款」之类，指这个权限。
- **密钥**：存进 options 的密钥，键名以 `Token`、`Key`、`Secret` 等结尾，`GET /api/option/` 不会返回它们；后台只能填新值，不显示旧值。
- **主节点**：定时任务（结算、补查、同步、关站执行等）只在主节点跑。

目录：充值和加赠 · USDT 和易支付多通道 · 充值退款 · 余额下限 · 推广返佣 · 站长分站 · 分组和后台权限 · 营销 · 网页客服、状态页、多语言、首页 SEO · 限流、脱敏和其他安全 · 官方价、接入文档、条款 · 品牌和 Logo。

## 充值和加赠

**做什么**：用户在钱包页（`/wallet`）选面额、选支付方式（USDT 或支付宝、微信）充值，按后台设的档位多送一部分额度，可以再用优惠码多送。

**规则**：

- **面额**：正整数美元，不低于最低充值额 `MinTopUp`（默认 1）。钱包总额不能超过 2^53 − 1 额度，下单时会检查。易支付还受通道的单笔和每日限额限制。
- **加赠档位**：`adty_topup_setting.bonus_rates`，JSON，键是整数美元，值是百分数，例如 `{"100": 2.5}` 表示 $100 起送 2.5%。保存时键必须是正整数，值在 0–100。**没有内置档位**（默认 `{}`，不送）。
- **取档**：取「不超过面额、比例大于 0」的最大一档，档和档之间不插值；比最小档还少就不送。自定义金额也按这条。比例填 0 的档会被跳过、下面一档照样生效，所以「填 0」做不到「某个金额以上不送」。
- 钱包页的金额按钮来自上游的 `payment_setting.amount_options`（默认 10、20、50、100、200、500），和加赠档位是两套配置。
- **公式**：
  - 加赠 = 面额 × 比例 ÷ 100（8 位小数）。
  - 入账额度 = (面额 + 加赠 + 优惠加赠) × QuotaPerUnit，向下取整。
  - USDT 应付 = 面额 × 分组充值倍率（8 位）；下单后改成网关返回的实际应付（可能多出几分钱，用来区分订单）。
  - 支付宝、微信应付 = 面额 × 人民币单价 `Price`（默认 7.3）× 分组充值倍率（2 位）。
  - 优惠码只多送额度，应付不变。返佣只按面额算，不含任何加赠。
- **优惠码**：类型是按比例（0 < 值 ≤ 100）或固定金额（0 < 值 ≤ $100000）；码由 3–32 位大写字母、数字、`_`、`-` 组成，不分大小写。报价时校验：启用、适用站点和用户所属站点一致、在有效期内、面额达到最低面额、总次数和每人次数没用完、「只限新用户」时这个人从没充值成功过。次数在入账事务里用条件更新扣，每笔订单只核销一次；订单过期或取消不占次数。入账时码已经不能用了（停用、用完、删掉），照常入账面额和档位加赠，只去掉优惠加赠（钱已经收到了，不拒绝入账）。
  - **平台出资**：后台「营销 → 优惠码」建，可以指定适用站点；不记站长收益。
  - **站长出资**：站长在站长中心建，只给本站客户用；站点档位加赠 + 优惠加赠不能超过 面额 ×（主站同档比例 + `adty_reseller_setting.max_site_bonus_extra`）÷ 100；入账时站长出的部分从站长收益里扣（见「站长分站」）。
- **分站客户**：按站点自己的档位（`adty_sites.bonus_rates`）算，不会回落到主站档位。

**算例**（档位 `{10: 1, 30: 1.5, 100: 2.5, 100000: 8}`）：

- $100 USDT：加赠 $2.5，入账 $102.5（51,250,000 额度），应付 100 USDT；用支付宝应付 ¥730.00，入账相同。
- 自定义 $150：落在 100 档，加赠 $3.75，入账 $153.75。
- $100 加 10% 平台优惠码：优惠加赠 $10，入账 $112.5（56,250,000），应付仍是 100。
- 分站上限：主站 100 档 2.5%、`max_site_bonus_extra` = 2，那么 $100 的分站订单，站点加赠加站长码合计不能超过 $4.5。

**后台**：

- 系统设置 → 计费 → 支付（只有超级管理员）：人民币单价 `Price`、最低充值 `MinTopUp`、金额按钮、加赠档位、回调地址 `CustomCallbackAddress`。
- 营销 → 优惠码（`/marketing`）：平台出资的码。
- 站长中心 → 加赠、优惠码（`/site-owner/bonus`、`/site-owner/promo-codes`）：站点档位和站长出资的码。

**默认**：没有档位，不送加赠。2026-11-01 起不再需要「支付合规确认」，支付默认就能用（见 deleted-features.md）。

**表和接口**：

- 表：`top_ups`（上游的充值记录，`Amount` 记美元面额、`Money` 记应付）、`adty_topup_orders`（面额、加赠、入账额度、优惠、站点、返佣快照）、`adty_promo_codes`、`adty_promo_redemptions`。
- 用户：`GET /api/adty/topup/info`、`POST /api/adty/topup/quote`、`GET /api/adty/topup/orders/:trade_no`、`POST /api/adty/topup/orders/:trade_no/cancel`、`GET /api/adty/topup/pending`、`GET /api/adty/topup/self`。
- 平台优惠码：`GET`、`POST /api/adty/admin/marketing/promo-codes`，`PUT /api/adty/admin/marketing/promo-codes/:id`。站长优惠码：`GET`、`POST /api/adty/site/promo-codes`，`PUT`、`DELETE /api/adty/site/promo-codes/:id`（写操作要重新验证身份）。

**限制**：

- 分站客户在钱包页看到的档位卡显示的是主站比例（`GET /api/adty/topup/info` 用主站档位），实际报价和入账按站点档位。待修。
- 站长删掉优惠码以后，已下单未付款的订单入账时会去掉这张码的优惠加赠，`controller/adty_promo.go` 的注释写的是「照样按快照入账」，注释和代码不一致，以代码为准。

任务 3.1、3.3、3.6、12.4。和方案不一致：方案 3.1 原来有 13 档内置加赠，已经按 3.6 删掉，全部在后台设置。

## USDT（BEpusdt）和易支付多通道

**USDT**：

- **链**：TRC20、BEP20、Polygon、X Layer（支付方式 `usdt.trc20`、`usdt.bep20`、`usdt.polygon`、`usdt.xlayer`）。只有勾在 `adty_bepusdt.trade_types` 里的链才显示、才能下单，默认只有 TRC20。
- **站内支付窗**：显示收款地址、应付数量和二维码，不跳出网站。
- **有效期**：`adty_topup_setting.usdt_order_minutes`，默认 15 分钟。下单时固定传 `fiat=USD`、`rate="1"`，应付数量以网关返回的为准。
- **回调** `POST /api/user/bepusdt/notify`：md5 验签；只有 `status=2` 入账；核对金额、实付 ≥ 应付、收款地址、网关订单号。金额不符、订单不存在、超出额度上限这类问题进「充值对账」的人工审核，回网关 ok；临时错误回 500 让网关重发。
- **换链**：先取消旧单（调网关的取消接口），网关已经检测到转账时不让取消，提示用户；取消成功后提示「不要再往旧地址转账」，再按新链下单。
- **设置**：系统设置 → 计费 → 支付网关 → USDT 页签（`?tab=usdt`）：`adty_bepusdt.enabled`、`.base_url`、`.secret`（只写）、`.trade_types`、`adty_topup_setting.usdt_order_minutes`。默认关，打开且填了地址和密钥才能用。旧地址 `/system-settings/billing/usdt` 会跳到这个页签。

**支付宝、微信（易支付多通道）**：

- **通道**：每个通道可以开支付宝、微信，每种方式可以设单笔最小、最大和每日限额（人民币，0 表示不限）。新建的通道默认停用，测试下单通过后手动启用；有订单的通道只能停用不能删；商户密钥不返回给前端。
- **挑通道**：先只留启用、支持这种方式、金额在单笔范围内、今天到账加这笔不超过每日限额的通道；在里面取优先级最高的一组；电脑端优先挑能返回二维码的；最后按权重随机。
- **自动切换**：一次下单最多试 5 个通道，单通道超时 `adty_epay_setting.channel_timeout_seconds`（默认 5 秒），总时长 `total_timeout_seconds`（默认 12 秒）。某个通道报错或超时，这笔单标失败、换下一个通道新建一单。用户也可以在支付窗点「换个通道」。
- **连续未付转维护**：订单过期或被用户取消，通道的连续未付数加一；达到通道自己的阈值或全局 `unpaid_threshold`（默认 10，0 表示不转）时转维护并通知超级管理员；任何一笔到账清零。
- **补查**：下单超过 `query_after_seconds`（默认 120 秒）还没回调、还没过期的订单，主节点每分钟去网关查一次（只对 https 网关），查到已付且方式、金额对得上就入账。
- **探活**：`probe_enabled` 默认关，间隔 `probe_interval_minutes`（默认 5 分钟）；失败转维护，探活转的维护连续 3 次正常就自动恢复，手动设的不会。
- **有效期**：`adty_topup_setting.epay_order_minutes`，默认 10 分钟。电脑端显示二维码，手机跳转支付。
- **回调**：每个通道独立回调 `/api/user/epay/notify/:channelId`（GET、POST 都收），用这个通道自己的密钥验签；停用或维护中的通道照样接收旧单回调。易支付没有取消接口，本地取消后用户仍然付了钱，照样入账。
- **设置**：系统设置 → 计费 → 支付网关 → Epay 页签（`?tab=epay`，上面这些参数和通道列表都在这里）。查看要 支付通道·查看，启停和测试要 支付通道·操作，新增、编辑、删除只有超级管理员。旧地址 `/payment-channels` 和 `/system-settings/billing/epay` 都会跳到这个页签。列表加载失败时显示错误并可以重试，不再显示成空列表。

**晚到的回调（两种都一样）**：订单是待支付、已过期、已失败、已取消，收到验签通过、金额一致的成功回调都照样入账，因为钱已经收到了。已经入账的直接返回成功，不会重复入账。已经退款的订单再收到回调，进人工审核，不再入账。

**充值对账**（`/recharge-audit`）：订单列表、汇总、对不上的回调（可以补入账或标已处理）、手动补单（必须填凭证和备注，不核对金额）、退款。

**表和接口**：`adty_topup_orders`、`top_ups`、`adty_epay_channels`、`adty_epay_channel_methods`、`adty_payment_orphan_events`。下单 `POST /api/user/bepusdt/pay`、`POST /api/adty/epay/pay`；回调 `POST /api/user/bepusdt/notify`、`GET`/`POST /api/user/epay/notify/:channelId`；通道 `GET /api/adty/admin/epay/channels`、`POST /api/adty/admin/epay/channels/:id/status`、`POST /api/adty/admin/epay/channels/:id/test`（新增、编辑、删除 `POST /api/adty/admin/epay/channels`、`PUT`/`DELETE /api/adty/admin/epay/channels/:id` 只有超级管理员）；对账 `GET /api/user/admin/finance/recharge-audit`、`GET /api/user/admin/finance/payment-orphans`，补单 `POST /api/adty/admin/topup/:trade_no/complete`；付完跳回 `GET /pay/result/:token`。

**限制**：补查入账失败只写日志，不进人工审核（回调失败会进）；订单过期以后不再补查，只能等回调或手动补单。

任务 3.2–3.5、4.1–4.4、5.1–5.5。

## 充值退款

**做什么**：管理员在后台给一笔已到账的充值订单退款。系统按用户现在的余额，扣回这笔订单还没花掉的额度，算出应退给用户多少钱。真正把钱退给用户在线下做，系统不调支付商的退款接口。

**规则**（2026-11-01 用户定）：只退没花掉的本金，赠金收回不退，余额不会变负。

设这笔订单：

- 入账额度 C = 本金 F + 赠金 G。F 是面额换算成额度（向下取整），G = C − F，包括档位加赠和优惠码加赠。
- 用户现在的余额是 B（锁住用户行后读取；开了 Redis 时以缓存里的余额为准，见下）。

那么：

- 扣回 R = min(B, C)；B ≤ 0 时 R = 0。余额最少扣到 0。
- 应退本金额度 P = max(0, R − G)。等于说用户花掉的额度先算成花本金，赠金全部收回。
- 应退金额 = 实付金额 × P ÷ F，按支付币种向下取到最小单位：人民币 2 位；USDT 按网关给的实付小数位（去掉末尾的 0 后数，至少 2 位）。
- P = 0 时应退 0，订单照样改成「已退款」。

**算例**：付 100 USDT，面额 $100、加赠 $10，到账 $110（F = $100，G = $10，C = $110）。

| 用户花了 | 退款时余额 B | 扣回 R | 应退本金 P | 应退 |
|---|---|---|---|---|
| $30 | $80 | $80 | $80 − $10 = $70 | 70 USDT |
| $105 | $5 | $5 | max(0, $5 − $10) = 0 | 0 |
| 没花 | $110 | $110 | $100 | 100 USDT |

用户名下有好几笔订单、余额比这笔的 C 还多时，只扣这笔的 C。

**佣金**（P6，推广返佣）：按退掉的本金比例 ratio = P ÷ F 处理。

- 佣金已经生成（待结算、冻结、可提现、已提现都算）：扣回 佣金 × ratio（8 位小数，向下取整）。
  - ratio = 1：整笔处理。待结算的作废，其余的扣回，和以前一样。
  - 0 < ratio < 1：佣金状态不变，记一笔部分扣回的返佣流水（`commission_clawback_partial`）。待结算的从待结算里扣，并把这笔佣金的金额减掉这部分；冻结、可提现、已提现的从可提现里扣，可以扣成负数，以后的新佣金结算时先抵扣。
  - ratio = 0（用户把本金花完了）：佣金不动，照样结算。
- 计佣任务还没跑（退款早于计佣）：计佣任务也处理已退款的订单，只按用户花掉的那部分计佣，基数 = 面额 × (1 − ratio)；一分没花（ratio = 1）时跳过。退款事务里不同步跑计佣，两边都先锁计佣任务那一行，不会重复计佣。
- 例：上面第一行（R = $80、P = $70），返佣比例 10%：佣金 $10，ratio = 0.7，扣回 $7，推广员留 $3。

**分站订单**（P11）：

- 站长出的加赠只按收回的比例还给站长：收回的赠金 = min(G, R)，还给站长 = 站长出资的加赠 × min(G, R) ÷ G。平台优惠码出的赠金收回了不还给任何人。
  - 例：赠金 $10 全部由站长出，用户花了 $105、余额 $5：R = $5，只收回 $5，还给站长 $5，不是 $10。
  - 例：站长加赠 $10 加平台码 $10（G = $20），余额 $10：收回 $10，还给站长 $10 × $10 ÷ $20 = $5。
- 站长的消费分成不扣：站长收益是按客户的消费记的，充值本身不产生站长收益；退款收回的是没花掉的余额，客户已经花掉的是真实消费。

**余额以缓存为准**：生产环境开着 Redis 和批量更新（`docker-compose.yml` 里 `BATCH_UPDATE_ENABLED=true`），消费先扣缓存，数据库默认 5 秒后才跟上。所以开了 Redis 时，R 由一个 Lua 脚本在缓存里按「活的」余额原子地算出并扣掉，数据库在同一个退款事务里扣同样的 R，批量刷新后两边一致；事务失败时把缓存还回去。Redis 关着或缓存不可用时，按数据库锁住的那一行算。还没刷进数据库的用户额度差额，要和这一行、缓存一起读。读差额时才持有用户额度批量队列锁，读完就放，不能把这把锁包住数据库事务：批量刷新先拿队列锁再写数据库，反过来会和正在锁用户行的调额、退款卡住。

**后台**：充值对账（`/recharge-audit`，需要 财务·查看）里选订单 → 退款，填原因（必填）。确认框里写着规则；提交成功后显示「已扣回 xx 额度，应退 xx USDT/元，请在线下退款」，P = 0 时另外说明没有可退的钱。同一个幂等键重复提交只退一次，返回第一次的结果。需要 财务·退款 权限。

**记录**：

- 余额流水（`adty_balance_ledgers`，类型 `refund`）记实际扣回的 −R，退款后余额不小于 0；原因后面带一段明细，例如 `[R=40000000 G=5000000 P=35000000 refund=70 USDT]`。
- 订单扩展表 `adty_topup_orders` 的 `refund_quota`（P）、`refund_amount`（应退金额）、`refunded_at`、`refund_reason`；上游的 `top_ups.status` 改成 `refunded`。
- 接口：`POST /api/adty/admin/topup/:trade_no/refund`，请求 `{reason, idempotency_key}`，返回 `deducted_quota`（R）、`bonus_quota`（G）、`refund_quota`（P）、`refund_amount`、`currency` 和流水。

**限制**：充值对账没有订单详情页，`refund_quota`、`refund_amount` 只在退款结果框里显示（接口和数据库里有）。只在 PostgreSQL 上测过。

任务 3.5；规则 2026-11-01 改（提交 c3fbbfc52、e701bf804、547ef68a3）。和方案不一致：方案 3.5 写的是「扣回面额和加赠（余额可以变负），作废或扣回佣金」，已按用户的新规则改掉。

## 余额下限（不透支）

**做什么**：余额不够的请求直接拒绝，余额永远不会变成负数。之前审查测出：余额 $10 的用户发一个大请求，结算后余额变成 −$215。

**规则**：

- **请求前**：每个钱包请求都先预扣估算的费用（上游原有的估算：提示词 tokens 加上 max_tokens，请求里设了才算；分站请求已经乘上站长倍率 k）。预扣是带条件的原子扣减（余额 ≥ 预扣额才扣），并发的请求不能一起通过。余额不够就返回上游原有的额度不足错误（HTTP 403，错误码 `insufficient_user_quota`），请求不发给上游。
  - 上游原来余额高于「信任额度」（`quota_setting.trust_quota_usd`，默认 $10）时跳过预扣，这条旁路对钱包请求已经关掉；设置项还在，但对钱包不再生效。
  - 图片张数、任务重试价、阶梯计费切换分组后的补预扣也一样：余额不够就 403。
- **结算时**：实际费用比预扣多，补扣的部分最多扣到 0；没扣到的差额由平台承担。余额到 0 以后，下一个请求在预扣时就被拒绝，所以每个用户最多少收一次请求的差额。实际费用比预扣少时，多扣的退回，和上游一样。
  - 覆盖所有扣费路径：文本结算、没有计费会话的旧路径、Realtime、违规扣费、Midjourney、异步任务的差额结算。
- **差额怎么记**：写进这次请求消费日志的 `other.admin_info.shortfall_quota`（只在 > 0 时写）。普通用户的日志投影去掉了 `admin_info`，只有管理员在后台日志里看得到。任务差额结算的差额写进任务日志的 `admin_info`。
- **站长收益按实扣算**：分站客户有差额的请求，站长按实际扣到的额度分成（日志额度 − 差额），差额不分给站长，也不从站长收益里扣。「客户付 = 平台收 + 站长赚」按实扣成立。
- **退款按实扣**：任务和 Midjourney 的 `quota` 记实际扣到的额度，以后任务失败退款不会退出平台没收到的钱。

**算例**：余额 $10，预扣 $10，实际费用 $225：结算补扣 $215，只扣得到 $0（余额已经是 0），差额 $215 由平台承担，记进 `shortfall_quota`；余额停在 0，下一个请求被拒。

**后台**：没有开关，一直生效。差额在后台「使用日志」的详情里看 `admin_info`。

**调额下限**（2026-11-06 选定）：

- 后台手动调额（`model/adty_quota_adjust.go`）：「减少」不能减到 0 以下，超出部分被拒绝，余额保持原值；「覆盖」不能写成负数。0 可以。这是 2026-11-06 选定的下限。下限按数据库余额加上还没刷进数据库的用户额度差额判断。差额在队列锁里读完就放，不能在队列刚刷库时把活余额调成负数，也不能把队列锁包住调额事务。
- 令牌额度（`remain_quota`）结算时仍按全额扣，照上游原样；它不影响钱包余额。

**相关代码**：`model.ChargeUserQuotaUpTo`（`model/quota_reserve.go`，开了 Redis 时用 Lua 脚本在缓存里扣，否则数据库锁行扣）、`service/adty_balance_floor.go`、`service/billing_session.go`。新写的扣费代码要用 `ChargeUserQuotaUpTo`，不要用 `model.DecreaseUserQuota`（生产代码里已经没有调用它的地方）。

2026-11-01 用户要求（提交 5a6cae6de、cc51f530e），方案里没有这一项。

## 推广返佣

**做什么**：用户分享推广链接，被邀请的人充值后，推广人按充值面额拿佣金，冻结一段时间后可以提现。只做一级。

**规则**：

- **谁有推广链接**：主站用户注册时自动成为推广员（8 位邀请码，直接是已通过）。分站客户不自动建推广员。
- **比例**：全局 `adty_referral_setting.default_rate`（百分数，10 表示 10%，没有默认值）；单个推广员可以单设（合作伙伴审批或后台调整时设，传空就回到全局）。下单时把推广员和比例快照进订单，之后改比例不影响已下的单。
- **佣金** = 面额 × 快照比例 ÷ 100（8 位小数），不含任何加赠。永久计佣，没有期限。
- **绑定**：打开 `/r/<邀请码>` 或任意页面带 `?ref=<邀请码>`，写一个签名 cookie `adty_ref`，有效期 `cookie_ttl_days`（默认 30 天），以最后一次点击为准；注册时绑定，绑定后不能改。拒绝自己邀请自己和互相邀请。部署时必须固定 `CRYPTO_SECRET` 或 `SESSION_SECRET`，否则重启后 cookie 全部失效。
- **冻结和结算**：入账时插一条计佣任务，主节点每 60 秒（环境变量 `REFERRAL_SETTLEMENT_INTERVAL_SECONDS`）跑一轮：先生成待结算佣金（`冻结到 = 现在 + freeze_days`，默认 7 天），再把到期的转成可提现。佣金最多晚一轮才出现。
- **提现**（`/referral` 页的提现抽屉）：
  - **转入余额**：随时、免费、免审，立即到账，额度 = 金额 × QuotaPerUnit（向下取整），写余额流水 `commission_transfer`。
  - **USDT**：先选链（TRC20、BEP20、Polygon）再填地址，按链校验地址；实到 = 金额 − `usdt_withdraw_fee`；要达到 `min_withdraw_usd`；人工审核，打款时填交易哈希。
  - **支付宝、微信收款码**：上传收款码和收款人姓名，免手续费，要达到 `min_withdraw_usd`；汇率 `withdraw_cny_rate` 由管理员实时改，提交时直接查库，和用户看到的不一样就让用户确认新汇率，一样就锁进提现单；人工审核。
  - 防并发多提：条件更新 `可提现 ≥ 金额`，加幂等键。提交后 `cancel_window_minutes`（默认 30 分钟）内待审核的单可以撤回。驳回要填原因，会解冻并发邮件。
- **防刷**：被邀请人注册时的 IP 或设备和推广人 `self_invite_window_days`（默认 30 天）内用过的相同，照样绑定但标成自邀、不计佣；同一个推广员 `batch_register_window_hours`（默认 24 小时）内同一个 IP 的绑定达到 `batch_register_threshold`（默认 3）个，停这个推广员的结算。IP 和 UA 只存哈希。
- **合作伙伴**：用户申请，后台通过时填单独的比例，驳回要填原因。
- **站长停止计佣**：用户开站（站点变成 active）后，他的推广链接照样能打开，但不再绑定、新订单不再带返佣快照；开站前的订单照常计佣。关站后不自动恢复，要后台「恢复邀请返佣」。站长的分站收益结算进同一个佣金账户，提现规则一样。分站客户充值不计任何人的佣金。
- **退款**：按退掉的本金比例扣回佣金，见「充值退款」。

**算例**：面额 $100（不管送了多少）× 10% = $10 佣金，7 天后转成可提现。USDT 提现 $150、手续费 2：实到 148 USDT。收款码提现 $50、汇率 7.1：锁定 ¥355.00。

**后台**：推广管理（`/admin-referral`）：提现审核、合作伙伴申请、推广员（调比例、停结算、停提现、恢复邀请返佣）、设置，以及「立即结算」按钮。用户侧：`/referral`（推广页和提现）、公开页 `/affiliate`、落地 `/r/$code`。

**默认**：总开关 `adty_referral_setting.enabled` 默认关；关着时点击、绑定、下单快照、提现全部不工作，站长的分站收益也提不了现。没有内置值的四项：`default_rate`（没设不能打开总开关）、`min_withdraw_usd`（没设不能提到 USDT 和收款码）、`usdt_withdraw_fee`（没设不能提到 USDT）、`withdraw_cny_rate`（没设不能提到收款码）。

**表和接口**：`referral_affiliates`、`referral_bindings`、`referral_clicks`、`referral_commission_accounts`、`referral_commissions`、`referral_commission_ledgers`、`referral_commission_jobs`、`referral_withdrawals` 等 12 张表，加上 `adty_user_fingerprints`。公开 `POST /api/adty/referral/click`、`GET /api/adty/referral/public-settings`；用户 `GET /api/user/referral/profile`、`POST /api/user/referral/apply`、`GET`/`POST /api/user/referral/withdrawals`、`POST /api/user/referral/withdrawals/:id/cancel`；后台都在 `/api/user/admin/referral/` 下。

**限制**：

- 分站客户打开推广页时也会建一个推广员和邀请码；如果有主站用户用这个链接注册，他的充值会给这个分站客户计佣。方案只排除了「被邀请人是分站客户」，没提到这种情况，待定。
- 上线时要先把批量注册阈值设成 0，真实 IP 验证通过后再改回 3（见上线清单）。代码默认是 3。

任务 6.1–6.8、11.1。和方案不一致：方案说合作伙伴比例 15–20，代码只限制 0–100；方案 11.1 要求「站长首次提现必须人工审核」，用户在 a75c8b3da（2026-11-01）去掉了，站长收益转余额和别的佣金一样立即完成。

## 站长分站

**做什么**：用户可以开一个分站（白标），用自己的站名和 Logo 拉客户。客户付「主站价 × 站长倍率 k」，平台收「主站价 × 拿货系数 d」，站长赚差价，收益按消费记账、冻结后可以提现。客户是平台的用户，钱由平台收。

**设置总则**：模块 `adty_reseller_setting`，只能在「分站管理 → 设置」（`/site-admin/settings`）或 `PUT /api/adty/admin/reseller/settings` 改（通用的 `PUT /api/option/` 拒绝这个前缀，防止绕过校验）。六个钱的参数**没有默认值**：`cost_factor`、`min_cost_factor`、`max_markup`、`max_site_bonus_extra`、`trial_grant_max_usd`、`trial_grant_daily_usd`。它们都设了、并且 `root_domains` 不为空，总开关 `enabled` 才能打开。

### 开站

三种方式，最后都走同一个开站函数：

- **申请加审核**：`POST /api/adty/site/apply {subdomain, contact_info, agree_version}`，建成待审核；管理员在分站管理的站点明细里通过或驳回（`POST /api/adty/admin/sites/:id/review`，可以同时设 d 和 k 上限）。驳回后站点直接关闭，子域名不释放。
- **管理员直接开**：`POST /api/adty/admin/sites {user_id, subdomain, ...}`，直接开成 active。
- **开站邀请码**：同一个申请接口带 `invite_code`，码有效就直接开成 active，并用码上预设的 d 和 k 上限；要先打开 `invite_code_enabled`（默认关）。码在「分站管理 → 邀请码」里增删改查，总次数用条件更新防超用。
- **共同条件**：分站总开关打开；用户是启用状态、不是分站客户；名下没有没关闭的站（待审核的也算）；子域名转小写后符合 DNS 规则（1–63 位、不以连字符开头或结尾）、不在保留名单 `reserved_subdomains`（默认 www、api、admin、pay、mail、news、links、status、docs、app）里、全表唯一（关闭或驳回的站也一直占着）。
- **开站协议**：内容在 `adty_legal_setting.site_agreement`（多语言条款里填），版本号 `adty_legal_setting.site_agreement_version`；申请时 `agree_version` 必须等于当前版本号。
- 开成 active 时停掉这个人的邀请返佣（见「推广返佣」）。

### 定价：d 和 k

- **d（拿货系数）**：管理员设，0 < d ≤ 1。取值顺序：单站设的 d → 业绩档位的 d → 全局 `cost_factor`；结果低于 `min_cost_factor` 时用下限。管理员在站点明细里改单站 d（`PUT /api/adty/admin/sites/:id`）。
- **k（站长倍率）**：站长设，1 ≤ k ≤ 上限（单站设的上限 → 全局 `max_markup`，1–100）。可以按全站、按分组、按单个客户设；取 k 的顺序：客户 + 分组 → 客户 → 全站 + 分组 → 全站 → 1。
- **改价**：降价和第一次设置立即生效；涨价写成待生效，`markup_increase_delay_hours`（默认 24）小时后由主节点生效（每 5 分钟检查）；涨价没生效前又降价，取消待生效的涨价。只有 active 的站能改价，暂停和关站中都不能。保存带版本号，别处改过就报冲突。站长中心 → 定价（`/site-owner/pricing`）显示待生效的涨价和生效时间。
- **扣费**：k 乘进分组倍率，所有扣费路径都覆盖（文本、按次、阶梯计价、图片、任务、Realtime、任务按 token 重算）；k 和 d 只记在日志的 `admin_info` 里，客户看不到。价格页和分组列表也按 k 显示分站价。
- **算例**：主站价 $0.40，k = 1.25，d = 0.8：客户扣 $0.50；平台收 $0.50 × 0.8 ÷ 1.25 = $0.32；站长收 $0.18，等于 $0.40 ×（1.25 − 0.8）。

### 加赠和试用额度

- **加赠**：站长在站长中心 → 加赠（`/site-owner/bonus`）设本站档位（要重新验证身份），每一档不能超过「主站同档比例 + `max_site_bonus_extra`」（没设按 0，即不能高于主站）。分站客户只用站点档位，站点没设就不送。加赠由站长出：客户充值入账时，给站长记一笔负的收益（`bonus`）；站长出资的优惠码一样。
- **试用额度**：站长在客户详情里给单个客户送（`POST /api/adty/site/customers/:id/trial`，要重新验证身份），单次不超过 `trial_grant_max_usd`，本站每天合计不超过 `trial_grant_daily_usd`，任一没设就不能送；给客户加额度并写余额流水（`site_trial`），给站长记一笔负的收益（`trial`）。

### 收益、冻结、结算、提现

- **记账**：每笔消费按扣费当时的 k、d 记：站长收益 = 扣费 ×（k − d）÷ k，平台收入 = 扣费 × d ÷ k，两项相加正好是客户付的钱。余额下限生效时按实际扣到的额度记（见「余额下限」）。按「站、客户、日期、类型」累加进日汇总表 `adty_site_earning_daily`，类型有 usage、refund、bonus、bonus_reversal、trial、skipped。渠道测试不记账；关掉消费日志也照样记账。
- **风控**：客户在分站注册时判一次：和站长 `fingerprint_window_days`（默认 30）天内用过同样的 IP 或设备，标「疑似自用」；和主站某个充过值的用户设备相同，标「疑似主站迁移」。两种检查各有开关（`self_use_check_enabled`、`migration_check_enabled`，默认都开）。被标记、没复核的客户照常扣费，收益记成 skipped、不结算。管理员复核后（`POST /api/adty/admin/sites/customers/:id/review`）之后的消费正常计，还可以把没结算的 skipped 改回 usage。
- **结算**：每天第一次过 `settle_hour`（默认 4 点，服务器时区）后跑一次：冻结满 `freeze_days`（默认 7）天的 pending 行按站汇总（负数一起加），加进站长的佣金账户，写流水 `site_earning`。暂停的站跳过。
- **提现**：在推广页（`/referral`）提，和推广佣金同一个账户、同一套规则：转余额立即完成，USDT 和收款码人工审核；可提现余额不够（包括为负）时提不了；还要求推广返佣总开关打开。
- **算例**（k 1.25、d 0.8、冻结 7 天）：10-01 当天客户扣 $125 → 站长 +$45；客户充 $100、站点加赠 5% → −$5；送试用 $2 → −$2；当天合计 $38，10-08 凌晨 4 点后转为可提现。

### 站长中心

站长中心（`/site-owner/<分区>`，侧边栏只对站长显示）：概览（收益数字、近 30 天、趋势）；客户（列表、精确查找、详情：请求记录、充值、密钥、送试用、单独定价、举报）；定价；加赠；优惠码；站点设置（品牌和 Logo、自定义域名、发件域名、SEO 开关）；收益流水（含 CSV 导出，邮箱一律打码）；开站协议（只读）。

改全站价、改客户价、改加赠、送试用、删自定义域名、建改删优惠码、删发件域名，都要重新验证身份（上游的二次验证：有 2FA 或 Passkey 用它们，没有就用密码；验证一次只能用一次，绑定具体操作和对象）。

Logo：PNG、JPEG 或不含脚本的 SVG，不超过 200 KB，存在 `/data/uploads/site-assets`；SVG 按白名单检查，带脚本、事件、外链的都拒。站名、标语、公告、客服联系方式按纯文本显示。

### 站长能看到客户的哪些信息

全局默认在 `adty_reseller_setting.visibility`，单站覆盖存在 `adty_sites.visibility`（只能通过 `PUT /api/adty/admin/sites/:id` 改，后台还没有界面）：

| 键 | 默认 | 打开时站长看到 |
|---|---|---|
| `customer_email` | masked | masked：`a***@域名`；full：完整邮箱 |
| `customer_balance` | on | 客户余额 |
| `customer_topups` | on | 本站订单的面额、加赠、到账额度、支付方式、状态、时间 |
| `customer_usage` | on | 累计用量 |
| `request_logs` | on | 请求时间、模型、令牌名、扣费（客户价）、token 数、耗时、是否流式（走客户自己看到的日志投影） |
| `customer_tokens` | on | 密钥名称、状态、最后使用时间 |

永远看不到：IP、设备、第三方登录、手机号、风控的判断依据（只给类别）、渠道、倍率、密钥本身、请求和回复内容、其他分站和主站的数据。精确查找（`GET /api/adty/site/customers/lookup?q=`）必须是完整的用户名或邮箱，每站每小时最多 `customer_lookup_per_hour`（默认 20）次，每次都记进 `adty_site_lookups`。

### 自定义域名和 Turnstile

- 要先打开 `custom_domain_enabled`（默认关），并填好 `cloudflare_apiToken`（密钥）、`cloudflare_zone_id`、`cname_target`，否则打不开。另有 `cloudflare_account_id`、`fallback_origin`（回源主机名）、`turnstile_widgets`（每个 widget 的 `id`、`site_key`）和 `turnstile_widget_secret`（widget id → secret，密钥）。
- **流程**：站长在站点设置里填子域名（不能是根域名，不能在我们的根域名、主站域名、CNAME 目标、回源主机名下）→ 后端调 Cloudflare 建自定义主机名（HTTP 验证、DV 证书），3 秒后再查一次取验证记录 → 站长中心显示「把这个域名 CNAME 到 `cname_target`」和要加的 TXT 记录 → 主节点每 5 分钟查一次，主机名和证书都是 active 才启用；72 小时还不通标失败，站长重新提交就是重试。
- **Turnstile**：域名启用后，按 `turnstile_widgets` 的顺序登记进第一个还有空位的 widget（每个 widget 最多 10 个域名）；名额满了域名照常能用，之后每 5 分钟重试登记。`/api/status` 按当前站点返回对应的 site key，校验时用对应的 secret，并核对 Cloudflare 返回的主机名。
- 表：`adty_site_domains`。接口：`GET`、`POST /api/adty/site/domains`，`DELETE /api/adty/site/domains/:id`（要重新验证）。
- 2026-11-01 还没用真实的 Cloudflare 账号测过，代码按官方 API 文档写，上线前按任务 11.0 实测。

### 发件域名

- 要先打开 `sender_domain_enabled`（默认关），并在「系统设置 → 邮件」填好上游的 `ResendApiKey`。
- **流程**：站长在站点设置里填域名 → 后端在 Resend 建域名，把要加的 DNS 记录给站长看 → 主节点每 10 分钟查一次，验证通过就启用。启用后，这个站客户的系统邮件用「站名 <noreply@站长域名>」发；没验证或停用时用平台发信地址、显示分站站名。营销邮件一律用平台营销地址，只换显示名。
- Resend 套餐有域名数上限，满了提示「No sender domain slots are left」。关站时自动删除。
- 表：`adty_site_sender_domains`。接口：`GET`、`POST`、`DELETE /api/adty/site/sender-domain`（删除要重新验证）；管理员停用 `DELETE /api/adty/admin/sites/:id/sender-domain`。

### 关站和暂停

- **管理员关站**：站点明细里的「关站」（`PUT /api/adty/admin/sites/:id/status`，`status=closed`），必须选客户去向（转回主站，或转给另一个 active 的站），可以选择扣回收益、可以立即执行。站点先变成关站中，`close_notice_days`（默认 7）天后由主节点执行。
- **执行时**：转回主站的客户改成主站价；转给别的站的，给这批客户写单独定价，保证 k 不高于原来的；扣回收益时作废待结算的、从佣金账户扣回已结算的（可以为负）；删除自定义域名和发件域名；平台子域名的页面跳到客户的新归属。
- 关站中：客户照常使用，不能注册新用户，站长不能改价。
- **暂停**：客户照常使用和注册，收益照常记但不结算；站长不能改价、改加赠、送试用。
- 站长申请关站有接口 `POST /api/adty/site/close`，只记下申请时间，站点状态不变。

### 按域名识别站点、注册和登录

- 每个请求按 Host 头去站点表找当前站点：先找已经启用的自定义域名，再按「子域名.根域名」找；都找不到就当主站，或者按 `unknown_host_mode=reject` 对页面请求返回 404（API 不受影响）。关闭的站的子域名，页面请求 302 跳到客户的新归属。
- 在分站注册的用户归这个站（`users.site_id`），以后不能改；分站客户不走推广绑定。
- `site_login_only`（默认开）：分站客户只能在自己的分站登录，主站用户只能在主站登录；不符时的提示和密码错一样，不暴露账号存在。只限制网页登录，API 令牌不受影响。
- 分站的 `/api/status` 返回分站的站名、Logo、地址和 Turnstile site key，并关掉第三方登录和 Passkey。
- 邮件链接、站名、支付跳回地址只从站点表取，不读请求的 Host 头。找回密码按用户所属站点生成链接。
- **支付跳回**：易支付付完回到主站的 `/pay/result/<订单号>.<签名>`，签名对就 302 到订单所属站点的钱包，不对就去主站钱包。
- **SEO**：分站首页默认 noindex，站长可以在站点设置里打开收录（`adty_sites.seo_index`），打开后 canonical 指向分站自己的地址；分站的 `/robots.txt` 没开收录时整站 Disallow。

### 主站的分站管理

分站管理（`/site-admin/<分区>`），权限是「分站」的查看、审核、调整、设置四项：

- 站点列表和明细（状态、站长、域名、d、k 上限、客户），通过或驳回申请，改 d 和 k 上限，暂停、恢复、关站，一键下线公告（`DELETE /api/adty/admin/sites/:id/notice`，只清空公告，写审计）。
- 设置：`adty_reseller_setting` 的全部填写项（Cloudflare Token 只写不读；没填时显示「未配置」，依赖它的开关打不开），用量卡片（Resend 发件域名按状态计数和最近一次「名额已满」的时间；每个 Turnstile widget 已登记的域名数 / 10，第一个 widget 含主站域名和根域名，剩余少于 5 个时提醒）。
- 邀请码的增删改查。
- 每个写操作都写审计（`adty_site_audits`）。

### 已知的缺口（2026-11-01）

- **前端没有「申请开站」页面**，后端接口已经有了；管理员直接开站也没有界面。
- 三个通知没有实现，代码里的挂点从来没有被赋值：关站前通知客户（前端文案写着「会发邮件通知客户」，实际不发）、涨价前通知客户、改 d 后通知站长。`close_notice_days` 现在只是推迟执行。
- **业绩档位的 d 不会生效**：`cost_factor_tiers` 可以设，但每月算档位的任务没有挂上，`tier_cost_factor` 一直为空。
- 分站管理的「总览」「风控复核和举报」两个分区、站点明细里的「经营数据」只是占位；后端接口已经有（`GET /api/adty/admin/sites/overview`、`/sites/:id/stats`、`/sites/flagged-customers`、`/sites/reports`）。站点列表只对当前页排序，没有本月金额列。
- 后台没有按钮停用某个站的自定义域名（函数有，没挂路由）和发件域名（接口有，没按钮）；单站的可见范围没有界面。
- 站长申请关站后，后台看不到申请，站长中心也没有申请关站的按钮。
- 站点在关站中时，站长中心可能打不开，提示「你没有开着的站」（推断，没有测试覆盖）。
- 分站客户条款：版本号大于 0 时，注册必须带查询参数 `site_terms_version` 且等于当前版本，通过后写进 `adty_site_customers` 的条款版本和时间。注册页仍然没有勾选框。
- 管理员直接开站不检查开站协议；版本号还是 0 时，申请传 `"0"` 就能通过。
- Turnstile 名额用完时，方案要求提示「请在平台子域名上注册」，没有实现；没登记进 widget 的自定义域名会拿到第一个 widget 的 site key，人机验证很可能过不了（推断）。
- `customer_usage` 只有累计用量，方案是按客户、模型、天统计。

任务 11.1–11.11。和方案不一致的还有：方案的键名 `cloudflare_api_token` 在代码里是 `cloudflare_apiToken`；方案写分站设置只有超级管理员能改，代码按「分站·设置」权限；站长首次提现人工审核已经去掉（a75c8b3da，用户 2026-11-01）；退款时站长加赠按收回比例还（见「充值退款」）。

## 分组和后台权限

**分组**：

- 每个分组有系统生成的固定 ID（`g` 加 4 位随机字符）和 8 种语言的显示名、描述，存在 `adty_groups`。用户、密钥、渠道里存的都是 ID，后台改显示名不影响他们；前台到处只显示显示名，不露 ID。
- 改法：在后台原来的分组倍率表上编辑显示名（系统设置 → 计费）。上游的 JSON 编辑模式关掉了，保存时校验：出现未知的分组 ID、或者删掉了还在用的分组，都会被拒绝。
- 迁移和删除：可以把一个分组的用户、密钥、渠道整体迁到另一个分组，迁完才能删（`POST /api/adty/admin/groups/:id/migrate`、`DELETE /api/adty/admin/groups/:id`）。
- 接口：公开 `GET /api/adty/groups/display`（已删除分组的名字也返回给登录用户）；后台 `GET`、`POST /api/adty/admin/groups`、`PUT /api/adty/admin/groups/:id`，都只有超级管理员。

任务 2.1–2.5。

**后台权限**：

- **超级管理员**（root）什么都能做。**普通管理员默认没有任何后台权限**，由超级管理员在「用户 → 编辑」里逐项打开，随时可以关。系统设置里的所有页面、易支付通道的新增和密钥、管理员任免、分组、官方价只有超级管理员能做。
- 权限按「区域·动作」分，我们加的区域：

| 区域 | 动作 |
|---|---|
| 用户 | 查看（列表、详情、余额流水）、编辑（资料、启停、新建、重置 2FA 和 Passkey、解绑、改密码）、改分组、调余额、删除 |
| 日志和统计 | 查看所有人的日志、任务和全站统计 |
| 模型 | 查看、编辑（模型、厂商、预填分组） |
| 充值财务 | 查看订单和充值对账、手动补单、退款 |
| 推广返佣 | 查看、审核（合作伙伴申请、提现）、标记已打款、调整（佣金、冻结结算、比例、停用推广员）、设置（含提现汇率） |
| 支付通道 | 查看状态、操作（启停、测试） |
| 营销 | 查看（设置、群发、统计、优惠码、注册来源）、编辑（设置、草稿、模板、自动邮件、优惠码）、发送（群发、定时、取消） |
| 分站 | 查看（站点、客户、统计）、审核（开站、复核风控客户、处理举报）、调整（d、k 上限、可见范围、暂停、恢复、关站）、设置（分站设置、开站邀请码） |

  上游原有的「渠道管理」「审计日志」「任务插件」三个区域照旧。
- **预设**：编辑管理员时可以一键套用「客服」（用户·查看、日志·查看、充值财务·查看、推广·查看）、「运营」（客服加上 用户·编辑、渠道·查看和操作、模型·查看和编辑、支付通道·查看和操作）、「财务」（用户·查看和调余额、充值财务三项、推广的查看、审核、打款、调整），套用后还能再改。
- 侧边栏和页面按权限显示；没权限调到的接口返回 403，错误码 `ADTY_PERMISSION_DENIED`。

任务 2.6、2.7。

## 营销

**同意和退订**：

- 注册页有「接收产品更新和优惠信息」勾选框。是不是欧盟按 Cloudflare 的 `CF-IPCountry` 头判断（Nginx 要透传），国家列表是 `adty_marketing_setting.eu_countries`（默认欧盟、EEA、英国、瑞士）；拿不到国家（空、XX、T1）当欧盟处理。欧盟一律默认不勾，其他地区按 `consent_default_non_eu`（默认不勾）。同意的时间、IP 哈希、语言、文案版本（`consent_version`）都存下来。
- 用户在个人设置里随时能改（`/api/adty/marketing/self`）；退订后重新勾选会解除退订。退信和投诉的标记永不解除。退订不影响系统邮件（验证码、找回密码、充值到账、额度提醒）。
- 只有同意、没退订、没退信、没投诉的人同步到 Resend 的联系人。Resend 的 webhook（`POST /api/adty/marketing/resend-webhook`）按 Svix 签名验签（密钥 `adty_marketing_setting.webhook_secret`），收到退订、退信、投诉就本地标记，以后不再发。
- 群发邮件带 Resend 的一键退订链接；自动邮件逐封发，用我们自己的签名退订链接（`/api/adty/marketing/unsubscribe`，GET 显示确认页、POST 退订），并带 `List-Unsubscribe` 和一键退订头。

**群发**（营销 → 群发，`/marketing`）：

- 按语言、注册时间、最后活跃、是否充过值、余额区间、分组、所属站点筛选，显示预计人数（只算能发的人）。分站客户默认不含（`include_site_customers_default`）；勾选包含时每个分站单独发一次，用分站站名。
- 8 种语言各写一版，没写的语言用英文版；变量映射成 Resend 的联系人属性；可以存成模板。
- 先测试发给自己，可以定时；发送前二次确认人数和语言版本，每次群发写审计。发送时在 Resend 建一个临时分组，再按语言建 broadcast。中途可以取消：定时的回到草稿，排队中的停止剩下没发的。
- 统计：送达、退信、投诉、退订、打开、点击，来自 webhook 和 broadcast 接口。
- 发件地址 `from_address`，底部的发件方名称和联系方式 `footer_company`、`footer_address`（CAN-SPAM 要求），打开和点击统计 `open_tracking`、`click_tracking`（默认开，只应在营销子域名上开）。

**自动邮件**（营销 → 自动邮件），每条单独开关，默认都关；只发给同意营销的人，同一用户同一规则只发一次，每条规则每小时最多发 `trigger_batch_size`（默认 200）封：

| 规则 | 初始值 | 条件 |
|---|---|---|
| 调用过但没充值 | 注册满 3 天 | 有消费、从没充值成功 |
| 余额快用完 | 余额低于 $1 | 付费用户；上游的额度不足提醒开着、这个人没关、阈值不低于营销阈值时，营销这封不发 |
| 付费用户召回 | 30 天没调用 | 充过值、最近 N 天没有调用 |

**优惠码**：见「充值和加赠」。

**注册来源统计**：首次访问时把 UTM 参数、落地页和来源域名写进 cookie `adty_src`（30 天），注册时记下（加上推广人、所属站点），存在 `adty_signup_sources`；后台「营销 → 来源统计」按来源统计注册数、付费数、充值额、消费额。隐私政策要写明会记录来源。

**默认**：营销总开关 `adty_marketing_setting.enabled` 默认关；发件地址、底部发件方名称和联系方式、webhook 签名密钥、Resend API Key（复用「系统设置 → 邮件」的 `ResendApiKey`）都填了才能打开，没填的项旁边显示「未配置」。

**表和接口**：`adty_marketing_contacts`、`adty_marketing_campaigns`、`adty_marketing_campaign_sends`、`adty_marketing_campaign_recipients`、`adty_marketing_webhook_events`、`adty_marketing_trigger_sends`、`adty_promo_codes`、`adty_promo_redemptions`、`adty_signup_sources`。后台接口都在 `/api/adty/admin/marketing/` 下：`settings`、`campaigns`（及 `/:id`、`/:id/plan`、`/:id/test`、`/:id/send`、`/:id/cancel`）、`audience/estimate`、`triggers`、`sources`、`promo-codes`。

**限制**：所有 Resend 调用只在假服务器上测过；自定义联系人属性的模板写法 `{{{contact.<键>}}}` 是照官方内置字段的格式类推的，上线前实测。

任务 12.1–12.5。

## 网页客服、状态页、多语言、首页 SEO

**网页客服（tawk.to）**：在「系统设置 → 站点 → ADTY AI Site」填 `adty_site_setting.tawk_property_id` 和 `tawk_widget_id`，两个都填了（只能是字母、数字、`_`、`-`）才加载，所有页面都显示，访客是匿名的。打开 USDT 或支付宝、微信支付窗时隐藏，关闭后再显示。默认关。限制：语言跟 tawk.to 后台的设置，不随界面切换；分站没有自己的配置，分站客户也连到主站客服；清空 id 后要刷新页面才消失。任务 8.8。

**服务状态页**（`/status`，公开）：显示 Uptime Kuma 里各监控的状态和 24 小时可用率，每 60 秒刷新，按当前站点显示站名，不显示 Uptime Kuma 自己的分组名。数据来自上游的「Uptime Kuma」设置（系统设置 → 内容），没配时显示「Service status is not available yet.」。限制：上游的 `GET /api/uptime/status` 不用登录、不看开关，返回里带分组名，「不显示渠道」只在前端做到；后端没有缓存；页脚和导航里没有指向它的链接。任务 11.10。

**多语言**：前端和后端都是 8 种（en、zh-CN、zh-TW、fr、ja、ko、ru、vi）。前端先看用户选过的语言，再看浏览器语言，都没有用英文；后端按用户设置、请求头，最后用环境变量 `DEFAULT_LANGUAGE`。漏翻检查：在 web 目录 `node scripts/adty/i18n-check.mjs`，只查我们加的键（`en.json` 减去 `upstream-en-keys.json`），8 个文件都要有、而且值不能和英文一样（品牌词和法文同形词在白名单里）；CI 和 `scripts/adty/check.sh` 都会跑。后端我们的键在 `i18n/locales/adty.<语言>.yaml`，由 Go 测试检查齐全。任务 9.1、9.2。

**首页 SEO**：

- 8 个首页网址：`/`（英文，另有 x-default 指向它）、`/zh-cn`、`/zh-tw`、`/fr`、`/ja`、`/ko`、`/ru`、`/vi`。用户选过语言、或者已登录且界面不是英文时，打开 `/` 跳到对应语言的首页，原来的查询参数保留；跳转用路由目标，不再把查询参数拼进路径，避免预加载反复请求 `/`。搜索引擎和第一次来的访客看到英文。
- Go 返回首页时写进标题、描述、canonical（不带查询参数）、8 条 hreflang、Open Graph、Twitter 卡片，只在 `/` 写 WebSite 结构化数据；正文在构建时预渲染进 `web/dist/adty-home.json`，插在 `#root` 前面。
- 后台「系统设置 → 站点 → Home Page SEO」：`adty_seo_setting.home_title`、`home_description`（按语言，没填用英文的，英文也没填时标题用 ADTY AI）、`og_image`（默认 `/adty-og.png`）。说明写的是 8 个首页网址。一次保存多个字段时，不会因为中途刷新设置把还没保存的草稿覆盖掉；站点设置页也一样。
- 其余页面一律 noindex。服务器地址（`ServerAddress`）没设或还是 `http://localhost:3000` 时，不写 canonical、hreflang 和结构化数据；请求的域名和服务器地址不同时首页加 noindex。分站见「站长分站」。
- 落地页保持白底。区块是首屏、支付方式、热门模型、三步、接入说明、卖点、返佣、常见问题。没有收尾的「今天就发出第一个请求」。接入说明链到 `/docs/claude-code`、`/docs/codex-cli`、`/docs/cherry-studio`、`/docs/openai-sdk`。
- `/robots.txt` 由 Go 返回：主站屏蔽 `/api/`、`/v1/`、`/pay/`。代码里没有 sitemap。
- 和方案不一致：方案第八节说 robots.txt 和 sitemap 在 Nginx 上配，代码把 robots.txt 放进了 Go，sitemap 没做；没开 SEO 的分站整站 `Disallow: /`，搜索引擎读不到页面里的 noindex。

任务 9.3。

## 限流、脱敏和其他安全

**按用户限流**：`adty_limit_setting.enabled`（默认关）、`default_rpm`、`default_tpm`、`default_concurrency`（默认 0 表示不限）；单个用户在「用户 → 编辑」里单独设（-1 跟默认、0 不限），存在 `adty_user_limits`。固定 1 分钟窗口；TPM 在写消费日志时累计（关掉消费日志也算）；并发按请求登记，请求结束释放。超限返回 HTTP 429 和 `Retry-After`，错误类型 `adtyai_error`、错误码 `rate_limit_exceeded`。开了 Redis 就只用 Redis 计数，不退回进程内存；客户端没接上或计数出错时返回 HTTP 503，错误码 `rate_limit_unavailable`，不放行。没开 Redis 时用进程内存。后台：系统设置 → 安全 → 用户限流。接口 `GET`、`PUT /api/adty/admin/users/:id/limits`。限制：TPM 是事后累计，同一分钟里同时进来的大请求拦不住；跨分钟边界时短时间内最多约 2 倍上限；同一用户所有令牌共用计数；演练场 `/pg` 没挂。任务 7.4。

**客户接口脱敏**：客户自己看到的日志去掉渠道 ID、渠道名、上游请求 ID，`other` 按白名单只留用量和状态（去掉倍率、模型价、上游模型名、`admin_info`）；任务列表不露渠道和上游模型名；任务提交不再返回 `X-New-Api-Other-Ratios` 头。作用于 `GET /api/log/self`、`/api/log/self/export`、`/api/log/token`、`/api/task/self`、`/api/mj/self`、站长看客户请求的接口。管理员的 `/api/log/`、`/api/task` 照常显示渠道。按接口脱敏，不按角色：管理员调 `/api/log/self` 看到的也是脱敏后的。任务 7.1。

**人工调额留痕**：后台「用户 → 编辑 → 调余额」原因必填（最多 200 字），带幂等键；写审计（带原因）、用户自己的日志（不带原因）和余额流水 `adty_balance_ledgers`（带原因，只给后台看，`GET /api/adty/admin/users/:id/ledger`）。非超级管理员不能调同级或更高角色的用户。「减少」不能减到 0 以下，「覆盖」不能写成负数（见「余额下限」）。任务 7.2。

**防邮箱枚举**：邮箱已注册时，发注册验证码不发码、改发一封提醒信（10 分钟最多一封），接口照样返回成功；注册时邮箱已占用返回和「验证码错误」一样的提示；找回密码不管邮箱存不存在都返回成功；三封信都异步发。开了 Redis 但客户端没接上时，两个发码接口都返回 HTTP 503，不发信。验证码或重置码写不进共享存储时不发信；已注册邮箱和未知邮箱都返回同一个空成功，不用 503 区分。限制：用户名仍可以被枚举；登录后绑定邮箱、第三方登录仍会提示邮箱已被占用；提醒信的频率限制在进程内存里。任务 7.3。

**登录失败锁定**：同一用户名密码错误累计 8 次后，15 分钟内照常返回用户名或密码错误，不再校验密码。开了 Redis 就用 Redis 共享计数；客户端没接上或读写出错时按已锁定处理，不退回进程内存。没开 Redis 时用进程内存。

**流式保活**：上游的 ping（`general_setting.ping_interval_enabled` 默认关、`ping_interval_seconds` 默认 60，后台在「系统设置 → 模型 → Global Model Configuration」的「Keep-alive Ping」）现在对所有流式格式（Realtime 除外）在请求上游之前就开始发 `: PING`，一直到第一次写出数据，最长 120 分钟，避免推理模型首字超过 125 秒被 Cloudflare 以 524 断开。上线时打开，间隔设 30 秒。限制：原生 Gemini 流式（`/v1beta`）上游本来关掉了 ping，现在首字前也会收到 ping（提前保活不看这个开关，对 Gemini 客户端的影响没有核实）；第一次 ping 发出后状态码固定为 200，之后上游再出错只能在流里返回错误事件。任务 7.5。

## 官方价、接入文档、条款

**官方价**：主节点启动时同步一次，之后每 24 小时（环境变量 `ADTY_OFFICIAL_PRICE_SYNC_HOURS`）从 models.dev 拉取输入、输出、缓存读、缓存写的每百万 tokens 美元价，存进 `adty_official_prices`；失败保留旧数据。只更新有映射、没开「手动覆盖」的行；没映射的模型按 `adty_official_price_setting.auto_map_providers`（默认 anthropic、openai、google、deepseek、xai、mistral）找完全同名的自动映射。按次价只能手填。价格页和首页显示「省 X%」= 1 − 本站价 ÷ 官方价（按输入价、美元，向下取整到整数百分比，为负显示「—」）。后台「系统设置 → 计费 → Official Prices」（只有超级管理员）：改映射、价格、手动覆盖，立即同步。公开接口 `GET /api/adty/pricing/official`。限制：不开手动覆盖时手填的价格会被下次同步改回；没有删除接口；`auto_map_providers` 没有后台界面。任务 8.4、8.5。

**接入文档**（`/docs`、`/docs/<页>`，公开）：快速开始、Claude Code、Codex CLI、Cherry Studio、OpenAI SDK、错误和限制、常见问题，8 种语言，是仓库里的 Markdown（`web/src/features/adty-docs/content/`），改内容要改代码后重新构建。文中的接入地址取 `adty_site_setting.api_base_url`，没填用服务器地址。限制：分站的文档也显示主站品牌，主站填了 `api_base_url` 时分站文档显示主站的接入地址。任务 8.6。

**条款**：用户协议、隐私政策、退款政策三份多语言条款，后台「系统设置 → 内容 → 多语言条款」按语言填，`GET /api/adty/legal/:doc?lang=` 按「本语言 → 英文 → 上游的 `legal.*`」回退；退款政策要在「站点 → ADTY AI Site」打开 `refund_policy_enabled` 才显示。注册和登录页的同意勾选框只看上游的 `legal.user_agreement`、`legal.privacy_policy` 有没有填，所以两边都要填（见上线清单）。主站发布了用户协议或隐私政策后，注册必须带 `agreement_accepted: true`，否则拒绝并不建账号；接受后写一条系统日志，不另记版本号。开站协议和分站客户条款（`site_agreement`、`site_customer_terms`）也在 `adty_legal_setting` 里。限制：登录页的勾选仍然只在前端检查；开站协议和分站客户条款没有后台编辑界面，只能直接写设置；主站页脚的法律链接没有退款政策（登录注册页的有）。任务 8.6。

## 品牌和 Logo

**站名**：ADTY AI，8 种语言都不翻译；程序对外的技术名字是 adtyai。代码里只在 `common/adty_brand.go`（`AdtyBrandName`、`AdtyBrandSlug`、`AdtyBrandURL`）和 `web/src/lib/adty-brand.ts`（`ADTY_BRAND_NAME` 等）两处写字面量。后台的系统名称默认就是 ADTY AI。

**Logo**：方案 H「层叠网关」。原稿和生成脚本在仓库外的 `C:\Users\Administrator\ai\adtyai-logo`（`source/build.py` 生成全部文件，说明见那里的 README.md）。`web/public` 里的文件：

| 文件 | 用途 |
|---|---|
| `favicon.ico` | 浏览器标签图标（内含 16、32、48 三种） |
| `favicon.svg` | 新式浏览器的矢量标签图标，跟随系统深浅色换底色 |
| `favicon-16x16.png`、`favicon-32x32.png` | PNG 标签图标 |
| `adty-apple-touch-icon.png` | iOS 添加到主屏幕（180×180） |
| `site.webmanifest` | 安卓添加到主屏幕和 PWA 的清单 |
| `android-chrome-192x192.png`、`android-chrome-512x512.png` | 安卓主屏图标（manifest 引用） |
| `maskable-512x512.png` | 安卓自适应图标（manifest 引用） |
| `safari-pinned-tab.svg` | Safari 固定标签（单色剪影） |
| `mstile-150x150.png` | Windows 开始菜单磁贴 |
| `logo.png` | 默认 Logo（180×180，简化版图形）：后台 Logo 没填时顶栏、侧边栏、页脚用它 |
| `adty-logo.svg` | 首页预渲染顶栏里的小图标（24px，简化版图形） |
| `adty-og.png` | 社交分享图（1200×630，只有图形和字标），SEO 设置 `og_image` 的默认值 |
| `pay-card.png`、`pay-apple.png`、`pay-google.png`、`waffo-logo-*.svg` | 上游 Waffo 支付的图标，没用到，照上游保留 |

`.webmanifest` 在 Go 里登记成 `application/manifest+json`（`middleware/adty_static_types.go`），否则镜像里会被当成纯文本返回。

**限制**：

- 后台「系统设置 → 站点 → 系统信息」填了 Logo 以后，前端会把所有标签图标换成这一个 Logo，`favicon.svg` 的深浅色和 16/32 PNG 就不再生效（读代码得出，没在浏览器实测）。想保留整套标签图标，Logo 可以留空。
- 控制台顶栏左上角的 Logo 盖住了「Home」导航（换 Logo 之前就有），记在上线清单里，代码审查时修。
- 公开页脚没有产品、客服、源码列，也不显示上游署名。客服邮箱不写在页脚；错误页反馈按钮仍读 `adty_site_setting.support_email`。
- 系统邮件和营销邮件的模板还没有图片页眉；要加时用 `adtyai-logo/email/logo-email-header.png`（见上线清单）。

任务 8.1、8.2、8.9；新 Logo 2026-11-01（提交 b87c3bff8）。公开页脚和首页接入说明 2026-11-02。
