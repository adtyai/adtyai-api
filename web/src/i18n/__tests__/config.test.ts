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
import { afterEach, describe, expect, test } from 'vitest'

import i18n, {
  loadInterfaceLanguage,
  whenInterfaceLanguageReady,
} from '@/i18n/config'

describe('interface language packs', () => {
  // 正式界面语言只有 en 和 zhCN（conventions.md 约定 E + config.ts 的
  // supportedLngs）。这三个用例原来的样本语言是 fr / ja / ru——那六种是
  // 「留在仓库但不再承诺」的历史包，选它们会被归一化回 en，于是断言
  // 「切到 fr」必然失败。测验的对象因此换成 zhCN：它同样是「非默认的正式
  // 语言」，能真正验证「首屏等包加载」「切换后仍被记住」「只加载一个包」。
  afterEach(async () => {
    window.localStorage.removeItem('i18nextLng')
    await i18n.changeLanguage('en')
  })

  test('the first paint waits until the selected language pack is loaded', async () => {
    window.localStorage.setItem('i18nextLng', 'zhCN')
    await whenInterfaceLanguageReady()
    expect(i18n.resolvedLanguage || i18n.language).toBe('zhCN')
    expect(i18n.t('Sign in')).not.toBe('Sign in')
    expect(i18n.t('Sign in')).toBe('登录')
  })

  test('a language switch waits for that pack and is still selected after reload', async () => {
    window.localStorage.removeItem('i18nextLng')
    await i18n.changeLanguage('zhCN')
    expect(i18n.hasResourceBundle('zhCN', 'translation')).toBe(true)
    expect(i18n.getResource('zhCN', 'translation', 'Sign in')).toBeTypeOf(
      'string'
    )
    expect(window.localStorage.getItem('i18nextLng')).toBe('zhCN')
  })

  test('loads one language without pulling the others into the bundle', async () => {
    await loadInterfaceLanguage('zhCN')
    expect(i18n.hasResourceBundle('zhCN', 'translation')).toBe(true)
    // en 是 fallback，init 时就带着；这里要证明的是没有把别的包一起拉进来。
    expect(i18n.hasResourceBundle('ja', 'translation')).toBe(false)
    expect(i18n.hasResourceBundle('ru', 'translation')).toBe(false)
    expect(i18n.getResource('zhCN', 'translation', 'Sign in')).toBeTypeOf(
      'string'
    )
  })

  // 历史语言的包还在磁盘上（左侧 loaders），但**永远不能被选成运行语言**。
  // 这条是上面三个用例原本想守、但用错了样本语言的那个不变量。
  test.each(['fr', 'ja', 'ru', 'ko', 'vi', 'zhTW'])(
    'normalizes the leftover %s pack back to an official language',
    async (legacy) => {
      await loadInterfaceLanguage(legacy)
      expect(i18n.resolvedLanguage || i18n.language).toBe('en')
      expect(i18n.hasResourceBundle(legacy, 'translation')).toBe(false)
    }
  )
})
