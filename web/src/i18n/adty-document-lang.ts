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
import type { i18n as I18n } from 'i18next'

import { toIntlLocale } from './languages'

/**
 * Keeps `<html lang>` on the interface language (adty task 9.2), so screen
 * readers, hyphenation and the `:lang(ja)` font stack follow a switch. The
 * interface codes zhCN / zhTW become the BCP 47 tags zh-CN / zh-TW.
 */
export function bindDocumentLang(instance: I18n): void {
  const apply = (lng: string | undefined) => {
    document.documentElement.lang = toIntlLocale(lng) ?? 'en'
  }
  apply(instance.resolvedLanguage || instance.language)
  instance.on('languageChanged', apply)
}
