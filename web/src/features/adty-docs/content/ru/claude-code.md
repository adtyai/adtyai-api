# Claude Code

Claude Code работает с Anthropic Messages API, поэтому указывайте ему адрес **без** `/v1`.

## Переменные окружения

macOS и Linux (добавьте эти строки в `~/.zshrc` или `~/.bashrc`, чтобы они сохранялись):

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-ваш-ключ"
claude
```

Windows PowerShell:

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-ваш-ключ"
claude
```

## Файл настроек

Вместо переменных окружения те же значения можно указать в `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-ваш-ключ"
  }
}
```

## Проверка

Запустите `claude` и введите `/status`. Базовый URL должен быть `{{BASE_URL}}`. Отправьте любое сообщение: после этого запрос появится в журнале использования в консоли.

## Советы

- Используйте `ANTHROPIC_AUTH_TOKEN`, а не `ANTHROPIC_API_KEY`. Если задана ещё и `ANTHROPIC_API_KEY`, Claude Code может спросить, какую из них использовать, или продолжить отправлять ваш официальный ключ; удалите её.
- Если раньше вы входили через аккаунт Claude, сначала выполните `/logout`, чтобы Claude Code использовал ключ.
- Чтобы выбрать модель, выполните `/model` в Claude Code или задайте `ANTHROPIC_MODEL`. Работают только модели со страницы [Цены](/pricing).
- Настройки также можно импортировать через CC Switch со страницы [API-ключи](/keys).
