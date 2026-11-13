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

import en from './locales/en.json'
import fr from './locales/fr.json'
import ja from './locales/ja.json'
import ko from './locales/ko.json'
import ru from './locales/ru.json'
import vi from './locales/vi.json'
import zhTW from './locales/zh-TW.json'
import zhCN from './locales/zh.json'
import {
  convertDetectedLanguage,
  INTERFACE_LANGUAGE_OPTIONS,
  LOADABLE_INTERFACE_LANGUAGE_CODES,
  normalizeInterfaceLanguage,
  toIntlLocale,
} from './languages'

const officialChineseCases = [
  ['zhCN', 'zhCN'],
  ['zhTW', 'zhCN'],
  ['zh', 'zhCN'],
  ['zh-CN', 'zhCN'],
  ['zh-SG', 'zhCN'],
  ['zh-TW', 'zhCN'],
  ['zh-HK', 'zhCN'],
  ['zh-MO', 'zhCN'],
  ['zh-Hans', 'zhCN'],
  ['zh-Hant', 'zhCN'],
  ['zh-Hans-CN', 'zhCN'],
  ['zh-Hant-TW', 'zhCN'],
  ['zh-Hans-TW', 'zhCN'],
  ['zh-Hans-HK', 'zhCN'],
  ['zh-Hant-CN', 'zhCN'],
  ['zh-CN-x-hant', 'zhCN'],
  ['zh-TW-u-nu-hanidec', 'zhCN'],
  [' ZH_hAnT_hk ', 'zhCN'],
  [' ZH_hAnS_cn ', 'zhCN'],
  [' ZHCN ', 'zhCN'],
  [' ZHTW ', 'zhCN'],
] as const

describe('INTERFACE_LANGUAGE_OPTIONS', () => {
  test('offers only official Simplified Chinese and English', () => {
    expect(INTERFACE_LANGUAGE_OPTIONS.map((lang) => lang.code)).toEqual([
      'zhCN',
      'en',
    ])
    expect(INTERFACE_LANGUAGE_OPTIONS.map((lang) => lang.code)).not.toEqual(
      expect.arrayContaining(['fr', 'ru', 'ja', 'ko', 'vi', 'zhTW'])
    )
  })

  test('keeps leftover unofficial locale resources on disk', () => {
    expect([...LOADABLE_INTERFACE_LANGUAGE_CODES]).toEqual(
      expect.arrayContaining([
        'zhCN',
        'en',
        'fr',
        'ru',
        'ja',
        'ko',
        'vi',
        'zhTW',
      ])
    )
    expect(zhCN).toBeTruthy()
    expect(en).toBeTruthy()
    expect(fr).toBeTruthy()
    expect(ru).toBeTruthy()
    expect(ja).toBeTruthy()
    expect(ko).toBeTruthy()
    expect(vi).toBeTruthy()
    expect(zhTW).toBeTruthy()
  })
})

describe('normalizeInterfaceLanguage', () => {
  test.each(['zhCN', 'en'] as const)(
    'keeps official interface code %s stable',
    (language) => {
      expect(normalizeInterfaceLanguage(language)).toBe(language)
    }
  )

  test.each([
    ['en-US', 'en'],
    ['en-GB', 'en'],
    ['fr', 'en'],
    ['fr-FR', 'en'],
    ['fr-CA', 'en'],
    ['ru', 'en'],
    ['ru-RU', 'en'],
    ['ja', 'en'],
    ['ja-JP', 'en'],
    ['ko', 'en'],
    ['ko-KR', 'en'],
    ['vi', 'en'],
    ['vi-VN', 'en'],
    [' EN_us ', 'en'],
    [' FR_fr ', 'en'],
    [' RU_ru ', 'en'],
    [' JA_jp ', 'en'],
    [' KO_kr ', 'en'],
    [' VI_vn ', 'en'],
    ['fr-FR-u-nu-latn', 'en'],
  ])('maps unofficial or regional locale %s to official %s', (locale, language) => {
    expect(normalizeInterfaceLanguage(locale)).toBe(language)
  })

  test.each(officialChineseCases)(
    'maps Chinese locale %s, including unofficial variants, to official %s',
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
  test.each(officialChineseCases)(
    'maps Chinese detection %s to official %s',
    (locale, language) => {
      expect(convertDetectedLanguage(locale)).toBe(language)
    }
  )

  test('maps browser BCP-47 Chinese tags onto official Simplified Chinese', () => {
    expect(convertDetectedLanguage('zh-TW')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-HK')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-MO')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-Hant-TW')).toBe('zhCN')
    expect(convertDetectedLanguage('zh')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-CN')).toBe('zhCN')
    expect(convertDetectedLanguage('zh-Hans')).toBe('zhCN')
  })

  test('coerces a leftover stored unofficial code to the official language', () => {
    expect(convertDetectedLanguage('zhTW')).toBe('zhCN')
    expect(convertDetectedLanguage('zhCN')).toBe('zhCN')
    expect(convertDetectedLanguage('fr')).toBe('en')
    expect(convertDetectedLanguage('ja')).toBe('en')
  })

  test('maps unofficial and unknown detections to official English', () => {
    expect(convertDetectedLanguage('en')).toBe('en')
    expect(convertDetectedLanguage('fr-FR')).toBe('en')
    expect(convertDetectedLanguage('ja')).toBe('en')
    expect(convertDetectedLanguage('es-ES')).toBe('en')
    expect(convertDetectedLanguage('zhx')).toBe('en')
    expect(convertDetectedLanguage('zh--TW')).toBe('en')
    expect(convertDetectedLanguage('fr--FR')).toBe('en')
    expect(convertDetectedLanguage(' FR_fr ')).toBe('en')
    expect(convertDetectedLanguage('')).toBe('en')
  })
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
