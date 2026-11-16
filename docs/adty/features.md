# 功能说明

给运营人员和以后接手的开发看：每个功能做什么、规则、后台在哪里设置或操作、默认开还是关、相关的表和接口、已知的限制。内容按代码写（main，2026-11-08），每节末尾是对应的实施方案任务号；代码和方案不一致的地方单独写出来。

几个通用的说明：

- **额度和美元**：余额沿用上游的额度整数，1 美元 = 500000 额度。这个尺度固定，后台保存 `QuotaPerUnit` 时只接受 500000，已存的其他值加载时不生效。下文的算例都用美元写，括号里是额度。
- **以约定 I 为准**：站长不可更换。优惠码已整条删除（报价和下单不再接受优惠码，新库不建优惠码表）。正式语言只有简体中文和英文。下文若仍写交接、优惠码可用或八种正式语言，那是旧说明，不能当成当前开发规则。当前边界见 [conventions.md](conventions.md) 的约定 I。
- **后台设置**：我们的设置都存在 options 表里，键是「模块名.键」，例如 `adty_topup_setting.bonus_rates`。和钱有关的参数没有内置默认值，没设之前对应功能不开（约定 I）。
- **后台权限**：后台接口按「区域·动作」授权，见「分组和后台权限」一节。下文写「需要 财务·退款」之类，指这个权限。
- **密钥**：存进 options 的密钥，键名以 `Token`、`Key`、`Secret` 等结尾，`GET /api/option/` 不会返回它们；后台只能填新值，不显示旧值。
- **主节点**：定时任务（结算、补查、同步、关站执行等）只在主节点跑。
- **菜单折叠**：有子项的父菜单默认折叠，当前路由不会自动展开；点击文字、箭头或键盘操作切换，子项继续标记当前页。复用现有 Sidebar/Collapsible；真实组件回归覆盖通用菜单，生产浏览器覆盖桌面与移动对话预设及无业务写请求。
- **签到已删除**：签到接口、签到设置和前端入口都已删除（2026-11-16）；旧签到设置书签回到额度设置。历史签到额度、记录及退款来源保护保留。
- **公告已读角标（N-02）**：登录账号的 Notice 与 announcements 回执保存在服务端，按账号和当前 Host 隔离，同账号其他设备或标签页可在重新读取后同步；按公告内容版本计算，编辑同 ID 公告会重新计为未读。匿名访客仍用本地回执，历史匿名数据不会迁入登录账号。此项只补已读链，不包含 N-01 定向发布或 N-04 业务待办。
- **定向通知（N-01，部分范围）**：Root 在系统设置→内容→公告的“定向通知”发布平台公告、系统通知、维护通知和经营规则调整；接收范围为全部用户、全部分站、指定单个分站。站长在站长中心→本站设置的“本站公告”管理自己站点的客户公告和经营说明，不能修改 Root 的通知。指定/全部分站的收件同时校验当前 Host 与 SQL 账号归属或站长身份；“全部用户”是 Root 明确授权全平台可见的范围。通知中心 Timeline 展示标题、发布人、类型、接收对象、时间及可选关联业务说明；每页 20 条，支持加载更多，角标包含后续页面未读数。登录账号可以在通知中心把单条公告「隐藏」（`POST /api/adty/notifications/hide`，PAT `profile:write`），只影响自己；隐藏前先记为已读，不会留下未读角标。登录账号才能收到新发布通知，匿名旧公告配置保持原有行为，旧公告开关不控制新发布通知。编辑会产生新的未读版本；版本冲突须重新读取，撤回后不再展示但保留发布及已读历史。表为 `adty_notification_publications`，管理接口分别为 `/api/adty/admin/notifications`（Root，PAT `option:read/write`）与 `/api/adty/site/notifications`（当前站长，PAT `wallet:read/write`）。指定业务范围的收件规则仍待确认，关联说明不构成收件筛选；员工授权与真实待办、外部邮件/短信/推送未由本批实现，N-01 不因本批部分范围关闭。

目录：充值和加赠 · USDT 和易支付多通道 · 充值退款 · 余额下限 · 推广返佣 · 站长分站 · 分组和后台权限 · 营销模块删除与优惠码退役 · 网页客服、状态页、多语言、首页 SEO · 限流、脱敏和其他安全 · 官方价、接入文档、条款 · 品牌和 Logo。

## 充值和加赠

**做什么**：用户在钱包页（`/wallet`）选面额、选已配置的支付方式充值，支持 BEpusdt 和易支付。加赠按支付方式配置：主站由 Root 设每种方式的比例；分站要先由 Root 逐站授权支付方式，站长再在授权范围内设本站比例。优惠码已退役，报价和写入一律拒绝；能主动加额度的只有充值额度兑换码（`adty_voucher`）。

**规则**：

- **面额**：普通自选面额为正整数美元，不低于最低充值额 `MinTopUp`（默认 1）。钱包总额不能超过 2^53 − 1 额度，下单时会检查。易支付还受通道的单笔和每日限额限制。
- **加赠按支付方式**：三个配置键 `adty_topup_setting.method_bonus`（主站规则）、`adty_topup_setting.site_method_auth`（Root 对分站的逐站授权）、`adty_topup_setting.site_method_bonus`（站长在本站授权范围内的规则）。每条规则是 `{"enabled": bool, "rate": "0.025"}`，`rate` 是 **0–1 的小数比例**（0.025 = 2.5%），不是百分数，也不是金额档位；没有规则或 `enabled=false` 就不送（`setting/adty_setting/bonus_method.go:20-23,186-195`）。支付方式规范化成 `alipay`、`wxpay`、`usdt`，USDT 的 TRC20 / BEP20 / Polygon / X Layer **共用一条 `usdt` 规则**（`service/adty_method_bonus.go:15-26`）。
- **金额档位已退役**：`adty_topup_setting.bonus_rates` 只剩历史值，结构体注释写明「已退役的金额档位，不参与新订单赠金」（`setting/adty_setting/topup.go:13`）。保存档位的 `SaveAdtySiteBonusRates` 一律返回 `ErrAdtyAmountTierRetired`（`service/adty_site_earning.go:370-376`）。旧档位唯一的用途是算分站加赠上限里的「主站同档比例」。
- **Root 授权流程**：主站比例 Root 用 `PUT /api/adty/admin/bonus-methods {method, enabled, rate}`。分站先由 Root 调 `PUT /api/adty/admin/sites/:id/bonus-methods {method, allowed}` 逐站授权；站长再调 `PUT /api/adty/site/bonus-methods {method, enabled, rate}` 设本站规则。未授权的支付方式站长存不进去（`MergeSiteMethodBonus` 直接拒绝），也不会付赠金（`setting/adty_setting/bonus_method.go:103-132`、`service/adty_method_bonus.go:47-55`）。
- **报价两道闸**（分站加赠为正值时）：
  1. **规则上限** `adtySiteBonusWithinCap`：本站加赠 ≤ 面额 ×（主站历史同档比例 + `adty_reseller_setting.max_site_bonus_extra`）÷ 100。主站同档比例从退役的 `bonus_rates` 按整数面额取「不超过它的最高档」，`max_site_bonus_extra` 没设按 0（`service/adty_site_earning.go:279-297`）。
  2. **保障** `SiteBonusEarningCovers`：站长收益钱包的可用余额（`balance − frozen_usd`）要够垫这笔赠金；不够时只有 Root 授权了保证金补位（`adty_topup_setting.site_bonus_margin_fill`）且保证金本周期还能支取才放行（`service/adty_method_bonus.go:86-121`）。两道都过才报价，否则直接拒绝（`service/adty_topup_quote.go:151-165`）。
- **公式**：
  - 加赠 = 面额 × rate（8 位小数）。
  - 入账额度 = (面额 + 加赠) × QuotaPerUnit，向下取整。
  - USDT 应付 = 面额 × 分组充值倍率（8 位）；下单后改成网关返回的实际应付。
  - 支付宝、微信应付 = 面额 × 人民币单价 `Price`（默认 7.3）× 分组充值倍率（2 位）。
  - 返佣只按面额算，不含任何加赠。
- **保障冻结与待履约**：下单/入账时按「站长出资的加赠」从收益钱包冻结，不够再用 Root 授权的保证金（流水 `gift_reserve`）；取消、过期、替换时按原路释放（`gift_release`）；客户消费赠金时按比例释放（`gift_spend`）（`service/adty_method_bonus.go:127-246`、`service/adty_gift_fulfillment.go:534-616`）。如果客户在订单取消或过期之后才付款、而站长当时保障不足：**本金照常入账，赠金记成「待履约」**（`adty_gift_fulfillments`，状态 `pending`），不可消费；站长把收益转进保证金（`POST /api/adty/site/wallets/earning-to-margin`）后，系统立即重取保障并补发（`service/adty_gift_fulfillment.go:340-472`、`service/adty_site_finance.go:420-425`）。整笔退款会把 pending 取消。
- **分站客户**：只用 Root 授权过的支付方式下站长自己的规则；站长没设或没开就不送，不存在「回落到主站规则」。
- **优惠码**：已整条删除（2026-11-16）：报价和下单不再接受优惠码，订单上没有优惠码字段，管理接口和面板也已删除。要主动加额度只能用充值额度兑换码。

**算例（主站，`method_bonus` = `usdt` 2.5%、`alipay` 1%）**：

- $100 USDT：加赠 $100 × 0.025 = $2.50，入账 $102.50 = 51,250,000 额度；应付 100 USDT。
- $100 支付宝（`Price` 7.3）：加赠 $1.00，入账 $101.00 = 50,500,000 额度；应付 ¥730.00。
- 自定义 $150 USDT：加赠 $3.75，入账 $153.75 = 76,875,000 额度。

**算例（分站）**：主站旧 100 档 2.5%、`max_site_bonus_extra` = 2，则 $100 订单的加赠上限 = 100 × (2.5 + 2) ÷ 100 = **$4.50**。站长把 USDT 规则设成 3%，加赠 $3.00 ≤ $4.50，且收益钱包可用余额 ≥ $3.00 时才能报价；可用余额只有 $2.00 又没授权保证金补位，报价被拒。站长把比例调到 5%，加赠 $5.00 > $4.50，报价被拒。

**后台**：

- 系统设置 → 计费 → 支付（只有超级管理员）：人民币单价 `Price`、最低充值 `MinTopUp`、金额按钮、各支付方式加赠、回调地址 `CustomCallbackAddress`。
- 分站管理 → 站点明细：Root 逐站勾选可加赠的支付方式。
- 站长中心 → 加赠（`/site-owner/bonus`）：已授权方式的开关和比例。

**默认**：没有任何支付方式规则，不送加赠。「支付合规确认」2026-11-01 取消，相关接口和设置 2026-11-16 删除，支付默认就能用（见 deleted-features.md）。

**表和接口**：

- 表：`top_ups`（上游充值记录，`Amount` 记美元面额、`Money` 记应付）、`adty_topup_orders`（面额、加赠、入账额度、站点、赠金保障来源与待履约状态）、`adty_gift_fulfillments`；`adty_promo_codes`、`adty_promo_redemptions` 只保留历史；兑换码用 `adty_vouchers`。
- 用户：`GET /api/adty/topup/info`、`POST /api/adty/topup/quote`、`GET /api/adty/topup/orders/:trade_no`、`POST /api/adty/topup/orders/:trade_no/cancel`、`GET /api/adty/topup/pending`、`GET /api/adty/topup/self`。
- 加赠配置：`PUT /api/adty/admin/bonus-methods`、`PUT /api/adty/admin/sites/:id/bonus-methods`（Root，PAT `option:write`）、`PUT /api/adty/site/bonus-methods`（站长，PAT `wallet:write`）。

**限制**：

- 上限和保障是两条独立判断：上限查「规则允不允许给」，保障查「站长的钱够不够垫」。二者都过才报价。
- `max_site_bonus_extra` 只放宽上限，不会给站长钱；收益不够、保证金没授权时同样报价失败。
- 待履约赠金在补足保障前不能消费；全额退款时取消，不会折现。
- 站长删掉旧的金额档位不影响新订单：新订单根本不读档位。

任务 3.1、3.3、3.6、12.4。和旧文档不一致：方案 3.1 的内置 13 档和后来的金额档位都已退役，现规则按支付方式计，见 `setting/adty_setting/bonus_method.go`；优惠码整条链路已删除。

## USDT（BEpusdt）和易支付多通道

**USDT**：

- **链**：TRC20、BEP20、Polygon、X Layer（支付方式 `usdt.trc20`、`usdt.bep20`、`usdt.polygon`、`usdt.xlayer`）。只有勾在 `adty_bepusdt.trade_types` 里的链才显示、才能下单，默认只有 TRC20。
- **站内支付窗**：显示收款地址、应付数量和二维码，不跳出网站。
- **有效期**：`adty_topup_setting.usdt_order_minutes`，默认 15 分钟。下单时固定传 `fiat=USD`、`rate="1"`，应付数量以网关返回的为准。
- **回调** `POST /api/user/bepusdt/notify`：md5 验签；只有 `status=2` 入账；核对金额、实付 ≥ 应付、收款地址、网关订单号。金额不符、订单不存在、超出额度上限这类问题进「充值对账」的人工审核，回网关 ok；临时错误回 500 让网关重发。
- **换链**：先取消旧单（调网关的取消接口），网关已经检测到转账时不让取消，提示用户；取消成功后提示「不要再往旧地址转账」，再按新链下单。
- **设置**：系统设置 → 计费 → 支付网关 → USDT 页签（`?tab=usdt`）：`adty_bepusdt.enabled`、`.base_url`、`.secret`（只写）、`.trade_types`、`adty_topup_setting.usdt_order_minutes`。默认关，打开且填了地址和密钥才能用。旧地址 `/system-settings/billing/usdt` 会跳到这个页签。

**支付宝、微信（易支付多通道）**：

- **通道**：每个通道可以开支付宝、微信，每种方式可以设显示名称（比如「支付宝1」，最多 32 个字）和单笔最小、最大、每日限额（人民币，0 表示不限）。新建的通道默认停用，测试下单通过后手动启用；有订单的通道只能停用不能删；商户密钥不返回给前端。
- **钱包按通道平铺（2026-11-16）**：钱包里每个启用的通道、每种启用的方式各一个按钮，按通道优先级排列，名字用显示名称；没填显示名称就显示「支付宝」「微信支付」，同名的自动编号。支付场景限定了通道时只列允许的通道。钱包页的团队充值也用同一排按钮。客户点哪个按钮，报价和下单就只用这个通道（请求带 `channel_id`）；这个通道下不了单（报错、超时、超限额、已停用）就提示「这个支付通道暂时不可用，请换一个支付方式」，不替客户换到别的通道。支付窗里的「换一个支付方式」取消这笔订单、回到钱包重新选。
- **订单记下单时的名字**：订单保存下单那一刻的显示名称（`adty_topup_orders.method_label`），之后后台改名不影响历史订单。用户的充值记录、待支付提醒、后台充值对账和站长订单都优先显示它；充值对账另外显示通道名称，没填显示名称的订单也能分清是哪个商户。
- **挑通道**（不带 `channel_id` 的下单，比如直接调接口）：先只留启用、支持这种方式、金额在单笔范围内、今天到账加这笔不超过每日限额的通道；在里面取优先级最高的一组；电脑端优先挑能返回二维码的；最后按权重随机。
- **自动切换**：一次下单最多试 5 个通道，单通道超时 `adty_epay_setting.channel_timeout_seconds`（默认 5 秒），总时长 `total_timeout_seconds`（默认 12 秒）。某个通道报错或超时，这笔单标失败、换下一个通道新建一单。只在不带 `channel_id` 时这样自动切换。
- **连续未付转维护**：订单过期或被用户取消，通道的连续未付数加一；达到通道自己的阈值或全局 `unpaid_threshold`（默认 10，0 表示不转）时转维护并通知超级管理员；任何一笔到账清零。
- **补查**：下单超过 `query_after_seconds`（默认 120 秒）还没回调、还没过期的订单，主节点每分钟去网关查一次（只对 https 网关），查到已付且方式、金额对得上就入账。
- **探活**：`probe_enabled` 默认关，间隔 `probe_interval_minutes`（默认 5 分钟）；失败转维护，探活转的维护连续 3 次正常就自动恢复，手动设的不会。
- **有效期**：`adty_topup_setting.epay_order_minutes`，默认 10 分钟。电脑端显示二维码，手机跳转支付。
- **回调**：每个通道独立回调 `/api/user/epay/notify/:channelId`（GET、POST 都收），用这个通道自己的密钥验签；停用或维护中的通道照样接收旧单回调。易支付没有取消接口，本地取消后用户仍然付了钱，照样入账。
- **设置**：系统设置 → 计费 → 支付网关 → Epay 页签（`?tab=epay`，上面这些参数和通道列表都在这里）。查看要 支付通道·查看，启停和测试要 支付通道·操作，新增、编辑、删除只有超级管理员。旧地址 `/payment-channels` 和 `/system-settings/billing/epay` 都会跳到这个页签。列表加载失败时显示错误并可以重试，不再显示成空列表。

**晚到的回调（两种都一样）**：订单是待支付、已过期、已失败、已取消，收到验签通过、金额一致的成功回调都照样入账，因为钱已经收到了。已经入账的直接返回成功，不会重复入账。已经退款的订单再收到回调，进人工审核，不再入账。

**充值对账**（`/recharge-audit`）：订单列表、汇总、对不上的回调（可以补入账或标已处理）、手动补单（必须填凭证和备注，不核对金额）、退款。

**支付商与财务动作（M-04、FIN-F-02）**：筛选和订单动作取自后端注册服务能力，包含 BEpusdt 和 Epay。支持的未入账订单可凭证补单，已入账订单可做账务退款，实际退钱仍需线下完成。两个服务均可通过 `POST /api/adty/admin/topup/:trade_no/query` 主动补查；该操作可能入账，必须具备「财务·手动补单」权限及对应 PAT scope。查询核对保存的订单事实并复用统一入账事务，重复请求不重复入账；未确认付款和查询失败分别显示，失败可以重试。缺扩展记录、替换、已退款等订单不显示不适用的动作。Epay 仅使用 HTTPS 查单，禁止重定向转发商户密钥；接口和审计不暴露原始支付查询错误。

BEpusdt 使用官方 `POST /api/v1/pay/info`，只信任与冻结付款链接同源的 HTTPS 网关，不跟随重定向，也不向公开查询接口发送商户密钥。只有成功状态且本地订单号、网关单号、链、收款地址、USD 面额和应付 USDT 完全匹配才入账；确认中、过期、取消和失败不算成功。缺少原始网关事实或网关更换时保留人工对账，不猜测到账。查询响应没有原始链上 hash，记录来源为 query，不伪造交易 hash。停用新收款后仍能对原订单补查。后台复用每分钟最多20单、创建至少120秒、最近七天及至少60秒查询间隔的恢复任务；并发回调和补查共用唯一入账流水。

**表和接口**：`adty_topup_orders`、`top_ups`、`adty_epay_channels`、`adty_epay_channel_methods`、`adty_payment_orphan_events`。下单 `POST /api/user/bepusdt/pay`、`POST /api/adty/epay/pay`；回调 `POST /api/user/bepusdt/notify`、`GET`/`POST /api/user/epay/notify/:channelId`；通道 `GET /api/adty/admin/epay/channels`、`POST /api/adty/admin/epay/channels/:id/status`、`POST /api/adty/admin/epay/channels/:id/test`（新增、编辑、删除 `POST /api/adty/admin/epay/channels`、`PUT`/`DELETE /api/adty/admin/epay/channels/:id` 只有超级管理员）；对账 `GET /api/user/admin/finance/recharge-audit`、`GET /api/user/admin/finance/payment-orphans`，补单 `POST /api/adty/admin/topup/:trade_no/complete`；付完跳回 `GET /pay/result/:token`。

**补查边界**：易支付补查发现已付款但方式不符或入账失败，会写入人工审核；易支付订单过期以后不再补查。BEpusdt 对超过 120 秒且在七天恢复窗口内的待付、失败、过期、取消订单自动执行认证查询，每单至少间隔 60 秒，每轮最多 20 单；核对下单时冻结的 trade_id、收款地址和金额后，调用相同入账事务。更早的订单通过支付商重送或有凭证的手动补单处理。存在尚未排除的撤销事件时，查询和成功回调都不能自动补入账。

**外部退款、撤销和拒付审核**：可信通知进入待核对事件，管理员填写原订单、支付商凭证、备注、累计实际撤销金额和币种。争议打开不等于已发生退款，可以先以 0 金额记录待核对事实。部分撤销保持待核对，保存审核不扣钱包、不自动触发整单退款。全额撤销以原单实际支付金额和币种为准；先通过原订单现有退款操作扣回可回收额度，再凭同单、同用户的真实退款流水结案。审核快照保存实际扣回额度、本金额度、按既定规则可覆盖的现金及未覆盖差额；差额只表示财务暴露，不创建负钱包或客户债务，也不与其他累计通知重复相加。从未入账的订单可在锁内核实后以「从未入账」结案，仍拦截迟到自动入账。忽略事件须说明它属于其他订单或撤销已取消，并提供凭证；该动作会解除此事件的入账拦截。结案事实不可改写，相同请求重放返回原审核，冲突请求被拒绝。

审核记录保存在 `adty_payment_orphan_reviews`，事件通过 `review_id` 关联最新快照；`POST /api/user/admin/finance/payment-orphans/:id/resolve` 需要「财务·手动补单」权限。事件尚未关联原订单时先等待关联，不能任意指定另一笔订单。

任务 3.2–3.5、4.1–4.4、5.1–5.5。

## 充值退款

**做什么**：管理员在后台给一笔已到账的充值订单退款。系统按用户现在的余额，扣回这笔订单还没花掉的额度，算出应退给用户多少钱。真正把钱退给用户在线下做，系统不调支付商的退款接口。

**规则**（2026-11-01 用户定）：只退没花掉的本金，赠金收回不退，余额不会变负。

设这笔订单：

- 入账额度 C = 本金 F + 赠金 G。F 是面额换算成额度（向下取整），G = C − F，即支付方式加赠（历史订单里还可能含已退役的档位或优惠码加赠）。
- 用户现在的活余额是 B（锁住用户行后读取 `quota + quota_pending`，见下）；还留在钱包的签到赠送额度为 X。

那么：

- 扣回 R = min(max(B − X, 0), C)。签到赠送不作为退款本金扣回，余额最少保留 X。
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

- 站长出的加赠只按收回的比例还给站长：收回的赠金 = min(G, R)，还给站长 = 站长出资的加赠 × min(G, R) ÷ G。平台出的赠金（历史平台优惠码）收回了不还给任何人。
  - 例：赠金 $10 全部由站长出，用户花了 $105、余额 $5：R = $5，只收回 $5，还给站长 $5，不是 $10。
  - 例：站长加赠 $10 加平台码 $10（G = $20），余额 $10：收回 $10，还给站长 $10 × $10 ÷ $20 = $5。
- 站长的消费分成不扣：站长收益是按客户的消费记的，充值本身不产生站长收益；退款收回的是没花掉的余额，客户已经花掉的是真实消费。

**余额以数据库持久化差额为准**：用户活余额为 `users.quota + users.quota_pending`。预扣、消费、返还差额和调额在用户行锁内判断并提交后才返回成功，Redis 只保留兼容投影，不能决定可花金额。批量更新时差额先写入 `quota_pending`；进程内队列只提示需要刷哪些 ID。启动和定时刷盘还会扫描非零持久化差额，在行锁内折入 `quota` 并清零，所以进程退出、队列丢失、Redis 删除、过期、断连或恢复旧缓存都不会重新放出已消费额度。

退款仍在原退款事务中直接从 `quota` 扣 R，不先强制刷盘，也不把原始库行归零。正的 `quota_pending` 可能使退款后的原始 `quota` 暂时为负；判断退款和可消费额度时继续使用两者之和，活余额不透支。退款事务失败时数据库一起回滚，不再依赖 Redis 补偿。令牌的剩余额度为 `remain_quota + quota_pending`，已用额度为 `used_quota - quota_pending`，按同样的持久化差额方式刷盘。手动替换令牌限额时先吸收旧差额对应的使用量，再设置新限额，旧消费不会再扣一次。首次从旧版切换必须按 [deploy.md 第 10 节](deploy.md#10-升级和回滚) 停写并排空全部节点，不能混跑新旧协议。

**后台**：充值对账（`/recharge-audit`，需要 财务·查看）里选订单 → 退款，填原因（必填）。确认框里写着规则；提交成功后显示「已扣回 xx 额度，应退 xx USDT/元，请在线下退款」，P = 0 时另外说明没有可退的钱。同一个幂等键重复提交只退一次，返回第一次的结果。需要 财务·退款 权限。

**记录**：

- 余额流水（`adty_balance_ledgers`，类型 `refund`）记实际扣回的 −R，退款后余额不小于 0；原因后面带一段明细，例如 `[R=40000000 G=5000000 P=35000000 refund=70 USDT]`。
- 订单扩展表 `adty_topup_orders` 的 `refund_quota`（P）、`refund_amount`（应退金额）、`refunded_at`、`refund_reason`；上游的 `top_ups.status` 改成 `refunded`。
- 接口：先读取退款预览，再调用 `POST /api/adty/admin/topup/:trade_no/refund`，请求 `{reason, idempotency_key, refund_quota}`；`refund_quota` 必须与锁定余额后重算的预览一致，否则拒绝并要求刷新。返回 `deducted_quota`（R）、`bonus_quota`（G）、`refund_quota`（P）、`refund_amount`、`currency` 和流水。

**退款追溯**：订单详情显示退款时间、扣回本金额度、应退金额和币种；后台同时显示退款原因，站长看不到后台原因。退款结果仍明确提示线下退钱。订单详情及关联流水分别在真实 SQLite、MySQL、PostgreSQL 上验证。

任务 3.5；规则 2026-11-01 改（提交 c3fbbfc52、e701bf804、547ef68a3）。和方案不一致：方案 3.5 写的是「扣回面额和加赠（余额可以变负），作废或扣回佣金」，已按用户的新规则改掉。

## 余额下限（不透支）

**做什么**：余额不够的请求直接拒绝，余额永远不会变成负数。之前审查测出：余额 $10 的用户发一个大请求，结算后余额变成 −$215。

**规则**：

- **请求前**：每个钱包请求都先预扣估算的费用（上游原有的估算：提示词 tokens 加上 max_tokens，请求里设了才算；分站请求已经按「分组倍率 × d × k」算好售价）。预扣是带条件的原子扣减（余额 ≥ 预扣额才扣），并发的请求不能一起通过。余额不够就返回上游原有的额度不足错误（HTTP 403，错误码 `insufficient_user_quota`），请求不发给上游。
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

- 后台手动调额（`model/adty_quota_adjust.go`）：「减少」不能把活余额减到 0 以下；「覆盖」的目标原始额度必须非负，覆盖后的活余额也不能为负。0 可以。这是 2026-11-06 选定的下限。在同一用户行锁内用数据库 `quota + quota_pending` 检查下限和钱包上限，原始 before/after 仍用于审计；算术溢出直接拒绝。「增加」保留减少已有债务的语义。普通用户资料更新不覆盖 `quota_pending` 和已消费的签到赠金。
- 令牌额度（`remain_quota`）结算时仍按全额扣，照上游原样；它不影响钱包余额。

**相关代码**：`model/billing_operation.go`（`ReserveBillingOperation` → `ApplyBillingSettlement`，在数据库用户行锁内按活余额扣减并记短差）、`model/billing_quota.go`、`service/adty_balance_floor.go`、`service/billing_session.go`。新写的扣费代码要走这条预扣分摊 + 结算的路，不要直接改余额，也不要用不执行余额下限准入的 `model.DecreaseUserQuota`。（`model.ChargeUserQuotaUpTo` 已在 `ISS-R4-0054` 删除，见 conventions.md 约定 C。）

2026-11-01 用户要求（提交 5a6cae6de、cc51f530e），方案里没有这一项。

## 推广返佣

**做什么**：用户分享推广链接，被邀请的人充值后，推广人按充值面额拿佣金，冻结一段时间后可以提现。只做一级。

**规则**：

- **谁有推广链接**：主站用户注册时自动成为推广员（8 位邀请码，直接是已通过）。分站客户不能成为推广员：打开推广页也不会建推广员和邀请码（`service/referral.go` 的 `EnsureAffiliate` 直接拒绝），所以主站用户不会通过分站客户的链接给分站客户计佣。
- **比例**：全局 `adty_referral_setting.default_rate`（百分数，10 表示 10%，没有默认值）；单个推广员可以单设（合作伙伴审批或后台调整时设，传空就回到全局）。下单时把推广员和比例快照进订单，之后改比例不影响已下的单。
- **佣金** = 面额 × 快照比例 ÷ 100（8 位小数），不含任何加赠。永久计佣，没有期限。
- **绑定**：打开 `/r/<邀请码>` 或任意页面带 `?ref=<邀请码>`，写一个签名 cookie `adty_ref`，有效期 `cookie_ttl_days`（默认 30 天），以最后一次点击为准；注册时绑定，绑定后不能改。拒绝自己邀请自己和互相邀请。部署时必须固定 `CRYPTO_SECRET` 或 `SESSION_SECRET`，否则重启后 cookie 全部失效。
- **冻结和结算**：入账时插一条计佣任务，主节点每 60 秒（环境变量 `REFERRAL_SETTLEMENT_INTERVAL_SECONDS`）跑一轮：先生成待结算佣金（`冻结到 = 现在 + freeze_days`，默认 7 天），再把到期的转成可提现。佣金最多晚一轮才出现。
- **提现**（`/referral` 页的提现抽屉）：
  - **转入余额**：随时、免费、免审，立即到账，额度 = 金额 × QuotaPerUnit（向下取整），写余额流水 `commission_transfer`。
  - **USDT**：先选链（TRC20、BEP20、Polygon、X Layer）再填地址，按链校验地址；TRC20使用T开头的34位base58格式，其余三网使用0x开头的40位十六进制地址。X Layer接口及存储代码为`XLAYER`，选择器、二次确认、本人记录及后台审核显示X Layer。实到 = 金额 − `usdt_withdraw_fee`；要达到 `min_withdraw_usd`；人工审核，打款时填交易哈希。格式校验不代表已确认链上地址归属，不会自动发起转账。
  - **支付宝、微信收款码**：上传收款码和收款人姓名，免手续费，要达到 `min_withdraw_usd`；汇率 `withdraw_cny_rate` 由管理员实时改，提交时直接查库，和用户看到的不一样就让用户确认新汇率，一样就锁进提现单；人工审核。
  - 防并发多提：条件更新 `可提现 ≥ 金额`，加幂等键。提交后 `cancel_window_minutes`（默认 30 分钟）内待审核的单可以撤回。驳回要填原因，会解冻并发邮件。
- **防刷**：被邀请人注册时的 IP 或设备和推广人 `self_invite_window_days`（默认 30 天）内用过的相同，照样绑定但标成自邀、不计佣；同一个推广员 `batch_register_window_hours`（默认 24 小时）内同一个 IP 的绑定达到 `batch_register_threshold`（默认 3）个，停这个推广员的结算。IP 和 UA 只存哈希。
- **合作伙伴**：用户申请，后台通过时填单独的比例，驳回要填原因。
- **站长停止计佣**：付款人或推广员开站（站点变成 active）后，他的推广链接照样能打开，但不再绑定、新订单不再带返佣快照；开站前创建的订单保留当时的快照，即使之后才支付也照常计佣。订单创建与开站使用同一用户行锁排序。关站后不自动恢复邀请返佣，要后台「恢复邀请返佣」。站长收益提现不走这套 `referral_*` 账户。分站客户充值不计主站推广佣金；一级邀请佣金见下一节。
- **退款**：按退掉的本金比例扣回佣金，见「充值退款」。

**算例**：面额 $100（不管送了多少）× 10% = $10 佣金，7 天后转成可提现。USDT 提现 $150、手续费 2：实到 148 USDT。收款码提现 $50、汇率 7.1：锁定 ¥355.00。

**后台**：推广管理（`/admin-referral`）：提现审核、合作伙伴申请、推广员（调比例、停结算、停提现、恢复邀请返佣）、计佣任务（按状态筛选，默认看失败的；失败或卡住的任务可以「重新执行」，需要「推广·调整」，已计佣或已跳过的不会重复计佣）、佣金流水（四个余额的每笔变动，可按推广员筛选）、后台操作记录（审核、推广员调整、调佣金、改提现汇率）、设置，以及「立即结算」按钮。接口 `GET /api/user/admin/referral/commission-jobs`、`POST .../commission-jobs/retry`、`GET .../ledgers`、`GET .../audit-logs`。用户侧：`/referral`（推广页和提现）、公开页 `/affiliate`、落地 `/r/$code`。

**默认**：总开关 `adty_referral_setting.enabled` 默认关；关着时点击、绑定、下单快照、推广佣金提现全部不工作；站长收益走独立的分站提现（见「站长收益提现」），不受这个开关影响。没有内置值的四项：`default_rate`（没设不能打开总开关）、`min_withdraw_usd`（没设不能提到 USDT 和收款码）、`usdt_withdraw_fee`（没设不能提到 USDT）、`withdraw_cny_rate`（没设不能提到收款码）。

**表和接口**：`referral_affiliates`、`referral_bindings`、`referral_clicks`、`referral_commission_accounts`、`referral_commissions`、`referral_commission_ledgers`、`referral_commission_jobs`、`referral_withdrawals` 等 12 张表，加上 `adty_user_fingerprints`。公开 `POST /api/adty/referral/click`、`GET /api/adty/referral/public-settings`；用户 `GET /api/user/referral/profile`、`POST /api/user/referral/apply`、`GET`/`POST /api/user/referral/withdrawals`、`POST /api/user/referral/withdrawals/:id/cancel`；后台都在 `/api/user/admin/referral/` 下。

**限制**：

- 上线时要先把批量注册阈值设成 0，真实 IP 验证通过后再改回 3（见上线清单）。代码默认是 3。

任务 6.1–6.8、11.1。和方案不一致：方案说合作伙伴比例 15–20，代码只限制 0–100（2026-11-17 用户确认保持现状，由 Root 自己填）；方案 11.1 要求「站长首次提现必须人工审核」：现在站长收益走独立提现，每笔都由 Root 审核后打款，不再转进推广佣金余额（见「站长收益提现」）。

## 分站邀请佣金

**做什么**：分站客户邀请本站客户充值后，邀请人拿到一级佣金。账户、规则和提现都跟主站 `referral_*`、站长收益提现分开。

**规则**：

- **成立**：先冻结本站收益。收益不够时，只在 Root 已授权的保证金周期额度内冻结。两边都不够就保持待覆盖，这时不能转余额，也不能提现。观察期从覆盖完成起算，天数记在佣金行上。
- **转站内余额**：扣掉对应冻结，并在同一事务里按 1 USD = 500000 给邀请人加可消费额度。
- **现金提现**：可转入余额；外部打款仅为支付宝或 USDT。只提交申请不释放冻结。待审驳回、申请人在主站撤销时限内取消、审核通过后打款失败，冻结都留着。站长标记打款完成后，冻结退回原来的收益或保证金。审核人只是该站站长。
- **拆开用**：转余额的部分扣冻结，外部打款的部分释放冻结，两边按金额分别记账。
- **退款**：按已完成本金比例。还冻着的释放；已转余额的冲回用户余额；已外部打款并释放的，不再扣站长，只记追偿。
- **未设参数**：`adty_site_invite_withdraw.min_usd`、`fee_usd`、`cny_rate` 没保存时，现金方式拒绝。不抄 `adty_site_withdraw.*`，也不调用 `Referral.CreateWithdrawal`。

**后台**：Root 在分站推广设置里保存这三项。站长在站长中心「客户」页审核。客户在钱包页申请。

**表和接口**：`adty_site_invite_commissions`、`adty_site_invite_withdrawals`、`adty_site_invite_withdrawal_items`。客户 `GET`/`POST /api/adty/site-commissions/withdrawals`；站长 `GET /api/adty/site/commission-withdrawals`、`POST /api/adty/site/commission-withdrawals/:id/review`、`POST /api/adty/site/commission-withdrawals/:id/paid`；Root `GET`/`PUT /api/adty/admin/site-invite-withdraw/rules`。

## 站长分站

**团队共享余额**：一个启用账号只能有一个活动团队。加入用一次性限时邀请。充值只由负责人记入共享额度。绑定到团队的 Key 调用只扣团队余额，不扣个人余额。协议价是站点正常售价乘团队折扣；分站结果低于供货价时按供货价计费（`max(供货价, 售价 × 折扣)`），不再拒绝请求。未绑定团队的 Key 不乘折扣。钱包页可以创建团队、查看共享额度和折扣，负责人可以发一次性邀请，成员可以用邀请码加入。页面不能自选站点。负责人注销或关闭团队后余额保留，Key 停用。分站关闭后存续团队迁到主站，旧 Key 停用，余额和成员关系不拆。

**优惠码已删除**：报价、下单、钱包和后台都没有优惠码；站长加赠只有支付方式加赠一种。

**充值额度兑换码**：全平台只有一种兑换码。Root 先设置主站发行额度后，才能发主站用户本金码或指定分站站长保证金码。站长用本站可用收益冻结后，发本站客户码或本人消费本金码。兑换成功写订单和流水，不加赠、不计邀请佣金。完整码只在生成时返回一次。未兑换码可以冻结，冻结后不能兑换，取消才释放来源。Root 可以授权站长用保证金发行。关站后未兑换码由主站承接原受众，不变成主站通用码。

**站长不可更换**：正式站长不能更换。`POST /api/adty/admin/sites/:id/owner` 仍只对 Root 和 `option:write` 开放，但调用一律拒绝，不改站长、协议、员工、会话或余额。分站详情不再提供更换表单。历史 `adty_site_ownerships` 记录保留，不作为新的转让入口。

已有交接记录仍按原财务归属解释：新请求、异步任务和充值冻结实际接受时的财务站长；后续调整、到账、加赠及退款沿原单归属。旧记录未记归属时仅归首份交接记录的原站长，不按秒级时间推断。当前产品不再产生新的站长交接。站长端不返回经营钱包和充值汇总；后台三钱包记账保留。站长收益钱包、结算和独立提现见「收益、冻结、结算、提现」和「新规范实施状态」。

**做什么**：用户可以开一个分站（白标），用自己的站名和 Logo 拉客户。客户付「供货价 × (1 + 加价比例)」，平台按供货价收，站长赚差价，收益按消费记账、冻结后可以提现。客户是平台的用户，钱由平台收。

**设置总则**：模块 `adty_reseller_setting`，只能在「分站管理 → 设置」（`/site-admin/settings`）或 `PUT /api/adty/admin/reseller/settings` 改（通用的 `PUT /api/option/` 拒绝这个前缀，防止绕过校验）。六个钱的参数**没有默认值**：`cost_factor`、`min_cost_factor`、`max_markup`、`max_site_bonus_extra`、`trial_grant_max_usd`、`trial_grant_daily_usd`。它们都设了、并且 `root_domains` 不为空，总开关 `enabled` 才能打开。

### 开站

按最新总方案，只有 Root 能创建分站：

- **Root 直接开**：`POST /api/adty/admin/sites {user_id, subdomain, ...}`，直接开成 active；普通管理员即使有创建权限也不能调用，PAT 还需要 `option:write`。
- **旧申请处理**：`POST /api/adty/admin/sites/:id/review` 也仅限 Root，PAT 需要 `option:write`，可以同时设 d 和 k 上限。驳回后站点直接关闭，子域名不释放。
- **自助入口已关闭**：推广页不再显示开站表单，`POST /api/adty/site/apply` 一律返回 403，带邀请码也不能绕过。旧邀请码数据和内部兼容服务仍保留，不再提供用户开站能力。
- **共同条件**：分站总开关打开；用户是启用状态、不是分站客户；名下没有没关闭的站（待审核的也算）；子域名转小写后符合 DNS 规则（1–63 位、不以连字符开头或结尾）、不在保留名单 `reserved_subdomains`（默认 www、api、admin、pay、mail、news、links、status、docs、app）里、全表唯一（关闭或驳回的站也一直占着）。
- **开站协议**：内容在 `adty_legal_setting.site_agreement`（多语言条款里填），版本号 `adty_legal_setting.site_agreement_version`；申请时 `agree_version` 必须等于当前版本号。
- 开成 active 时停掉这个人的邀请返佣（见「推广返佣」）。

### 定价：d 和 k

分站价格只有一条公式，不是「客户付主站价 × k、平台收主站价 × d」两套独立价格（`service/adty_site_markup.go:18-27,167-184`）：

```
供货价 = 分组倍率 × d
售价   = 供货价 × (1 + 加价比例) = 分组倍率 × d × k
```

「分组倍率」是上游按分组算出的基础倍率（主站价 = 基础价 × 分组倍率）。存储字段仍叫 k、按**倍率**保存：**1 表示不加价**，1.2 表示加价 20%；计费时加价比例 = k − 1（`service/adty_site_markup.go:139-142`）。k 小于 1 表示低于供货价卖，收益会是负数。

- **d（供货倍率）**：管理员设，正数即可，**可以大于 1**，不再要求 ≤ 1（`service/adty_site_markup.go:26,115-137`）。取值：单站 d → 等级/业绩档位的 d → 全局 `cost_factor`；结果低于 `min_cost_factor` 时抬到下限。管理员用 `PUT /api/adty/admin/sites/:id` 改单站 d。
- **k（站长加价倍率）**：站长设，必须是正数且不超过上限。上限取「单站上限、等级上限、全局 `max_markup`」三者中最小的；实际扣费时还会按当前上限再截一次（`service/adty_site_markup.go:394-431`）。
- **按模型加价**：同一张定价表 `adty_site_prices` 可以有模型级行。解析顺序：客户+分组 → 客户 → 全站+分组 → 全站 → 1；带模型名的行优先于同层级不带模型的行（`service/adty_site.go:973-1018`）。价格页按模型有效 k 相对同分组通用 k 的比例缩放该模型标价（`service/adty_site_markup.go:930-979`）。
- **改价**：第一次设置或降价立即生效；涨价写待生效，`markup_increase_delay_hours`（默认 24）小时后由主节点每 5 分钟的任务生效；涨价没生效前又降价，取消待生效值（`service/adty_site_markup.go:433-623,874-897`）。active 站和遗留的 paused 行可以改价，pending、closing、closed 拒绝。保存带版本号，别处改过返回冲突。
- **团队协议价**：分站团队折后价 = 分站普通售价 × 团队支付比例，最终价不得低于供货价：`max(供货价, 售价 × 比例)`；主站团队没有这个下限（`service/adty_site_markup.go:268-329`）。
- **扣费与显示**：d、k 只乘进分组倍率，文本、按次、阶梯、图片、任务、Realtime 全走同一入口；两个值只记在日志的 `admin_info`，客户看不到。价格页和分组列表按分站售价显示。

**算例**（主站基础价 $0.40，分组倍率 1）：

- d = 0.7、k = 1.5：供货价 = $0.40 × 0.7 = $0.28；售价 = $0.28 × 1.5 = **$0.42**；平台收 $0.42 ÷ 1.5 = $0.28；站长赚 $0.14。
- d = 1.1、k = 1.2（d > 1）：供货价 = $0.40 × 1.1 = $0.44；售价 = $0.44 × 1.2 = **$0.528**；平台收 $0.528 ÷ 1.2 = $0.44；站长赚 $0.088。
- 同一个站再给某个模型单独设 k = 1.8：该模型售价 = $0.40 × 0.7 × 1.8 = **$0.504**，其他模型仍是 $0.42。

### 加赠和试用额度

- **加赠**：按支付方式，不按金额档位。Root 先逐站授权支付方式（`site_method_auth`），站长再在站长中心 → 加赠（`/site-owner/bonus`）设本站比例（`site_method_bonus`，要重新验证身份）。上限、收益保障、赠金冻结和待履约见「充值和加赠」。旧站点金额档位 `adty_sites.bonus_rates` 不再接受新保存（`SaveAdtySiteBonusRates` 返回 `ErrAdtyAmountTierRetired`）。
- **试用额度**：站长在客户详情里给单个客户送（`POST /api/adty/site/customers/:id/trial`，要重新验证身份），单次不超过 `trial_grant_max_usd`、本站每天合计不超过 `trial_grant_daily_usd`，任一没设就不能送。钱从站长的推广佣金可提现余额里扣（`referral_commission_accounts.available_amount`），不够就拒绝；给客户加额度、写余额流水 `site_trial`，并记一条已结算的 `trial` 日汇总行（只做报表，结算任务不再取它）（`service/adty_site_earning.go:381-506`）。

### 收益、冻结、结算、提现

- **记账**：每笔消费按扣费当时的 k 拆：客户付 q（额度按 QuotaPerUnit 折成美元），平台供货成本 = q ÷ k，站长收益 = q − q ÷ k（`service/adty_site_earning.go:35-54`）。d 已经包含在 q 里，不再单独参与拆分；k 小于 1 时收益为负。余额下限生效时按实际扣到的额度记（见「余额下限」）。按「站、客户、日期、类型」累加进日汇总表 `adty_site_earning_daily`，类型有 usage、refund、bonus、bonus_reversal、trial、skipped。渠道测试不记账；关掉消费日志也照样记账。
- **风控**：客户在分站注册时判一次：和站长 `fingerprint_window_days`（默认 30）天内用过同样的 IP 或设备，标「疑似自用」；和主站某个充过值的用户设备相同，标「疑似主站迁移」。两种检查各有开关（`self_use_check_enabled`、`migration_check_enabled`，默认都开）。被标记、没复核的客户照常扣费，收益记成 skipped、不结算。管理员复核后（`POST /api/adty/admin/sites/customers/:id/review`）之后的消费正常计，还可以把没结算的 skipped 改回 usage。
- **结算**：每天第一次过 `settle_hour`（默认 4 点，服务器时区）后跑一次。pending 行里，负数随时结；正数要等日期 ≤ 今天 − `freeze_days`（默认 7）（`service/adty_site_settlement_task.go:18-33,120-138`）。站点待审、有结算暂停原因、有未复核的收益事件或消费对账对不上时，正收益继续挂起。未复核的收益事件由财务在充值对账页「待复核的分站收益」里处理：按任务一次决定原始扣费计入或不计入分站收益，必须填理由并重新验证身份（`GET /api/user/admin/finance/site-earning-reviews`、`POST /api/user/admin/finance/site-earning-reviews/resolve`）。汇总后**只记站点收益钱包**（`adty_site_wallets` 的 earnings），流水类型 `site_earning_settlement`；**不再记推广佣金账户 `referral_*`**，避免同一笔钱在两个账户各提一次（`service/adty_site_settlement_task.go:143-184`）。行改成 available。bonus / bonus_reversal 只更新日汇总报表，不再动钱包（赠金保障在充值环节已经冻结）。
- **算例**（主站基础价 $100、d = 0.7、k = 1.5、冻结 7 天）：客户扣 $105 → 平台 $70、站长 +$35；当天客户充 $100、站点 USDT 加赠 2% → −$2；送试用 $2 → −$2；当天合计 $31，10-08 04:00 后结进收益钱包。

#### 站长收益提现

- **独立链路**：不走 `/referral` 的推广佣金账户，直接从站点收益钱包提（`service/adty_site_withdraw.go`）。
- **配置**：Root 必须先设 `adty_site_withdraw.min_usd`、`fee_usd`、`alipay_cny_rate`、`usdt_usd_rate` 四项，缺一项或非正数就整条链路拒绝（`service/adty_site_withdraw.go:14-19,55-116`）。接口 `PUT /api/adty/admin/site-withdrawals/rules`（Root）。
- **方式**：支付宝或 USDT。USDT 按所选链校验地址；支付宝要收款人姓名和账号。金额不低于 `min_usd`，且必须大于 `fee_usd`。
- **实付**：`(申请金额 − fee_usd) × 对应汇率`。支付宝按 `alipay_cny_rate` 出人民币，USDT 按 `usdt_usd_rate` 出 USDT。用户提交时要带看到的汇率，汇率变了会要求确认新汇率。
- **资金**：申请时当场从 earnings 钱包余额扣下，写流水 `earning_withdraw_freeze`；幂等键重复提交返回原单。
- **状态机**：`pending` →（Root 标记打款中）`paying` →（Root 记录打款）`paid`；`pending` 可由站长 `POST /api/adty/site/withdrawals/:id/cancel` 撤销，或由 Root `reject` 驳回，两条都把金额退回 earnings（`earning_withdraw_release`）。USDT 标记 `paid` 必须填交易号；两种方式至少要有交易号或凭证（`service/adty_site_withdraw.go:236-317`）。
- **算例**：申请 $150、`fee_usd` = 2、`usdt_usd_rate` = 1：实付 148 USDT。申请 $50、`fee_usd` = 2、`alipay_cny_rate` = 7.1：实付 (50 − 2) × 7.1 = **¥340.80**。
- **接口**：站长 `GET /api/adty/site/withdrawals`、`GET /api/adty/site/withdrawals/rules`、`POST /api/adty/site/withdrawals`、`POST /api/adty/site/withdrawals/:id/cancel`；Root `GET /api/adty/admin/site-withdrawals`（全部站点，可按 `status` 筛选，带站点子域名）、`POST /api/adty/admin/site-withdrawals/:id/paying|reject|paid`。界面：分站后台「提现」分区（只有 Root 看得到），按状态筛选，标记打款中、记录打款（USDT 必须填交易哈希）、驳回（必须填理由）；站长在「收益」页看到的状态改成文字。

### 站长中心

站长中心（`/site-owner/<分区>`，侧边栏只对站长显示）：概览（收益数字、近 30 天、趋势）；客户（列表、精确查找、详情：请求记录、充值、密钥、送试用、单独定价、举报）；定价；加赠；收益提现；站点设置（品牌和 Logo、自定义域名、发件域名、SEO 开关）；收益流水（含 CSV 导出，邮箱一律打码）；开站协议（只读）。

改全站价、改客户价、改加赠、送试用、提现、删自定义域名、删发件域名，都要重新验证身份（上游的二次验证：有 2FA 或 Passkey 用它们，没有就用密码；验证一次只能用一次，绑定具体操作和对象）。

品牌图片：Logo 和独立 Favicon 均接受 PNG、JPEG、WebP，不超过 200 KB，存在 `/data/uploads/site-assets`；新上传拒绝 SVG。站长可保存最多 80 字符的网站标题和 `#RRGGBB` 主色；标题、站名、标语、公告、客服联系方式按纯文本显示。标题留空沿用站名/标语；Favicon 留空回退 Logo，再回退平台图标；主题色留空恢复平台默认。保存按当前站长归属更新，公开配置按 Host 隔离。

### 站长能看到客户的哪些信息

全局默认在 `adty_reseller_setting.visibility`，单站覆盖存在 `adty_sites.visibility`。在分站管理的站点明细里，有「分站·调整」权限的管理员可逐项选择继承全局或覆盖，并填写调整原因；保存时通过 `PUT /api/adty/admin/sites/:id` 只提交变化项。继承会移除对应的单站覆盖，不改全局设置。保存失败、后台重新取数或关闭明细时，尚未保存的草稿受到保护：

| 键 | 默认 | 打开时站长看到 |
|---|---|---|
| `customer_email` | masked | masked：`a***@域名`；full：完整邮箱 |
| `customer_balance` | on | 客户余额 |
| `customer_topups` | on | 本站订单的面额、加赠、到账额度、支付方式、状态、时间 |
| `customer_usage` | on | 累计用量 |
| `request_logs` | on | 请求时间、模型、令牌名、扣费（客户价）、token 数、耗时、是否流式（走客户自己看到的日志投影） |
| `customer_tokens` | on | 密钥名称、状态、最后使用时间 |

永远看不到：IP、设备、手机号、风控的判断依据（只给类别）、渠道、倍率、密钥本身、请求和回复内容、其他分站和主站的数据。精确查找（`GET /api/adty/site/customers/lookup?q=`）必须是完整的用户名或邮箱，每站每小时最多 `customer_lookup_per_hour`（默认 20）次，每次都记进 `adty_site_lookups`。

### 自定义域名和 Turnstile

- 要先打开 `custom_domain_enabled`（默认关），并填好 `cloudflare_apiToken`（密钥）、`cloudflare_zone_id`、`cname_target`，否则打不开。另有 `cloudflare_account_id`、`fallback_origin`（回源主机名）、`turnstile_widgets`（每个 widget 的 `id`、`site_key`）和 `turnstile_widget_secret`（widget id → secret，密钥）。
- Cloudflare Token 和 widget secret 在 options 中以 AES-256-GCM 加密；只有部署固定的 `CRYPTO_SECRET` 能解密，数据库备份不包含该 key。历史明文首次读取时在行锁内升级，重复加载保持密文不变。错误 key 或损坏密文会清空本节点此域的运行密钥并记录不含秘密的错误，其余安全设置仍可加载。Root 可替换/清空密钥；空串保存保持旧值，任何角色均不能从接口读到原值或密文。
- **两种 role**：每个域名带 `role`，建站时用 `POST /api/adty/site/domains {hostname, role}` 指定（`controller/adty_reseller.go:47-52`、`model/adty_site_domain.go:13-28`）：
  - `role=site`（默认）：站点域名，承接完整页面和账号流程——浏览、注册、登录、充值、密钥、用量都行；只有平台级管理接口（`/api/adty/admin`、`/api/option`、`/api/channel` 等）返回 403（`middleware/adty_site_context.go:115-120,168-179`）。
  - `role=base_url`：API 地址，只放行 relay（`/v1`、`/v1beta`、`/mj`、`/pg`）和探活/元信息接口（`/api/status`、`/api/ready`、`/api/uptime/status`、`/api/ratio_config`、`/api/adty/groups/display`、`/api/adty/site`、`/api/adty/pricing*`）。其他页面一律 404，其他账号接口一律 403（`middleware/adty_site_context.go:100-114,181-196`）。
- **流程**：站长在站点设置里填子域名（不能是根域名，不能在我们的根域名、主站域名、CNAME 目标、回源主机名下）→ 后端调 Cloudflare 建自定义主机名（HTTP 验证、DV 证书），3 秒后再查一次取验证记录 → 站长中心显示「把这个域名 CNAME 到 `cname_target`」和要加的 TXT 记录 → 主节点每 5 分钟查一次，主机名和证书都是 active 才启用；72 小时还不通标失败，站长重新提交就是重试。
- **Turnstile**：域名启用后，按 `turnstile_widgets` 的顺序登记进第一个还有空位的 widget（每个 widget 最多 10 个域名）；名额满了域名照常能用，之后每 5 分钟重试登记。`/api/status` 按当前站点返回对应的 site key，校验时用对应的 secret，并核对 Cloudflare 返回的主机名。
- **`/api/status` 的 `server_address`**：优先返回这个站已启用的 `role=base_url` 域名（专门给客户端复制），没有时用访客当前所在的站点域名，最后回落到平台子域名（`service/adty_site_resolve.go:350-389`）。
- 表：`adty_site_domains`（`role` 列）。接口：`GET`、`POST /api/adty/site/domains`，`DELETE /api/adty/site/domains/:id`（要重新验证）。
- 2026-11-01 还没用真实的 Cloudflare 账号测过，代码按官方 API 文档写，上线前按任务 11.0 实测。

### 发件域名

- 要先打开 `sender_domain_enabled`（默认关），并在「系统设置 → 邮件」填好上游的 `ResendApiKey`。
- **流程**：站长在站点设置里填域名 → 后端在 Resend 建域名，把要加的 DNS 记录给站长看 → 主节点每 10 分钟查一次，验证通过就启用。启用后，这个站客户的系统邮件用「站名 <noreply@站长域名>」发；没验证或停用时用平台发信地址、显示分站站名。营销发送任务已删除。
- Resend 套餐有域名数上限，满了提示「No sender domain slots are left」。关站时自动删除。
- 表：`adty_site_sender_domains`。接口：`GET`、`POST`、`DELETE /api/adty/site/sender-domain`（删除要重新验证）；管理员停用 `DELETE /api/adty/admin/sites/:id/sender-domain`。

### 关站和暂停

- **Root 关站**：站点明细里的「关站」（`PUT /api/adty/admin/sites/:id/status`，`status=closed`）仅 Root 可执行，普通管理员获授「分站·调整」也不能安排或立即关站。客户固定由主站继承，可以立即执行。站点先变成关站中，`close_notice_days`（默认 7）天后由主节点执行。前端同步隐藏普通管理员入口；弹窗打开或请求排队后 Root 身份失效，也不发送关站请求。
- **执行时**：客户改成主站价；关站不扣回也不作废收益：已结算的留在站长的收益钱包（可提现），未结算的保持 pending 等自然结算；删除自定义域名和发件域名；平台子域名的页面跳到主站。
- **迁移凭证**：客户归属变更与旧 API Key 软删除在同一事务内提交；旧 Key 永久失效，普通编辑不能重新启用。Key 行、历史 ID、额度和消费日志保留，客户在新站点重新创建 Key。创建 Key 与迁移使用同一用户锁：先完成创建的 Key 随迁移撤销，迁移后创建的保留。迁移同时递增用户认证版本，拒绝旧用户缓存与旧浏览器会话，客户需重新登录；PAT 凭证本身不因此撤销。事务回滚时归属、Key 和版本一并回滚，已发布的临时认证屏障到期前可能暂时拒绝鉴权。
- **迁移验收边界**：上述凭证撤销不代表资金来源、赠金、冻结金额、未结佣金、提现及在途请求/任务已经完成结清验收；切换时点与对账规则仍待确认，不据此关闭 FIN-I-06。
- 关站中：客户照常使用，不能注册新用户，站长不能改价。
- **暂停已退役**：暂停/恢复的写入口不再接受（遗留 `status=paused` 的行只能由 Root 用 `POST /api/adty/admin/sites/:id/paused-repair` 一次性修复成 active 或关站）。遗留 paused 行按 active 处理：客户照常使用、收益照常结算、站长可改价和提现（`service/adty_site.go:738-770`、`service/adty_site_settlement_task.go:107`）。
- 站长申请关站有接口 `POST /api/adty/site/close`，只记下申请时间，站点状态不变。

### 按域名识别站点、注册和登录

- 每个请求按实际 Host 头识别主站、已启用自定义域名或「子域名.根域名」；未知 Host 的页面和 API 都返回 404，不受旧 `unknown_host_mode` 配置影响。关闭分站的 API 返回 403，页面继续跳到客户的新归属。分站总开关关闭时，所有非主站 Host 都返回 404。
- 在分站注册的用户归这个站（`users.site_id`），以后不能改；分站客户不走推广绑定。
- `site_login_only`（默认开）：分站客户只能在自己的分站登录，主站用户只能在主站登录；不符时的提示和密码错一样，不暴露账号存在。API Key 独立按所属站点绑定实际 Host：主站 Key 只用于主站，分站 Key 只用于所属站的有效域名，跨站返回 403，未知 Host 返回 404；只读用量、日志和模型接口也执行，不能用 `X-Forwarded-Host` 绕过。
- 分站的 `/api/status` 返回分站的站名、Logo、`site_title`、独立 `favicon`、`theme_color`、地址和 Turnstile site key，并把历史第三方登录字段（GitHub、Discord、LinuxDO、Telegram、微信、OIDC 等）清成关闭、关掉 Passkey。前端统一应用标题、图标、主色及浏览器主题色；主题切换保留站点主色，前景按亮度选择黑或白。
- 邮件链接、站名、支付跳回地址只从站点表取，不读请求的 Host 头。找回密码按用户所属站点生成链接。
- **支付跳回**：易支付付完回到主站的 `/pay/result/<订单号>.<签名>`，签名对就 302 到订单所属站点的钱包，不对就去主站钱包。
- **SEO**：分站首页默认 noindex，站长可以在站点设置里打开收录（`adty_sites.seo_index`），打开后 canonical 指向分站自己的地址；分站的 `/robots.txt` 没开收录时整站 Disallow。

### 主站的分站管理

分站管理（`/site-admin/<分区>`），权限按「分站」的 read、create、review、adjust、delete、settings_read、settings 细分：

- 站点列表和明细（状态、站长、域名、d、k 上限、客户），通过或驳回旧申请，改 d 和 k 上限，关站，一键下线公告（`DELETE /api/adty/admin/sites/:id/notice`，只清空公告，写审计）。
- 设置：`adty_reseller_setting` 的全部填写项（Cloudflare Token 只写不读；没填时显示「未配置」，依赖它的开关打不开），用量卡片（Resend 发件域名按状态计数和最近一次「名额已满」的时间；每个 Turnstile widget 已登记的域名数 / 10，第一个 widget 含主站域名和根域名，剩余少于 5 个时提醒）。
- 邀请码的增删改查。
- 每个写操作都写审计（`adty_site_audits`）。

### 当前实现与边界（2026-11-09）

- 推广中心已移除申请开站表单；分站管理的直接创建表单和旧申请审批动作仅 Root 可见，提交前再次检查当前角色，后端独立强制 Root 权限。站长敏感操作继续按当前协议版本校验。
- 本轮验证覆盖返佣下单快照、Host与Key隔离、Root开站和旧审批，以及只读可见性设置。SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 17.11 已实际执行返佣日限额及开站并发矩阵；不据此扩大为所有资金行为的三库验收。
- Linux副本中 `bash scripts/adty/check.sh --backend` 和 `--frontend` 均退出0；前端295文件、3341项测试通过，1194个ADTY语言键八语检查通过。生产浏览器门禁 `bash scripts/adty/browser-business.sh` 在隔离PostgreSQL 17与Redis上通过，覆盖真实页面下单、续付、验签、入账、回调幂等、取消、审核记录、可见性保存及权限；仅外部支付网关使用mock。
- 直接开站已与佣金转余额统一为先锁推广员、再锁用户的顺序；开站失败回滚站长标记和新账户。MySQL 开站事务使用 READ COMMITTED，资格检查包含等待用户锁之前已提交的待审核站点；并发推广页创建记录仍补齐站长标记，重开保留首次时间且阻止并行恢复返佣清零。真实三库回归、历史审批锁等待和本轮完整后端门禁通过；原 PostgreSQL 死锁、MySQL 双站及并发漏标记均先观察失败后修复。
- 以上是本轮局部证据，整体仍为“代码级验收未通过”。团队、通知、员工权限等总方案缺口尚未闭环；三钱包和收益提现的实现状态见「新规范实施状态」和「站长收益提现」。外部 DNS、Cloudflare、SSL 及支付供应商仍待实际联调。
- 关站前客户通知、涨价前客户通知、改 d 后站长通知已接入站点系统邮件；营销发送任务已删除。发送结果及失败可重试状态按邮件任务记录。
- 每月业绩档位任务已注册，按上月业绩计算 `tier_cost_factor`，未达到档位时恢复默认规则。
- 总览、风控复核和举报、经营数据均已接入实际接口；站点列表显示本月充值、营业额和收益，排序由后端执行。
- 站点明细提供自定义域名、发件域名停用操作。域名生命周期使用数据库锁，并在恢复外部创建操作时核验站点、域名和 generation 所有权。
- 站长可以申请关站，后台和站长中心展示关站中状态；关站中的站点仍可进入站长中心处理已有业务。
- 分站客户注册表单展示当前条款并记录接受版本和时间；条款升级后，提交任务、协议请求和 WebSocket 消息也校验当前版本。
- `customer_usage` 客户列表字段是累计用量；请求记录按客户查询，经营数据另按模型和日期统计。

任务 11.1–11.11。和方案不一致的还有：方案的键名 `cloudflare_api_token` 在代码里是 `cloudflare_apiToken`；方案写分站设置只有超级管理员能改，代码按「分站·设置」权限；推广佣金的首次提现人工审核已经去掉（a75c8b3da，用户 2026-11-01）；站长收益不走推广佣金，改走独立的收益提现，每笔经 Root 审核后打款（见「站长收益提现」）；退款时站长加赠按收回比例还（见「充值退款」）。

### 新规范实施状态：等级和三钱包（2026-11-09 起）

工作区《ADTYAI_分站系统开发规范》是新的业务基线。旧 P11 说明描述的兼容流程已被下面的实现替换；差异和风险清单见 [site-system-implementation.md](site-system-implementation.md)。

- **等级自动考核已实现**：动态等级定义存在 `adty_site_tiers`，Root 用 `GET/PUT /api/adty/admin/reseller/tiers` 管理代码、名称、供货倍率、加价上限和保证金要求，不硬编码等级数值。主节点每小时跑一次自动考核（`service/adty_site_finance.go:795-833`），按 UTC 日期做周期 ID，同一站同一周期重复跑不会重复写等级、保证金要求或预警（`service/adty_site_finance.go:715-784`）。门槛是开站以来累计的三个指标：有效客户数、留存本金、已结算消费；取所有已启用等级里三项都满足的最高档（`service/adty_site_finance.go:580-661`）。手动钉住的站（`tier_mode=manual`）不会被自动考核覆盖；三项都达不到时通知 Root「未达最低档」，只预警、不关站（`service/adty_site_finance.go:766-793`）。界面：Root 在分站后台「设置」页的「分站等级」里新增、编辑、停用等级，按代码保存，不能删除；分站详情的「分站等级」可以把站点固定到某个已启用等级，或改回自动考核（`PUT /api/adty/admin/sites/:id/tier`，Root，PAT `option:write`；改回自动会先清空等级，等下一次整点考核）。
- **等级落地**：切换等级会在同一事务更新站点的 `tier_cost_factor`、`tier_max_markup` 和保证金钱包的 `required_balance`；保证金要求变更只改应缴额，不会凭空增加余额（`service/adty_site_finance.go:679-698`）。手动等级模式不受旧月度业绩档位和旧延迟生效任务覆盖。
- **三钱包已实现**：`adty_site_wallets`（经营 / 收益 / 保证金）和 `adty_site_wallet_ledgers`。钱包唯一绑定站点和类型，变动在事务内锁钱包并追加含前后余额、业务来源和幂等键的流水；流水禁止通过模型更新或删除，重复幂等键返回原流水。站点查询 `GET /api/adty/site/wallets`、`GET /api/adty/site/wallet-ledgers`（PAT `wallet:read`）；Root 查询 `GET /api/adty/admin/sites/:id/wallets`、`GET /api/adty/admin/sites/:id/wallet-ledgers`（PAT `option:read`），普通管理员不能使用。界面：Root 在分站详情的资金区看三个钱包的余额和流水；站长在「收益」页看收益钱包、保证金钱包的余额（含冻结）和流水，看不到经营钱包；两边都能按钱包筛选、分页。
- **经营钱包本金已联动**：客户充值成功时按面额把本金记进经营钱包（`principal_credit`），消费结算时按「站点本金垫付」的部分扣经营钱包（`principal_consume`）。扣款允许透支，余额变负是对账信号，不是拒绝计费的理由（`service/adty_operating_principal.go:14-51`）。
- **日结单已实现**：`adty_site_settlement_sheets` 保存每个站每个营业日的核对单，主节点每 10 分钟补齐到昨天，重复跑只刷新数字、不动钱包（`service/adty_site_settlement_sheet.go:56-171,379-395`）。营业日时区由 Root 存 `adty.business_timezone`，没设按 UTC（`service/adty_site_settlement_sheet.go:15-54`）。站长 `GET /api/adty/site/settlement-sheets`，Root 可查全部。
- **旧站兼容**：查询旧站时缺失的钱包显示零余额，不创建余额或历史流水；收益钱包只投影功能启用后的新结算，不回填旧推广佣金账户。
- **记账口径**：所有钱包、收益和结算金额按 USD 记账；规范里的人民币保证金示例没有写成美元默认值。`AllowOverdraft` 目前只给经营钱包和结算路径内部使用，不是面向客户的 Root 授权透支产品。

## 分组和后台权限

**分组**：

- 每个分组有系统生成的固定 ID（`g` 加 4 位随机字符）。正式显示名和说明只有简体中文和英文，稳定 ID 不翻译。旧的其他语言字段可以留存，但不作为正式语言。用户、密钥、渠道里存的都是 ID，后台改显示名不影响他们；前台到处只显示显示名，不露 ID。
- 改法：在后台原来的分组倍率表上编辑显示名（系统设置 → 计费）。上游的 JSON 编辑模式关掉了，保存时校验：出现未知的分组 ID、或者删掉了还在用的分组，都会被拒绝。
- 迁移和删除：可以把一个分组的用户、密钥、渠道整体迁到另一个分组，迁完才能删（`POST /api/adty/admin/groups/:id/migrate`、`DELETE /api/adty/admin/groups/:id`）。
- 接口：公开 `GET /api/adty/groups/display`（已删除分组的名字也返回给登录用户）；后台 `GET`、`POST /api/adty/admin/groups`、`PUT /api/adty/admin/groups/:id`，都只有超级管理员。

任务 2.1–2.5。

**后台权限**：

- **超级管理员**（root）什么都能做。**普通管理员默认没有任何后台权限**，由超级管理员在「用户 → 编辑」里逐项打开，随时可以关。系统设置里的所有页面、易支付通道的新增和密钥、管理员任免、分组、官方价只有超级管理员能做。
- 权限按「区域·动作」分，我们加的区域：

| 资源（权限项前缀） | 动作 |
|---|---|
| 用户 `adty_user` | `read` 查看（列表、详情、余额流水）、`create` 新建账号、`write` 编辑（资料、启停、重置 2FA 和 Passkey、解绑、改密码）、`group` 改分组、`quota` 调余额、`delete` 删除 |
| 日志和统计 `adty_log` | `read` 查看所有人的日志、任务和全站统计；`export` 导出所有人的用量日志 |
| 模型 `adty_model` | `read` 查看；`create` 新建模型、厂商、预填分组（含上游同步新增）；`write` 编辑；`delete` 删除模型、厂商、预填分组 |
| 充值财务 `adty_finance` | `read` 查看订单和充值对账；`manual_credit` 手动补单；`refund` 退款 |
| 推广返佣 `adty_referral` | `read` 查看；`review` 审核合作伙伴申请和提现；`payout` 标记已打款；`adjust` 调整佣金、冻结结算、比例、停用推广员；`settings_read` 查看设置；`settings` 修改设置（含提现汇率） |
| 支付通道 `adty_payment` | `read` 查看状态；`operate` 启停和测试 |
| 分站 `adty_site` | `read` 查看站点、客户、统计；`create` 建站；`review` 审核旧申请、复核风控客户、处理举报；`adjust` 改 d、k 上限、可见范围、关站；`delete` 删除站点附属记录（域名、发件域名、公告）；`settings_read` 查看分站设置；`settings` 修改分站设置 |

开站、旧申请激活和关站迁移仍只对 Root 开放，拥有 `adty_site:create` 的普通管理员也不能调用。

上游原有的「渠道管理」「审计日志」「任务插件」三个区域照旧。

- **预设**：编辑管理员时可以一键套用「客服」（用户·查看、日志·查看、充值财务·查看、推广·查看）、「运营」（客服加上 用户·编辑、渠道·查看和操作、模型·查看和编辑、支付通道·查看和操作）、「财务」（用户·查看和调余额、充值财务三项、推广的查看、审核、打款、调整）。套用会替换整张权限矩阵，套用后还能逐项改。
- 侧边栏和页面按权限显示；没权限调到的接口返回 403，错误码 `ADTY_PERMISSION_DENIED`。

**权限目录迁移（catalog v2）**：启动时 `MigrateAdtyActionCatalog` 会把旧的粗动作一次性展开成新动作——`adty_user:write → adty_user:create`、`adty_model:write → adty_model:create/delete`、`adty_log:read → adty_log:export`、`adty_site:adjust → adty_site:create/delete`、`adty_site:settings → adty_site:settings_read`、`adty_referral:settings → adty_referral:settings_read`，渠道的敏感写也会展开（`service/authz/action_migration.go:14-73`）。展开只在版本号从旧值升到 2 时跑一次；完成状态存 `authz_action_catalog_states`（键 `adty_action_catalog`），之后撤销的新动作不会被再次派生。库里的版本比程序支持的版本新时拒绝启动；副本也要等迁移完成后才能加载权限（`service/authz/action_migration.go:75-86`）。

任务 2.6、2.7。

## 登录方式

- **账号密码**是唯一的登录凭据（`POST /api/user/login`），没有第三方账号登录。
- **邮箱验证**用于注册和绑定/改绑邮箱：开启 `EmailVerificationEnabled` 时，注册必须提交邮箱和验证码；绑定邮箱走 `/api/oauth/email/bind/*`。它不是独立的登录方式。
- **Passkey**：用户可以在个人页注册，之后用 `/api/user/login/passkey/begin|finish` 免密码登录；分站关闭 Passkey（`/api/status` 的 `passkey_login` 为 false）。
- **TOTP**：登录第二因子（`POST /api/user/login/2fa`、`POST /api/user/login/verify`），也用于敏感操作的二次验证。
- **没有第三方登录**：GitHub、Discord、LinuxDO、微信、OIDC、自定义 OAuth 和 Telegram 登录都已删除，OAuth 框架（`oauth/` 包、`/api/oauth/state`、`/api/oauth/:provider`、Telegram 旧路由）也已删除。二次验证只有密码、邮箱验证码、TOTP 和 Passkey，不再有“已绑定的第三方账号”这一项。
- **Telegram 只做账号绑定和通知**：用户 `/api/adty/telegram/bind/start|confirm`，站长 `/api/adty/site/telegram/bind/start|confirm`。不做登录、找回密码或二次验证。
- 历史 OAuth 身份行保留，升级不删除；新登录不再使用。分站 `/api/status` 会把历史第三方登录字段统一置 false（`service/adty_site_resolve.go:423-426`）。

## 营销模块删除与优惠码退役

按最新产品范围，独立营销中心、营销群发和自动邮件、注册营销同意、个人营销偏好、注册来源采集及统计已删除。`/marketing` 无页面，旧 `/api/adty/marketing` 与 `/api/adty/admin/marketing` 命名空间返回 JSON 404；插件及外置前端不能接管这些入口。旧营销权限不再列入权限目录。

**付费充值优惠码已删除**：管理接口和面板、开关 `adty_marketing_setting.promo_enabled`、站长二次验证动作 `promo` 先删（2026-11-16）；随后报价和下单的优惠码参数、只对带码请求限速的中间件、订单上的优惠码字段、入账时的核销、站长收益和赠金预留里的站长出资优惠码、订单详情的「历史优惠码赠金」分区也一并删除。新库不再创建 `adty_promo_codes`、`adty_promo_redemptions`；升级库里的旧表和旧行原样留着，代码不再读写。历史优惠码审计事件标识保留。

**现在能主动加额度的只有充值额度兑换码**（`adty_voucher`）：Root 设主站发行额度后，发主站用户本金码或指定分站的保证金码；站长用本站收益或 Root 授权的保证金冻结后，发本站客户码或本人消费码。兑换成功写正式订单、余额流水（`voucher`）和资金批次，**不加赠、不计邀请佣金**；完整码只在生成时返回一次，未兑换的可以冻结或取消，取消才释放来源。关站后未兑换的客户码由主站承接原受众，不会变成主站通用码。详见下面的「充值额度兑换码」。

新库不再创建营销联系人、群发、发送、收件人、webhook、触发记录和注册来源七张表。升级不删除或改写这些旧表及历史行；优惠码与核销表也不再新建，升级库里的旧表同样原样保留。真实 SQLite 3.50.4、MySQL 8.4.13、PostgreSQL 17.11 已覆盖本次迁移入口的新建、升级及两次迁移，保留历史数据和唯一约束。

完整启动的条件唯一约束已统一处理方言：SQLite/PostgreSQL 使用部分索引；MySQL 使用保留原字段类型、字符集与排序规则的可空 STORED 生成列加完整唯一索引。OAuth 各提供方的非空身份不能重复，软删除不释放身份；活动插件、未删除模型名与 Root 沿用原有条件。历史 OAuth 重复会阻止迁移，不自动归属账号。MySQL 新建表默认 DYNAMIC 行格式以容纳现有 Passkey 索引，调用方明确的行格式及其他表选项优先保留。

上述三库及实际 MySQL 5.7.8-rc 已验证完整主库和独立日志库的新建、代表性历史升级、两次迁移、数据与唯一约束保留；还覆盖并发 OAuth 绑定、源字段扩容/排序规则变化和表选项。5.7.8-rc 的虚拟生成列索引曾在原生 SQL 与完整升级回归中漏掉已有身份，故采用 STORED；修复后完整升级连续 12 次拒绝重复。验证范围仅限本次启动与约束变更，不代表全平台数据库业务验收。

系统通知、公告、验证码、找回密码、充值到账、额度提醒、站点经营通知及发件域名管理保留。外部 Resend 与正式支付供应商仍需独立联调验收。

## 网页客服、状态页、多语言、首页 SEO

**网页客服（tawk.to）**：旧挂件配置仍可能存在，但不是统一客服中心。当前客服入口是站内工单；附件只允许图片。不要把 tawk.to 当成工单；Telegram 只做绑定和通知，不做登录。

**服务状态页**：不建设公开平台服务状态页。前端原有的 `/status` 页面已删除（2026-11-16），维护页也不再链接它；`/api/uptime/status` 只给后台仪表盘用，不作为正式产品承诺。

**多语言**：正式语言只有简体中文和英文。浏览器偏好为中文时用简体中文，否则用英文；手动选择优先。`zh-TW`、`fr`、`ja`、`ko`、`ru`、`vi` 的语言包和文档站内容已删除，仓库只保留两种正式语言；带这些语言码的请求和旧偏好按中文变体归简体中文、其余归英文处理。固定通知使用中文在前、英文在后的同一条模板。

**首页 SEO**：

- 8 个首页网址：`/`（英文，另有 x-default 指向它）、`/zh-cn`、`/zh-tw`、`/fr`、`/ja`、`/ko`、`/ru`、`/vi`。用户选过语言、或者已登录且界面不是英文时，打开 `/` 跳到对应语言的首页，原来的查询参数保留；跳转用路由目标，不再把查询参数拼进路径，避免预加载反复请求 `/`。搜索引擎和第一次来的访客看到英文。
- Go 返回首页时写进标题、描述、canonical（不带查询参数）、8 条 hreflang、Open Graph、Twitter 卡片，只在 `/` 写 WebSite 结构化数据；正文在构建时预渲染进 `web/dist/adty-home.json`，插在 `#root` 前面。
- 后台「系统设置 → 站点 → Home Page SEO」：`adty_seo_setting.home_title`、`home_description`（按语言，没填用英文的，英文也没填时标题用 ADTY AI）、`og_image`（默认 `/adty-og.png`）。说明写的是 8 个首页网址。一次保存多个字段时，不会因为中途刷新设置把还没保存的草稿覆盖掉；站点设置页也一样。
- 其余页面一律 noindex。服务器地址（`ServerAddress`）没设或还是 `http://localhost:3000` 时，不写 canonical、hreflang 和结构化数据；请求的域名和服务器地址不同时首页加 noindex。分站见「站长分站」。
- 落地页保持白底。页面只渲染五段：Hero（首屏）、Migration（迁移说明）、Endpoints（接入地址）、AffiliateBanner（返佣）、ClosingCta（收尾 CTA，标题是「Start with one request. / Decide about the invoice later.」），见 `web/src/features/adty-home/index.tsx:43-51` 和 `web/src/features/adty-home/components/closing-cta.tsx:47-49`。Hero 的「Read the docs」指向 `/docs`；三步、热门模型、卖点、常见问题等旧区块不在当前首页渲染。
- `/robots.txt` 和 `/sitemap.xml` 都由 Go 返回。主站 robots 屏蔽 `/api/`、`/v1/`、`/pay/`，sitemap 只列允许索引的首页语言网址并附 hreflang；没开 SEO 的分站整站 `Disallow: /`，不列入 sitemap。
- 方案第八节原先要求在 Nginx 配置 robots 和 sitemap；当前由应用按主站、分站和 SEO 设置生成，反代无需复制固定文件。

任务 9.3。

## 限流、脱敏和其他安全

**按用户限流**：`adty_limit_setting.enabled`（默认关）、`default_rpm`、`default_tpm`、`default_concurrency`（默认 0 表示不限）；单个用户在「用户 → 编辑」里单独设（-1 跟默认、0 不限），存在 `adty_user_limits`。固定 1 分钟窗口；TPM 在写消费日志时累计（关掉消费日志也算）；并发按请求登记，请求结束释放。超限返回 HTTP 429 和 `Retry-After`，错误类型 `adtyai_error`、错误码 `rate_limit_exceeded`。开了 Redis 就只用 Redis 计数，不退回进程内存；客户端没接上或计数出错时返回 HTTP 503，错误码 `rate_limit_unavailable`，不放行。没开 Redis 时用进程内存。后台：系统设置 → 安全 → 用户限流。接口 `GET`、`PUT /api/adty/admin/users/:id/limits`。限制：TPM 是事后累计，同一分钟里同时进来的大请求拦不住；跨分钟边界时短时间内最多约 2 倍上限；同一用户所有令牌共用计数；演练场 `/pg` 没挂。任务 7.4。

**客户接口脱敏**：客户自己看到的日志去掉渠道 ID、渠道名、上游请求 ID，`other` 按白名单只留用量和状态（去掉倍率、模型价、上游模型名、`admin_info`）；任务列表不露渠道和上游模型名；任务提交不再返回 `X-New-Api-Other-Ratios` 头。作用于 `GET /api/log/self`、`/api/log/self/export`、`/api/log/token`、`/api/task/self`、`/api/mj/self`、站长看客户请求的接口。管理员的 `/api/log/`、`/api/task` 照常显示渠道。按接口脱敏，不按角色：管理员调 `/api/log/self` 看到的也是脱敏后的。任务 7.1。

**人工调额留痕**：后台「用户 → 编辑 → 调余额」原因必填（最多 200 字），带幂等键；写审计（带原因）、用户自己的日志（不带原因）和余额流水 `adty_balance_ledgers`（带原因，只给后台看，`GET /api/adty/admin/users/:id/ledger`）。非超级管理员不能调同级或更高角色的用户。「减少」不能减到 0 以下，「覆盖」不能写成负数（见「余额下限」）。任务 7.2。

**防邮箱枚举**：邮箱已注册时，发注册验证码不发码、改发一封提醒信（10 分钟最多一封），接口照样返回成功；注册时邮箱已占用返回和「验证码错误」一样的提示；找回密码不管邮箱存不存在都返回成功；三封信都异步发。开了 Redis 但客户端没接上时，两个发码接口都返回 HTTP 503，不发信。验证码或重置码写不进共享存储时不发信；已注册邮箱和未知邮箱都返回同一个空成功，不用 503 区分。限制：用户名仍可以被枚举；登录后绑定邮箱仍会提示邮箱已被占用；提醒信的频率限制在进程内存里。任务 7.3。

**密码重置链接**：新邮件把邮箱和重置凭证放在 `/user/reset` 的 URL fragment，首次文档请求不携带它们。页面在加载资源和分析脚本前捕获新 fragment 或历史 query 链接并清除地址栏，只在当前页面内存中保留表单所需值，不写浏览器持久存储或历史状态；刷新清洁后的网址需重新打开邮件链接。Go 和随仓 Nginx 配置对重置入口及尾斜杠入口返回 `no-referrer`，Nginx 访问日志移除历史重置 query 并遮蔽来自重置页的 Referer。旧邮件首次请求仍会经过外部代理，已有日志及仓外 CDN 的处理需单独核实。

**登录失败锁定**：同一用户名密码错误累计 8 次后，15 分钟内照常返回用户名或密码错误，不再校验密码。开了 Redis 就用 Redis 共享计数；客户端没接上或读写出错时按已锁定处理，不退回进程内存。没开 Redis 时用进程内存。

**流式保活**：上游的 ping（`general_setting.ping_interval_enabled` 默认关、`ping_interval_seconds` 默认 60，后台在「系统设置 → 模型 → Global Model Configuration」的「Keep-alive Ping」）现在对所有流式格式（Realtime 除外）在请求上游之前就开始发 `: PING`，一直到第一次写出数据，最长 120 分钟，避免推理模型首字超过 125 秒被 Cloudflare 以 524 断开。上线时打开，间隔设 30 秒。限制：原生 Gemini 流式（`/v1beta`）上游本来关掉了 ping，现在首字前也会收到 ping（提前保活不看这个开关，对 Gemini 客户端的影响没有核实）；第一次 ping 发出后状态码固定为 200，之后上游再出错只能在流里返回错误事件。任务 7.5。

**Realtime 连接与用量**：双向 WebSocket 消息遵守 `MAX_REQUEST_BODY_MB` 上限（未设或非正时为 128 MiB）。客户端输入与上游输出的共享计量、工具和音频配置受同一锁保护；请求取消直接关闭连接并等待两读协程停止后结算。任一方向结束或客户端写失败时，先通知对端关闭，最多收敛 2 秒；写入上限为 30 秒。无 `response` 的 `response.done` 使用本地计量回退；预扣失败也只向最终结算累计一次该段用量。这些连接回归不代表全部协议或全平台资金验收已经通过。

**渠道配置生效**：删除、禁用、模型或凭据变更提交后，旧渠道快照立即失效；重载失败时，路由、指定渠道、轮询信息和高级配置改查数据库，读取失败则拒绝使用该渠道。普通定时同步失败仍保留未被撤销的有效快照。并发中的旧重载不能覆盖后来提交的配置；多 Key 轮询在回退及恢复后继续推进。插件批量解绑、上游模型批量应用、凭据自动刷新只完成部分写入后失败时，已提交的部分也会刷新。数据库写入已成功的删除操作不会因缓存重载失败误报为未提交。

## 统一用户查询

Root 和获授用户读取权限的管理员在统一用户列表、搜索及详情中可查看站点名称和 Site ID；站点单选筛选支持主站 `0`，可与关键词、状态、角色、排序和分页组合，刷新后从 URL 恢复。未指定站点表示全部可见用户，筛选不扩大调用者原有角色权限。站点选项只含可见用户涉及的 ID 和名称，归档用户涉及的站点也保留；主站取系统名称，分站取品牌名，空品牌名回退子域名，已不存在的历史站点仍展示 ID。

接口 `GET /api/user/sites` 复用用户读取权限（ADTY 模式 PAT 使用 `adty_user:read`）；列表和搜索接受非负整数 `site_id`。空值、负数、重复及非法参数拒绝，不默默回退全部用户。站长仍使用本站客户接口；本批未实现员工主体、团队隔离或管理员订单聚合。

## 官方价、接入文档、条款

**官方价**：主节点启动时同步一次，之后每 24 小时（环境变量 `ADTY_OFFICIAL_PRICE_SYNC_HOURS`）从 models.dev 拉取输入、输出、缓存读、缓存写的每百万 tokens 美元价，存进 `adty_official_prices`；失败保留旧数据。只更新有映射、没开「手动覆盖」的行；没映射的模型按 `adty_official_price_setting.auto_map_providers`（默认 anthropic、openai、google、deepseek、xai、mistral）找完全同名的自动映射。按次价只能手填。价格页和首页显示「省 X%」= 1 − 本站价 ÷ 官方价（按输入价、美元，向下取整到整数百分比，为负显示「—」）。后台「系统设置 → 计费 → Official Prices」（只有超级管理员）：改映射、价格、手动覆盖，立即同步。公开接口 `GET /api/adty/pricing/official`。限制：不开手动覆盖时手填的价格会被下次同步改回；没有删除接口；`auto_map_providers` 没有后台界面。任务 8.4、8.5。

**接入文档**（`/docs`、`/docs/<页>`，公开）：快速开始、Claude Code、Codex CLI、Cherry Studio、OpenAI SDK、错误和限制、常见问题。正式维护简体中文和英文，是仓库里的 Markdown（`web/src/features/adty-docs/content/`），改内容要改代码后重新构建。其他语言目录如仍存在，不在前台承诺。文中的接入地址取当前站点配置，没填用服务器地址。示例密钥使用占位符。任务 8.6。

**条款**：后台「系统设置 → 内容 → 多语言条款」可编辑用户协议、隐私政策、退款政策、开站协议和分站客户条款。正式文本是简体中文和英文。旧的其他语言字段可以保留，但不作为正式语言。开站协议和分站客户条款另有非负整数版本号；修改已发布协议后提高版本号，以要求相关用户重新接受。保存仅提交当前文档和对应版本，其他文档的未保存内容继续保留，离开页面时会提示。`GET /api/adty/legal/:doc?lang=` 按「本语言 → 英文 → 上游的 `legal.*`」回退；退款政策由 `refund_policy_enabled` 控制显示。主站发布用户协议或隐私政策后，注册必须带 `agreement_accepted: true`，否则拒绝并不建账号；接受后写系统日志。分站申请核对当前开站协议版本；分站客户使用受保护的接口、任务和长连接时核对当前客户条款版本。任务 8.6、11.1。

## 品牌和 Logo

**站名**：ADTY AI，正式语言和保留语言包都不翻译；程序对外的技术名字是 adtyai。代码里只在 `common/adty_brand.go`（`AdtyBrandName`、`AdtyBrandSlug`、`AdtyBrandURL`）和 `web/src/lib/adty-brand.ts`（`ADTY_BRAND_NAME` 等）两处写字面量。后台的系统名称默认就是 ADTY AI。

**Logo**：方案 H「层叠网关」。原稿和生成脚本在仓库外的 `C:\Users\Administrator\ai\adtyai-logo`（`source/build.py` 生成全部文件，说明见那里的 README.md）。`web/public` 里的文件：

| 文件 | 尺寸 / 格式 | 用途 |
|---|---|---|
| `favicon.ico` | 内含 16、32、48 | 浏览器标签图标（`web/index.html` 第一个 icon） |
| `favicon-16x16.png`、`favicon-32x32.png`、`favicon-48x48.png` | 16×16、32×32、48×48 | PNG 标签图标 |
| `favicon-128x128.png`、`favicon-256x256.png` | 128×128、256×256 | 高分辨率标签图标 / PWA |
| `adty-apple-touch-icon.png` | 180×180 | iOS 添加到主屏幕 |
| `site.webmanifest` | JSON | 安卓主屏和 PWA 清单，引用下面三个图标 |
| `android-chrome-192x192.png`、`android-chrome-512x512.png` | 192×192、512×512 | 安卓主屏图标（manifest 引用） |
| `maskable-512x512.png` | 512×512 | 安卓自适应图标（manifest 引用） |
| `mstile-150x150.png` | 150×150 | Windows 开始菜单磁贴 |
| `logo.png` | 522×128 | 默认 Logo（字标）：后台 Logo 没填时顶栏、侧边栏、页脚用它 |
| `logo-dark.png` | 522×128 | 深色底用的默认字标 |
| `adty-logo.png` | 1383×339 | 高分辨率字标（浅色底），后台 Logo 可填 `/adty-logo.png` |
| `adty-logo-dark.png` | 1383×339 | 高分辨率字标（深色底） |
| `adty-logo-480.png`、`adty-logo-960.png` | 480×118、960×235 | 按宽度缩放的导出 |
| `adty-mark.png` | 364×256 | 单独的图形标记（不带字标） |
| `adty-og.png` | 1200×630 | 社交分享图，SEO 设置 `og_image` 的默认值 |

2026-11-15 的 `2df2e4c` 删掉了 `favicon.svg`、`safari-pinned-tab.svg`、`adty-logo.svg`，`index.html` 也不再引用它们。现有 PNG 是从新字标导出的位图，不再提供 SVG。

`.webmanifest` 在 Go 里登记成 `application/manifest+json`（`middleware/adty_static_types.go`），否则镜像里会被当成纯文本返回。

**限制**：

- 主站后台 Logo 仍作为标签图标的兼容回退。分站可以独立设置 Favicon；延迟完成的 Logo 预加载不覆盖它，移除独立图标后回退 Logo，两者均空时使用 `/favicon.ico`。
- 控制台顶栏左上角的 Logo 盖住了「Home」导航（换 Logo 之前就有），记在上线清单里，代码审查时修。
- 公开页脚没有产品、客服、源码列，也不显示上游署名。客服邮箱不写在页脚；错误页反馈按钮仍读 `adty_site_setting.support_email`。
- 系统邮件模板还没有图片页眉；要加时用 `adtyai-logo/email/logo-email-header.png`（见上线清单）。

任务 8.1、8.2、8.9；新 Logo 2026-11-01（提交 b87c3bff8）。公开页脚和首页接入说明 2026-11-02。

## 统一订单查询

订单管理沿用 `/recharge-audit` 书签与财务读取权限，统一展示主站和分站充值订单的站点名称、site_id、类型及完成时间；支持站点（含主站 0）、类型与既有筛选组合。归属取下单时的扩展订单快照，用户迁站不改写历史；无快照的旧订单显示“归属未知”。站长中心新增本站订单页签，只读且不加载全局异常支付面板；接口按当前 SQL 站长身份和冻结 site_id 隔离，PAT 需 `wallet:read`。当前只有充值订单来源。

点击订单号打开详情：冻结订单与赠金快照、退款结果，以及独立分页的支付记录、消费余额流水、分站钱包流水、对账事件和财务审核。后台另有该订单的推广佣金与佣金钱包流水。每个分区提供真实总数，仅按订单号、供应商、用户和明确的佣金关联读取，不把同用户其他流水或分站日汇总收益算进本单。

后台详情为 `GET /api/user/admin/finance/recharge-audit/:trade_no`，站长详情为 `GET /api/adty/site/orders/:trade_no`，支持 `section`、`p`、`page_size`。站长隐藏人工付款凭证、后台原因与操作人、用户跨站余额前后值及主站推广佣金；当前账号或权限变更后立即关闭原详情。

支付事件追加到 `adty_order_payment_events`：六服务验签后的通知接收、可信查询的未付款/失败/核对拒绝、成功入账和重放。成功及重放与入账共享事务，记录失败则整单回滚；重复通知只入账一次。不保存原始载荷、商户密钥、供应商错误文本或人工凭证。新增记录以前的回调历史不能还原，页面明确提示历史记录可能不完整；现有赠金来源分配及其他资金规则仍按各自未闭事项实施，不用订单追溯替代。

## 资金与运维补充

### 日结单

每个站每个营业日一张核对单，存 `adty_site_settlement_sheets`。主节点每 10 分钟补齐到昨天，重复跑只刷新数字、不加钱也不扣钱（`service/adty_site_settlement_sheet.go:56-171,379-395`）。营业日时区由 Root 存 `adty.business_timezone`，没设按 UTC；单子保留首次生成时的时区和窗口。内容有充值面额/实付、消费、供货成本、价差、赠金冻结/出资/释放、试用、邀请佣金、待覆盖、钱包增减、提现、财务调整和异常数。站长 `GET /api/adty/site/settlement-sheets`，Root 可查全部。

### 资金异常与处置

检测 9 类问题：余额与流水对不上、未结的短差占用、保证金异常、重复入账、重复退款、成功订单未记账、赠金保障不足、站点收益与供货成本不匹配、退款金额不匹配（`service/adty_fund_anomalies.go:14-24`）。Root 用 `GET /api/adty/admin/fund-anomalies?site_id=` 触发一次检测，结果写进 `adty_anomaly_dispositions`；`GET /api/adty/admin/fund-anomalies/dispositions` 看列表，`POST .../dispositions/:id/review` 把状态从 `open` 改成 `reviewing` 或 `closed`。`reviewing` 会限制该账号/订单/站点的相关资金写入，`closed` 解除；复核动作本身不动钱包和客户额度（`service/adty_site_finance.go:326-352`）。

### 人工退款状态机

一笔订单可以有多笔人工退款，存 `adty_manual_refunds`。状态：`submitted`（申请）→ `approved`（审核）→ 回收本金 / 处理赠金 / 处理佣金 → 记录外部打款 → `completed`；任一步可 `terminated` 终止。占用按 S/H/U 三个桶算：已完成的 S、进行中的 H、外部打款金额未知的 U，三者合计不能超过原单实付本金 L；终止的行释放占用（`model/adty_manual_refund.go:12-119,235-246`）。每一步都带幂等键和安全证明，接口在 `/api/adty/admin/topup/:trade_no/refunds/:refund_id/*`，都要「财务·退款」。

### 充值额度兑换码

全平台只有一种兑换码。用途有四种：主站用户本金码、分站保证金码（Root 发）、本站客户码、站长本人消费码（站长发）。发行来源是平台发行额度、站点收益或 Root 授权的保证金；平台额度用 `adty_voucher_liabilities` 控制上限，站点来源从对应钱包冻结。兑换写正式订单（provider=`voucher`）、`top_ups` 兼容投影、余额流水 `voucher` 和资金批次；**不加赠、不计邀请佣金**。完整码只在生成时返回一次，库里只存 hash 和后四位；未兑换的可以冻结或取消，取消释放来源；同一用户 15 分钟内最多试 8 次（`service/adty_voucher.go:20-24,137-271`）。

接口与界面：

- Root：`GET /api/adty/admin/vouchers`（按 `status`、`purpose` 筛选）、`GET`/`PUT /api/adty/admin/vouchers/limit`（读取时返回额度、未兑换占用、已兑换）、`POST /api/adty/admin/vouchers`、`POST /api/adty/admin/vouchers/:id/freeze`、`POST /api/adty/admin/vouchers/:id/cancel`、`GET`/`PUT /api/adty/admin/vouchers/sites/:id/margin`。分站后台「兑换码」分区（只有 Root 看得到）管发行额度、签发主站用户码、查看全部兑换码并冻结或取消；分站详情的资金区签发保证金码，并有「允许用保证金发兑换码」开关。
- 站长：`GET /api/adty/site/vouchers` 列出本人从本站钱包签发的码，并返回是否允许用保证金发行；签发、冻结、取消都要重新验证身份（`site.owner.manage`，动作 `voucher.issue` 绑定站点、`voucher.freeze`/`voucher.cancel` 绑定兑换码）。站长中心「充值码」页签可以选出资来源（Root 授权后才有保证金）、查看已签发的码并冻结或取消。

### 资金批次与分摊

`adty_fund_batches` 按 `(user_id, source, kind)` 记录每一笔入账的本金批次和赠金批次，字段 `original` / `remaining`，`site_id` 可以为 NULL（证不出站点时不猜）。消费按 FIFO 先扣本金、再扣赠金；升级前没有批次的历史余额会补一条 `legacy` 本金批次，否则老用户一笔都扣不出去。批次扣不动时返回 `ErrAdtyFundBatchShortfall`，外层钱包事务整笔回滚；退款按 `source` 回收对应批次。站点经营钱包的本金扣减和赠金保障释放都挂在这条分摊链上（`model/adty_fund_batch.go:28-39,246-274,379-445`）。

### 数据保留与清理

Root 在「数据保留」页按分类设保留期和动作：用户资料、登录/安全记录、API 调用日志、订单及支付事件、资金流水、客服工单、通知历史、操作审计、任务产物。主节点每 24 小时跑一次系统任务，按已设策略删除过期数据；**没设的分类不自动删**，订单、支付事件、资金流水和任务产物**永远不自动删**，通知历史按永久保留处理。API 日志和工单有 90 天安全下限；用户资料只清理已注销账号的剩余 PII，不删账号行（`service/adty_data_retention.go:17-27,79-140,597-627`）。接口 `GET/PUT /api/adty/admin/data-retention`、`GET /api/adty/admin/data-retention/runs`，仅 Root，PAT 需 `option:read/write`。「数据保留」页下方列出最近 10 次清理：执行时间、定时或手动，以及每类删除、去标识化、失败的条数。

### 按 API Key 限流

单把 API Key 可以设每分钟请求上限，存 `adty_key_limits`（`token_id`、`per_minute`）。创建或编辑 Key 时传 `rate_limit_per_minute`，正数生效、0 删除限制；没有行的 Key 不受额外限制。它只加在 relay 入口（`/v1`、`/v1beta`、`/mj`、`/pg`、任务提交、插件和视频），不替代按用户限流。超限返回 429；计数存储不可用返回 503 `rate_limit_unavailable`，不放行；开 Redis 用 Redis，没开用进程内存（`middleware/adty_key_limit.go:16-98`）。

## 动态支付场景（FIN-F-01）

Root 在支付设置的 General 页新增、编辑、停用或删除支付场景，按版本整体保存。每个场景配置名称/标识、真实业务、支付商、方式、易支付通道、币种、站点/用户类型及美元面额范围；当前真实业务为钱包充值，其他资金业务在各自实现后接入。

钱包按服务端实际可用配置显示场景和方式，报价与下单均携带场景标识。六支付商的原生入口和统一入口在生产建单事务汇合，读取最新配置及 SQL 用户身份，验证金额、币种、归属和通道后冻结场景名称、标识、版本及不含密钥的策略快照。省略标识仅指默认 wallet_topup，默认场景删除或停用后不能绕过。易支付报价、选通道与降级均限定在场景允许范围。

配置变更与持久审计共享 CAS 事务；版本冲突返回 409，前端保留草稿。停用/删除约束新订单，可信旧订单回调仍按原快照幂等入账。订单历史和财务详情显示已冻结的场景名称、标识、版本，接口不公开完整策略快照。迁移只在首次建立兼容默认场景，重复启动不覆盖 Root 配置；旧订单保留未记录场景的历史事实。

## 分站员工（O-01，授权工作区）

站长中心的员工页签支持独立账号创建、姓名/角色/自定义权限、启停、改密和删除，以及带员工/客户编号的分页操作记录。客服、财务、运营、管理四种角色默认均只读，客户安全操作必须独立勾选；明确的空权限数组不授予任何客户权限。创建不走注册赠金，不生成客户归属关系。员工管理需站长本人、单次且绑定操作与目标的安全证明和当前版本，事务内重新验证站长及协议。

员工使用现有登录进入 `/site-staff`，逐请求从 SQL 检查成员、用户状态/授权版本、本站当前站长和站点协议。客户列表/余额/充值/消费权限分别控制，且与平台可见性取交集；邮箱始终遮罩，客户 Key 不返回。充值和消费只读取加入本站之后的安全投影，消费不含请求体、Key/Token 名、IP、渠道或原始附加数据。读取须成功写入脱敏审计后才返回数据。站主中心标签栏在窄屏换行并自动增高，员工管理按钮保持可点击。

修改权限、禁用、恢复、改密和删除均在同一审计事务内递增授权版本，旧 JWT 和 refresh 不能恢复；删除保留员工身份墓碑。管理及员工端点只接受会话，PAT 不可替代；员工默认拒绝白名单外的后台接口，钱包、提现、调额、客户 Key、站长/全局管理和跨站访问始终禁止。缓存 schema 5 包含员工标记，局部更新遇旧 schema 会丢弃旧缓存并重新从 SQL 读取。

站长可授予 `customers.disable`，员工逐次以 `site.staff.customer.manage` 验证本人，单次证明绑定动作与客户编号。员工不能替客户重置密码、取得客户凭据或代登；`POST /api/adty/staff/customers/:id/password-reset` 对已授权员工亦拒绝，不发送重置邮件或令牌。客户密码恢复由客户本人通过已验证的自助渠道完成。`POST /api/adty/staff/customers/:id/disable` 禁用客户并递增授权版本，旧会话失效；员工不能恢复客户。事务内重新核对授权、站点协议、客户归属并原子记录脱敏审计，不能修改站长、历史站长、管理员、员工或跨站客户，关闭中的站点拒绝客户写操作。

员工给客户加额度（`approve` 通过）时，资金已由站点钱包按 **经营 → 收益 → 保证金** 的顺序出资：一个钱包够就扣一个；三个都不够就整笔拒绝，客户余额一分不动。出资和加额度在同一个事务里，批次记在申请单所在站（`service/adty_remaining_gaps.go:716-735,749-799`）。所以「凭空加额度」不再可能，O-01 的资金来源和承担规则按这条实现；整项认证安全标准仍不因本批关闭。

## 已排除的模型部署管理

平台不提供 io.net 模型部署、硬件资源报价、实例和容器管理。模型页保留元数据和厂商分类，渠道页保留中转配置与云供应商区域；旧部署 API 返回 JSON 404，旧页面和设置书签回退保留的模型设置。历史配置、渠道数据和审计保留，旧部署配置不再对外返回或接受写入，旧 PAT 权限不能恢复功能。

## 团队页面、通知机器人、工单、翻译和分组健康

**团队页面**：钱包页展示当前团队的名称、共享额度、折扣、角色、成员和已绑定的团队 Key。负责人可发一次性邀请、禁用成员、绑定或撤销自己的团队 Key、关闭团队。成员只能绑定和撤销自己的团队 Key，可以退出，不能更换负责人。关闭后共享额度保留，团队 Key 停用。站点由服务端按当前账号确定，页面不提供站点选择，也不提供凭空加额度的按钮。

**Telegram**：平台只有两个通知机器人，用户通知和管理通知。Token 只由 Root 在系统设置 → 运维 → Telegram bots 保存，加密存储、脱敏展示。保存、身份验证和测试消息分开记录。用户在个人页绑定用户机器人，站长在站长中心 → Notices 绑定管理机器人。绑定用短时一次性挑战，网站确认 Telegram 数字 ID；转发、过期、重放、跨账号和跨站都拒绝。机器人不执行账户操作。「发送测试」要重新验证身份：先向 Telegram 校验 Token，再给当前 Root 自己绑定的这个机器人发一条测试通知（没绑定就只校验 Token）；每次测试都是一条新的投递记录。

**工单**：个人页工单中心可创建、回复和上传图片。附件只收图片，校验类型、大小和像素，并去掉 JPEG/PNG 的附加元数据。客服内部备注不出现在客户接口。客服角色不增加退款、调额、提现或 Key 权限。被移除的员工不能再打开旧附件。充值记录可带订单号提交工单，不带完整 Key。

**翻译**：正式动态译文由 Root 配置。相同内容复用；旧任务晚返回不能覆盖新版本；人工修改不被普通重试覆盖。失败时保留中文并标明英文未就绪。不扣客户或团队余额，不用客户 Key，不把翻译请求发回本站中转。Root 在系统设置 → 运维 → System translation 配置：平台模型或独立接口（OpenAI Chat Completions / Anthropic Messages，https 地址和密钥），模型、备用模型、超时、并发、单次字符上限、每日调用次数、备用模型重试次数和是否启用。表单从已保存的配置读出来，保存只改动过的项；保存和「测试连接」各要一次重新验证。页面显示今天（UTC）的调用次数、失败次数和平均耗时。

**分组健康**：创建或编辑 API Key 选择分组时，展示近 24 小时请求成功率和主动探测可用率。两者分开。没有样本或采集中断时显示暂无数据或监控中断，不显示 100% 绿色。客户侧拒绝不计入上游失败。用户只看自己可用的分组，不看上游渠道。监控不自动改 Key 分组。Root 在系统设置 → 运维 → 分组健康设置「成功率不低于多少算健康」和「多久没有样本算监控中断」（`GET`/`PUT /api/adty/admin/group-health`，PAT `option:read/write`；默认 95%、6 小时），并能看到最近一次样本时间。
