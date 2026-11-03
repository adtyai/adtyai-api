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
import { ADTY_BRAND_NAME, ADTY_BRAND_SLUG } from '@/lib/adty-brand'

/**
 * Client configuration snippets shown on the console quick-start card
 * (task 8.7) and the home page code card (task 8.3).
 */
export type ClientSnippetId =
  | 'claude-code'
  | 'codex'
  | 'openai-sdk'
  | 'cherry-studio'

export const CLIENT_SNIPPET_LABELS: Record<ClientSnippetId, string> = {
  'claude-code': 'Claude Code',
  codex: 'Codex CLI',
  'openai-sdk': 'OpenAI SDK',
  'cherry-studio': 'Cherry Studio',
}

export const CLIENT_SNIPPET_LANGUAGES: Record<ClientSnippetId, string> = {
  'claude-code': 'bash',
  codex: 'toml',
  'openai-sdk': 'python',
  'cherry-studio': 'plaintext',
}

export const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>'
export const DEFAULT_SNIPPET_MODEL = 'gpt-4o-mini'

/** Field names of the Cherry Studio snippet, translated by the caller. */
export type ClientSnippetLabels = {
  provider: string
  address: string
  key: string
  model: string
}

export function buildClientSnippet(
  client: ClientSnippetId,
  baseUrl: string,
  model: string,
  labels: ClientSnippetLabels
): string {
  const envKey = `${ADTY_BRAND_SLUG.toUpperCase()}_API_KEY`
  switch (client) {
    case 'claude-code':
      return [
        `export ANTHROPIC_BASE_URL=${baseUrl}`,
        `export ANTHROPIC_AUTH_TOKEN=${API_KEY_PLACEHOLDER}`,
        'claude',
      ].join('\n')
    case 'codex':
      return [
        '# ~/.codex/config.toml',
        `model_provider = "${ADTY_BRAND_SLUG}"`,
        `model = "${model}"`,
        '',
        `[model_providers.${ADTY_BRAND_SLUG}]`,
        `name = "${ADTY_BRAND_NAME}"`,
        `base_url = "${baseUrl}/v1"`,
        'wire_api = "responses"',
        `env_key = "${envKey}"`,
        '',
        `# export ${envKey}=${API_KEY_PLACEHOLDER}`,
      ].join('\n')
    case 'openai-sdk':
      return [
        'from openai import OpenAI',
        '',
        `client = OpenAI(base_url="${baseUrl}/v1", api_key="${API_KEY_PLACEHOLDER}")`,
        'resp = client.chat.completions.create(',
        `    model="${model}",`,
        '    messages=[{"role": "user", "content": "Hello"}],',
        ')',
        'print(resp.choices[0].message.content)',
      ].join('\n')
    case 'cherry-studio':
      // Cherry Studio appends /v1 to an OpenAI-type address itself.
      return [
        `${labels.provider}: OpenAI`,
        `${labels.address}: ${baseUrl}`,
        `${labels.key}: ${API_KEY_PLACEHOLDER}`,
        `${labels.model}: ${model}`,
      ].join('\n')
  }
}
