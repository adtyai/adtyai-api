# 智能體

用任何相容 OpenAI 的客戶端呼叫支援工具的模型。基礎位址設為 `{{BASE_URL}}/v1`，金鑰使用 ADTY AI 的金鑰。代理迴圈留在你的程式裡：站點只回傳工具呼叫，你把工具結果放進下一次請求。

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-你的金鑰")
```

這把金鑰依一般 API 金鑰計費，不能讀取錢包或當成登入憑證。
