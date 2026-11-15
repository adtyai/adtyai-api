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
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import { bindDocumentLang } from './adty-document-lang'
import {
  createI18nOptions,
  installInterfaceLanguageNormalization,
  toInterfaceLanguage,
} from './init-options'
import { normalizeInterfaceLanguage } from './languages'

// The product ships two interface languages only: Simplified Chinese and
// English (conventions E / A.20.1). Leftover packs on disk are never loaded.
const localeLoaders = {
  en: () => import('./locales/en.json'),
  zhCN: () => import('./locales/zh.json'),
} as const

/** Load one language pack. The other languages stay out of the first page load. */
export async function loadInterfaceLanguage(
  value: string | undefined
): Promise<void> {
  const code = toInterfaceLanguage(value)
  if (i18n.hasResourceBundle(code, 'translation')) return
  const pack = await localeLoaders[code]()
  const messages = pack.default as { translation?: object }
  i18n.addResourceBundle(
    code,
    'translation',
    messages.translation ?? pack.default,
    true,
    true
  )
}

export function readStoredInterfaceLanguage(): string {
  if (typeof window === 'undefined') return 'en'
  let storedLanguage: string | null = null
  try {
    storedLanguage = window.localStorage.getItem('i18nextLng')
  } catch {
    // A blocked Storage getter or read must not prevent the first render.
  }
  return toInterfaceLanguage(storedLanguage ?? window.navigator.language)
}

i18n.use(initReactI18next).init({
  ...createI18nOptions(readStoredInterfaceLanguage()),
  debug: import.meta.env.DEV,
})

// Every switch — header, login, profile, home URL — waits for that pack
// and remembers the official language only.
i18n.on('languageChanged', (language) => {
  try {
    window.localStorage.setItem(
      'i18nextLng',
      normalizeInterfaceLanguage(language)
    )
  } catch {
    // Private mode: the choice simply is not remembered.
  }
})
installInterfaceLanguageNormalization(i18n, loadInterfaceLanguage)
export function whenInterfaceLanguageReady(): Promise<unknown> {
  return i18n.changeLanguage(readStoredInterfaceLanguage())
}

// ADTY task 9.2: <html lang> follows the interface language.
bindDocumentLang(i18n)

export default i18n
