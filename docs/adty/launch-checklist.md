# 上线检查清单

首期上线前逐条做完、打勾（任务 10.2）。部署本身按 [deploy.md](deploy.md)，这里只列部署之外要设、要测的事。括号里是后台的位置或配置键。

## 1. 部署和初始化

- [ ] 按 [deploy.md](deploy.md) 第 1–4 节起好容器，`docker network inspect` 确认网关和 `TRUSTED_PROXIES` 一致，日志里没有 `TRUSTED_PROXIES is unset`。
- [ ] 配 Nginx 之前，用 SSH 隧道（`ssh -L 3000:127.0.0.1:3000 <服务器>`）打开 `http://localhost:3000`，完成初始化向导，只建超级管理员。
- [ ] 建好超级管理员后再按 deploy.md 第 5 节开放 Nginx 和防火墙；之后的后台设置都用正式域名登录后改。
- [ ] 生产环境用 sha 标签（`ghcr.io/adtyai/adtyai-api:sha-<7 位>`），不跟 `main` 或 `latest`；GHCR 包的可见性和 deploy.md 第 3 节选的拉取方式一致。

## 2. 环境变量

- [ ] `SESSION_SECRET`、`CRYPTO_SECRET` 分开设、固定不改，`.env` 另存一份到密码管理器。
- [ ] `SESSION_COOKIE_SECURE=true` 和 `SESSION_COOKIE_TRUSTED_URL` 同时设。
- [ ] `ERROR_LOG_ENABLED=true`。
- [ ] `TRUSTED_PROXIES` 是 compose 网络的网关（默认 `172.30.0.1`）。

## 3. Nginx、Cloudflare 和防火墙

- [ ] `client_max_body_size 100m`。
- [ ] Cloudflare 网段是最新的（跑过一次 `update-cloudflare-ips.sh`），每周的定时任务已加。
- [ ] 防火墙规则生效：443 只放行 Cloudflare 网段，SSH 能连。
- [ ] Cloudflare 面板按 deploy.md 第 6 节设好。

## 4. 后台设置

### 品牌和站点

- [ ] 系统名称 `ADTY AI`。
- [ ] Logo 设成 `/adty-logo.svg`。
- [ ] ServerAddress 设成已经决定的主站 HTTPS Origin。不要用仓库里的示例域名。
- [ ] API 地址：站点设置里的 API 地址（`adty_site_setting.api_base_url`）和内容设置里的 API 信息面板都填好。
- [ ] 站点设置里填客服邮箱、Telegram、tawk.to 的 property id 和 widget id。
- [ ] tawk.to 后台把挂件语言设好：网页不会按用户语言切换挂件语言，用的是 tawk.to 后台给这个 widget 设的语言。填好 id 后用国内网络打开首页，看挂件的加载速度；打开支付窗时挂件应该隐藏，关掉后重新出现。
- [ ] 顶栏导航关掉 Console、Rankings、About。
- [ ] docs_link 保持为空（8.9 已把默认值改成空，顶栏的「文档」指向 /docs）。

### 注册和登录

- [ ] 打开邮箱验证。
- [ ] 打开注册。
- [ ] 关第三方登录。
- [ ] 关 Turnstile。

### 额度和运营

- [ ] 货币显示 USD。
- [ ] 关签到。
- [ ] 新用户额度 0。
- [ ] 余额不足提醒打开。
- [ ] 渠道亲和打开。
- [ ] **不要关数据导出**（DataExportEnabled），概览的数据依赖它。
- [ ] 流式保活：「系统设置 → 模型 → Global Model Configuration」打开「Keep-alive Ping」，间隔设 30 秒（初始是关、60 秒）。不开的话，推理模型等首字超过 125 秒会被 Cloudflare 以 524 断开。
- [ ] 按用户限流（安全设置 → 按用户限流）：定好开关和默认的 RPM、TPM、并发数（`adty_limit_setting`）。多实例时必须先接好 Redis。

### 条款

- [ ] 用户协议、隐私政策：上游 `legal.*` 那份（站点 → 系统信息）也要填，建议填英文。不填的话 `/api/status` 里的条款开关为假，注册页不显示协议勾选框。
- [ ] 多语言版本在内容设置的「多语言条款」里填，三份条款（用户协议、隐私政策、退款政策）都有 8 种语言，包含韩语。
- [ ] 打开退款政策（`adty_site_setting.refund_policy_enabled`）。

### 支付

- [ ] 充值加赠：没有内置档位，按需在支付设置里填加赠档位。
- [ ] 易支付换通道的时限（单通道、总时限）在「支付宝和微信支付」分区里设。

### 推广返佣

- [ ] 先设默认返佣比例、最低提现、USDT 提现手续费、人民币提现汇率（`adty_referral_setting` 的 `default_rate`、`min_withdraw_usd`、`usdt_withdraw_fee`、`withdraw_cny_rate`）。
- [ ] 再打开推广开关。没设默认比例时打不开。
- [ ] 批量注册阈值（`batch_register_threshold`）先设 0，第 9 节真实 IP 验证通过后再改回 3。

## 5. 邮件

- [ ] 发件走 Resend 或 Amazon SES。
- [ ] adtyai.com 配好 SPF、DKIM、DMARC。
- [ ] 8 种语言各发一封测试邮件，收件箱里显示正常、没进垃圾箱。
- [ ] 邮件页眉 Logo 待接入：现在的系统邮件和营销邮件模板都没有图片页眉。要加时用 `adtyai-logo/email/logo-email-header.png`（白底，480 宽），放进 `web/public` 用主站的绝对地址引用；分站邮件仍用站长上传的 Logo（11.3）。

## 6. 支付实测

- [ ] BEpusdt：金额匹配用 has_prefix、订单有效期 15 分钟、确认区块数设好、回调地址公网可达。
- [ ] BEpusdt：每条链小额实付一次，其中一次让回调在本地订单超时之后才到，确认照样入账。
- [ ] 易支付：每个通道测试下单、实付 ¥1、回调验签、补查、维护切换和恢复。

## 7. 推广和后台权限实测

- [ ] 用两个账号走完 点击 → 注册 → 充值 → 冻结 → 结算 → 三种提现 → 退款扣回。
- [ ] 建一个管理员，分别套用客服、运营、财务预设，确认菜单和接口都按权限放行。

## 8. 浏览器和客户端回归

8.9 改了登录 cookie 名（`adtyai_refresh`、`adtyai_has_session`）和浏览器本地存储的键（前缀 `adtyai:`），旧会话在部署后要重新登录一次。

- [ ] 浏览器里实际走一遍：登录、退出、两个标签页同步登录（一个标签页登录后另一个跟着登录，退出同理）、切换主题后刷新还在。
- [ ] 控制台顶栏左上角的站点 Logo 盖住了顶部导航的「Home」（1280px 宽，侧边栏展开和收起都一样，浅色、深色都有，换 Logo 之前就有）。代码审查时修：`web/src/components/layout/components/app-header.tsx` 的品牌区和 `TopNav` 挤在一起。修完在 1280px 和 1440px 下各看一次。
- [ ] 一键导入实测：Cherry Studio、AionUI、DeepChat、AQBot。
- [ ] Claude Code、Codex CLI、Cherry Studio：流式输出、工具调用、提示缓存命中、思考模式、长上下文、图片输入。

## 9. 真实 IP、备份和日志

- [ ] 按 deploy.md 第 7 节做真实 IP 验证（用户设置里要打开「记录 IP」）。
- [ ] 真实 IP 验证通过之后，把批量注册阈值改回 3。
- [ ] 数据库和 /data 备份按 deploy.md 第 9 节做一次恢复演练，最好在另一台机器上。
- [ ] 日志清理：上游没有自动保留期，只有超级管理员手动触发的清理任务（`POST /api/system-task/log-cleanup`，后台「日志清理」）。约定每月清理一次 90 天前的日志，把这件事排进日程。

## 10. SEO

- [ ] 服务器地址是已经决定的主站 HTTPS Origin，不是仓库示例域名。
- [ ] 后台填好 8 种语言的首页标题和描述，包含韩语。
- [ ] 逐个打开 8 个首页网址（含 `/ko`）看网页源码：有标题、描述、canonical、hreflang 和正文，没有 noindex。
- [ ] 随便开一个控制台页面，源码里有 noindex。
- [ ] 用 Google 的富媒体搜索结果测试检查首页的 WebSite 数据。
- [ ] 在 Telegram 里发一次 adtyai.com，预览显示 ADTY AI 和描述。

## 11. 站长分站（P11）

部署按 [deploy.md](deploy.md) 第 11 节。

### 先填好 Cloudflare 和 Resend，做完实测

- [ ] Cloudflare 各项在后台「分站管理 → 设置」里填好：API Token（这个 zone 的 `SSL and Certificates Write`，账号的 `Turnstile Sites Write`）、Zone ID、Account ID、CNAME 目标、回源主机名、Turnstile widget 列表（每个的 id、site key、secret）。旁边不再显示「未配置」。
- [ ] Resend：API Key 填在「系统设置 → 邮件」（`ResendApiKey`）；营销发件地址、webhook 签名密钥在「营销 → 设置」里填。
- [ ] 按方案任务 11.0 实测四项，结果记下来，任何一项不通先改代码再上线：
  - [ ] Cloudflare 自定义主机名：用 API 建一个（`ssl.method=http`），另一个域名的子域名 CNAME 过来，等 `status` 和 `ssl.status` 都是 active，curl 确认证书有效，记下用了多久。
  - [ ] Turnstile：把这个自定义域名加进 widget 的 domains，在这个域名上能过验证，后端校验通过、返回的 hostname 对。
  - [ ] Resend：加一个发件域名并验证通过，从它发一封，邮件原文里 SPF、DKIM 都通过；建 contact 和 segment，发一次 Broadcast，退订链接能用。
  - [ ] SSE：自定义域名下跑首字延迟 100 秒和 150 秒的流式请求，开 30 秒 ping 时都不断，关了 ping 时 150 秒那个报 524。

### 设置

- [ ] 填好 `adty_reseller_setting` 的金额项（拿货系数 d、d 的下限、站长倍率 k 的上限、分站加赠的额外上限、试用额度单次和每天上限）和根域名，再打开分站总开关。没填时开关打不开。
- [ ] 开站协议和分站客户条款都有 8 种语言，包含韩语（内容设置的「多语言条款」）。

### 走一遍

- [ ] 用测试账号：申请开站 → 审核 → 设倍率和加赠 → 在分站注册客户 → 充值 → 调用（流式和非流式各一次）→ 第二天看站长收益 → 冻结期后提现 → 关站并把客户转回主站。每一步都核对一次「客户付 = 平台收 + 站长赚」。
- [ ] 自定义域名：加一个测试域名，等证书生效后在这个域名上注册、登录、充值、调用一次。
- [ ] 邮件：分站客户的验证码、找回密码邮件，链接是分站地址，站名是分站站名；用伪造的 Host 请求找回密码，收到的链接仍然是用户所属站点的地址。
- [ ] 支付：分站客户用支付宝付款，付完回到分站钱包。
- [ ] 搜索引擎：没开 SEO 的分站，首页网页源码里有 noindex。
- [ ] 后台能看到 Turnstile widget 用量和 Resend 域名用量（这两处显示还没做，做好后再勾）。

## 12. 收尾

- [ ] 设计稿 §13 剩下的事项都有结论。
