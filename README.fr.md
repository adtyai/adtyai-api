<div align="center">

![adtyai](/web/public/logo.png)

# ADTY AI

**Passerelle IA auto-hébergée (fork adtyai)**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <strong>Français</strong> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/license-AGPL--3.0-brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/adtyai/adtyai-api">
    <img src="https://img.shields.io/badge/version-v1.0.0--adtyai-brightgreen" alt="version">
  </a><!--
  --><a href="https://github.com/adtyai/adtyai-api/pkgs/container/adtyai-api">
    <img src="https://img.shields.io/badge/image-ghcr.io%2Fadtyai%2Fadtyai--api-blue" alt="ghcr">
  </a>
</p>

<p align="center">
  <a href="#a-propos">À propos</a> •
  <a href="#fonctionnalites-adty">Fonctionnalités ADTY</a> •
  <a href="#fonctionnalites-amont">Fonctionnalités amont</a> •
  <a href="#demarrage-rapide">Démarrage rapide</a> •
  <a href="#deploiement">Déploiement</a> •
  <a href="#licence">Licence</a>
</p>

</div>

---

<a id="a-propos"></a>

## À propos

**ADTY AI** (dépôt **adtyai-api**) est une passerelle IA auto-hébergée pour opérateurs de sites relais. Elle conserve les canaux, modèles et console New API amont, et ajoute facturation, recharge, affiliation, revue des risques et ops propres à ce fork.

- Dépôt : <https://github.com/adtyai/adtyai-api>
- Image : `ghcr.io/adtyai/adtyai-api` (ex. `:main` et tags de build datés ; publication multi-arch selon les tags git)
- Branche par défaut : `main` (modifier et pousser uniquement sur `main`)

Fork de [New API](https://github.com/QuantumNous/new-api) sous AGPL-3.0. Merci aux auteurs et contributeurs amont.

> [!IMPORTANT]
> Uniquement pour passerelle et déploiement privé légalement autorisés. Les opérateurs doivent obtenir les clés amont légalement et respecter la conformité locale (déclarations, paiements, sécurité des contenus, etc.).

---

<a id="fonctionnalites-adty"></a>

## Fonctionnalités ADTY

| Domaine | Contenu |
| --- | --- |
| Facturation double portefeuille | Bonus d’abord, puis solde payé ; répartition horaire selon le financement réel ; réconciliation quotidienne |
| Recharge | Cadeaux par paliers uniquement (pas de bonus première recharge) ; EPay et BEpusdt (USDT) |
| Affiliation | Commission sur payé moins remboursements ; revue des retraits ; rattrapage au démarrage en cas d’omissions |
| Remboursements & risques | Reprise proportionnelle des cadeaux/affiliation ; file de risque manuelle ; commandes et grands livres admin |
| Tarifs entreprise | Console : Facturation & paiements → Tarifs entreprise (multiplicateurs de groupe masqués) |
| Routage intelligent | Menu latéral dédié ; routage par score / canary / schémas de poids ; garde-fou coût `/smart-routing/cost` |
| Modules de barre latérale | Intégré aux interrupteurs officiels de modules ; store menu-visibility indépendant supprimé |
| Support | Chat en direct Chatwoot (widget + réglages admin ; script réservé au root ; contact de secours si la sonde échoue) |
| Paramètres métier | Paramètres d’ops réglables dans la console |
| Ops UX | Tâches de sauvegarde/rapport sous les groupes de barre latérale officiels |

La marque visible par l’utilisateur est **ADTY AI**, pas New API comme nom de site.

---

<a id="fonctionnalites-amont"></a>

## Fonctionnalités amont (héritées)

Vous disposez toujours des capacités New API : canaux multi-fournisseurs, quotas/abonnements, journaux d’usage, utilisateurs/groupes/clés API, OAuth/passkeys, plugins de tâches async, console multilingue. Points d’entrée protocole : OpenAI Chat/Responses, Anthropic Messages, Gemini, etc. (voir le code et la doc amont).

---

<a id="demarrage-rapide"></a>

## Démarrage rapide

### Docker (nœud unique)

```bash
mkdir -p data
docker run --name adtyai-api -d --restart unless-stopped \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v "$(pwd)/data:/data" \
  ghcr.io/adtyai/adtyai-api:main
```

Ouvrez [http://localhost:3000](http://localhost:3000) et créez un compte admin. Les données sont dans le répertoire `data` monté.

### Premier appel API

1. Configurez un canal et les tarifs ; vérifiez que l’utilisateur a du quota.
2. Créez une clé API dans la console.
3. Définissez l’URL de base du client sur `http://localhost:3000/v1` et utilisez une clé émise par ce site.

```bash
curl --fail-with-body http://localhost:3000/v1/models \
  -H "Authorization: Bearer ${ADTY_API_KEY}"
```

---

<a id="deploiement"></a>

## Déploiement

Préférez [docker-compose.yml](./docker-compose.yml) (PostgreSQL + Redis par défaut) :

```bash
git clone https://github.com/adtyai/adtyai-api.git
cd adtyai-api
# Éditer compose : remplacer les mots de passe d’exemple ; définir SESSION_SECRET
docker compose up -d
```

Configurez HTTPS, secrets et sauvegardes en production. La publication d’images suit GitHub Actions / tags. Les URL de prévisualisation sont publiées par l’ops (exemple fixe : `https://newapi.n8-n.top`).

---

<a id="licence"></a>

## Licence

[AGPL-3.0](./LICENSE). Respectez l’AGPL lors de la redistribution ou de l’offre d’un service réseau.

Basé sur [New API](https://github.com/QuantumNous/new-api). Merci, amont.
