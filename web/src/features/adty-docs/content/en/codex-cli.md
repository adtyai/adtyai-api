# Codex CLI

Codex CLI reads its providers from `~/.codex/config.toml` (on Windows `%USERPROFILE%\.codex\config.toml`). Add ADTY AI as a provider and make it the default.

## 1. Edit config.toml

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` must match the name after `model_providers.`.
- `base_url` ends with `/v1`.
- Set `model` to a model listed on the [Pricing](/pricing) page.

## 2. Set the key

macOS and Linux:

```bash
export ADTYAI_API_KEY="sk-your-key"
codex
```

Windows PowerShell:

```powershell
$env:ADTYAI_API_KEY = "sk-your-key"
codex
```

The variable name is whatever you put in `env_key`; the key itself never goes into `config.toml`.

## Tips

- If Codex keeps asking you to sign in with ChatGPT, check that `model_provider` is at the top level of the file, before any `[section]` line. TOML keys written after a section header belong to that section.
- To keep your existing setup, put the provider in a profile instead and start Codex with `codex --profile adtyai`:

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
