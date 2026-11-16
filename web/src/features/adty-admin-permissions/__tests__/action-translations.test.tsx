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
import { act, cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it } from 'vitest'

import { PermissionMatrix } from '@/components/permission-matrix'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

const packs = { en, zh }
const expected = {
  en: [
    'Create users',
    'Export all logs',
    'View reseller settings',
    'Create user accounts. Administrator appointments remain Root-only.',
    "Export every user's usage logs.",
    'View reseller configuration and site invite codes without changing them.',
  ],
  zh: [
    '创建用户',
    '导出全部日志',
    '查看分站设置',
    '创建用户账号。仅Root可任命管理员。',
    '导出所有用户的使用日志。',
    '查看分站配置和站点邀请码，不进行更改。',
  ],
}

afterEach(cleanup)

it.each(Object.keys(packs) as Array<keyof typeof packs>)(
  'switches independent action labels and descriptions to %s',
  async (language) => {
    const i18n = createInstance()
    await i18n.use(initReactI18next).init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      resources: { en, [language]: packs[language] },
      interpolation: { escapeValue: false },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <PermissionMatrix
          resources={[
            {
              resource: 'adty_user',
              label_key: 'Users',
              actions: [
                {
                  action: 'create',
                  label_key: 'Create users',
                  description_key:
                    'Create user accounts. Administrator appointments remain Root-only.',
                },
              ],
            },
            {
              resource: 'adty_log',
              label_key: 'Logs',
              actions: [
                {
                  action: 'export',
                  label_key: 'Export all logs',
                  description_key: "Export every user's usage logs.",
                },
              ],
            },
            {
              resource: 'adty_site',
              label_key: 'Sites',
              actions: [
                {
                  action: 'settings_read',
                  label_key: 'View reseller settings',
                  description_key:
                    'View reseller configuration and site invite codes without changing them.',
                },
              ],
            },
          ]}
          value={{}}
          onChange={() => undefined}
        />
      </I18nextProvider>
    )
    expect(
      screen.getByRole('button', { name: 'Create users' })
    ).toBeInTheDocument()
    await act(async () => {
      await i18n.changeLanguage(language)
    })
    for (let index = 0; index < 3; index++) {
      expect(
        screen.getByRole('button', { name: expected[language][index] })
      ).toHaveAccessibleDescription(expected[language][index + 3])
    }
  }
)
