/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// ADTY task 9.2: missing-translation check for the keys we added.
//
//   node scripts/adty/i18n-check.mjs
//
// "Our keys" are the en.json keys that are not in upstream-en-keys.json (the
// en.json key list of upstream v1.0.0-rc.47, refreshed on every upstream
// merge). Each of them must be in all eight locale files with a value that
// differs from English: `bun run i18n:sync` fills missing keys with the
// English text, so checking for missing keys alone is not enough.
// Exits with 1 when anything is wrong. Run from the web directory.
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const webDir = resolve(import.meta.dirname, '../..')
const localesDir = resolve(webDir, 'src/i18n/locales')
const LOCALES = ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ko', 'ru', 'vi']

// Copied from BRAND_AND_LITERAL_KEYS in scripts/sync-i18n.mjs, which is not
// exported; importing that file would run its main() and rewrite every
// locale file. Keep the two lists in step when upstream changes it.
const UPSTREAM_LITERAL_KEYS = [
  '1M token',
  'AI Proxy',
  'AIGC2D',
  'Alipay',
  'Anthropic',
  'API URL',
  'API2GPT',
  'AccessKey / SecretAccessKey',
  'AZURE_OPENAI_ENDPOINT *',
  'Baidu V2',
  'CC Switch',
  'ChatGPT',
  'ChatGPT Subscription (Codex)',
  'Claude',
  'Client ID',
  'Client Secret',
  'Cloudflare',
  'Cohere',
  'credit',
  'DeepSeek',
  'Discord',
  'DoubaoVideo',
  'FastGPT',
  'Gemini',
  'Gemini Image 4K',
  'GitHub',
  'Jimeng',
  'JustSong',
  'LingYiWanWu',
  'LinuxDO',
  'MjProxy',
  'MjProxyPlus',
  'MiniMax',
  'Mistral',
  'MokaAI',
  'Moonshot',
  'New API',
  'New API &lt;noreply@example.com&gt;',
  'NewAPI',
  'OAuth Client Secret',
  'OhMyGPT',
  'Ollama',
  'One API',
  'OpenAI',
  'OpenAIMax',
  'OpenRouter',
  'Pancake',
  'Passkey',
  'Perplexity',
  'QuantumNous',
  'Quota:',
  'Replicate',
  'SiliconFlow',
  'Stripe',
  'Submodel',
  'SunoAPI',
  'Telegram',
  'Tencent',
  'TTFT P50',
  'TTFT P95',
  'TTFT P99',
  'Uptime Kuma',
  'Uptime Kuma URL',
  'Vertex AI',
  'VolcEngine',
  'Waffo Pancake Dashboard',
  'Waffo Pancake MoR',
  'WeChat',
  'WeChat Pay',
  'Webhook URL',
  'Webhook URL:',
  'Well-Known URL',
  'Worker URL',
  'Xinference',
  'Xunfei',
  'Zhipu V4',
  '"default": "us-central1", "claude-3-5-sonnet-20240620": "europe-west1"',
  'edit_this',
  'footer.columns.related.links.midjourney',
  'footer.columns.related.links.newApiKeyTool',
  'my-status',
  'new-api-key-tool',
  'price_xxx',
  'whsec_xxx',
]

// Our own brand and technical names, kept as they are in every language.
const ADTY_LITERAL_KEYS = [
  'ADTY AI',
  'USDT',
  'TRC20',
  'BEP20',
  'Polygon',
  'X Layer',
  'Base URL',
  'Claude Code',
  'Codex CLI',
  'Cherry Studio',
  'OpenAI SDK',
  'CC Switch',
  // Network labels, amount formats and an ID name with no words to translate
  'USDT · TRC20',
  'USDT · BEP20',
  'USDT · Polygon',
  'USDT · X Layer',
  'USDT (BEpusdt)',
  '{{amount}} USDT (1 USDT = 1 USD)',
  '¥{{amount}} (1 USD = {{rate}} CNY)',
  '{{amount}} → {{net}} USDT',
  'models.dev ID',
]

// Words spelled the same in a language as in English; the value is the
// correct translation, so it may equal the English text there.
const SAME_AS_ENGLISH = {
  fr: [
    'Bonus',
    'Bonus %',
    'Bonus (USD)',
    'Commission',
    'Date',
    'Description ({{language}})',
    'Document',
    'Documentation',
    'Global',
    'Site',
    'Sites',
    'Support',
    'maintenance',
  ],
}

const LITERAL_KEYS = new Set([...UPSTREAM_LITERAL_KEYS, ...ADTY_LITERAL_KEYS])

function readTranslations(name) {
  const file = resolve(localesDir, `${name}.json`)
  return JSON.parse(readFileSync(file, 'utf8')).translation
}

const upstreamKeys = new Set(
  JSON.parse(
    readFileSync(resolve(import.meta.dirname, 'upstream-en-keys.json'), 'utf8')
  )
)
const locales = Object.fromEntries(
  LOCALES.map((name) => [name, readTranslations(name)])
)
const english = locales.en
const ourKeys = Object.keys(english).filter((key) => !upstreamKeys.has(key))

const problems = []
for (const key of ourKeys) {
  if (english[key] !== key) {
    problems.push(`en: value differs from its key: ${JSON.stringify(key)}`)
  }
  if (LITERAL_KEYS.has(key)) continue
  for (const name of LOCALES.slice(1)) {
    const value = locales[name][key]
    if (value === undefined) {
      problems.push(`${name}: missing ${JSON.stringify(key)}`)
    } else if (
      value === english[key] &&
      !SAME_AS_ENGLISH[name]?.includes(key)
    ) {
      problems.push(`${name}: same as English ${JSON.stringify(key)}`)
    }
  }
}

if (problems.length > 0) {
  for (const problem of problems) console.error(problem)
  console.error(
    `i18n-check: ${problems.length} problem(s) in ${ourKeys.length} of our keys. Add the translations (or, for a brand or technical name, add it to ADTY_LITERAL_KEYS).`
  )
  process.exit(1)
}
console.log(
  `i18n-check: ${ourKeys.length} of our keys translated in ${LOCALES.length} languages`
)
