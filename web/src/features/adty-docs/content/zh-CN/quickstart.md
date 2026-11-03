# 快速开始

ADTY AI 是兼容 OpenAI 和 Anthropic 接口的 API 网关。用一个 API 密钥，就能在你常用的工具里调用 Claude、GPT、Gemini、DeepSeek 等模型。

## 1. 注册并充值

1. [注册账号](/sign-up)并验证邮箱。
2. 打开[钱包](/wallet)充值。按量计费：每次请求按实际用掉的 tokens 扣费，单价见[模型价格](/pricing)页。

## 2. 创建 API 密钥

打开[令牌](/keys)页创建密钥，密钥以 `sk-` 开头。复制后妥善保管，拿到密钥的人可以花你的余额。可以建多个密钥（比如每个工具一个），随时停用其中任何一个。

## 3. 填写 API 地址

| 客户端类型 | 填写的地址 |
|---|---|
| OpenAI 兼容（OpenAI SDK、Codex CLI、大部分聊天客户端） | `{{BASE_URL}}/v1` |
| Anthropic 兼容（Claude Code、Anthropic SDK） | `{{BASE_URL}}` |

支持的接口包括 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/embeddings`、`/v1/images/generations` 和 `/v1/models`。

## 4. 发一个测试请求

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-你的密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

把 `gpt-4o-mini` 换成[模型价格](/pricing)页里列出的任一模型，模型名照页面上的原样填写。

## 下一步

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [错误码和限制](/docs/errors-and-limits)
