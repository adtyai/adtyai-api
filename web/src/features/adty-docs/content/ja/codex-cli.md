# Codex CLI

Codex CLI はプロバイダーの設定を `~/.codex/config.toml`（Windows では `%USERPROFILE%\.codex\config.toml`）から読み込みます。ADTY AI をプロバイダーとして追加し、デフォルトに設定します。

## 1. config.toml を編集する

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` は `model_providers.` の後ろの名前と一致させる必要があります。
- `base_url` は `/v1` で終わります。
- `model` には[料金](/pricing)ページに載っているモデルを指定します。

## 2. キーを設定する

macOS と Linux：

```bash
export ADTYAI_API_KEY="sk-あなたのキー"
codex
```

Windows PowerShell：

```powershell
$env:ADTYAI_API_KEY = "sk-あなたのキー"
codex
```

変数名は `env_key` に書いた名前になります。キー自体を `config.toml` に書くことはありません。

## ヒント

- Codex が ChatGPT でのサインインを求め続ける場合は、`model_provider` がファイルのトップレベル、つまりどの `[section]` 行よりも前にあるか確認してください。TOML では、セクションヘッダーの後に書いたキーはそのセクションに属します。
- 既存の設定を残したい場合は、プロバイダーをプロファイルに書き、`codex --profile adtyai` で Codex を起動します。

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
