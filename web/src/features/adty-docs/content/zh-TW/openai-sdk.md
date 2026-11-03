# OpenAI SDK

任何 OpenAI SDK 都能用：把 base URL 設成 `{{BASE_URL}}/v1`，金鑰用 ADTY AI 的金鑰。模型可以是[模型價格](/pricing)頁裡的任一模型，包括 Claude 和 Gemini。

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-你的金鑰",  # 更好的做法：從環境變數讀取
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "你好"}],
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
  messages: [{ role: 'user', content: '你好' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## 環境變數

較新的 OpenAI SDK 也會讀 `OPENAI_BASE_URL` 和 `OPENAI_API_KEY`，很多以它為基礎的工具不改程式碼就能用：

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-你的金鑰"
```

範例特意用了串流輸出，原因見[錯誤碼和限制](/docs/errors-and-limits)：很長的非串流請求可能會被中斷。
