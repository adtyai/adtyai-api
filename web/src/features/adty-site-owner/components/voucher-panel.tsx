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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { VoucherTable } from '@/features/adty-ledger/voucher-table'
import { PageSwitcher } from '@/features/referral/components/page-switcher'

import {
  cancelOwnerVoucher,
  freezeOwnerVoucher,
  getOwnerVouchers,
  issueOwnerVoucher,
  siteOwnerKeys,
} from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'

const PAGE_SIZE = 20

/**
 * Recharge codes funded by the site: issuing freezes the amount in the chosen
 * wallet, freezing stops redemption, cancelling releases the amount. Every
 * write asks the owner to verify first.
 */
export function VoucherPanel(props: { siteId: number }) {
  const { t } = useTranslation()
  const [purpose, setPurpose] = useState('site_customer')
  const [source, setSource] = useState('earnings')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [code, setCode] = useState('')
  const [page, setPage] = useState(1)
  const list = useQuery({
    queryKey: siteOwnerKeys.list('vouchers', { page }),
    queryFn: () => getOwnerVouchers(page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const marginAllowed = list.data?.margin_allowed === true
  const issue = useVerifiedOwnerMutation<void>({
    action: 'voucher.issue',
    target: () => props.siteId,
    run: async (_vars, proof) => {
      const issued = await issueOwnerVoucher(
        {
          purpose,
          amount_usd: amount.trim(),
          source: marginAllowed ? source : 'earnings',
          reason: reason.trim(),
        },
        proof
      )
      setCode(issued.code)
      setAmount('')
      setReason('')
    },
    successMessage: t('Code issued'),
  })
  const freeze = useVerifiedOwnerMutation<number>({
    action: 'voucher.freeze',
    target: (id) => id,
    run: (id, proof) => freezeOwnerVoucher(id, proof),
    successMessage: t('Code frozen'),
  })
  const cancel = useVerifiedOwnerMutation<number>({
    action: 'voucher.cancel',
    target: (id) => id,
    run: (id, proof) => cancelOwnerVoucher(id, proof),
    successMessage: t('Code cancelled'),
  })
  const rows = Array.isArray(list.data?.items) ? list.data.items : []

  return (
    <div className='flex flex-col gap-6'>
      <form
        className='flex max-w-xl flex-col gap-3'
        onSubmit={(event) => {
          event.preventDefault()
          setCode('')
          issue.mutate()
        }}
      >
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Recharge codes')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Issuing a code freezes the same amount in the wallet it is funded from. The full code is shown once.'
            )}
          </p>
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='voucher-purpose'>{t('Purpose')}</Label>
          <NativeSelect
            id='voucher-purpose'
            value={purpose}
            onChange={(event) => setPurpose(event.target.value)}
          >
            <option value='site_customer'>{t('Customer principal')}</option>
            <option value='site_owner'>{t('My spending principal')}</option>
          </NativeSelect>
        </div>
        {marginAllowed && (
          <div className='flex flex-col gap-1'>
            <Label htmlFor='voucher-source'>{t('Funded from')}</Label>
            <NativeSelect
              id='voucher-source'
              value={source}
              onChange={(event) => setSource(event.target.value)}
            >
              <option value='earnings'>{t('Earnings wallet')}</option>
              <option value='margin'>{t('Margin wallet')}</option>
            </NativeSelect>
          </div>
        )}
        <div className='flex flex-col gap-1'>
          <Label htmlFor='voucher-amount'>{t('Amount (USD)')}</Label>
          <Input
            id='voucher-amount'
            inputMode='decimal'
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
          />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='voucher-reason'>{t('Reason')}</Label>
          <Input
            id='voucher-reason'
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </div>
        <Button
          variant='brand'
          type='submit'
          disabled={
            issue.isPending || amount.trim() === '' || reason.trim() === ''
          }
        >
          {t('Issue code')}
        </Button>
        {code && (
          <div className='flex items-center gap-2 text-sm'>
            <span>{t('Voucher code')}:</span>
            <code className='font-mono break-all'>{code}</code>
            <CopyButton
              value={code}
              size='icon'
              aria-label={t('Copy voucher code')}
            />
          </div>
        )}
      </form>

      <section className='flex flex-col gap-3'>
        <h2 className='text-sm font-medium'>{t('Issued codes')}</h2>
        {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
        {list.isPending ? (
          <LoadingState />
        ) : (
          <>
            <VoucherTable
              label={t('Issued codes')}
              rows={rows}
              busy={freeze.isPending || cancel.isPending}
              onFreeze={(voucher) => freeze.mutate(voucher.id)}
              onCancel={(voucher) => cancel.mutate(voucher.id)}
            />
            <PageSwitcher
              page={page}
              total={Number(list.data?.total) || 0}
              pageSize={PAGE_SIZE}
              onPageChange={setPage}
            />
          </>
        )}
      </section>
    </div>
  )
}
