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
import { describe, expect, test } from 'vitest'

import {
  convertDetectedLanguage,
  normalizeInterfaceLanguage,
  toIntlLocale,
} from './languages'

const chineseLanguageCases = [
  ['zhCN', 'zhCN'],
  ['zhTW', 'zhTW'],
  ['zh', 'zhCN'],
  ['zh-CN', 'zhCN'],
  ['zh-SG', 'zhCN'],
  ['zh-TW', 'zhTW'],
  ['zh-HK', 'zhTW'],
  ['zh-MO', 'zhTW'],
  ['zh-Hans', 'zhCN'],
  ['zh-Hant', 'zhTW'],
  ['zh-Hans-CN', 'zhCN'],
  ['zh-Hant-TW', 'zhTW'],
  ['zh-Hans-TW', 'zhCN'],
  ['zh-Hans-HK', 'zhCN'],
  ['zh-Hant-CN', 'zhTW'],
  ['zh-CN-x-hant', 'zhCN'],
  ['zh-TW-u-nu-hanidec', 'zhTW'],
  [' ZH_hAnT_hk ', 'zhTW'],
  [' ZH_hAnS_cn ', 'zhCN'],
  [' ZHCN ', 'zhCN'],
  [' ZHTW ', 'zhTW'],
] as const

describe('normalizeInterfaceLanguage', () => {
  test.each(['zhCN', 'en', 'fr', 'ru', 'ja', 'ko', 'vi', 'zhTW'])(
    'keeps supported interface code %s stable',
    (language) => {
      expect(normalizeInterfaceLanguage(language)).toBe(language)
    }
  )

  test.each([
    ['en-US', 'en'],
    ['en-GB', 'en'],
    ['fr-FR', 'fr'],
    ['fr-CA', 'fr'],
    ['ru-RU', 'ru'],
    ['ja-JP', 'ja'],
    ['ko-KR', 'ko'],
    ['vi-VN', 'vi'],
    [' EN_us ', 'en'],
    [' FR_fr ', 'fr'],
    [' RU_ru ', 'ru'],
    [' JA_jp ', 'ja'],
    [' KO_kr ', 'ko'],
    [' VI_vn ', 'vi'],
    ['fr-FR-u-nu-latn', 'fr'],
  ])('maps supported browser locale %s to %s', (locale, language) => {
    expect(normalizeInterfaceLanguage(locale)).toBe(language)
  })

  test.each(chineseLanguageCases)(
    'maps Chinese locale %s to %s, preserving script preference',
    (locale, language) => {
      expect(normalizeInterfaceLanguage(locale)).toBe(language)
    }
  )

  test.each([
    null,
    undefined,
    '',
    '   ',
    'es-ES',
    'de-DE',
    'fr--FR',
    'zh--TW',
    'zhx',
  ])(
    'falls back to English for missing, unsupported, or invalid locale %s',
    (locale) => {
      expect(normalizeInterfaceLanguage(locale)).toBe('en')
    }
  )
})

describe('convertDetectedLanguage', () => {
  test.each(chineseLanguageCases)(
    'maps Chinese detection %s to the same interface code %s',
    (locale, language) => {
      expect(convertDetectedLanguage(locale)).toBe(language)
    }
  )

  test('maps browser BCP-47 Chinese tags onto interface codes', () => {
    expect(convertDetectedLanguage('zh-TW')).toBe('zhTW')
    expect(convertDetectedLanguage('zh-HK')).toBe('zhTW')
    expect(convertDetectedLanguage('zh-MO')).toBe('zhTW')
    expect(convertDetectedLanguage('zh-Hant-TW')).toBe('zhTW')
    expect(convertDetectedLanguage('zh')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-CN')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-Hans')).toBe('zhCN')
  })

  test('keeps already-normalized interface codes stable (localStorage round-trip)', () => {
    // i18next caches `zhTW`/`zhCN` (the supportedLngs codes) to localStorage,
    // and the detector runs this converter on the cached value at every page
    // load — if `zhTW` does not survive the round-trip, a user who picked
    // Traditional Chinese is flipped to Simplified on the next load and the
    // cache is overwritten, making the flip permanent.
    expect(convertDetectedLanguage('zhTW')).toBe('zhTW')
    expect(convertDetectedLanguage('zhCN')).toBe('zhCN')
  })

  test('passes non-Chinese values through unchanged', () => {
    expect(convertDetectedLanguage('en')).toBe('en')
    expect(convertDetectedLanguage('fr-FR')).toBe('fr-FR')
    expect(convertDetectedLanguage('ja')).toBe('ja')
  })

  test.each(['es-ES', 'zhx', 'zh--TW', 'fr--FR', ' FR_fr ', ''])(
    'preserves detection %s when it does not identify a supported Chinese locale',
    (locale) => {
      expect(convertDetectedLanguage(locale)).toBe(locale)
    }
  )
})

describe('toIntlLocale', () => {
  test.each([
    ['zhCN', 'zh-CN'],
    ['zhTW', 'zh-TW'],
    ['fr-FR', 'fr-FR'],
    ['en-us', 'en-US'],
    ['ja', 'ja'],
  ])('converts %s into the valid Intl locale %s', (language, locale) => {
    expect(toIntlLocale(language)).toBe(locale)
  })

  test.each([null, undefined, '', '   ', 'fr_FR', 'fr--FR'])(
    'uses the runtime default for invalid or missing Intl locale %s',
    (locale) => {
      expect(toIntlLocale(locale)).toBeUndefined()
    }
  )
})
