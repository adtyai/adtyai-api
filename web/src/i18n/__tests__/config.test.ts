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

import i18n, {
  loadInterfaceLanguage,
  whenInterfaceLanguageReady,
} from '@/i18n/config'

describe('interface language packs', () => {
  test('the first paint waits until the selected language pack is loaded', async () => {
    window.localStorage.setItem('i18nextLng', 'fr')
    await whenInterfaceLanguageReady()
    expect(i18n.resolvedLanguage || i18n.language).toBe('fr')
    expect(i18n.t('Sign in')).not.toBe('Sign in')
  })

  test('a language switch waits for that pack and is still selected after reload', async () => {
    window.localStorage.removeItem('i18nextLng')
    await i18n.changeLanguage('fr')
    expect(i18n.hasResourceBundle('fr', 'translation')).toBe(true)
    expect(i18n.getResource('fr', 'translation', 'Sign in')).toBeTypeOf(
      'string'
    )
    expect(window.localStorage.getItem('i18nextLng')).toBe('fr')
  })

  test('loads one language without pulling the other six into the bundle', async () => {
    await loadInterfaceLanguage('ja')
    expect(i18n.hasResourceBundle('ja', 'translation')).toBe(true)
    expect(i18n.hasResourceBundle('ru', 'translation')).toBe(false)
    expect(
      i18n.getResource('ja', 'translation', 'Sign in')
    ).toBeTypeOf('string')
  })
})
