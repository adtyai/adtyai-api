# クイックスタート

ADTY AI は、OpenAI API と Anthropic API に互換の API ゲートウェイです。1 つの API キーで、普段使っているツールから Claude、GPT、Gemini、DeepSeek などのモデルを呼び出せます。

## 1. アカウントを作成してチャージする

1. [サインアップ](/sign-up)して、メールアドレスを認証します。
2. [ウォレット](/wallet)を開いてチャージします。料金は従量課金です。各リクエストは使用したトークン数に応じて、[料金](/pricing)ページに表示されている価格で課金されます。

## 2. API キーを作成する

[API キー](/keys)を開いてキーを作成します。キーは `sk-` で始まります。コピーして、他人に知られないよう保管してください。キーを持っている人は誰でもあなたの残高を使えます。キーは複数作成でき（たとえばツールごとに 1 つ）、どのキーもいつでも無効にできます。

## 3. API アドレスを設定する

| クライアントの種類 | 入力するアドレス |
|---|---|
| OpenAI 互換（OpenAI SDK、Codex CLI、ほとんどのチャットアプリ） | `{{BASE_URL}}/v1` |
| Anthropic 互換（Claude Code、Anthropic SDK） | `{{BASE_URL}}` |

対応しているエンドポイントには `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/embeddings`、`/v1/images/generations`、`/v1/models` があります。

## 4. テストリクエストを送る

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-あなたのキー" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "こんにちは"}]
  }'
```

`gpt-4o-mini` は、[料金](/pricing)ページに載っている任意のモデルに置き換えられます。モデル名は、そこに表示されているとおり正確に入力してください。

## 次のステップ

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [エラーと制限](/docs/errors-and-limits)
