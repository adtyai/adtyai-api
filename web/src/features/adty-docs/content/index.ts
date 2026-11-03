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

import enCherryStudio from './en/cherry-studio.md?raw'
import enClaudeCode from './en/claude-code.md?raw'
import enCodexCli from './en/codex-cli.md?raw'
import enErrorsAndLimits from './en/errors-and-limits.md?raw'
import enFaq from './en/faq.md?raw'
import enOpenaiSdk from './en/openai-sdk.md?raw'
import enQuickstart from './en/quickstart.md?raw'
import frCherryStudio from './fr/cherry-studio.md?raw'
import frClaudeCode from './fr/claude-code.md?raw'
import frCodexCli from './fr/codex-cli.md?raw'
import frErrorsAndLimits from './fr/errors-and-limits.md?raw'
import frFaq from './fr/faq.md?raw'
import frOpenaiSdk from './fr/openai-sdk.md?raw'
import frQuickstart from './fr/quickstart.md?raw'
import jaCherryStudio from './ja/cherry-studio.md?raw'
import jaClaudeCode from './ja/claude-code.md?raw'
import jaCodexCli from './ja/codex-cli.md?raw'
import jaErrorsAndLimits from './ja/errors-and-limits.md?raw'
import jaFaq from './ja/faq.md?raw'
import jaOpenaiSdk from './ja/openai-sdk.md?raw'
import jaQuickstart from './ja/quickstart.md?raw'
import ruCherryStudio from './ru/cherry-studio.md?raw'
import ruClaudeCode from './ru/claude-code.md?raw'
import ruCodexCli from './ru/codex-cli.md?raw'
import ruErrorsAndLimits from './ru/errors-and-limits.md?raw'
import ruFaq from './ru/faq.md?raw'
import ruOpenaiSdk from './ru/openai-sdk.md?raw'
import ruQuickstart from './ru/quickstart.md?raw'
import viCherryStudio from './vi/cherry-studio.md?raw'
import viClaudeCode from './vi/claude-code.md?raw'
import viCodexCli from './vi/codex-cli.md?raw'
import viErrorsAndLimits from './vi/errors-and-limits.md?raw'
import viFaq from './vi/faq.md?raw'
import viOpenaiSdk from './vi/openai-sdk.md?raw'
import viQuickstart from './vi/quickstart.md?raw'
import zhCherryStudio from './zh-CN/cherry-studio.md?raw'
import zhClaudeCode from './zh-CN/claude-code.md?raw'
import zhCodexCli from './zh-CN/codex-cli.md?raw'
import zhErrorsAndLimits from './zh-CN/errors-and-limits.md?raw'
import zhFaq from './zh-CN/faq.md?raw'
import zhOpenaiSdk from './zh-CN/openai-sdk.md?raw'
import zhQuickstart from './zh-CN/quickstart.md?raw'
import zhTwCherryStudio from './zh-TW/cherry-studio.md?raw'
import zhTwClaudeCode from './zh-TW/claude-code.md?raw'
import zhTwCodexCli from './zh-TW/codex-cli.md?raw'
import zhTwErrorsAndLimits from './zh-TW/errors-and-limits.md?raw'
import zhTwFaq from './zh-TW/faq.md?raw'
import zhTwOpenaiSdk from './zh-TW/openai-sdk.md?raw'
import zhTwQuickstart from './zh-TW/quickstart.md?raw'

/** Doc pages in sidebar order; doc-page.tsx holds their titles. */
export const DOC_PAGES = [
  'quickstart',
  'claude-code',
  'codex-cli',
  'cherry-studio',
  'openai-sdk',
  'errors-and-limits',
  'faq',
] as const

export type DocSlug = (typeof DOC_PAGES)[number]

export const DEFAULT_DOC_SLUG: DocSlug = 'quickstart'

/** Markdown by content language; docs-complete.test.ts checks all seven are complete. */
const DOC_CONTENT: Partial<
  Record<ContentLang, Partial<Record<DocSlug, string>>>
> = {
  en: {
    quickstart: enQuickstart,
    'claude-code': enClaudeCode,
    'codex-cli': enCodexCli,
    'cherry-studio': enCherryStudio,
    'openai-sdk': enOpenaiSdk,
    'errors-and-limits': enErrorsAndLimits,
    faq: enFaq,
  },
  'zh-CN': {
    quickstart: zhQuickstart,
    'claude-code': zhClaudeCode,
    'codex-cli': zhCodexCli,
    'cherry-studio': zhCherryStudio,
    'openai-sdk': zhOpenaiSdk,
    'errors-and-limits': zhErrorsAndLimits,
    faq: zhFaq,
  },
  fr: {
    quickstart: frQuickstart,
    'claude-code': frClaudeCode,
    'codex-cli': frCodexCli,
    'cherry-studio': frCherryStudio,
    'openai-sdk': frOpenaiSdk,
    'errors-and-limits': frErrorsAndLimits,
    faq: frFaq,
  },
  ja: {
    quickstart: jaQuickstart,
    'claude-code': jaClaudeCode,
    'codex-cli': jaCodexCli,
    'cherry-studio': jaCherryStudio,
    'openai-sdk': jaOpenaiSdk,
    'errors-and-limits': jaErrorsAndLimits,
    faq: jaFaq,
  },
  ru: {
    quickstart: ruQuickstart,
    'claude-code': ruClaudeCode,
    'codex-cli': ruCodexCli,
    'cherry-studio': ruCherryStudio,
    'openai-sdk': ruOpenaiSdk,
    'errors-and-limits': ruErrorsAndLimits,
    faq: ruFaq,
  },
  vi: {
    quickstart: viQuickstart,
    'claude-code': viClaudeCode,
    'codex-cli': viCodexCli,
    'cherry-studio': viCherryStudio,
    'openai-sdk': viOpenaiSdk,
    'errors-and-limits': viErrorsAndLimits,
    faq: viFaq,
  },
  'zh-TW': {
    quickstart: zhTwQuickstart,
    'claude-code': zhTwClaudeCode,
    'codex-cli': zhTwCodexCli,
    'cherry-studio': zhTwCherryStudio,
    'openai-sdk': zhTwOpenaiSdk,
    'errors-and-limits': zhTwErrorsAndLimits,
    faq: zhTwFaq,
  },
}

export function isDocSlug(value: string): value is DocSlug {
  return DOC_PAGES.some((slug) => slug === value)
}

/** The page in the reader's language, falling back to English. */
export function getDocMarkdown(slug: DocSlug, lang: ContentLang): string {
  return DOC_CONTENT[lang]?.[slug] ?? DOC_CONTENT.en?.[slug] ?? ''
}

/** Replaces {{BASE_URL}} with the site's API address (no trailing slash). */
export function fillDocPlaceholders(markdown: string, baseUrl: string): string {
  return markdown.replaceAll('{{BASE_URL}}', baseUrl)
}
