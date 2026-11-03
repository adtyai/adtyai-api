# Codex CLI

Codex CLI lit ses fournisseurs dans `~/.codex/config.toml` (sous Windows, `%USERPROFILE%\.codex\config.toml`). Ajoutez ADTY AI comme fournisseur et définissez-le par défaut.

## 1. Modifier config.toml

```toml
model = "gpt-5-codex"
model_provider = "adtyai"

[model_providers.adtyai]
name = "ADTY AI"
base_url = "{{BASE_URL}}/v1"
env_key = "ADTYAI_API_KEY"
wire_api = "responses"
```

- `model_provider` doit correspondre au nom qui suit `model_providers.`.
- `base_url` se termine par `/v1`.
- Donnez à `model` le nom d'un modèle listé sur la page [Tarifs](/pricing).

## 2. Définir la clé

macOS et Linux :

```bash
export ADTYAI_API_KEY="sk-votre-cle"
codex
```

Windows PowerShell :

```powershell
$env:ADTYAI_API_KEY = "sk-votre-cle"
codex
```

Le nom de la variable est celui que vous avez mis dans `env_key` ; la clé elle-même ne va jamais dans `config.toml`.

## Conseils

- Si Codex vous demande sans cesse de vous connecter avec ChatGPT, vérifiez que `model_provider` se trouve au niveau supérieur du fichier, avant toute ligne `[section]`. Les clés TOML écrites après un en-tête de section appartiennent à cette section.
- Pour conserver votre configuration actuelle, placez plutôt le fournisseur dans un profil et lancez Codex avec `codex --profile adtyai` :

```toml
[profiles.adtyai]
model = "gpt-5-codex"
model_provider = "adtyai"
```
