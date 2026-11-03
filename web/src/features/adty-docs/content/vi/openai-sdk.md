# OpenAI SDK

Mọi OpenAI SDK đều dùng được: đặt base URL là `{{BASE_URL}}/v1` và dùng khóa ADTY AI của bạn. Mô hình có thể là bất kỳ mô hình nào trên trang [Bảng giá](/pricing), kể cả các mô hình Claude và Gemini.

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-khoa-cua-ban",  # tốt hơn: đọc từ biến môi trường
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Xin chào"}],
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
  messages: [{ role: 'user', content: 'Xin chào' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## Biến môi trường

Các phiên bản OpenAI SDK gần đây cũng đọc `OPENAI_BASE_URL` và `OPENAI_API_KEY`, nên nhiều công cụ xây dựng trên SDK này hoạt động mà không cần sửa mã:

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-khoa-cua-ban"
```

Các ví dụ cố ý dùng streaming; xem [Lỗi và giới hạn](/docs/errors-and-limits) để biết vì sao các yêu cầu dài không dùng streaming có thể bị cắt ngang.
