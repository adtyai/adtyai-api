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
import type { i18n as I18n, InitOptions, Resource } from 'i18next'

import {
  convertDetectedLanguage,
  normalizeInterfaceLanguage,
  type InterfaceLanguageCode,
} from './languages'

/**
 * The only interface languages the product offers (convention E). Anything
 * else on disk is a leftover pack that must never become the running language.
 */
export const SUPPORTED_INTERFACE_LANGUAGES = ['en', 'zhCN'] as const

/** Coerce any requested code onto an official interface language. */
export function toInterfaceLanguage(value?: string): InterfaceLanguageCode {
  return normalizeInterfaceLanguage(convertDetectedLanguage(value ?? ''))
}

/**
 * The one i18next init shape the product runs on. `main.tsx` and the test
 * runtime must both come through here: a bare init without `supportedLngs`
 * accepts unofficial codes and makes tests assert behaviour the product
 * never has.
 */
export function createI18nOptions(
  lng: string,
  resources: Resource = {}
): InitOptions {
  return {
    lng,
    fallbackLng: 'en',
    supportedLngs: [...SUPPORTED_INTERFACE_LANGUAGES],
    load: 'currentOnly',
    partialBundledLanguages: true,
    resources,
    nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
  }
}

/**
 * Wrap `changeLanguage` with the same normalization the product uses, so a
 * request for an unofficial code resolves to an official language instead of
 * being accepted verbatim. `beforeSwitch` is where production loads the pack.
 */
export function installInterfaceLanguageNormalization(
  instance: I18n,
  beforeSwitch: (code: string) => Promise<unknown> = async () => undefined
): void {
  const originalChangeLanguage = instance.changeLanguage.bind(instance)
  instance.changeLanguage = async (language, callback) => {
    const code = toInterfaceLanguage(language as string)
    await beforeSwitch(code)
    return originalChangeLanguage(code, callback)
  }
}
