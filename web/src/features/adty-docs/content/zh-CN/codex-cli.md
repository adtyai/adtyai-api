# Codex CLI

Codex CLI 从 `~/.codex/config.toml`（Windows 是 `%USERPROFILE%\.codex\config.toml`）读取服务商配置。把 ADTY AI 加成一个服务商并设为默认即可。

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

- `model_provider` 要和 `model_providers.` 后面的名字一致。
- `base_url` 以 `/v1` 结尾。
- `model` 填[模型价格](/pricing)页里列出的模型。

## 2. 设置密钥

macOS、Linux：

```bash
export ADTYAI_API_KEY="sk-你的密钥"
codex
```

Windows PowerShell：

```powershell
$env:ADTYAI_API_KEY = "sk-你的密钥"
codex
```

环境变量名就是 `env_key` 里写的名字，密钥本身不要写进 `config.toml`。

## 注意

- 如果 Codex 一直要求用 ChatGPT 登录，检查 `model_provider` 是不是写在文件最上面、所有 `[小节]` 之前。写在小节标题之后的键属于那个小节。
- 想保留原来的配置，可以把它放进 profile，启动时用 `codex --profile adtyai`：

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
