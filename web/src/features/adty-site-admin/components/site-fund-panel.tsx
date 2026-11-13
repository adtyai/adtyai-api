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

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminAnomalyDispositions,
  getAdminFundAnomalies,
  getAdminSiteCommissionMargin,
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
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  return (
    <section className='flex flex-col gap-3'>
      <h3 className='text-sm font-medium'>{t('Wallets')}</h3>
      {wallets.isPending && <LoadingState />}
      {wallets.isError && <ErrorState onRetry={() => void wallets.refetch()} />}
      {wallets.isSuccess && (
        <ul className='flex flex-col gap-1 text-sm tabular-nums'>
          {wallets.data.map((wallet) => (
            <li key={wallet.id}>
              {wallet.kind} {wallet.balance}
              {wallet.kind === 'margin' && wallet.cycle_tier_usd
                ? ` · ${wallet.cycle_used_usd}/${wallet.cycle_tier_usd}`
                : ''}
            </li>
          ))}
        </ul>
      )}
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
          checked={marginAuth.data.allowed}
          disabled={saveMargin.isPending}
          onCheckedChange={(value) => saveMargin.mutate(value)}
        />
      )}
      <h3 className='text-sm font-medium'>{t('Fund anomalies')}</h3>
      {anomalies.isPending && <LoadingState />}
      {anomalies.isError && <ErrorState onRetry={() => void anomalies.refetch()} />}
      {anomalies.isSuccess && anomalies.data.length === 0 && (
        <p className='text-sm'>{t('No data')}</p>
      )}
      {anomalies.isSuccess && anomalies.data.length > 0 && (
        <ul className='flex flex-col gap-1 text-sm'>
          {anomalies.data.map((kind) => (
            <li key={kind} className='font-mono text-xs'>
              {kind}
            </li>
          ))}
        </ul>
      )}
      {dispositions.isSuccess && dispositions.data.length > 0 && (
        <div className='flex flex-col gap-2'>
          <Label htmlFor='anomaly-note'>{t('Note')}</Label>
          <Input id='anomaly-note' value={note} onChange={(event) => setNote(event.target.value)} />
          {dispositions.data.map((row) => (
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
