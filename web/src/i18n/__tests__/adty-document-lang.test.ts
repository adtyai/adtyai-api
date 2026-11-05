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
import i18next from 'i18next'
import { afterEach, describe, expect, it } from 'vitest'

import { bindDocumentLang } from '../adty-document-lang'

const resources = {
  en: { translation: {} },
  zhCN: { translation: {} },
  zhTW: { translation: {} },
  fr: { translation: {} },
  ja: { translation: {} },
  ko: { translation: {} },
  ru: { translation: {} },
  vi: { translation: {} },
}

async function createI18n(lng: string) {
  const instance = i18next.createInstance()
  await instance.init({ lng, fallbackLng: 'en', resources })
  return instance
}

afterEach(() => {
  document.documentElement.lang = ''
})

describe('bindDocumentLang', () => {
  it('sets the html lang of the current language when bound', async () => {
    const instance = await createI18n('ja')

    bindDocumentLang(instance)

    expect(document.documentElement.lang).toBe('ja')
  })

  it.each([
    ['zhCN', 'zh-CN'],
    ['zhTW', 'zh-TW'],
    ['en', 'en'],
    ['fr', 'fr'],
    ['ru', 'ru'],
    ['vi', 'vi'],
    ['ja', 'ja'],
    ['ko', 'ko'],
  ])('writes %s as the BCP 47 tag %s after a switch', async (lng, tag) => {
    const instance = await createI18n('en')
    bindDocumentLang(instance)

    await instance.changeLanguage(lng)

    expect(document.documentElement.lang).toBe(tag)
  })

  it('falls back to en for a code that is not a language tag', async () => {
    const instance = await createI18n('en')
    bindDocumentLang(instance)

    await instance.changeLanguage('not a tag!')

    expect(document.documentElement.lang).toBe('en')
  })
})
