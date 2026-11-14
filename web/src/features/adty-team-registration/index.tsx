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

import { useTranslation } from 'react-i18next'

import { AuthLayout } from '@/features/auth/auth-layout'

import { TeamRegistrationCompleteForm } from './components/team-registration-complete-form'
import { TeamRegistrationLinksPanel } from './components/team-registration-links-panel'

export { TeamRegistrationLinksPanel }

export function TeamRegistrationCompletePage(props: { code: string }) {
  const { t } = useTranslation()
  return (
    <AuthLayout>
      <div className='w-full space-y-8'>
        <div className='space-y-2'>
          <h2 className='text-center text-2xl font-semibold tracking-tight sm:text-left'>
            {t('Complete team registration')}
          </h2>
          <p className='text-muted-foreground text-left text-sm sm:text-base'>
            {t('Use this link to set a username, password, and team name.')}
          </p>
        </div>
        <TeamRegistrationCompleteForm code={props.code} />
      </div>
    </AuthLayout>
  )
}
