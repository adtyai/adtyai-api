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
/**
 * Content languages the product edits (group names, legal pages, SEO). The
 * product only supports Simplified Chinese and English, so those are the only
 * tabs the admin editors offer.
 */
export const CONTENT_LANGS = ['en', 'zh-CN'] as const

export type ContentLang = (typeof CONTENT_LANGS)[number]

/**
 * Leftover keys that may still sit inside stored JSON from before the product
 * converged on two content languages. They are read back and preserved when
 * saving, but never offered as a tab.
 */
export const LEGACY_CONTENT_LANGS = [
  'zh-TW',
  'fr',
  'ja',
  'ko',
  'ru',
  'vi',
] as const

export type LegacyContentLang = (typeof LEGACY_CONTENT_LANGS)[number]

/** Every content-language key that can appear in stored texts. */
export type StoredContentLang = ContentLang | LegacyContentLang

/** Stable key order for saved JSON: offered languages first, then leftovers. */
export const STORED_CONTENT_LANGS = [
  ...CONTENT_LANGS,
  ...LEGACY_CONTENT_LANGS,
] as const satisfies readonly StoredContentLang[]

/** Native names for the language tabs in admin editors; not translated. */
export const CONTENT_LANG_LABELS: Record<ContentLang, string> = {
  en: 'English',
  'zh-CN': '简体中文',
}

/** Maps an i18next interface language (zhCN, zh, …) to a content language. */
export function toContentLang(lng: string | undefined | null): ContentLang {
  if (lng === 'zhCN' || lng === 'zh' || lng === 'zh-CN') return 'zh-CN'
  // Traditional Chinese is not a product language: it reads the Simplified text.
  if (lng === 'zhTW' || lng === 'zh-TW') return 'zh-CN'
  const match = CONTENT_LANGS.find((lang) => lang === lng)
  return match ?? 'en'
}

/** Picks the text for a content language, then English; '' when neither is set. */
export function pickContentText(
  texts: Partial<Record<StoredContentLang, string>> | undefined | null,
  lang: ContentLang
): string {
  const own = texts?.[lang]?.trim()
  if (own) return own
  return texts?.en?.trim() ?? ''
}
