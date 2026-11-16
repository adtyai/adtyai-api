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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import type { ContentLang } from '@/lib/adty-lang'

import { DOC_PAGES, getDocMarkdown } from '../content'

const contentDir = resolve(import.meta.dirname, '../content')

// 文档只有简体中文和英文（conventions.md 约定 E），两种语言都必须有全部页面。
const OFFICIAL_DOC_LANGS = [
  'en',
  'zh-CN',
] as const satisfies readonly ContentLang[]

describe('docs content', () => {
  it.each(OFFICIAL_DOC_LANGS)('has every page in %s', (lang) => {
    for (const slug of DOC_PAGES) {
      const file = resolve(contentDir, lang, `${slug}.md`)
      const markdown = readFileSync(file, 'utf8')
      expect(markdown.trim(), `${lang}/${slug}.md`).not.toBe('')
      // Registered in content/index.ts, not only present on disk.
      expect(getDocMarkdown(slug, lang), `${lang}/${slug} registered`).toBe(
        markdown
      )
    }
  })

  it.each(OFFICIAL_DOC_LANGS)(
    'keeps the base URL placeholder in %s',
    (lang) => {
      for (const slug of DOC_PAGES) {
        const english = getDocMarkdown(slug, 'en').split('{{BASE_URL}}').length
        expect(
          getDocMarkdown(slug, lang).split('{{BASE_URL}}').length,
          `${lang}/${slug}`
        ).toBe(english)
      }
    }
  )
})
