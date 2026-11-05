# Codex CLI

Codex CLI는 제공자를 `~/.codex/config.toml`(Windows에서는 `%USERPROFILE%\.codex\config.toml`)에서 읽습니다. ADTY AI를 제공자로 추가하고 기본값으로 지정하세요.

## 1. config.toml 수정

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider`는 `model_providers.` 뒤의 이름과 같아야 합니다.
- `base_url`은 `/v1`로 끝납니다.
- `model`은 [가격](/pricing) 페이지에 있는 모델로 설정하세요.

## 2. 키 설정

macOS와 Linux:

```bash
export ADTYAI_API_KEY="sk-your-key"
codex
```

Windows PowerShell:

```powershell
$env:ADTYAI_API_KEY = "sk-your-key"
codex
```

변수 이름은 `env_key`에 적은 이름입니다. 키 자체는 `config.toml`에 넣지 않습니다.

## 참고

- Codex가 ChatGPT 로그인을 계속 요구하면 `model_provider`가 파일 최상단, 어떤 `[section]`보다 앞에 있는지 확인하세요. 섹션 헤더 뒤에 쓴 TOML 키는 그 섹션에 속합니다.
- 기존 설정을 유지하려면 제공자를 프로필에 넣고 `codex --profile adtyai`로 시작하세요.

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
