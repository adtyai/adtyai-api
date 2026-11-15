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

import { CONTENT_LANGS, type ContentLang } from '@/lib/adty-lang'

import { DOC_PAGES, getDocMarkdown, type DocSlug } from '../content'

const contentDir = resolve(import.meta.dirname, '../content')

/**
 * 文档承诺的语言只有简中和英文（conventions.md 约定 E：正式语言只有 zh-CN 和
 * en；其余六种「不得在选择器、首页、文档和验收中承诺」）。
 *
 * 这里不是把 `CONTENT_LANGS` 改小——那是后端存储文案用的 8 种内容语言，
 * 群组名、法务页、SEO 都还在用。改的是**文档这一项的验收范围**：正式语言
 * 必须有全部 10 篇；其余六种保留仓库里已有的那 8 篇，只要求「在的不能是空的」。
 */
const OFFICIAL_DOC_LANGS = ['en', 'zh-CN'] as const satisfies readonly ContentLang[]
const LEGACY_DOC_LANGS = CONTENT_LANGS.filter(
  (lang) => !(OFFICIAL_DOC_LANGS as readonly string[]).includes(lang)
)

/** 某语言实际存在的页面（历史语言缺 models / roles 是预期状态）。 */
function presentPagesOf(lang: string): DocSlug[] {
  return DOC_PAGES.filter((slug) => {
    try {
      readFileSync(resolve(contentDir, lang, `${slug}.md`), 'utf8')
      return true
    } catch {
      return false
    }
  })
}

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

  // 历史语言不做全量承诺：仓库里有什么就验什么，缺的页面按「未承诺」处理。
  it.each(LEGACY_DOC_LANGS)(
    'keeps every existing %s page non-empty and registered',
    (lang) => {
      const present = presentPagesOf(lang)
      expect(present.length).toBeGreaterThan(0)
      for (const slug of present) {
        const markdown = readFileSync(resolve(contentDir, lang, `${slug}.md`), 'utf8')
        expect(markdown.trim(), `${lang}/${slug}.md`).not.toBe('')
        expect(getDocMarkdown(slug, lang), `${lang}/${slug} registered`).toBe(
          markdown
        )
      }
    }
  )

  it.each(LEGACY_DOC_LANGS)(
    'translates every existing %s page instead of copying English',
    (lang) => {
      for (const slug of presentPagesOf(lang)) {
        expect(getDocMarkdown(slug, lang), `${lang}/${slug}`).not.toBe(
          getDocMarkdown(slug, 'en')
        )
      }
    }
  )

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

  it.each(LEGACY_DOC_LANGS)(
    'keeps the base URL placeholder in every existing %s page',
    (lang) => {
      for (const slug of presentPagesOf(lang)) {
        const english = getDocMarkdown(slug, 'en').split('{{BASE_URL}}').length
        expect(
          getDocMarkdown(slug, lang).split('{{BASE_URL}}').length,
          `${lang}/${slug}`
        ).toBe(english)
      }
    }
  )
})
