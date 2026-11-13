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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { SiteInviteWithdrawPanel } from './site-invite-withdraw-panel'

export type SiteCommission = {
  id: number
  trade_no: string
  status: string
  amount_usd: string
  rate: string
  clawed_usd: string
  converted_usd: string
  withdrawn_usd: string
  frozen_usd: string
  available_at: number
}

type ApiResponse<T> = { success: boolean; data: T }

async function listSiteCommissions(): Promise<SiteCommission[]> {
  const res = await api.get<ApiResponse<{ items?: SiteCommission[] }>>(
    '/api/adty/site-commissions'
  )
  return requireServerSuccess(res.data).data.items ?? []
}

async function convertSiteCommission(id: number, amount: string): Promise<void> {
  const res = await api.post<ApiResponse<{ converted: boolean }>>(
    `/api/adty/site-commissions/${id}/convert`,
    { amount_usd: amount },
    { headers: { 'Idempotency-Key': crypto.randomUUID() } }
  )
  requireServerSuccess(res.data)
}

/** Moves an amount the inviter types from an available site commission into balance. */
export function SiteCommissionPanel(props: { onConverted: () => void }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const rows = useQuery({
    queryKey: ['adty-site-commissions'],
    queryFn: listSiteCommissions,
  })
  if (rows.isSuccess && rows.data.length === 0) return null
  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Available commission')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t('The amount moves from available commission to your balance.')}
      </p>
      {rows.isError && <ErrorState onRetry={() => void rows.refetch()} />}
      {rows.data?.map((row) => (
        <CommissionRow
          key={row.id}
          row={row}
          onConverted={() => {
            props.onConverted()
            void client.invalidateQueries({ queryKey: ['adty-site-commissions'] })
          }}
        />
      ))}
      {rows.isSuccess && rows.data.length > 0 && (
        <SiteInviteWithdrawPanel
          rows={rows.data}
          onChanged={() => {
            props.onConverted()
            void client.invalidateQueries({ queryKey: ['adty-site-commissions'] })
          }}
        />
      )}
    </section>
  )
}

function CommissionRow(props: { row: SiteCommission; onConverted: () => void }) {
  const { t } = useTranslation()
  const [amount, setAmount] = useState('')
  const convert = useMutation({
    mutationFn: () => convertSiteCommission(props.row.id, amount.trim()),
    onSuccess: () => {
      setAmount('')
      props.onConverted()
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <div className='flex flex-col gap-2'>
      <p className='text-sm tabular-nums'>
        {props.row.trade_no} · {props.row.amount_usd} ·{' '}
        {commissionStatusLabel(props.row.status, (key) => t(key))}
      </p>
      {props.row.status === 'available' && (
        <>
          <div className='flex flex-col gap-1'>
            <Label htmlFor={`site-commission-${props.row.id}`}>{t('Amount')}</Label>
            <Input
              id={`site-commission-${props.row.id}`}
              value={amount}
              inputMode='decimal'
              autoComplete='off'
              onChange={(event) => setAmount(event.target.value)}
            />
          </div>
          <Button
            type='button'
            variant='outline'
            disabled={convert.isPending || amount.trim() === ''}
            onClick={() => convert.mutate()}
          >
            {t('Convert commission to balance')}
          </Button>
        </>
      )}
    </div>
  )
}

function commissionStatusLabel(status: string, t: (key: string) => string) {
  if (status === 'pending_coverage') return t('Pending coverage')
  if (status === 'covered') return t('Covered')
  if (status === 'available') return t('Available')
  if (status === 'clawed_back') return t('Clawed back')
  return status
}
