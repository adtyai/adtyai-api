<div align="center">

![adtyai](/web/public/logo.png)

# ADTY AI

**セルフホスト AI ゲートウェイ二開（adtyai）**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <strong>日本語</strong>
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
  <a href="#概要">概要</a> •
  <a href="#adty機能">ADTY 機能</a> •
  <a href="#上流機能">上流機能</a> •
  <a href="#クイックスタート">クイックスタート</a> •
  <a href="#デプロイ">デプロイ</a> •
  <a href="#ライセンス">ライセンス</a>
</p>

</div>

---

<a id="概要"></a>

## 概要

**ADTY AI**（リポジトリ名 **adtyai-api**）は中継サイト運営向けのセルフホスト AI ゲートウェイです。上流 New API のチャネル・モデル・コンソール機能を維持しつつ、本フォーク向けの課金・チャージ・アフィリエイト・リスク審査・運用機能を追加しています。

- リポジトリ: <https://github.com/adtyai/adtyai-api>
- イメージ: `ghcr.io/adtyai/adtyai-api`（例: `:main`、日付付きビルドタグ；正式マルチアーキテクチャ公開は git タグに従う）
- デフォルトブランチ: `main`（変更と push はこのブランチのみ）

[New API](https://github.com/QuantumNous/new-api) を AGPL-3.0 の下でフォーク。上流の作者と貢献者に感謝します。

> [!IMPORTANT]
> 合法・認可されたゲートウェイおよびプライベートデプロイ用途のみ。運営者は上流キーを合法に取得し、届出・決済・コンテンツ安全など現地コンプライアンスを満たす必要があります。

---

<a id="adty機能"></a>

## ADTY 機能

| 分野 | 内容 |
| --- | --- |
| 二重ウォレット課金 | ボーナス残高優先、実チャージ次；時間集計は実出資で按分；日次突合 |
| チャージ | ティアギフトのみ（初回チャージ加算なし）；易支付と BEpusdt（USDT） |
| 招待アフィリエイト | 実支払から返金控除後でコミッション；出金審査；漏れは起動時バックフィル可 |
| 返金とリスク | ギフト/アフィリエイトを比例回収；リスク人手待ち；管理側の注文と台帳 |
| 企業協議価格 | システム設定 → 課金と決済 → 企業協議価格（グループ非表示倍率） |
| スマートルーティング | 独立サイドバーメニュー；スコア選路 / カナリア / 重みスキーム；コストガード `/smart-routing/cost` |
| メニュー表示 | ユーザー/管理の切替；二開入口は公式サイドバーグループへ融合 |
| サポート | Chatwoot ライブチャット（ウィジェット＋管理設定；スクリプトはルートのみ；プローブ失敗時は予備連絡先） |
| 業務パラメータ | コンソールで運用パラメータを調整可 |
| 運用 UX | バックアップ/日報ジョブ（公式サイドバー分類下） |

ユーザー向けブランドは **ADTY AI** であり、サイト名に New API は使いません。

---

<a id="上流機能"></a>

## 上流機能（継承）

マルチプロバイダチャネル、クォータ/サブスクリプション、利用ログ、ユーザー/グループ/API キー、OAuth/パスキー、非同期タスクプラグイン、多言語コンソールなど New API 機能を引き続き利用できます。プロトコル入口は OpenAI Chat/Responses、Anthropic Messages、Gemini など（ソースと上流ドキュメント参照）。

---

<a id="クイックスタート"></a>

## クイックスタート

### Docker（単一ノード）

```bash
mkdir -p data
docker run --name adtyai-api -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/data" \
  ghcr.io/adtyai/adtyai-api:main
```

[http://localhost:3000](http://localhost:3000) を開き、管理者アカウントを作成します。データはマウントした `data` ディレクトリに保存されます。

### 最初の API 呼び出し

1. チャネルと価格を設定し、ユーザーにクォータがあることを確認。
2. コンソールで API キーを作成。
3. クライアントの Base URL を `http://localhost:3000/v1` にし、本サイト発行のキーを使用。

```bash
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${ADTY_API_KEY}"
```

---

<a id="デプロイ"></a>

## デプロイ

[docker-compose.yml](./docker-compose.yml) を推奨（デフォルトで PostgreSQL + Redis）：

```bash
git clone https://github.com/adtyai/adtyai-api.git
cd adtyai-api
# compose を編集：サンプルパスワードを差し替え、SESSION_SECRET を設定
docker compose up -d
```

本番では HTTPS・シークレット・バックアップを自ら設定。イメージ公開は GitHub Actions / タグに従います。プレビュー URL は運用側の公表に従います（固定プレビュー例: `https://newapi.n8-n.top`）。

---

<a id="ライセンス"></a>

## ライセンス

[AGPL-3.0](./LICENSE)。再配布やネットワークサービス提供時は AGPL に従ってください。

[New API](https://github.com/QuantumNous/new-api) ベース。上流に感謝。
