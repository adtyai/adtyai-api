<div align="center">

![adtyai](/web/public/logo.png)

# ADTY AI

**Self-hosted AI gateway (adtyai fork)**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>English</strong> |
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
  <a href="#about">About</a> •
  <a href="#adty-features">ADTY features</a> •
  <a href="#upstream-features">Upstream features</a> •
  <a href="#quick-start">Quick start</a> •
  <a href="#deployment">Deployment</a> •
  <a href="#license">License</a>
</p>

</div>

---

<a id="about"></a>

## About

**ADTY AI** (repo **adtyai-api**) is a self-hosted AI gateway for relay-site operators. It keeps upstream New API channel, model, and console capabilities, and adds billing, top-up, affiliate, risk review, and ops features for this fork.

- Repo: <https://github.com/adtyai/adtyai-api>
- Image: `ghcr.io/adtyai/adtyai-api` (e.g. `:main` and dated build tags; multi-arch release follows git tags)
- Default branch: `main` (change and push only on `main`)

Forked from [New API](https://github.com/QuantumNous/new-api) under AGPL-3.0. Thanks to upstream authors and contributors.

> [!IMPORTANT]
> For lawful, authorized gateway and private deployment only. Operators must obtain upstream keys lawfully and meet local compliance (filings, payments, content safety, etc.).

---

<a id="adty-features"></a>

## ADTY features

| Area | What you get |
| --- | --- |
| Dual-wallet billing | Bonus first, then paid; hourly consume split; daily reconcile |
| Top-up | Tier gifts only (no first-top-up bonus); EPay and BEpusdt (USDT) |
| Affiliate | Commission on paid minus refunds; withdraw review; startup backfill for misses |
| Refunds & risk | Proportional clawback of gift/affiliate; manual risk queue; admin orders & ledgers |
| Enterprise rates | Console: Billing & payments → Enterprise rates (hidden group multipliers) |
| Smart routing | Dedicated sidebar menu; scored routing / canary / weight schemes; cost guardrail at `/smart-routing/cost` |
| Sidebar modules | Folded into stock sidebar-module toggles; independent menu-visibility store removed |
| Support | Chatwoot live chat (widget + admin settings; script root-only; fallback contact if probe fails) |
| Business params | Tunable ops settings in the console |
| Ops UX | Backup/report jobs under stock sidebar groups |

User-facing brand is **ADTY AI**, not New API as the site name.

---

<a id="upstream-features"></a>

## Upstream features (inherited)

You still get New API capabilities such as multi-provider channels, quotas/subscriptions, usage logs, users/groups/API keys, OAuth/passkeys, async task plugins, and a multi-language console. Protocol entrypoints include OpenAI Chat/Responses, Anthropic Messages, Gemini, and more (see source and upstream docs).

---

<a id="quick-start"></a>

## Quick start

### Docker (single node)

```bash
mkdir -p data
docker run --name adtyai-api -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/data" \
  ghcr.io/adtyai/adtyai-api:main
```

Open [http://localhost:3000](http://localhost:3000) and create an admin account. Data lives in the mounted `data` directory.

### First API call

1. Configure a channel and pricing; ensure the user has quota.
2. Create an API key in the console.
3. Set client Base URL to `http://localhost:3000/v1` and use a key issued by this site.

```bash
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${ADTY_API_KEY}"
```

---

<a id="deployment"></a>

## Deployment

Prefer [docker-compose.yml](./docker-compose.yml) (PostgreSQL + Redis by default):

```bash
git clone https://github.com/adtyai/adtyai-api.git
cd adtyai-api
# Edit compose: replace sample passwords; set SESSION_SECRET
docker compose up -d
```

Default service and container names are `adtyai-api-app`, `adtyai-api-postgres`, and `adtyai-api-redis`. The PostgreSQL database name is `adty-ai`; that is not the container name.

Configure HTTPS, secrets, and backups for production. Image publishing follows GitHub Actions / tags. Preview URLs are published by ops.

---

<a id="license"></a>

## License

[AGPL-3.0](./LICENSE). Comply with AGPL when redistributing or offering network service.

Based on [New API](https://github.com/QuantumNous/new-api). Thanks, upstream.
