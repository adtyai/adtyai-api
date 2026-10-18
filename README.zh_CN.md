<div align="center">

![adtyai](/web/public/logo.png)

# ADTY AI

**自托管 AI 网关二开（adtyai）**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/license-AGPL--3.0-brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/adtyai/adtyai-api">
    <img src="https://img.shields.io/badge/version-v1.0.0--adtyai-brightgreen" alt="version">
  </a><!--
  --><a href="https://github.com/adtyai/adtyai-api/pkgs/container/adtyai-api">
    <img src="https://img.shields.io/badge/image-ghcr.io%2Fadtyai%2Fadtyai--api-blue" alt="ghcr">
  </a>
</p>

<p align="center">
  <a href="#项目说明">说明</a> •
  <a href="#二开能力">二开能力</a> •
  <a href="#官方能力">官方能力</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#部署">部署</a> •
  <a href="#许可证">许可证</a>
</p>

</div>

---

<a id="项目说明"></a>

## 项目说明

**ADTY AI**（仓库名 **adtyai-api**）是面向中转站运营的自托管 AI 网关：在保留官方 New API 渠道、模型与控制台能力的基础上，加上本站计费、充值、返佣、风控与运维二开。

- 仓库：<https://github.com/adtyai/adtyai-api>
- 镜像：`ghcr.io/adtyai/adtyai-api`（如 `:main`、带日期的构建 tag；正式多架构发布随 tag）
- 默认分支：`main`（请只在此分支改与推）

本项目基于 [New API](https://github.com/QuantumNous/new-api) 二开，遵循 AGPL-3.0；感谢上游作者与贡献者。

> [!IMPORTANT]
> 仅用于合法授权的网关与私有化部署。使用者须合法取得上游密钥与权限，并自行完成所在地备案、支付与内容安全等合规义务。

---

<a id="二开能力"></a>

## 二开能力（ADTY）

面向站长与运营的能力摘要（以当前 `main` 源码为准）：

| 方向 | 说明 |
| --- | --- |
| 双钱包计费 | 赠送余额优先、实充其次；小时汇总按实际出资拆分；日常对账 |
| 充值 | 档位赠送（无首充加送）；易支付与 BEpusdt（USDT）收款 |
| 邀请返佣 | 按实付减已退计佣；提现审核；漏发可启动补偿 |
| 退款与风控 | 按比例收回赠送/返佣；风控人工待审；管理端订单与流水 |
| 企业协议价 | 系统设置 → 计费与支付 → 企业协议价（分组隐藏倍率） |
| 智能路由 | 独立侧栏菜单；评分选路 / 半开 / 权重方案；成本护栏入口 `/smart-routing/cost` |
| 侧边栏模块 | 已并进官方侧边栏模块统一开关；废独立菜单显隐数据面 |
| 客服 | Chatwoot 实时客服（挂件 + 后台设置；脚本仅超管可改；探活失败降级备用联系方式） |
| 业务参数 | 控制台可调运营参数 |
| 运维 | 备份/日报相关任务（入口收在官方侧栏分类下） |

用户可见品牌为 **ADTY AI**，不以 New API 作为站名展示。

---

<a id="官方能力"></a>

## 官方能力（继承）

仍可使用上游 New API 能力，例如：多上游渠道与模型映射、额度与订阅、用量日志、用户/分组/API Key、OAuth 与通行密钥、异步任务插件、多语言控制台等。协议入口包括 OpenAI Chat/Responses、Anthropic Messages、Gemini 等（详见源码与上游文档）。

---

<a id="快速开始"></a>

## 快速开始

### Docker 单机体验

```bash
mkdir -p data
docker run --name adtyai-api -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/data" \
  ghcr.io/adtyai/adtyai-api:main
```

浏览器打开 [http://localhost:3000](http://localhost:3000)，按向导创建管理员。数据在挂载的 `data` 目录。

### 第一次调用

1. 配置渠道与模型价格，确认用户有额度。
2. 在控制台创建 API Key。
3. 客户端 Base URL 设为 `http://localhost:3000/v1`，使用本站签发的 Key。

```bash
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${ADTY_API_KEY}"
```

---

<a id="部署"></a>

## 部署

推荐使用仓库内 [docker-compose.yml](./docker-compose.yml)（默认 PostgreSQL + Redis）。克隆后进入本仓库目录再启动：

```bash
git clone https://github.com/adtyai/adtyai-api.git
cd adtyai-api
# 编辑 compose：改掉示例密码，设置 SESSION_SECRET
docker compose up -d
```

生产环境请自行配置 HTTPS、密钥与备份；推送镜像以 GitHub Actions / tag 为准。联调预览地址以运维公布为准（当前固定入口示例：`https://newapi.n8-n.top`）。

---

<a id="许可证"></a>

## 许可证

本项目以 [AGPL-3.0](./LICENSE) 授权。二次分发或提供网络服务时，请遵守 AGPL 要求。

基于 [New API](https://github.com/QuantumNous/new-api) 二开，致谢上游。
