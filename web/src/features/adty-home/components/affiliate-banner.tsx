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

import { buttonVariants } from '@/components/ui/button'

/**
 * Referral strip between the endpoints and the closing card. Hidden while the
 * program is off or its settings are unknown; without a default rate it
 * promises no number.
 */
export function AffiliateBanner(props: { open: boolean; rate: number | null }) {
  const { t } = useTranslation()
  if (!props.open) return null

  return (
    <section
      id='affiliate'
      aria-labelledby='affiliate-title'
      className='px-6 pb-24 md:px-10 md:pb-28'
    >
      <div className='border-border mx-auto flex max-w-[1180px] flex-wrap items-center justify-between gap-6 border-t pt-10'>
        <div className='space-y-2'>
          <h2 id='affiliate-title' className='text-[15px] font-semibold'>
            {t('Invite friends, earn on every top-up')}
          </h2>
          <p className='text-muted-foreground max-w-2xl text-sm leading-[1.7]'>
            {props.rate !== null
              ? t(
                  'Earn {{rate}}% of every top-up the people you invite make, for as long as they stay.',
                  { rate: props.rate }
                )
              : t(
                  'Earn a commission on every top-up the people you invite make, for as long as they stay.'
                )}
          </p>
        </div>
        <a
          href='/affiliate'
          className={buttonVariants({ variant: 'outline', size: 'lg' })}
        >
          {t('Affiliate Program')}
        </a>
      </div>
    </section>
  )
}
