# Claude Code

Claude Code 走 Anthropic Messages 介面，位址填**不帶** `/v1` 的那個。

## 環境變數

macOS、Linux（想一直生效就寫進 `~/.zshrc` 或 `~/.bashrc`）：

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-你的金鑰"
claude
```

Windows PowerShell：

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-你的金鑰"
claude
```

## 設定檔

不想設環境變數，也可以把同樣的值寫進 `~/.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-你的金鑰"
  }
}
```

## 確認已生效

執行 `claude`，輸入 `/status`，顯示的 Base URL 應該是 `{{BASE_URL}}`。隨便傳一句話，控制台的使用紀錄裡就能看到這次請求。

## 注意

- 用 `ANTHROPIC_AUTH_TOKEN`，不要用 `ANTHROPIC_API_KEY`。如果同時設了 `ANTHROPIC_API_KEY`，Claude Code 可能會問你用哪一個，或者繼續用官方金鑰，請把它刪掉。
- 以前用 Claude 帳號登入過的，先在 Claude Code 裡執行 `/logout`，它才會改用金鑰。
- 選模型：在 Claude Code 裡執行 `/model`，或者設定 `ANTHROPIC_MODEL`。只有[模型價格](/pricing)頁裡列出的模型可以用。
- 也可以在[令牌](/keys)頁用 CC Switch 一鍵匯入。
