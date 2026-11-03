# OpenAI SDK

N'importe quel OpenAI SDK fonctionne : définissez l'URL de base sur `{{BASE_URL}}/v1` et utilisez votre clé ADTY AI. Le modèle peut être n'importe quel modèle de la page [Tarifs](/pricing), y compris les modèles Claude et Gemini.

## Python

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(
    base_url="{{BASE_URL}}/v1",
    api_key="sk-votre-cle",  # mieux : la lire depuis une variable d'environnement
)

stream = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Bonjour"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

## Node.js

```bash
npm install openai
```

```javascript
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: '{{BASE_URL}}/v1',
  apiKey: process.env.ADTYAI_API_KEY,
})

const stream = await client.chat.completions.create({
  model: 'gpt-4o-mini',
  messages: [{ role: 'user', content: 'Bonjour' }],
  stream: true,
})
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content ?? '')
}
```

## Variables d'environnement

Les versions récentes des OpenAI SDK lisent aussi `OPENAI_BASE_URL` et `OPENAI_API_KEY` : de nombreux outils construits dessus fonctionnent donc sans modification du code :

```bash
export OPENAI_BASE_URL="{{BASE_URL}}/v1"
export OPENAI_API_KEY="sk-votre-cle"
```

Les exemples utilisent volontairement le streaming ; voir [Erreurs et limites](/docs/errors-and-limits) pour comprendre pourquoi les requêtes longues sans streaming peuvent être coupées.
