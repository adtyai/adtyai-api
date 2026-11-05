# 빠른 시작

ADTY AI는 OpenAI와 Anthropic API와 호환되는 API 게이트웨이입니다. 하나의 API 키로 이미 쓰던 도구에서 Claude, GPT, Gemini, DeepSeek 등 모델을 호출할 수 있습니다.

## 1. 계정을 만들고 충전하기

1. [가입](/sign-up)하고 이메일을 인증하세요.
2. [지갑](/wallet)을 열고 충전하세요. 요금은 사용한 만큼입니다. 각 요청은 사용한 토큰에 따라 [가격](/pricing) 페이지의 가격으로 청구됩니다.

## 2. API 키 만들기

[API 키](/keys)를 열고 키를 만드세요. 키는 `sk-`로 시작합니다. 복사한 뒤 비밀로 보관하세요. 키를 가진 사람은 잔액을 사용할 수 있습니다. 도구마다 키를 나누어 만들고, 언제든 그중 하나를 끌 수 있습니다.

## 3. API 주소 사용하기

| 클라이언트 종류 | 입력할 주소 |
|---|---|
| OpenAI 호환 (OpenAI SDK, Codex CLI, 대부분의 채팅 앱) | `{{BASE_URL}}/v1` |
| Anthropic 호환 (Claude Code, Anthropic SDK) | `{{BASE_URL}}` |

지원 엔드포인트에는 `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations`, `/v1/models`가 있습니다.

## 4. 테스트 요청 보내기

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

`gpt-4o-mini`를 [가격](/pricing) 페이지에 있는 모델 이름으로 바꾸세요. 표시된 이름을 그대로 써야 합니다.

## 다음 단계

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [오류와 한도](/docs/errors-and-limits)
