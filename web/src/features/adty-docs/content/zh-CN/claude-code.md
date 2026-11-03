# Claude Code

Claude Code 走 Anthropic Messages 接口，地址填**不带** `/v1` 的那个。

## 环境变量

macOS、Linux（想一直生效就写进 `~/.zshrc` 或 `~/.bashrc`）：

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-你的密钥"
claude
```

Windows PowerShell：

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-你的密钥"
claude
```

## 配置文件

不想设环境变量，也可以把同样的值写进 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-你的密钥"
  }
}
```

## 确认已生效

运行 `claude`，输入 `/status`，显示的 Base URL 应该是 `{{BASE_URL}}`。随便发一句话，控制台的使用日志里就能看到这次请求。

## 注意

- 用 `ANTHROPIC_AUTH_TOKEN`，不要用 `ANTHROPIC_API_KEY`。如果同时设了 `ANTHROPIC_API_KEY`，Claude Code 可能会问你用哪个，或者继续用官方密钥，请把它删掉。
- 以前用 Claude 账号登录过的，先在 Claude Code 里执行 `/logout`，它才会改用密钥。
- 选模型：在 Claude Code 里执行 `/model`，或者设置 `ANTHROPIC_MODEL`。只有[模型价格](/pricing)页里列出的模型可以用。
- 也可以在[令牌](/keys)页用 CC Switch 一键导入。
