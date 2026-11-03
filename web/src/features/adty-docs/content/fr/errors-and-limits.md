# Erreurs et limites

Les erreurs suivent le format OpenAI : un code de statut HTTP accompagné d'un corps JSON contenant `error.message` et `error.code`. Le message indique ce qui s'est mal passé ; le code de statut indique quoi faire.

## Codes de statut

| Code | Signification | Que faire |
|---|---|---|
| 400 | La requête est invalide, par exemple un corps mal formé ou un paramètre que le modèle n'accepte pas | Corrigez la requête ; renvoyer la même requête échouera à nouveau |
| 401 | La clé API est absente, incorrecte, désactivée, expirée ou a épuisé son propre quota | Vérifiez l'en-tête `Authorization: Bearer sk-...` et l'état de la clé sur la page [Clés API](/keys) |
| 402 | Paiement requis. ADTY AI signale un solde insuffisant par un 403 (voir ci-dessous) : un 402 provient donc normalement du fournisseur en amont | Réessayez plus tard ; contactez le support si le problème persiste |
| 403 | Votre solde est insuffisant (`insufficient_user_quota`), ou la clé n'est pas autorisée à utiliser ce modèle ou ce groupe | [Rechargez](/wallet), ou vérifiez les paramètres de modèle et de groupe de la clé |
| 404 | Chemin incorrect, ou le modèle n'existe pas | Vérifiez l'adresse (avec ou sans `/v1`, voir les guides des clients) et le nom du modèle sur la page [Tarifs](/pricing) |
| 429 | Trop de requêtes pour votre compte (`rate_limit_exceeded`), ou le fournisseur en amont est saturé | Attendez puis réessayez ; voir les limites ci-dessous |
| 500, 502, 503 | Le fournisseur en amont a échoué ou aucun canal n'est disponible pour le modèle | Réessayez avec un délai croissant ; contactez le support si le problème persiste |
| 524 | La requête a pris trop de temps et la connexion a été fermée (voir ci-dessous) | Utilisez le streaming |

Les requêtes rejetées avec une erreur ne sont pas facturées.

## Limites par compte

Un compte peut avoir trois limites, comptées sur l'ensemble de ses clés :

- **RPM** : requêtes par minute.
- **TPM** : tokens par minute.
- **Concurrence** : requêtes exécutées en même temps.

Le dépassement de l'une d'elles renvoie un 429 avec un en-tête `Retry-After` qui indique combien de secondes attendre. Réessayez après ce délai plutôt qu'immédiatement. Si votre charge de travail nécessite des limites plus élevées, contactez le support.

## Utilisez le streaming pour les requêtes longues

Le trafic passe par Cloudflare, qui ferme une connexion n'envoyant rien pendant environ **125 secondes**. Une requête sans streaming qui dure plus longtemps (réponses longues, modèles de raisonnement, grosses modifications de code) est coupée et votre client reçoit une erreur, généralement 524, alors que le modèle était encore en train de travailler.

Définissez `"stream": true`. Une réponse en streaming commence à envoyer des données immédiatement et maintient la connexion ouverte jusqu'à ce que le modèle ait terminé. Claude Code, Codex CLI et la plupart des clients de chat utilisent déjà le streaming par défaut.
