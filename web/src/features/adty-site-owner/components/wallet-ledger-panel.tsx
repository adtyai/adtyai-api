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
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { siteWalletKindLabel } from '@/features/adty-ledger/site-wallet-labels'
import { SiteWalletLedger } from '@/features/adty-ledger/site-wallet-ledger'

import { getOwnerWalletLedger, getOwnerWallets, siteOwnerKeys } from '../api'
import { OwnerStatGrid } from './stat-grid'

/**
 * Earnings and margin wallet balances with every movement behind them. The
 * operating wallet is platform bookkeeping and is never shown to the owner.
 */
export function WalletLedgerPanel() {
  const { t } = useTranslation()
  const wallets = useQuery({
    queryKey: siteOwnerKeys.list('wallets', {}),
    queryFn: getOwnerWallets,
  })
  const rows = (Array.isArray(wallets.data) ? wallets.data : []).filter(
    (wallet) => wallet.kind !== 'operating'
  )
  return (
    <section className='flex flex-col gap-3'>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Wallets')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Frozen amounts are held for recharge codes, commission or withdrawals and cannot be used until released.'
          )}
        </p>
      </div>
      {wallets.isPending && <LoadingState />}
      {wallets.isError && <ErrorState onRetry={() => void wallets.refetch()} />}
      {rows.length > 0 && (
        <OwnerStatGrid
          label={t('Wallets')}
          items={rows.flatMap((wallet) => [
            {
              label: siteWalletKindLabel(t, wallet.kind),
              value: `${wallet.balance} USD`,
              negative: Number(wallet.balance) < 0,
            },
            {
              label: `${siteWalletKindLabel(t, wallet.kind)} · ${t('Frozen')}`,
              value: `${wallet.frozen_usd || '0'} USD`,
            },
          ])}
        />
      )}
      <SiteWalletLedger
        label={t('Wallet ledger')}
        queryKey={siteOwnerKeys.list('wallet-ledger', {})}
        kinds={['earnings', 'margin']}
        fetchPage={getOwnerWalletLedger}
      />
    </section>
  )
}
