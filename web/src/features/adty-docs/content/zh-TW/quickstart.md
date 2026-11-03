# 快速開始

ADTY AI 是相容 OpenAI 和 Anthropic 介面的 API 閘道。用一把 API 金鑰，就能在你常用的工具裡呼叫 Claude、GPT、Gemini、DeepSeek 等模型。

## 1. 註冊並儲值

1. [註冊帳號](/sign-up)並驗證電子郵件。
2. 開啟[錢包](/wallet)儲值。按量計費：每次請求按實際用掉的 tokens 扣費，單價見[模型價格](/pricing)頁。

## 2. 建立 API 金鑰

開啟[令牌](/keys)頁建立金鑰，金鑰以 `sk-` 開頭。複製後妥善保管，拿到金鑰的人可以花你的餘額。可以建多把金鑰（例如每個工具一把），隨時停用其中任何一把。

## 3. 填寫 API 位址

| 用戶端類型 | 填寫的位址 |
|---|---|
| OpenAI 相容（OpenAI SDK、Codex CLI、大部分聊天用戶端） | `{{BASE_URL}}/v1` |
| Anthropic 相容（Claude Code、Anthropic SDK） | `{{BASE_URL}}` |

支援的介面包括 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/embeddings`、`/v1/images/generations` 和 `/v1/models`。

## 4. 傳送測試請求

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-你的金鑰" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

把 `gpt-4o-mini` 換成[模型價格](/pricing)頁裡列出的任一模型，模型名稱照頁面上的原樣填寫。

## 下一步

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [錯誤碼和限制](/docs/errors-and-limits)
