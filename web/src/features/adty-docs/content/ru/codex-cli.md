# Codex CLI

Codex CLI читает список провайдеров из `~/.codex/config.toml` (в Windows — `%USERPROFILE%\.codex\config.toml`). Добавьте ADTY AI как провайдера и сделайте его провайдером по умолчанию.

## 1. Отредактируйте config.toml

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` должен совпадать с именем после `model_providers.`.
- `base_url` заканчивается на `/v1`.
- В `model` укажите модель со страницы [Цены](/pricing).

## 2. Задайте ключ

macOS и Linux:

```bash
export ADTYAI_API_KEY="sk-ваш-ключ"
codex
```

Windows PowerShell:

```powershell
$env:ADTYAI_API_KEY = "sk-ваш-ключ"
codex
```

Имя переменной — то, которое вы указали в `env_key`; сам ключ в `config.toml` никогда не записывается.

## Советы

- Если Codex постоянно предлагает войти через ChatGPT, проверьте, что `model_provider` находится на верхнем уровне файла, до любой строки `[section]`. Ключи TOML, записанные после заголовка секции, относятся к этой секции.
- Чтобы сохранить текущую настройку, вынесите провайдера в профиль и запускайте Codex командой `codex --profile adtyai`:

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
