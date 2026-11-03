# Claude Code

Claude Code talks to the Anthropic Messages API, so point it at the address **without** `/v1`.

## Environment variables

macOS and Linux (add the lines to `~/.zshrc` or `~/.bashrc` to keep them):

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

## Settings file

Instead of environment variables you can put the same values in `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-your-key"
  }
}
```

## Check it works

Run `claude`, then type `/status`. The base URL shown should be `{{BASE_URL}}`. Send any message; the request then appears in your usage logs in the console.

## Tips

- Use `ANTHROPIC_AUTH_TOKEN`, not `ANTHROPIC_API_KEY`. If `ANTHROPIC_API_KEY` is also set, Claude Code may ask which one to use or keep sending your official key; unset it.
- If you were signed in with a Claude account before, run `/logout` first so Claude Code uses the key.
- To pick a model, run `/model` inside Claude Code or set `ANTHROPIC_MODEL`. Only models listed on the [Pricing](/pricing) page work.
- You can also import the settings with CC Switch from the [API Keys](/keys) page.
