# OpenAI SDK

任何 OpenAI SDK 都能用：把 base URL 设成 `{{BASE_URL}}/v1`，密钥用 ADTY AI 的密钥。模型可以是[模型价格](/pricing)页里的任一模型，包括 Claude 和 Gemini。

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-你的密钥",  # 更好的做法：从环境变量读取
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

## 环境变量

较新的 OpenAI SDK 也会读 `OPENAI_BASE_URL` 和 `OPENAI_API_KEY`，很多基于它的工具不改代码就能用：

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-你的密钥"
```

示例特意用了流式输出，原因见[错误码和限制](/docs/errors-and-limits)：很长的非流式请求可能会被断开。
