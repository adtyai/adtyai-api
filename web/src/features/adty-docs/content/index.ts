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
import type { ContentLang } from '@/lib/adty-lang'

import enAgents from './en/agents.md?raw'
import enCherryStudio from './en/cherry-studio.md?raw'
import enClaudeCode from './en/claude-code.md?raw'
import enCodexCli from './en/codex-cli.md?raw'
import enErrorsAndLimits from './en/errors-and-limits.md?raw'
import enFaq from './en/faq.md?raw'
import enModels from './en/models.md?raw'
import enOpenaiSdk from './en/openai-sdk.md?raw'
import enQuickstart from './en/quickstart.md?raw'
import enRoles from './en/roles.md?raw'
import zhAgents from './zh-CN/agents.md?raw'
import zhCherryStudio from './zh-CN/cherry-studio.md?raw'
import zhClaudeCode from './zh-CN/claude-code.md?raw'
import zhCodexCli from './zh-CN/codex-cli.md?raw'
import zhErrorsAndLimits from './zh-CN/errors-and-limits.md?raw'
import zhFaq from './zh-CN/faq.md?raw'
import zhModels from './zh-CN/models.md?raw'
import zhOpenaiSdk from './zh-CN/openai-sdk.md?raw'
import zhQuickstart from './zh-CN/quickstart.md?raw'
import zhRoles from './zh-CN/roles.md?raw'

/** Doc pages in sidebar order; doc-page.tsx holds their titles. */
export const DOC_PAGES = [
  'quickstart',
  'models',
  'roles',
  'claude-code',
  'codex-cli',
  'cherry-studio',
  'openai-sdk',
  'agents',
  'errors-and-limits',
  'faq',
] as const

export type DocSlug = (typeof DOC_PAGES)[number]

export const DEFAULT_DOC_SLUG: DocSlug = 'quickstart'

/** Markdown by content language; docs-complete.test.ts checks both languages. */
const DOC_CONTENT: Record<ContentLang, Record<DocSlug, string>> = {
  en: {
    quickstart: enQuickstart,
    models: enModels,
    roles: enRoles,
    'claude-code': enClaudeCode,
    'codex-cli': enCodexCli,
    'cherry-studio': enCherryStudio,
    'openai-sdk': enOpenaiSdk,
    agents: enAgents,
    'errors-and-limits': enErrorsAndLimits,
    faq: enFaq,
  },
  'zh-CN': {
    quickstart: zhQuickstart,
    models: zhModels,
    roles: zhRoles,
    'claude-code': zhClaudeCode,
    'codex-cli': zhCodexCli,
    'cherry-studio': zhCherryStudio,
    'openai-sdk': zhOpenaiSdk,
    agents: zhAgents,
    'errors-and-limits': zhErrorsAndLimits,
    faq: zhFaq,
  },
}

export function isDocSlug(value: string): value is DocSlug {
  return DOC_PAGES.some((slug) => slug === value)
}

/** The page in the reader's language. */
export function getDocMarkdown(slug: DocSlug, lang: ContentLang): string {
  return DOC_CONTENT[lang][slug]
}

/** Replaces {{BASE_URL}} with the site's API address (no trailing slash). */
export function fillDocPlaceholders(markdown: string, baseUrl: string): string {
  return markdown.replaceAll('{{BASE_URL}}', baseUrl)
}
