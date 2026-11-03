# Claude Code

Claude Code communique avec l'API Messages d'Anthropic : indiquez-lui donc l'adresse **sans** `/v1`.

## Variables d'environnement

macOS et Linux (ajoutez ces lignes à `~/.zshrc` ou `~/.bashrc` pour les conserver) :

```bash
export ANTHROPIC_BASE_URL="{{BASE_URL}}"
export ANTHROPIC_AUTH_TOKEN="sk-votre-cle"
claude
```

Windows PowerShell :

```powershell
$env:ANTHROPIC_BASE_URL = "{{BASE_URL}}"
$env:ANTHROPIC_AUTH_TOKEN = "sk-votre-cle"
claude
```

## Fichier de paramètres

Au lieu des variables d'environnement, vous pouvez mettre les mêmes valeurs dans `~/.claude/settings.json` :

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "{{BASE_URL}}",
    "ANTHROPIC_AUTH_TOKEN": "sk-votre-cle"
  }
}
```

## Vérifier le fonctionnement

Lancez `claude`, puis tapez `/status`. L'URL de base affichée doit être `{{BASE_URL}}`. Envoyez un message quelconque : la requête apparaît ensuite dans vos journaux d'utilisation dans la console.

## Conseils

- Utilisez `ANTHROPIC_AUTH_TOKEN`, et non `ANTHROPIC_API_KEY`. Si `ANTHROPIC_API_KEY` est également définie, Claude Code peut vous demander laquelle utiliser ou continuer à envoyer votre clé officielle ; supprimez-la.
- Si vous étiez connecté avec un compte Claude auparavant, lancez d'abord `/logout` pour que Claude Code utilise la clé.
- Pour choisir un modèle, lancez `/model` dans Claude Code ou définissez `ANTHROPIC_MODEL`. Seuls les modèles listés sur la page [Tarifs](/pricing) fonctionnent.
- Vous pouvez aussi importer les paramètres avec CC Switch depuis la page [Clés API](/keys).
