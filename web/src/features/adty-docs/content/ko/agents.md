# 에이전트

OpenAI 호환 클라이언트로 도구를 지원하는 모델을 호출합니다. 기본 주소는 `{{BASE_URL}}/v1` 이고, 키는 ADTY AI 키입니다. 에이전트 루프는 프로그램에 남습니다. 사이트는 도구 호출만 돌려주고, 결과는 다음 요청에 담아 보냅니다.

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-your-key")
```

이 키는 일반 API 키처럼 과금됩니다. 지갑을 읽거나 로그인으로 쓸 수 없습니다.
