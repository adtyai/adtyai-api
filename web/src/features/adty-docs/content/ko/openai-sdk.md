# OpenAI SDK

어떤 OpenAI SDK든 기본 URL을 `{{BASE_URL}}/v1`로 두고 ADTY AI 키를 사용하면 됩니다. 모델은 [가격](/pricing) 페이지의 Claude와 Gemini를 포함한 어떤 모델이든 가능합니다.

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-your-key",  # 더 안전하게는 환경 변수에서 읽으세요
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello"}],
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
  messages: [{ role: 'user', content: 'Hello' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## 환경 변수

최근 OpenAI SDK는 `OPENAI_BASE_URL`과 `OPENAI_API_KEY`도 읽으므로, 이를 쓰는 많은 도구는 코드를 바꾸지 않아도 됩니다.

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-your-key"
```

예제는 일부러 스트리밍을 사용합니다. 스트리밍이 아닌 긴 요청이 끊길 수 있는 이유는 [오류와 한도](/docs/errors-and-limits)를 보세요.
