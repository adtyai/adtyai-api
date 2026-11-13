# Agents

Call a tool-using model with any OpenAI-compatible client. Set the base URL to `{{BASE_URL}}/v1` and use an ADTY AI key. Your program owns the agent loop: the site returns the tool call, and you send the tool result back on the next request.

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-your-key")
response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Add 2 and 2."}],
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

The key is billed like any other API key. It cannot read a wallet, change a site, or act as a login.
