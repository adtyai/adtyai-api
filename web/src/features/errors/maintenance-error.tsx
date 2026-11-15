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
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function MaintenanceError() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <div className='flex min-h-svh w-full items-center justify-center px-6 py-24'>
      <div className='flex w-full max-w-lg flex-col items-center gap-4 text-center'>
        <h1 className='text-faint/40 text-[120px] leading-[0.9] font-extrabold tracking-[-0.05em] tabular-nums select-none'>
          503
        </h1>
        <p className='text-foreground text-lg font-semibold tracking-tight'>
          {t('Website is under maintenance!')}
        </p>
        <p className='text-muted-foreground max-w-sm text-sm leading-relaxed'>
          {t('The site is not available at the moment.')}{' '}
          {t("We'll be back online shortly.")}
        </p>
        <div className='mt-6 flex flex-wrap justify-center gap-3'>
          <Button variant='brand' onClick={() => navigate({ to: '/status' })}>
            {t('Learn more')}
          </Button>
        </div>
      </div>
    </div>
  )
}
