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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { invalidateSidebarBadges } from '@/hooks/use-sidebar-badges'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { siteWalletKindLabel } from '@/features/adty-ledger/site-wallet-labels'
import { SiteWalletLedger } from '@/features/adty-ledger/site-wallet-ledger'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminAnomalyDispositions,
  getAdminFundAnomalies,
  getAdminSiteCommissionMargin,
  getAdminSiteWalletLedger,
  getAdminSiteWallets,
  issueAdminMarginVoucher,
  reviewAdminAnomalyDisposition,
  saveAdminSiteCommissionMargin,
  siteAdminKeys,
} from '../api'
import { SwitchRow } from './setting-fields'

/** Root sees the site anomaly scan and can issue a margin code. Amounts are typed, not suggested. */
export function SiteFundPanel(props: { siteId: number }) {
  const { t } = useTranslation()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const client = useQueryClient()
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [code, setCode] = useState('')
  const [note, setNote] = useState('')
  const wallets = useQuery({
    queryKey: siteAdminKeys.list('wallets', { siteId: props.siteId }),
    queryFn: () => getAdminSiteWallets(props.siteId),
    enabled: isRoot,
  })
  const dispositions = useQuery({
    queryKey: siteAdminKeys.list('dispositions', { siteId: props.siteId }),
    queryFn: () => getAdminAnomalyDispositions(props.siteId),
    enabled: isRoot,
  })
  const review = useMutation({
    mutationFn: (input: { id: number; status: 'reviewing' | 'closed' }) =>
      reviewAdminAnomalyDisposition(input.id, input.status, note.trim()),
    onSuccess: () => {
      setNote('')
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
      void invalidateSidebarBadges(client)
    },
    onError: (error) => handleServerError(error),
  })
  const anomalies = useQuery({
    queryKey: siteAdminKeys.list('fund-anomalies', { siteId: props.siteId }),
    queryFn: () => getAdminFundAnomalies(props.siteId),
    enabled: isRoot,
  })
  const marginAuth = useQuery({
    queryKey: siteAdminKeys.list('commission-margin', { siteId: props.siteId }),
    queryFn: () => getAdminSiteCommissionMargin(props.siteId),
    enabled: isRoot,
  })
  const saveMargin = useMutation({
    mutationFn: (allowed: boolean) =>
      saveAdminSiteCommissionMargin(props.siteId, allowed),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
      void invalidateSidebarBadges(client)
    },
    onError: (error) => handleServerError(error),
  })
  const issue = useMutation({
    mutationFn: () =>
      issueAdminMarginVoucher({
        site_id: props.siteId,
        amount_usd: amount.trim(),
        reason: reason.trim(),
      }),
    onSuccess: (issued) => {
      setCode(issued.code)
      setAmount('')
      setReason('')
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
      void invalidateSidebarBadges(client)
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  // 缓存里可能留着旧形状的响应；列表只认数组，其它一律当空，否则 `.map` 会让整页白屏。
  const walletRows = Array.isArray(wallets.data) ? wallets.data : []
  const anomalyKinds = Array.isArray(anomalies.data) ? anomalies.data : []
  // 异常处置列表同样是「成功但形状不可信」的高危点：崩了站长就看不出哪笔资金
  // 异常还没处置完，所以和 wallets/anomalies 一样只认数组。
  const dispositionRows = Array.isArray(dispositions.data)
    ? dispositions.data
    : []
  return (
    <section className='flex flex-col gap-3'>
      <h3 className='text-sm font-medium'>{t('Wallets')}</h3>
      {wallets.isPending && <LoadingState />}
      {wallets.isError && <ErrorState onRetry={() => void wallets.refetch()} />}
      {wallets.isSuccess && walletRows.length === 0 && (
        <p className='text-sm'>{t('No data')}</p>
      )}
      {wallets.isSuccess && walletRows.length > 0 && (
        <ul className='flex flex-col gap-1 text-sm tabular-nums'>
          {walletRows.map((wallet) => (
            <li key={wallet.id}>
              {siteWalletKindLabel(t, wallet.kind)}: {wallet.balance} USD
              {Number(wallet.frozen_usd) > 0
                ? ` · ${t('Frozen')} ${wallet.frozen_usd}`
                : ''}
              {wallet.kind === 'margin' && wallet.cycle_tier_usd
                ? ` · ${wallet.cycle_used_usd}/${wallet.cycle_tier_usd}`
                : ''}
            </li>
          ))}
        </ul>
      )}
      <SiteWalletLedger
        label={t('Wallet ledger')}
        queryKey={siteAdminKeys.list('wallet-ledger', { siteId: props.siteId })}
        kinds={['operating', 'earnings', 'margin']}
        fetchPage={(kind, page, pageSize) =>
          getAdminSiteWalletLedger(props.siteId, kind, page, pageSize)
        }
        showOperator
      />
      <h3 className='text-sm font-medium'>{t('Commission coverage')}</h3>
      {marginAuth.isPending && <LoadingState />}
      {marginAuth.isError && (
        <ErrorState onRetry={() => void marginAuth.refetch()} />
      )}
      {marginAuth.isSuccess && (
        <SwitchRow
          label={t('Cover commission from margin')}
          hint={t(
            'After earnings, the open margin cycle can cover the rest. No amount is set here.'
          )}
          checked={marginAuth.data?.allowed === true}
          disabled={saveMargin.isPending}
          onCheckedChange={(value) => saveMargin.mutate(value)}
        />
      )}
      <h3 className='text-sm font-medium'>{t('Fund anomalies')}</h3>
      {anomalies.isPending && <LoadingState />}
      {anomalies.isError && <ErrorState onRetry={() => void anomalies.refetch()} />}
      {anomalies.isSuccess && anomalyKinds.length === 0 && (
        <p className='text-sm'>{t('No data')}</p>
      )}
      {anomalies.isSuccess && anomalyKinds.length > 0 && (
        <ul className='flex flex-col gap-1 text-sm'>
          {anomalyKinds.map((kind) => (
            <li key={kind} className='font-mono text-xs'>
              {kind}
            </li>
          ))}
        </ul>
      )}
      {dispositions.isSuccess && dispositionRows.length === 0 && (
        <p className='text-sm'>{t('No data')}</p>
      )}
      {dispositions.isSuccess && dispositionRows.length > 0 && (
        <div className='flex flex-col gap-2'>
          <Label htmlFor='anomaly-note'>{t('Note')}</Label>
          <Input id='anomaly-note' value={note} onChange={(event) => setNote(event.target.value)} />
          {dispositionRows.map((row) => (
            <div key={row.id} className='flex flex-wrap items-center gap-2 text-sm'>
              <span className='font-mono text-xs'>{row.kind}</span>
              <span>{row.status}</span>
              {row.difference ? <span className='tabular-nums'>{row.difference}</span> : null}
              {row.account_id ? <span>{row.account_id}</span> : null}
              {row.order_ref ? <span className='font-mono text-xs'>{row.order_ref}</span> : null}
              {row.evidence ? (
                <span className='text-muted-foreground text-xs break-all'>{row.evidence}</span>
              ) : null}
              {row.status !== 'closed' && (
                <>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={review.isPending}
                    onClick={() => review.mutate({ id: row.id, status: 'reviewing' })}
                  >
                    {t('Mark reviewing')}
                  </Button>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={review.isPending || note.trim() === ''}
                    onClick={() => review.mutate({ id: row.id, status: 'closed' })}
                  >
                    {t('Close')}
                  </Button>
                </>
              )}
            </div>
          ))}
        </div>
      )}
      <h3 className='text-sm font-medium'>{t('Issue margin code')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t('Only the site owner can redeem a margin code.')}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='margin-code-amount'>{t('Amount')}</Label>
          <Input id='margin-code-amount' value={amount} inputMode='decimal' onChange={(event) => setAmount(event.target.value)} />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='margin-code-reason'>{t('Reason')}</Label>
          <Input id='margin-code-reason' value={reason} onChange={(event) => setReason(event.target.value)} />
        </div>
      </div>
      <Button
        type='button'
        variant='outline'
        disabled={issue.isPending || amount.trim() === '' || reason.trim() === ''}
        onClick={() => issue.mutate()}
      >
        {t('Issue margin code')}
      </Button>
      {code && <p className='text-sm break-all'>{code}</p>}
    </section>
  )
}
