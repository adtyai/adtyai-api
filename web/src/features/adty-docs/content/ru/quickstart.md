# Быстрый старт

ADTY AI — это API-шлюз, совместимый с API OpenAI и Anthropic. Один API-ключ позволяет вызывать Claude, GPT, Gemini, DeepSeek и другие модели из инструментов, которыми вы уже пользуетесь.

## 1. Создайте аккаунт и пополните баланс

1. [Зарегистрируйтесь](/sign-up) и подтвердите адрес электронной почты.
2. Откройте [Кошелёк](/wallet) и пополните баланс. Оплата по факту использования: каждый запрос тарифицируется по израсходованным токенам по ценам со страницы [Цены](/pricing).

## 2. Создайте API-ключ

Откройте [API-ключи](/keys) и создайте ключ. Ключ начинается с `sk-`. Скопируйте его и храните в секрете: любой, у кого есть ключ, может тратить ваш баланс. Можно создать несколько ключей, например по одному на каждый инструмент, и в любой момент отключить любой из них.

## 3. Укажите адрес API

| Тип клиента | Адрес для ввода |
|---|---|
| Совместимые с OpenAI (OpenAI SDK, Codex CLI, большинство чат-приложений) | `{{BASE_URL}}/v1` |
| Совместимые с Anthropic (Claude Code, Anthropic SDK) | `{{BASE_URL}}` |

Поддерживаемые эндпоинты включают `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations` и `/v1/models`.

## 4. Отправьте тестовый запрос

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-ваш-ключ" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "Привет"}]
  }'
```

Замените `gpt-4o-mini` на любую модель со страницы [Цены](/pricing). Указывайте название модели точно так, как оно там написано.

## Что дальше

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [Ошибки и лимиты](/docs/errors-and-limits)
