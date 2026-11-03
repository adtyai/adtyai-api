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

/** One language URL of the home page (adty task 9.3, decision 22). */
export type HomeLang = {
  /** URL segment: the content language code in lower case; '' for `/`. */
  segment: string
  /** i18next interface language. */
  interfaceLang: string
  contentLang: ContentLang
}

/** `/` is English; the other six languages have their own home URL. */
export const HOME_LANGS: readonly HomeLang[] = [
  { segment: '', interfaceLang: 'en', contentLang: 'en' },
  { segment: 'zh-cn', interfaceLang: 'zhCN', contentLang: 'zh-CN' },
  { segment: 'zh-tw', interfaceLang: 'zhTW', contentLang: 'zh-TW' },
  { segment: 'fr', interfaceLang: 'fr', contentLang: 'fr' },
  { segment: 'ja', interfaceLang: 'ja', contentLang: 'ja' },
  { segment: 'ru', interfaceLang: 'ru', contentLang: 'ru' },
  { segment: 'vi', interfaceLang: 'vi', contentLang: 'vi' },
]

/** localStorage key of the language picked on the home page. */
export const HOME_LANG_STORAGE_KEY = 'adtyai:lang'

export function findHomeLangBySegment(segment: string): HomeLang | undefined {
  const lower = segment.toLowerCase()
  return HOME_LANGS.find(
    (lang) => lang.segment !== '' && lang.segment === lower
  )
}

export function findHomeLangByInterface(
  interfaceLang: string | null | undefined
): HomeLang | undefined {
  return HOME_LANGS.find((lang) => lang.interfaceLang === interfaceLang)
}

/** Path of the home page in an interface language; `/` for English or unknown. */
export function homePathFor(interfaceLang: string | null | undefined): string {
  const lang = findHomeLangByInterface(interfaceLang)
  return lang && lang.segment ? `/${lang.segment}` : '/'
}

/** True for `/` and the six language home paths (trailing slash allowed). */
export function isHomePath(pathname: string): boolean {
  const trimmed = pathname.replace(/\/+$/, '')
  if (trimmed === '') return true
  return findHomeLangBySegment(trimmed.slice(1)) !== undefined
}
