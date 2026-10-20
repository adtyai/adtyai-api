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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

/**
 * 控制台顶栏「余额 + 充值」。
 * 余额读现有 /api/user/self（与钱包页同源），无数据时回退到 auth store；
 * 充值跳现有钱包页 /wallet。未登录或钱包模块被隐藏时不渲染。
 * 小屏只显示钱包图标按钮，隐藏余额数字。
 */
export function HeaderBalance() {
  const { t } = useTranslation()
  const storeUser = useAuthStore((s) => s.auth.user)
  const isWalletVisible = useIsSidebarModuleVisible('/wallet')
  const enabled = Boolean(storeUser) && isWalletVisible

  const { data } = useQuery({
    queryKey: ['header-balance', 'self'],
    queryFn: async () => {
      const res = await getSelf()
      return res?.success
        ? (res.data as { quota?: number; bonus_quota?: number })
        : null
    },
    enabled,
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  })

  if (!enabled) return null

  const paid = data?.quota ?? storeUser?.quota ?? 0
  const bonus = data?.bonus_quota ?? storeUser?.bonus_quota ?? 0
  const balance = formatQuota(paid + bonus)

  return (
    <div
      className='flex items-center gap-1.5'
      data-testid='header-balance'
      aria-label={t('Account balance')}
    >
      <span className='text-muted-foreground hidden text-sm whitespace-nowrap sm:inline'>
        {t('Balance')}{' '}
        <span className='text-foreground font-semibold tabular-nums'>
          {balance}
        </span>
      </span>
      <Button
        size='sm'
        render={<Link to='/wallet' />}
        className='h-8 rounded-full px-2.5 sm:px-3'
        aria-label={t('Recharge')}
      >
        <Wallet className='size-4' />
        <span className='hidden sm:inline'>{t('Recharge')}</span>
      </Button>
    </div>
  )
}
