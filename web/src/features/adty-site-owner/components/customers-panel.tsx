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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { WalletLotProjectionLines } from '@/features/wallet/components/wallet-lot-projection'
import { formatQuota, formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getOwnerCustomers,
  lookupOwnerCustomer,
  OWNER_PAGE_SIZE,
  siteOwnerKeys,
} from '../api'
import type { OwnerCustomer } from '../types'
import { CustomerSheet } from './customer-sheet'

function RiskBadge(props: { customer: OwnerCustomer }) {
  const { t } = useTranslation()
  if (!props.customer.risk_flag || props.customer.reviewed) return null
  return (
    <StatusBadge
      variant='warning'
      label={
        props.customer.risk_flag === 'self_use'
          ? t('Suspected self-use')
          : t('Suspected move from the main site')
      }
    />
  )
}

/** Customer list by sign-up time, exact lookup and the detail drawer (tasks 11.6, 11.7). */
export function CustomersPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [query, setQuery] = useState('')
  const [openId, setOpenId] = useState<number | null>(null)
  const list = useQuery({
    queryKey: siteOwnerKeys.list('customers', { p: page }),
    queryFn: () => getOwnerCustomers(page),
  })
  const lookup = useMutation({
    mutationFn: () => lookupOwnerCustomer(query.trim()),
    onSuccess: (customer) => setOpenId(customer.id),
    onError: (err) => {
      handleServerError(err)
    },
  })
  const balanceCell = (customer: OwnerCustomer) =>
    customer.balance === undefined ? (
      '—'
    ) : (
      <div>
        <div>{formatQuota(customer.balance)}</div>
        <WalletLotProjectionLines
          compact
          projection={customer.wallet_lot_projection}
        />
      </div>
    )
  if (list.isError && !list.data) {
    return <ErrorState onRetry={() => void list.refetch()} />
  }

  return (
    <div className='flex flex-col gap-4'>
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      <form
        className='flex flex-wrap items-end gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          if (!query.trim()) {
            toast.error(t('Enter the full username or email'))
            return
          }
          lookup.mutate()
        }}
      >
        <div className='space-y-2'>
          <Label htmlFor='owner-lookup'>{t('Find a customer')}</Label>
          <Input
            id='owner-lookup'
            className='w-72'
            value={query}
            placeholder={t('Full username or email')}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>
        <Button type='submit' variant='outline' disabled={lookup.isPending}>
          {t('Find')}
        </Button>
        <p className='text-muted-foreground w-full text-xs'>
          {t(
            'Only exact matches are found, and lookups per hour are limited. Every lookup is recorded.'
          )}
        </p>
      </form>
      <StaticDataTable
        tableClassName='min-w-[720px]'
        tableProps={{ 'aria-label': t('Customers') }}
        data={list.data?.items ?? []}
        getRowKey={(row) => String(row.id)}
        emptyContent={
          <EmptyState className='min-h-32' title={t('No customers yet')} />
        }
        columns={[
          { id: 'id', header: t('ID'), cell: (row) => row.id },
          {
            id: 'username',
            header: t('Username'),
            cell: (row) => (
              <span className='flex items-center gap-2'>
                {row.username}
                <RiskBadge customer={row} />
              </span>
            ),
          },
          { id: 'email', header: t('Email'), cell: (row) => row.email || '—' },
          { id: 'balance', header: t('Balance'), cell: balanceCell },
          {
            id: 'joined',
            header: t('Signed up'),
            cell: (row) => formatTimestamp(row.created_at),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                onClick={() => setOpenId(row.id)}
              >
                {t('Details')}
              </Button>
            ),
          },
        ]}
      />
      <PageSwitcher
        page={page}
        total={list.data?.total ?? 0}
        pageSize={OWNER_PAGE_SIZE}
        onPageChange={setPage}
      />
      <CustomerSheet
        customerId={openId}
        onOpenChange={(open) => !open && setOpenId(null)}
      />
    </div>
  )
}
