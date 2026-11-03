# Codex CLI

Codex CLI 從 `~/.codex/config.toml`（Windows 是 `%USERPROFILE%\.codex\config.toml`）讀取供應商設定。把 ADTY AI 加成一個供應商並設為預設即可。

## 1. 修改 config.toml

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` 要和 `model_providers.` 後面的名稱一致。
- `base_url` 以 `/v1` 結尾。
- `model` 填[模型價格](/pricing)頁裡列出的模型。

## 2. 設定金鑰

macOS、Linux：

```bash
export ADTYAI_API_KEY="sk-你的金鑰"
codex
```

Windows PowerShell：

```powershell
$env:ADTYAI_API_KEY = "sk-你的金鑰"
codex
```

環境變數名稱就是 `env_key` 裡寫的名稱，金鑰本身不要寫進 `config.toml`。

## 注意

- 如果 Codex 一直要求用 ChatGPT 登入，檢查 `model_provider` 是不是寫在檔案最上面、所有 `[小節]` 之前。寫在小節標題之後的鍵屬於那個小節。
- 想保留原來的設定，可以把它放進 profile，啟動時用 `codex --profile adtyai`：

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
