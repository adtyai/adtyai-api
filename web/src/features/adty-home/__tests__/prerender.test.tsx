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
// @vitest-environment node
// Runs without jsdom, so a block that reads window, document or localStorage
// while rendering fails here, as it would in the build-time prerender.
import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ko from '@/i18n/locales/ko.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zhCN from '@/i18n/locales/zh.json'

import { renderAdtyHome, renderAllAdtyHomes } from '../prerender'

const HERO_TITLE = 'Claude, GPT and Gemini with one API key'

const officialHeroByLang = {
  en: en.translation[HERO_TITLE],
  'zh-CN': zhCN.translation[HERO_TITLE],
} as const

describe('home page prerender', () => {
  it('renders only the official home languages', async () => {
    const pages = await renderAllAdtyHomes()

    expect(Object.keys(pages).sort()).toEqual(['en', 'zh-CN'])
    // Leftover unofficial locale files remain on disk (A.20.1).
    expect(fr).toBeTruthy()
    expect(ja).toBeTruthy()
    expect(ko).toBeTruthy()
    expect(ru).toBeTruthy()
    expect(vi).toBeTruthy()
    expect(zhTW).toBeTruthy()
  })

  it.each(Object.entries(officialHeroByLang))(
    'puts the %s hero title in the page',
    async (lang, title) => {
      expect(title).toBeTruthy()
      const html = await renderAdtyHome(lang as 'en' | 'zh-CN')

      expect(html).toContain(`<h1 id="hero-title"`)
      // React escapes text; the titles have no characters it would change.
      expect(html).toContain(title)
    }
  )

  it('never shows the upstream name', async () => {
    const pages = await renderAllAdtyHomes()

    for (const [lang, html] of Object.entries(pages)) {
      expect(html, lang).not.toMatch(/new api/i)
    }
  })

  it('has the static header with the site name and no footer', async () => {
    const html = await renderAdtyHome('en')

    expect(html).toContain('<header')
    expect(html).toContain('ADTY AI')
    expect(html).not.toContain('<footer')
  })

  it('links with plain anchors to the sign-up and pricing pages', async () => {
    const html = await renderAdtyHome('en')

    expect(html).toContain('href="/sign-up"')
    expect(html).toContain('href="/pricing"')
  })
})
