# Agents

Appelez un modèle avec outils via un client compatible OpenAI. L'adresse de base est `{{BASE_URL}}/v1`, avec une clé ADTY AI. La boucle de l'agent reste dans votre programme : le site renvoie l'appel d'outil, et vous renvoyez le résultat ensuite.

```python
from openai import OpenAI

client = OpenAI(base_url="{{BASE_URL}}/v1", api_key="sk-votre-cle")
```

La clé est facturée comme une clé d'API ordinaire. Elle ne lit pas un portefeuille et ne sert pas de connexion.
