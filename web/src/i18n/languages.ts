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

/** Official interface languages offered to users (A.20.1): Simplified Chinese and English. */
export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zhCN', label: '简体中文' },
  { code: 'en', label: 'English' },
] as const

/**
 * Locale resource codes that may still exist on disk, including leftover
 * unofficial packs. These are not official product languages and must not be
 * offered in official selectors (A.20.1).
 */
export const LOADABLE_INTERFACE_LANGUAGE_CODES = [
  'zhCN',
  'en',
  'fr',
  'ru',
  'ja',
  'ko',
  'vi',
  'zhTW',
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

export function normalizeInterfaceLanguage(
  value?: string | null
): InterfaceLanguageCode {
  if (!value) return 'en'

  const normalized = value.trim().replaceAll('_', '-').toLowerCase()
  if (normalized === 'zhcn' || normalized === 'zhtw') return 'zhCN'

  try {
    const locale = new Intl.Locale(toIntlLocale(normalized) ?? 'en')
    if (locale.language === 'zh') {
      // Official product language is Simplified Chinese. Unofficial Traditional
      // / Taiwan / HK / MO / Hant variants fall back to zhCN, not zhTW.
      return 'zhCN'
    }

    return 'en'
  } catch {
    return 'en'
  }
}

/**
 * Map a browser-detected or previously stored locale onto an official
 * interface language (`zhCN` or `en`). Unofficial leftover codes such as
 * `zhTW` or `fr` are coerced so a leftover cookie or profile value cannot
 * keep the UI running in an unofficial language.
 */
export function convertDetectedLanguage(value: string): InterfaceLanguageCode {
  return normalizeInterfaceLanguage(value)
}

/**
 * Convert an interface language code (the values i18next uses, such as `zhCN` /
 * `zhTW`) into a valid BCP-47 locale tag that the `Intl.*` APIs accept.
 *
 * `new Intl.NumberFormat('zhCN')` throws `RangeError: Invalid language tag`, so
 * any locale derived from `i18n.language` / `i18n.resolvedLanguage` MUST be run
 * through this before it reaches an `Intl` constructor. Unknown values fall back
 * to `undefined`, which makes `Intl` use the runtime default locale.
 */
export function toIntlLocale(value?: string | null): string | undefined {
  if (!value) return undefined
  switch (value) {
    case 'zhCN':
      return 'zh-CN'
    case 'zhTW':
      return 'zh-TW'
    default:
      break
  }
  try {
    return Intl.getCanonicalLocales(value)[0]
  } catch {
    return undefined
  }
}
