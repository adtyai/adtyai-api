# Claude Code

Claude Code は Anthropic Messages API で通信するため、`/v1` を**付けない**アドレスを指定します。

## 環境変数

macOS と Linux（設定を残すには、これらの行を `~/.zshrc` または `~/.bashrc` に追加します）：

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-あなたのキー"
claude
```

Windows PowerShell：

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-あなたのキー"
claude
```

## 設定ファイル

環境変数の代わりに、同じ値を `~/.claude/settings.json` に書くこともできます。

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-あなたのキー"
  }
}
```

## 動作を確認する

`claude` を起動して `/status` と入力します。表示されるベース URL が `{{BASE_URL}}` になっていれば正しく設定されています。何かメッセージを送ると、そのリクエストがコンソールの利用履歴に表示されます。

## ヒント

- `ANTHROPIC_API_KEY` ではなく `ANTHROPIC_AUTH_TOKEN` を使ってください。`ANTHROPIC_API_KEY` も設定されていると、Claude Code がどちらを使うか尋ねてきたり、公式のキーを送り続けたりすることがあります。`ANTHROPIC_API_KEY` は削除してください。
- 以前に Claude アカウントでログインしていた場合は、Claude Code がキーを使うように、先に `/logout` を実行してください。
- モデルを選ぶには、Claude Code 内で `/model` を実行するか、`ANTHROPIC_MODEL` を設定します。使えるのは[料金](/pricing)ページに載っているモデルだけです。
- [API キー](/keys)ページから CC Switch で設定をインポートすることもできます。
