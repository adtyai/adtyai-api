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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import type { OwnerCustomer } from '@/features/adty-site-owner/types'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  getStaffCustomers,
  getStaffSelf,
  STAFF_PAGE_SIZE,
  type StaffCustomerAction,
} from './api'
import { StaffBalanceRequestDialog } from './balance-request-dialog'
import { StaffCustomerActionDialog } from './customer-actions'
import { StaffRecordsDialog } from './records-dialog'

export function StaffWorkspace() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const [page, setPage] = useState(1)
  const [customerAction, setCustomerAction] = useState<{
    customer: OwnerCustomer
    action: StaffCustomerAction
  } | null>(null)
  const [balanceCustomer, setBalanceCustomer] = useState<OwnerCustomer | null>(
    null
  )
  const [records, setRecords] = useState<{
    customer: OwnerCustomer
    kind: 'topups' | 'logs'
  } | null>(null)
  const self = useQuery({
    queryKey: ['adty-staff', userId, 'self'],
    queryFn: getStaffSelf,
    retry: false,
    refetchInterval: 30000,
  })
  const grants = self.data?.staff.permissions ?? []
  const canList = grants.includes('customers.read')
  const list = useQuery({
    queryKey: [
      'adty-staff',
      userId,
      self.data?.staff.version,
      'customers',
      page,
    ],
    queryFn: () => getStaffCustomers(page),
    enabled: self.isSuccess && canList,
    retry: false,
  })
  const columns: StaticDataTableColumn<OwnerCustomer>[] = [
    {
      id: 'username',
      header: t('Username'),
      cell: (customer) => customer.username,
    },
    {
      id: 'email',
      header: t('Email'),
      cell: (customer) => customer.email || '—',
    },
    {
      id: 'status',
      header: t('Status'),
      cell: (customer) =>
        customer.status === 1 ? t('Enabled') : t('Disabled'),
    },
  ]
  if (grants.includes('customers.balance')) {
    columns.push({
      id: 'balance',
      header: t('Balance'),
      cell: (customer) =>
        customer.balance === undefined ? '—' : formatQuota(customer.balance),
    })
  }
  if (grants.includes('customers.usage')) {
    columns.push({
      id: 'usage',
      header: t('Usage'),
      cell: (customer) =>
        customer.used_quota === undefined
          ? '—'
          : formatQuota(customer.used_quota),
    })
  }
  if (
    canList ||
    grants.includes('customers.topups') ||
    grants.includes('customers.usage') ||
    grants.includes('customers.password') ||
    grants.includes('customers.disable')
  ) {
    columns.push({
      id: 'actions',
      header: t('Actions'),
      cell: (customer) => (
        <div className='flex flex-wrap gap-1'>
          {canList && (
            <Button
              size='sm'
              variant='outline'
              onClick={() => setBalanceCustomer(customer)}
            >
              {t('Request balance change')}
            </Button>
          )}
          {grants.includes('customers.topups') && (
            <Button
              size='sm'
              variant='outline'
              onClick={() => setRecords({ customer, kind: 'topups' })}
            >
              {t('Recharge records')}
            </Button>
          )}
          {grants.includes('customers.usage') && (
            <Button
              size='sm'
              variant='outline'
              onClick={() => setRecords({ customer, kind: 'logs' })}
            >
              {t('Usage records')}
            </Button>
          )}
          {customer.status === 1 && grants.includes('customers.password') && (
            <Button
              size='sm'
              variant='outline'
              onClick={() =>
                setCustomerAction({ customer, action: 'password-reset' })
              }
            >
              {t('Send password reset instructions')}
            </Button>
          )}
          {customer.status === 1 && grants.includes('customers.disable') && (
            <Button
              size='sm'
              variant='destructive'
              onClick={() => setCustomerAction({ customer, action: 'disable' })}
            >
              {t('Disable customer')}
            </Button>
          )}
        </div>
      ),
    })
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Staff workspace')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {self.isPending && <Skeleton className='h-32' />}
        {self.isError && <ErrorState onRetry={() => void self.refetch()} />}
        {self.isSuccess && (
          <div className='flex flex-col gap-4'>
            <p>
              {self.data.site_name} · {self.data.staff.display_name}
            </p>
            <Button
              variant='outline'
              onClick={() => {
                void self.refetch()
                if (canList) void list.refetch()
              }}
            >
              {t('Refresh')}
            </Button>
            {!canList && (
              <EmptyState
                title={t('No customer permissions have been assigned.')}
              />
            )}
            {canList && list.isPending && <Skeleton className='h-32' />}
            {canList && list.isError && (
              <ErrorState onRetry={() => void list.refetch()} />
            )}
            {canList && list.isSuccess && (
              <>
                <StaticDataTable
                  data={list.data.items ?? []}
                  getRowKey={(customer) => customer.id}
                  columns={columns}
                  tableProps={{ 'aria-label': t('Customers') }}
                  emptyContent={<EmptyState title={t('No customers yet')} />}
                />
                <PageSwitcher
                  page={page}
                  total={list.data.total}
                  pageSize={STAFF_PAGE_SIZE}
                  onPageChange={setPage}
                />
              </>
            )}
            {records &&
              canList &&
              grants.includes(
                records.kind === 'topups'
                  ? 'customers.topups'
                  : 'customers.usage'
              ) && (
                <StaffRecordsDialog
                  key={`${self.data.staff.version}-${records.customer.id}-${records.kind}`}
                  userId={userId ?? 0}
                  staffVersion={self.data.staff.version}
                  customer={records.customer}
                  kind={records.kind}
                  onClose={() => setRecords(null)}
                />
              )}
            {balanceCustomer && canList && (
              <StaffBalanceRequestDialog
                key={`${self.data.staff.version}-${balanceCustomer.id}`}
                customer={balanceCustomer}
                userId={userId ?? 0}
                onClose={() => setBalanceCustomer(null)}
              />
            )}
            {customerAction &&
              canList &&
              grants.includes(
                customerAction.action === 'password-reset'
                  ? 'customers.password'
                  : 'customers.disable'
              ) && (
                <StaffCustomerActionDialog
                  key={`${self.data.staff.version}-${customerAction.customer.id}-${customerAction.action}`}
                  customer={customerAction.customer}
                  action={customerAction.action}
                  userId={userId ?? 0}
                  onClose={() => setCustomerAction(null)}
                />
              )}
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
