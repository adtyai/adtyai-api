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
/** Content languages the backend stores texts in (group names, legal pages, SEO). */
export const CONTENT_LANGS = [
  'en',
  'zh-CN',
  'zh-TW',
  'fr',
  'ja',
  'ru',
  'vi',
] as const

export type ContentLang = (typeof CONTENT_LANGS)[number]

/** Native names for the language tabs in admin editors; not translated. */
export const CONTENT_LANG_LABELS: Record<ContentLang, string> = {
  en: 'English',
  'zh-CN': '简体中文',
  'zh-TW': '繁體中文',
  fr: 'Français',
  ja: '日本語',
  ru: 'Русский',
  vi: 'Tiếng Việt',
}

/** Maps an i18next interface language (zhCN, zhTW, …) to a content language. */
export function toContentLang(lng: string | undefined | null): ContentLang {
  if (lng === 'zhCN' || lng === 'zh' || lng === 'zh-CN') return 'zh-CN'
  if (lng === 'zhTW' || lng === 'zh-TW') return 'zh-TW'
  const match = CONTENT_LANGS.find((lang) => lang === lng)
  return match ?? 'en'
}

/** Picks the text for a content language, then English; '' when neither is set. */
export function pickContentText(
  texts: Partial<Record<string, string>> | undefined | null,
  lang: ContentLang
): string {
  const own = texts?.[lang]?.trim()
  if (own) return own
  return texts?.en?.trim() ?? ''
}
