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
 * Black referral banner. Hidden while the program is off or its settings are
 * unknown; without a default rate it promises no number.
 */
export function AffiliateBanner(props: { open: boolean; rate: number | null }) {
  const { t } = useTranslation()
  if (!props.open) return null

  return (
    <section
      id='affiliate'
      aria-labelledby='affiliate-title'
      className='mx-auto max-w-6xl px-4 pb-16 md:px-6 md:pb-20'
    >
      <div className='dark bg-background text-foreground flex flex-wrap items-center justify-between gap-6 rounded-xl border px-6 py-8 md:px-10'>
        <div className='space-y-2'>
          <h2 id='affiliate-title' className='text-xl font-semibold'>
            {t('Invite friends, earn on every top-up')}
          </h2>
          <p className='text-muted-foreground max-w-2xl text-sm'>
            {props.rate !== null
              ? t(
                  'Earn {{rate}}% of every top-up the people you invite make, for as long as they stay. Withdraw to USDT, Alipay or WeChat Pay.',
                  { rate: props.rate }
                )
              : t(
                  'Earn a commission on every top-up the people you invite make, for as long as they stay. Withdraw to USDT, Alipay or WeChat Pay.'
                )}
          </p>
        </div>
        <a
          href='/affiliate'
          className={buttonVariants({ size: 'lg', variant: 'outline' })}
        >
          {t('Affiliate Program')}
        </a>
      </div>
    </section>
  )
}
