# エージェント

OpenAI 互換クライアントでツール対応モデルを呼び出します。ベース URL は `{{BASE_URL}}/v1`、鍵は ADTY AI の鍵です。エージェントのループはあなたのプログラム側に残ります。サイトはツール呼び出しを返し、結果は次のリクエストで送ります。

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-your-key")
```

この鍵は通常の API 鍵として課金されます。ウォレットの参照やログインには使えません。
