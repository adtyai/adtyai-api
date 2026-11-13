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
  convertDetectedLanguage,
  normalizeInterfaceLanguage,
} from './languages'

const localeLoaders = {
  en: () => import('./locales/en.json'),
  zhCN: () => import('./locales/zh.json'),
  fr: () => import('./locales/fr.json'),
  ru: () => import('./locales/ru.json'),
  ja: () => import('./locales/ja.json'),
  ko: () => import('./locales/ko.json'),
  vi: () => import('./locales/vi.json'),
  zhTW: () => import('./locales/zh-TW.json'),
} as const

type InterfaceLanguage = keyof typeof localeLoaders

function interfaceLanguage(value: string | undefined): InterfaceLanguage {
  // Official product languages only. Leftover loaders stay for on-disk packs
  // but are never selected as the running interface language.
  return normalizeInterfaceLanguage(convertDetectedLanguage(value ?? ''))
}

/** Load one language pack. The other languages stay out of the first page load. */
export async function loadInterfaceLanguage(
  value: string | undefined
): Promise<void> {
  const code = interfaceLanguage(value)
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
  return interfaceLanguage(storedLanguage ?? window.navigator.language)
}

i18n.use(initReactI18next).init({
  lng: readStoredInterfaceLanguage(),
  fallbackLng: 'en',
  supportedLngs: ['en', 'zhCN'],
  load: 'currentOnly',
  partialBundledLanguages: true,
  resources: {},
  nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
  debug: import.meta.env.DEV,
  interpolation: {
    escapeValue: false, // not needed for react as it escapes by default
  },
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
const originalChangeLanguage = i18n.changeLanguage.bind(i18n)
i18n.changeLanguage = async (language, callback) => {
  const code = interfaceLanguage(language)
  await loadInterfaceLanguage(code)
  return originalChangeLanguage(code, callback)
}
export function whenInterfaceLanguageReady(): Promise<unknown> {
  return i18n.changeLanguage(readStoredInterfaceLanguage())
}

// ADTY task 9.2: <html lang> follows the interface language.
bindDocumentLang(i18n)

export default i18n
