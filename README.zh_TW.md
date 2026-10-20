<div align="center">

![adtyai](/web/public/logo.png)

# ADTY AI

**自託管 AI 閘道二開（adtyai）**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>繁體中文</strong> |
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
  <a href="#專案說明">說明</a> •
  <a href="#二開能力">二開能力</a> •
  <a href="#官方能力">官方能力</a> •
  <a href="#快速開始">快速開始</a> •
  <a href="#部署">部署</a> •
  <a href="#授權條款">授權條款</a>
</p>

</div>

---

<a id="專案說明"></a>

## 專案說明

**ADTY AI**（倉庫名 **adtyai-api**）是面向中轉站營運的自託管 AI 閘道：在保留官方 New API 渠道、模型與控制台能力的基礎上，加上本站計費、儲值、返佣、風控與運維二開。

- 倉庫：<https://github.com/adtyai/adtyai-api>
- 映像：`ghcr.io/adtyai/adtyai-api`（如 `:main`、帶日期的建置 tag；正式多架構發佈隨 tag）
- 預設分支：`main`（請只在此分支改與推）

本專案基於 [New API](https://github.com/QuantumNous/new-api) 二開，遵循 AGPL-3.0；感謝上游作者與貢獻者。

> [!IMPORTANT]
> 僅用於合法授權的閘道與私有化部署。使用者須合法取得上游金鑰與權限，並自行完成所在地備案、支付與內容安全等合規義務。

---

<a id="二開能力"></a>

## 二開能力（ADTY）

面向站長與營運的能力摘要（以當前 `main` 原始碼為準）：

| 方向 | 說明 |
| --- | --- |
| 雙錢包計費 | 贈送餘額優先、實充其次；小時彙總按實際出資拆分；日常對帳 |
| 儲值 | 檔位贈送（無首儲加送）；易支付與 BEpusdt（USDT）收款 |
| 邀請返佣 | 按實付減已退計佣；提現審核；漏發可啟動補償 |
| 退款與風控 | 按比例收回贈送/返佣；風控人工待審；管理端訂單與流水 |
| 企業協議價 | 系統設定 → 計費與支付 → 企業協議價（分組隱藏倍率） |
| 智慧路由 | 獨立側欄選單；評分選路 / 半開 / 權重方案；成本護欄入口 `/smart-routing/cost` |
| 側邊欄模組 | 已併進官方側邊欄模組統一開關；廢獨立選單顯隱資料面 |
| 客服 | Chatwoot 即時客服（掛件 + 後台設定；腳本僅超管可改；探活失敗降級備用聯絡方式） |
| 業務參數 | 控制台可調營運參數 |
| 運維 | 備份/日報相關任務（入口收在官方側欄分類下） |

使用者可見品牌為 **ADTY AI**，不以 New API 作為站名展示。

---

<a id="官方能力"></a>

## 官方能力（繼承）

仍可使用上游 New API 能力，例如：多上游渠道與模型對應、額度與訂閱、用量日誌、使用者/分組/API Key、OAuth 與通行金鑰、非同步任務外掛、多語言控制台等。協定入口包括 OpenAI Chat/Responses、Anthropic Messages、Gemini 等（詳見原始碼與上游文件）。

---

<a id="快速開始"></a>

## 快速開始

### Docker 單機體驗

```bash
mkdir -p data
docker run --name adtyai-api -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/data" \
  ghcr.io/adtyai/adtyai-api:main
```

瀏覽器開啟 [http://localhost:3000](http://localhost:3000)，依嚮導建立管理員。資料在掛載的 `data` 目錄。

### 第一次呼叫

1. 設定渠道與模型價格，確認使用者有額度。
2. 在控制台建立 API Key。
3. 用戶端 Base URL 設為 `http://localhost:3000/v1`，使用本站簽發的 Key。

```bash
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${ADTY_API_KEY}"
```

---

<a id="部署"></a>

## 部署

建議使用倉庫內 [docker-compose.yml](./docker-compose.yml)（預設 PostgreSQL + Redis）。複製後進入本倉庫目錄再啟動：

```bash
git clone https://github.com/adtyai/adtyai-api.git
cd adtyai-api
# 編輯 compose：改掉範例密碼，設定 SESSION_SECRET
docker compose up -d
```

預設服務名和容器名是 `adtyai-api-app`、`adtyai-api-postgres`、`adtyai-api-redis`。PostgreSQL 資料庫名是 `adty-ai`，不要把它當成容器名。

正式環境請自行設定 HTTPS、金鑰與備份；推送映像以 GitHub Actions / tag 為準。聯調預覽位址以運維公佈為準。

---

<a id="授權條款"></a>

## 授權條款

本專案以 [AGPL-3.0](./LICENSE) 授權。二次散佈或提供網路服務時，請遵守 AGPL 要求。

基於 [New API](https://github.com/QuantumNous/new-api) 二開，致謝上游。
