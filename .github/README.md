# ADTY AI

ADTY AI ([adtyai.com](https://adtyai.com)) is an AI API gateway. One API key reaches Claude, GPT, Gemini and other models through OpenAI-, Claude- and Gemini-compatible endpoints, billed per use. Top-ups accept USDT and Alipay or WeChat Pay. The console is available in Simplified Chinese and English only; the public landing page is pre-rendered in eight languages, of which six are kept solely so old links keep working and are not offered in the UI.

## Upstream and license

This repository is a modified version of [QuantumNous/new-api](https://github.com/QuantumNous/new-api). It is distributed under the same license, the GNU Affero General Public License v3.0 (see [LICENSE](../LICENSE), [NOTICE](../NOTICE) and [THIRD-PARTY-LICENSES.md](../THIRD-PARTY-LICENSES.md)). The upstream README content is kept in [README.md](../README.md), with a section added for this fork and a note on how the build sets `VERSION`.

The changes made in this fork are described in [docs/adty/README.md](../docs/adty/README.md) (in Chinese). Each changed upstream file is listed in [docs/adty/upstream-touchpoints.md](../docs/adty/upstream-touchpoints.md), and deleted features in [docs/adty/deleted-features.md](../docs/adty/deleted-features.md).

## Deployment

See [docs/adty/deploy.md](../docs/adty/deploy.md) for Docker Compose, Nginx and Cloudflare. Images are built by GitHub Actions and published to GHCR. This fork supports PostgreSQL only.
