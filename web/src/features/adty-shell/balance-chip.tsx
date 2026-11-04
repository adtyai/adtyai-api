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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { getSelf } from '@/lib/api'
import { getCurrencyDisplay } from '@/lib/currency'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

// Under the 'adty-topup' prefix on purpose: the wallet invalidates that prefix
// once a payment settles, so the header balance refreshes with it.
const adtyBalanceQueryKey = ['adty-topup', 'balance'] as const

/**
 * Header balance and the "Add funds" shortcut to the wallet (task 8.2).
 * Shows the session's quota until /api/user/self answers, and keeps it if
 * that request fails.
 */
export function BalanceChip() {
  const { t, i18n } = useTranslation()
  const sessionQuota = useAuthStore((state) => state.auth.user?.quota)
  const balanceQuery = useQuery({
    queryKey: adtyBalanceQueryKey,
    queryFn: async () => {
      const response = requireServerSuccess(await getSelf())
      return Number(response.data?.quota ?? 0)
    },
    // Payment settlement already invalidates this key. Do not refetch on
    // every tab focus: that redraws the header while the admin is working.
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    meta: { errorToast: false },
  })

  const quota = balanceQuery.data ?? sessionQuota
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // The site is priced in USD (decision 6); always two decimals, like the wallet.
  const balance = formatUsd(
    (quota ?? 0) / getCurrencyDisplay().config.quotaPerUnit,
    locale
  )

  return (
    <div className='flex shrink-0 items-center gap-1.5'>
      <span className='text-muted-foreground hidden h-8 items-center gap-1.5 rounded-lg border px-2.5 text-xs whitespace-nowrap sm:inline-flex'>
        {t('Balance')}
        <span className='text-foreground font-semibold tabular-nums'>
          {balance}
        </span>
      </span>
      <Button size='sm' render={<Link to='/wallet' />}>
        {t('Add funds')}
      </Button>
    </div>
  )
}
