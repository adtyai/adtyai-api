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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getSelf } from '@/lib/api'

import { adtyTopupQueryKeys, getAdtyTopupInfo } from './adty/api'
import { AdtyPaymentDialogs } from './adty/components/adty-payment-dialogs'
import { AdtyPendingBanner } from './adty/components/adty-pending-banner'
import { AdtyRechargeCard } from './adty/components/adty-recharge-card'
import { AdtyTopupHistory } from './adty/components/adty-topup-history'
import type { ActivePayment } from './adty/lib/active-payment'
import type { AdtyMethod, AdtyOrder } from './adty/types'
import { WalletStatsCard } from './components/wallet-stats-card'
import type { UserWalletData } from './types'

interface WalletProps {
  /** Scrolls to the top-up history (kept for the Waffo return URL). */
  initialShowHistory?: boolean
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [activePayment, setActivePayment] = useState<ActivePayment | null>(null)

  const infoQuery = useQuery({
    queryKey: adtyTopupQueryKeys.info,
    queryFn: getAdtyTopupInfo,
  })

  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } finally {
      setUserLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (!props.initialShowHistory) return
    document.querySelector('#wallet-history')?.scrollIntoView()
  }, [props.initialShowHistory])

  const handlePay = (amount: number, method: AdtyMethod, promoCode: string) => {
    setActivePayment({
      kind: method.kind,
      method: method.id,
      amount,
      promoCode,
    })
  }

  const handleResume = (order: AdtyOrder) => {
    const kind = order.payment_provider === 'bepusdt' ? 'usdt' : 'epay'
    setActivePayment({
      kind,
      method: order.payment_method,
      amount: order.amount,
      tradeNo: order.trade_no,
    })
  }

  const handleSettled = async () => {
    await Promise.all([
      fetchUser(),
      queryClient.invalidateQueries({ queryKey: ['adty-topup'] }),
    ])
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Wallet')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <WalletStatsCard user={user} loading={userLoading} />
            <AdtyPendingBanner
              order={infoQuery.data?.pending}
              onResume={handleResume}
            />
            <div id='wallet-add-funds' className='scroll-mt-4'>
              <AdtyRechargeCard
                info={infoQuery.data}
                loading={infoQuery.isLoading}
                paying={activePayment !== null}
                onPay={handlePay}
              />
            </div>
            <AdtyTopupHistory />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <AdtyPaymentDialogs
        payment={activePayment}
        onClose={() => setActivePayment(null)}
        onSettled={handleSettled}
      />
    </>
  )
}
