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
import { Link } from '@tanstack/react-router'
import { Activity, BarChart3, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <div className='grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]'>
        <Skeleton className='h-28 rounded-xl' />
        <Skeleton className='h-28 rounded-xl' />
        <Skeleton className='h-28 rounded-xl' />
      </div>
    )
  }

  const sideStats = [
    {
      label: t('Total Usage'),
      value: formatQuota(props.user?.used_quota ?? 0),
      description: t('Total consumed quota'),
      icon: BarChart3,
    },
    {
      label: t('API Requests'),
      value: (props.user?.request_count ?? 0).toLocaleString(),
      description: t('Total requests made'),
      icon: Activity,
    },
  ]

  return (
    <div className='grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)]'>
      <div className='from-primary to-violet-600 text-primary-foreground flex flex-col justify-between gap-3 rounded-xl bg-gradient-to-br p-4 shadow-sm sm:p-5'>
        <div className='flex items-center justify-between gap-2'>
          <span className='text-[13px] text-white/80'>
            {t('Available Balance')}
          </span>
          <span className='grid size-8 place-items-center rounded-lg bg-white/15'>
            <WalletCards className='size-4' aria-hidden='true' />
          </span>
        </div>
        <div className='text-[28px] leading-none font-semibold tracking-tight'>
          {formatQuota(props.user?.quota ?? 0)}
        </div>
        <div className='flex flex-wrap items-center justify-between gap-2 text-xs text-white/80'>
          <span>{t('Remaining quota')}</span>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            className='h-7 px-2 text-xs text-white hover:bg-white/15 hover:text-white'
            render={<Link to='/wallet/ledger' />}
          >
            {t('View billing & ledger')} →
          </Button>
        </div>
      </div>

      {sideStats.map((item) => (
        <div
          key={item.label}
          className='bg-card flex flex-col justify-between gap-2 rounded-xl border border-border/80 p-4 shadow-[0_1px_2px_rgba(16,24,40,.04)] sm:p-5'
        >
          <div className='flex items-center justify-between gap-2'>
            <span className='text-muted-foreground text-[13px]'>
              {item.label}
            </span>
            <span className='bg-muted text-muted-foreground grid size-8 place-items-center rounded-lg border border-border/60'>
              <item.icon className='size-4' aria-hidden='true' />
            </span>
          </div>
          <div className='text-foreground text-2xl font-semibold tracking-tight tabular-nums'>
            {item.value}
          </div>
          <div className='text-muted-foreground text-xs'>{item.description}</div>
        </div>
      ))}
    </div>
  )
}
