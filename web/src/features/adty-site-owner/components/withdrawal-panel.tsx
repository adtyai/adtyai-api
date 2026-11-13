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
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { handleServerError } from '@/lib/handle-server-error'

import {
  cancelSiteWithdrawal,
  createSiteWithdrawal,
  getSiteWithdrawals,
  getSiteWithdrawRules,
  siteOwnerKeys,
} from '../api'

const NETWORKS = ['TRC20', 'BEP20', 'POLYGON', 'XLAYER'] as const

/** Owner request against the site earnings wallet. Amounts come from Root rules, not from this page. */
export function WithdrawalPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const rules = useQuery({
    queryKey: siteOwnerKeys.list('withdraw-rules', {}),
    queryFn: getSiteWithdrawRules,
  })
  const list = useQuery({
    queryKey: siteOwnerKeys.list('withdrawals', {}),
    queryFn: getSiteWithdrawals,
  })
  const [amount, setAmount] = useState('')
  const [method, setMethod] = useState<'alipay' | 'usdt'>('alipay')
  const [network, setNetwork] = useState<(typeof NETWORKS)[number]>('TRC20')
  const [accountNo, setAccountNo] = useState('')
  const [accountName, setAccountName] = useState('')
  const [confirmed, setConfirmed] = useState(false)

  const submit = useMutation({
    mutationFn: () => {
      const rate =
        method === 'usdt'
          ? rules.data?.usdt_usd_rate
          : rules.data?.alipay_cny_rate
      if (!rate) throw new Error(t('Withdrawal rules are not set'))
      return createSiteWithdrawal({
        amount_usd: amount,
        method,
        network: method === 'usdt' ? network : '',
        account_no: accountNo,
        account_name: accountName,
        expected_rate: rate,
        idempotencyKey: crypto.randomUUID(),
      })
    },
    onSuccess: () => {
      setAmount('')
      setAccountNo('')
      setAccountName('')
      setConfirmed(false)
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const cancel = useMutation({
    mutationFn: cancelSiteWithdrawal,
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all }),
    onError: (err) => handleServerError(err),
  })

  const configured = rules.data?.configured === true
  const rate =
    method === 'usdt' ? rules.data?.usdt_usd_rate : rules.data?.alipay_cny_rate

  return (
    <section className='flex flex-col gap-4' aria-label={t('Earnings wallet withdrawal')}>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Earnings wallet withdrawal')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'This request freezes the site earnings wallet. Referral commission withdrawals stay on the Referral page.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Customer invite commission withdrawals are reviewed on the Customers tab.'
          )}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t('Invite commission holds are not available for this withdrawal.')}
        </p>
      </div>
      {rules.isError && (
        <p className='text-destructive text-sm'>{t('Could not load withdrawal rules')}</p>
      )}
      {rules.data && !configured && (
        <p className='text-muted-foreground text-sm'>
          {t('Root has not set the withdrawal minimum, fee, or exchange rate.')}
        </p>
      )}
      {configured && (
        <form
          className='flex flex-col gap-4'
          onSubmit={(event) => {
            event.preventDefault()
            if (!confirmed) return
            submit.mutate()
          }}
        >
          <p className='text-muted-foreground text-sm tabular-nums'>
            {t('Minimum {{min}} USD. Fee {{fee}} USD. Rate {{rate}}.', {
              min: rules.data?.min_usd,
              fee: rules.data?.fee_usd,
              rate,
            })}
          </p>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='flex flex-col gap-2'>
              <Label htmlFor='withdraw-amount'>{t('Amount USD')}</Label>
              <Input
                id='withdraw-amount'
                inputMode='decimal'
                value={amount}
                onChange={(event) => setAmount(event.target.value)}
                required
              />
            </div>
            <div className='flex flex-col gap-2'>
              <Label htmlFor='withdraw-method'>{t('Method')}</Label>
              <NativeSelect
                id='withdraw-method'
                value={method}
                onChange={(event) =>
                  setMethod(event.target.value === 'usdt' ? 'usdt' : 'alipay')
                }
              >
                <NativeSelectOption value='alipay'>{t('Alipay')}</NativeSelectOption>
                <NativeSelectOption value='usdt'>USDT</NativeSelectOption>
              </NativeSelect>
            </div>
            {method === 'usdt' && (
              <div className='flex flex-col gap-2'>
                <Label htmlFor='withdraw-network'>{t('Network')}</Label>
                <NativeSelect
                  id='withdraw-network'
                  value={network}
                  onChange={(event) =>
                    setNetwork(event.target.value as (typeof NETWORKS)[number])
                  }
                >
                  {NETWORKS.map((item) => (
                    <NativeSelectOption key={item} value={item}>
                      {item === 'XLAYER' ? 'X Layer' : item}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
            )}
            <div className='flex flex-col gap-2'>
              <Label htmlFor='withdraw-account'>{t('Account')}</Label>
              <Input
                id='withdraw-account'
                value={accountNo}
                onChange={(event) => setAccountNo(event.target.value)}
                required
              />
            </div>
            <div className='flex flex-col gap-2'>
              <Label htmlFor='withdraw-name'>{t('Payee name')}</Label>
              <Input
                id='withdraw-name'
                value={accountName}
                onChange={(event) => setAccountName(event.target.value)}
                required={method === 'alipay'}
              />
            </div>
          </div>
          <label className='flex items-start gap-2 text-sm'>
            <input
              type='checkbox'
              className='mt-1'
              checked={confirmed}
              onChange={(event) => setConfirmed(event.target.checked)}
            />
            <span>{t('I confirm the payee details before this request is frozen.')}</span>
          </label>
          <div>
            <Button type='submit' disabled={!confirmed || submit.isPending}>
              {t('Submit withdrawal')}
            </Button>
          </div>
        </form>
      )}
      <ul className='flex flex-col gap-2'>
        {(list.data ?? []).map((row) => (
          <li
            key={row.id}
            className='flex flex-wrap items-center justify-between gap-2 text-sm tabular-nums'
          >
            <span>
              {row.amount_usd} USD · {row.method} · {row.status}
            </span>
            {row.status === 'pending' && (
              <Button
                type='button'
                size='sm'
                variant='outline'
                disabled={cancel.isPending}
                onClick={() => cancel.mutate(row.id)}
              >
                {t('Cancel')}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
