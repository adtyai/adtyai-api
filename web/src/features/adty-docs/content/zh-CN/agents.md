# 智能体

用任何兼容 OpenAI 的客户端调用支持工具的模型。把基础地址设为 `{{BASE_URL}}/v1`，密钥用 ADTY AI 的密钥。智能体循环留在你的程序里：站点只返回工具调用，你把工具结果放进下一次请求。

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-你的密钥")
response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "计算 2 加 2。"}],
    tools=[{
        "type": "function",
        "function": {
            "name": "add",
            "parameters": {
                "type": "object",
                "properties": {"a": {"type": "number"}, "b": {"type": "number"}},
            },
        },
    }],
)
print(response.choices[0].message)
```

这把密钥按普通 API 密钥计费。它不能读取钱包、修改分站，也不能当作登录凭证。
