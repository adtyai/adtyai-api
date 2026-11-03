# Démarrage rapide

ADTY AI est une passerelle d'API compatible avec les API OpenAI et Anthropic. Utilisez une seule clé API pour appeler Claude, GPT, Gemini, DeepSeek et d'autres modèles depuis les outils que vous utilisez déjà.

## 1. Créer un compte et le recharger

1. [Inscrivez-vous](/sign-up) et vérifiez votre adresse e-mail.
2. Ouvrez [Portefeuille](/wallet) et rechargez votre compte. La facturation se fait à l'usage : chaque requête est facturée selon les tokens qu'elle consomme, aux prix indiqués sur la page [Tarifs](/pricing).

## 2. Créer une clé API

Ouvrez [Clés API](/keys) et créez une clé. La clé commence par `sk-`. Copiez-la et gardez-la secrète : toute personne qui possède la clé peut dépenser votre solde. Vous pouvez créer plusieurs clés, par exemple une par outil, et désactiver n'importe laquelle à tout moment.

## 3. Utiliser l'adresse de l'API

| Type de client | Adresse à saisir |
|---|---|
| Compatible OpenAI (OpenAI SDK, Codex CLI, la plupart des applications de chat) | `{{BASE_URL}}/v1` |
| Compatible Anthropic (Claude Code, Anthropic SDK) | `{{BASE_URL}}` |

Les endpoints pris en charge incluent `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations` et `/v1/models`.

## 4. Envoyer une requête de test

```bash
curl {{BASE_URL}}/v1/chat/completions \
  -H "Authorization: Bearer sk-votre-cle" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "messages": [{"role": "user", "content": "Bonjour"}]
  }'
```

Remplacez `gpt-4o-mini` par n'importe quel modèle listé sur la page [Tarifs](/pricing). Utilisez le nom du modèle exactement comme il y est affiché.

## Étapes suivantes

- [Claude Code](/docs/claude-code)
- [Codex CLI](/docs/codex-cli)
- [Cherry Studio](/docs/cherry-studio)
- [OpenAI SDK](/docs/openai-sdk)
- [Erreurs et limites](/docs/errors-and-limits)
