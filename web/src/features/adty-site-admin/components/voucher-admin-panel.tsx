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
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  VOUCHER_STATUSES,
  voucherPurposeLabel,
  voucherStatusLabel,
} from '@/features/adty-ledger/voucher-labels'
import { VoucherTable } from '@/features/adty-ledger/voucher-table'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { handleServerError } from '@/lib/handle-server-error'

import {
  cancelAdminVoucher,
  freezeAdminVoucher,
  getAdminVouchers,
  getVoucherIssuance,
  issueMainUserVoucher,
  saveVoucherIssuanceLimit,
  siteAdminKeys,
} from '../api'

const PAGE_SIZE = 20
const PURPOSES = [
  'main_user',
  'site_margin',
  'site_customer',
  'site_owner',
] as const

/**
 * Root manages recharge codes: the main-site issuance limit, main-site user
 * codes, and every issued code with freeze and cancel. Margin codes for one
 * site are issued from that site's fund area.
 */
export function VoucherAdminPanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [limit, setLimit] = useState('')
  const [amount, setAmount] = useState('')
  const [reason, setReason] = useState('')
  const [code, setCode] = useState('')
  const [filters, setFilters] = useState({ status: '', purpose: '' })
  const [page, setPage] = useState(1)
  const issuance = useQuery({
    queryKey: siteAdminKeys.list('voucher-issuance', {}),
    queryFn: getVoucherIssuance,
  })
  const list = useQuery({
    queryKey: siteAdminKeys.list('vouchers', { ...filters, page }),
    queryFn: () => getAdminVouchers(filters, page, PAGE_SIZE),
    placeholderData: keepPreviousData,
  })
  const refresh = () =>
    void client.invalidateQueries({ queryKey: siteAdminKeys.all })
  const saveLimit = useMutation({
    mutationFn: () => saveVoucherIssuanceLimit(limit.trim()),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setLimit('')
      refresh()
    },
    onError: (error) => handleServerError(error),
  })
  const issue = useMutation({
    mutationFn: () =>
      issueMainUserVoucher({
        amount_usd: amount.trim(),
        reason: reason.trim(),
      }),
    onSuccess: (issued) => {
      setCode(issued.code)
      setAmount('')
      setReason('')
      refresh()
    },
    onError: (error) => handleServerError(error),
  })
  const change = useMutation({
    mutationFn: (input: { kind: 'freeze' | 'cancel'; id: number }) =>
      input.kind === 'freeze'
        ? freezeAdminVoucher(input.id)
        : cancelAdminVoucher(input.id),
    onSuccess: (_data, input) => {
      toast.success(
        input.kind === 'freeze' ? t('Code frozen') : t('Code cancelled')
      )
      refresh()
    },
    onError: (error) => handleServerError(error),
  })
  const rows = Array.isArray(list.data?.items) ? list.data.items : []

  return (
    <div className='flex flex-col gap-6'>
      <section className='flex flex-col gap-3'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>
            {t('Main-site issuance limit')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Main-site user codes and site margin codes are both counted against this limit. Codes cannot be issued until it is set.'
            )}
          </p>
        </div>
        {issuance.isPending && <LoadingState />}
        {issuance.isError && (
          <ErrorState onRetry={() => void issuance.refetch()} />
        )}
        {issuance.data && (
          <dl className='grid max-w-md grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 text-sm'>
            <dt className='text-muted-foreground'>{t('Limit')}</dt>
            <dd className='text-right tabular-nums'>
              {issuance.data.limit_usd} USD
            </dd>
            <dt className='text-muted-foreground'>
              {t('Held by unredeemed codes')}
            </dt>
            <dd className='text-right tabular-nums'>
              {issuance.data.frozen_usd} USD
            </dd>
            <dt className='text-muted-foreground'>{t('Redeemed')}</dt>
            <dd className='text-right tabular-nums'>
              {issuance.data.spent_usd} USD
            </dd>
          </dl>
        )}
        <div className='flex max-w-md flex-wrap items-end gap-2'>
          <div className='flex flex-1 flex-col gap-1'>
            <Label htmlFor='voucher-limit'>{t('New limit (USD)')}</Label>
            <Input
              id='voucher-limit'
              inputMode='decimal'
              value={limit}
              onChange={(event) => setLimit(event.target.value)}
            />
          </div>
          <Button
            type='button'
            variant='outline'
            disabled={saveLimit.isPending || limit.trim() === ''}
            onClick={() => saveLimit.mutate()}
          >
            {t('Save')}
          </Button>
        </div>
      </section>

      <section className='flex max-w-md flex-col gap-3'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>
            {t('Issue a main-site user code')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Only main-site users can redeem it. The full code is shown once.'
            )}
          </p>
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='main-voucher-amount'>{t('Amount (USD)')}</Label>
          <Input
            id='main-voucher-amount'
            inputMode='decimal'
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
          />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='main-voucher-reason'>{t('Reason')}</Label>
          <Input
            id='main-voucher-reason'
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </div>
        <Button
          type='button'
          variant='brand'
          disabled={
            issue.isPending || amount.trim() === '' || reason.trim() === ''
          }
          onClick={() => issue.mutate()}
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
      </section>

      <section className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h2 className='text-sm font-medium'>{t('Issued codes')}</h2>
          <div className='flex flex-wrap gap-2'>
            <NativeSelect
              aria-label={t('Purpose')}
              value={filters.purpose}
              onChange={(event) => {
                setFilters({ ...filters, purpose: event.target.value })
                setPage(1)
              }}
            >
              <NativeSelectOption value=''>
                {t('All purposes')}
              </NativeSelectOption>
              {PURPOSES.map((value) => (
                <NativeSelectOption key={value} value={value}>
                  {voucherPurposeLabel(t, value)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <NativeSelect
              aria-label={t('Status')}
              value={filters.status}
              onChange={(event) => {
                setFilters({ ...filters, status: event.target.value })
                setPage(1)
              }}
            >
              <NativeSelectOption value=''>{t('All')}</NativeSelectOption>
              {VOUCHER_STATUSES.map((value) => (
                <NativeSelectOption key={value} value={value}>
                  {voucherStatusLabel(t, value)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
        </div>
        {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
        {list.isPending ? (
          <LoadingState />
        ) : (
          <>
            <VoucherTable
              label={t('Issued codes')}
              rows={rows}
              busy={change.isPending}
              showIssuer
              onFreeze={(voucher) =>
                change.mutate({ kind: 'freeze', id: voucher.id })
              }
              onCancel={(voucher) =>
                change.mutate({ kind: 'cancel', id: voucher.id })
              }
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
