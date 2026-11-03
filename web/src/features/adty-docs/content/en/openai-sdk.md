# OpenAI SDK

Any OpenAI SDK works: set the base URL to `{{BASE_URL}}/v1` and use your ADTY AI key. The model can be any model on the [Pricing](/pricing) page, including Claude and Gemini models.

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-your-key",  # better: read it from an environment variable
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

## Environment variables

Recent OpenAI SDKs also read `OPENAI_BASE_URL` and `OPENAI_API_KEY`, so many tools built on them work without code changes:

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-your-key"
```

The examples use streaming on purpose; see [Errors and limits](/docs/errors-and-limits) for why long non-streaming requests can be cut off.
