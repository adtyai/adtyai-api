# ADTY AI

ADTY AI ([adtyai.com](https://adtyai.com)) is an AI API gateway. One API key reaches Claude, GPT, Gemini and other models through OpenAI-, Claude- and Gemini-compatible endpoints, billed per use. Top-ups accept USDT and Alipay or WeChat Pay, and the site is available in seven languages.

## Upstream and license

This repository is a modified version of [QuantumNous/new-api](https://github.com/QuantumNous/new-api). It is distributed under the same license, the GNU Affero General Public License v3.0 (see [LICENSE](../LICENSE), [NOTICE](../NOTICE) and [THIRD-PARTY-LICENSES.md](../THIRD-PARTY-LICENSES.md)). The upstream project's own README is kept unchanged at [README.md](../README.md).

The changes made in this fork are described in [docs/adty/README.md](../docs/adty/README.md) (in Chinese). Each changed upstream file is listed in [docs/adty/upstream-touchpoints.md](../docs/adty/upstream-touchpoints.md), and deleted features in [docs/adty/deleted-features.md](../docs/adty/deleted-features.md).

## Deployment

See [docs/adty/deploy.md](../docs/adty/deploy.md) for Docker Compose, Nginx and Cloudflare. Images are built by GitHub Actions and published to GHCR. This fork supports PostgreSQL only.
