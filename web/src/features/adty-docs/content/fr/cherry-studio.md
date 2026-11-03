# Cherry Studio

1. Ouvrez **Paramètres → Fournisseurs de modèles**, cliquez sur **Ajouter** en bas, saisissez `ADTY AI` comme nom et choisissez **OpenAI** comme type de fournisseur.
2. Renseignez :

| Champ | Valeur |
|---|---|
| Clé API | `sk-votre-cle` |
| Hôte API | `{{BASE_URL}}` |

3. Cliquez sur **Vérifier** pour tester la clé, puis activez le fournisseur.
4. Cliquez sur **Gérer** (ou **Ajouter**) sous la liste des modèles et ajoutez les modèles voulus, avec les noms indiqués sur la page [Tarifs](/pricing), par exemple `claude-sonnet-4-5` ou `gpt-4o-mini`.
5. Choisissez le modèle en haut d'une conversation et commencez à discuter.

## Conseils

- Saisissez l'hôte API sans `/v1` : Cherry Studio ajoute lui-même `/v1/chat/completions`. Si les requêtes échouent avec une erreur 404, vérifiez que l'hôte ne se termine pas par `/v1`.
- La vérification échoue avec 401 : la clé est incorrecte ou désactivée. La vérification échoue avec 403 : le solde est épuisé ou la clé n'est pas autorisée à utiliser ce modèle. Voir [Erreurs et limites](/docs/errors-and-limits).
