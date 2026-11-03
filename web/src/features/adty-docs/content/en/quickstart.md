# Quick start

ADTY AI is an API gateway compatible with the OpenAI and Anthropic APIs. Use one API key to call Claude, GPT, Gemini, DeepSeek and other models from the tools you already use.

## 1. Create an account and top up

1. [Sign up](/sign-up) and verify your email.
2. Open [Wallet](/wallet) and top up. Billing is pay-as-you-go: every request is charged by the tokens it uses, at the prices shown on the [Pricing](/pricing) page.

## 2. Create an API key

Open [API Keys](/keys) and create a key. The key starts with `sk-`. Copy it and keep it secret; anyone with the key can spend your balance. You can create several keys, for example one per tool, and disable any of them at any time.

## 3. Use the API address

| Client type | Address to enter |
|---|---|
| OpenAI-compatible (OpenAI SDK, Codex CLI, most chat apps) | `{{BASE_URL}}/v1` |
| Anthropic-compatible (Claude Code, Anthropic SDK) | `{{BASE_URL}}` |

Supported endpoints include `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations` and `/v1/models`.

## 4. Send a test request

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-your-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

Replace `gpt-4o-mini` with any model listed on the [Pricing](/pricing) page. Use the model name exactly as shown there.

## Next steps

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [Errors and limits](/docs/errors-and-limits)
