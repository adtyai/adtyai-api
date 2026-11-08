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
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ko from '@/i18n/locales/ko.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

const packs = { en, fr, ja, ko, ru, vi, 'zh-TW': zhTW, zh }
const expected = {
  en: [
    'Create users',
    'Export all logs',
    'View reseller settings',
    'Create user accounts. Administrator appointments remain Root-only.',
    "Export every user's usage logs.",
    'View reseller configuration and site invite codes without changing them.',
  ],
  fr: [
    'Créer des utilisateurs',
    'Exporter tous les journaux',
    'Consulter les paramètres revendeur',
    'Créer des comptes utilisateur. La nomination des administrateurs reste réservée à Root.',
    'Exporter les journaux d’utilisation de tous les utilisateurs.',
    'Consulter la configuration revendeur et les codes d’invitation des sites sans les modifier.',
  ],
  ja: [
    'ユーザーを作成',
    'すべてのログをエクスポート',
    'リセラー設定を表示',
    'ユーザーアカウントを作成します。管理者の任命は引き続きRootのみが行えます。',
    'すべてのユーザーの使用ログをエクスポートします。',
    'リセラー設定とサイト招待コードを変更せずに表示します。',
  ],
  ko: [
    '사용자 생성',
    '모든 로그 내보내기',
    '리셀러 설정 보기',
    '사용자 계정을 생성합니다. 관리자 임명은 계속 Root만 할 수 있습니다.',
    '모든 사용자의 사용 로그를 내보냅니다.',
    '리셀러 설정과 사이트 초대 코드를 변경하지 않고 봅니다.',
  ],
  ru: [
    'Создание пользователей',
    'Экспорт всех журналов',
    'Просмотр настроек реселлеров',
    'Создание учётных записей пользователей. Назначать администраторов по-прежнему может только Root.',
    'Экспорт журналов использования всех пользователей.',
    'Просмотр настроек реселлеров и кодов приглашения на сайты без их изменения.',
  ],
  vi: [
    'Tạo người dùng',
    'Xuất tất cả nhật ký',
    'Xem cài đặt đại lý',
    'Tạo tài khoản người dùng. Chỉ Root được bổ nhiệm quản trị viên.',
    'Xuất nhật ký sử dụng của mọi người dùng.',
    'Xem cấu hình đại lý và mã mời của trang web mà không thay đổi chúng.',
  ],
  'zh-TW': [
    '建立使用者',
    '匯出全部日誌',
    '檢視分站設定',
    '建立使用者帳號。僅Root可任命管理員。',
    '匯出所有使用者的使用日誌。',
    '檢視分站設定和站點邀請碼，不進行變更。',
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
