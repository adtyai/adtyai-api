# OpenAI SDK

どの OpenAI SDK でも使えます。ベース URL を `{{BASE_URL}}/v1` に設定し、ADTY AI のキーを使ってください。モデルには、Claude や Gemini のモデルも含め、[料金](/pricing)ページに載っている任意のモデルを指定できます。

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-あなたのキー",  # 環境変数から読み込むほうが安全です
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "こんにちは"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

## Node.js

```bash
npm install openai
```

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: '{{BASE_URL}}/v1',
  apiKey: process.env.ADTYAI_API_KEY,
})

const stream = await client.chat.completions.create({
  model: 'gpt-4o-mini',
  messages: [{ role: 'user', content: 'こんにちは' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## 環境変数

最近の OpenAI SDK は `OPENAI_BASE_URL` と `OPENAI_API_KEY` も読み込むため、それらを使って作られた多くのツールはコードを変更せずに動作します。

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-あなたのキー"
```

例では意図的にストリーミングを使っています。長い非ストリーミングのリクエストが途中で切断される理由は、[エラーと制限](/docs/errors-and-limits)を参照してください。
