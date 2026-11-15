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

import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'
import { WalletLotProjectionLines } from './wallet-lot-projection'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const hasUser = props.user != null

  if (props.loading) {
    return (
      <div className='bg-card border-border overflow-hidden rounded-[14px] border'>
        <div className='bg-border grid grid-cols-3 gap-px'>
          {['balance', 'usage', 'requests'].map((key) => (
            <div key={key} className='bg-card min-w-0 px-4 py-4'>
              <Skeleton className='h-3 w-20' />
              <Skeleton className='mt-2 h-7 w-full' />
              <Skeleton className='mt-2 hidden h-3 w-24 md:block' />
            </div>
          ))}
        </div>
      </div>
    )
  }

  // Design system §5: a missing value reads `—`, never a fabricated 0.
  const stats: { label: string; value: string; description: string }[] = [
    {
      label: t('Current Balance'),
      value: hasUser ? formatQuota(props.user?.quota ?? 0) : '—',
      description: t('Remaining quota'),
    },
    {
      label: t('Total Usage'),
      value: hasUser ? formatQuota(props.user?.used_quota ?? 0) : '—',
      description: t('Total consumed quota'),
    },
    {
      label: t('API Requests'),
      value:
        hasUser && props.user?.request_count != null
          ? props.user.request_count.toLocaleString()
          : '—',
      description: t('Total requests made'),
    },
  ]

  return (
    <div className='bg-card border-border overflow-hidden rounded-[14px] border'>
      <div className='bg-border grid grid-cols-3 gap-px'>
        {stats.map((item) => (
          <div key={item.label} className='bg-card min-w-0 px-4 py-4'>
            <div className='text-faint truncate text-[12px] leading-none font-medium'>
              {item.label}
            </div>

            <div
              className={
                item.value === '—'
                  ? 'text-faint mt-2.5 font-mono text-[26px] leading-none font-bold tracking-[-0.02em] tabular-nums'
                  : 'text-foreground mt-2.5 truncate font-mono text-[26px] leading-none font-bold tracking-[-0.02em] tabular-nums'
              }
            >
              {item.value}
            </div>
            <div className='text-dim mt-1.5 hidden text-[11.5px] leading-snug md:block'>
              {item.description}
            </div>
            {item.label === t('Current Balance') && (
              <WalletLotProjectionLines
                projection={props.user?.wallet_lot_projection}
              />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
