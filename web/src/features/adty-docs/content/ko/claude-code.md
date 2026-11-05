# Claude Code

Claude Code는 Anthropic Messages API를 사용하므로 주소에서 `/v1`을 **빼야** 합니다.

## 환경 변수

macOS와 Linux (`~/.zshrc` 또는 `~/.bashrc`에 넣으면 유지됩니다):

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-your-key"
claude
```

Windows PowerShell:

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-your-key"
claude
```

## 설정 파일

환경 변수 대신 `~/.claude/settings.json`에 같은 값을 넣을 수 있습니다.

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-your-key"
  }
}
```

## 동작 확인

`claude`를 실행한 뒤 `/status`를 입력하세요. 표시된 기본 주소가 `{{BASE_URL}}`이어야 합니다. 메시지를 보내면 콘솔의 사용 로그에 요청이 나타납니다.

## 참고

- `ANTHROPIC_API_KEY`가 아니라 `ANTHROPIC_AUTH_TOKEN`을 사용하세요. `ANTHROPIC_API_KEY`도 설정되어 있으면 Claude Code가 어느 키를 쓸지 묻거나 공식 키를 계속 보낼 수 있습니다. 그 변수는 지우세요.
- 예전에 Claude 계정으로 로그인한 적이 있다면 먼저 `/logout`을 실행해 키를 쓰게 하세요.
- 모델을 고르려면 Claude Code 안에서 `/model`을 실행하거나 `ANTHROPIC_MODEL`을 설정하세요. [가격](/pricing) 페이지에 있는 모델만 동작합니다.
- [API 키](/keys) 페이지의 CC Switch로 설정을 가져올 수도 있습니다.
