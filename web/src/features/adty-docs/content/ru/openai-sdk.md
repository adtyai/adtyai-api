# OpenAI SDK

Подойдёт любой OpenAI SDK: укажите базовый URL `{{BASE_URL}}/v1` и используйте свой ключ ADTY AI. В качестве модели можно указать любую модель со страницы [Цены](/pricing), включая модели Claude и Gemini.

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-ваш-ключ",  # лучше: читайте его из переменной окружения
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Привет"}],
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
  messages: [{ role: 'user', content: 'Привет' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## Переменные окружения

Свежие версии OpenAI SDK также читают `OPENAI_BASE_URL` и `OPENAI_API_KEY`, поэтому многие инструменты на их основе работают без изменения кода:

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-ваш-ключ"
```

В примерах намеренно используется потоковая передача; почему долгие запросы без неё могут обрываться, см. в разделе [Ошибки и лимиты](/docs/errors-and-limits).
