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

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  deleteStaffAccount,
  getOwnerBalanceRequests,
  getStaffAccounts,
  getStaffAudits,
  reviewOwnerBalanceRequest,
  saveStaffAccount,
  STAFF_PAGE_SIZE,
  type StaffAccount,
  type StaffBalanceRequest,
} from '@/features/adty-site-staff/api'
import { PageSwitcher } from '@/features/referral/components/page-switcher'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { siteOwnerKeys } from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'
import { StaffDialog } from './staff-dialog'

export function StaffPanel(props: { siteId: number }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<{
    mode: 'create' | 'edit' | 'password'
    staff: StaffAccount | null
  } | null>(null)
  const [confirming, setConfirming] = useState<{
    staff: StaffAccount
    action: 'status' | 'delete'
  } | null>(null)
  const client = useQueryClient()
  const requests = useQuery({
    queryKey: siteOwnerKeys.list('balance-requests', { siteId: props.siteId }),
    queryFn: getOwnerBalanceRequests,
  })
  const review = useMutation({
    mutationFn: (input: { id: number; approve: boolean }) =>
      reviewOwnerBalanceRequest(input.id, input.approve),
    onSuccess: () =>
      void client.invalidateQueries({
        queryKey: siteOwnerKeys.list('balance-requests', {
          siteId: props.siteId,
        }),
      }),
      void invalidateSidebarBadges(client)
    onError: (error) => handleServerError(error),
  })
  const list = useQuery({
    queryKey: siteOwnerKeys.list('staff', { p: page }),
    queryFn: () => getStaffAccounts(page),
  })
  const changeStatus = useVerifiedOwnerMutation<StaffAccount>({
    action: 'staff.update',
    target: (staff) => staff.id,
    run: (staff, proof) =>
      saveStaffAccount(
        staff.id,
        {
          display_name: staff.display_name,
          role: staff.role,
          permissions: staff.permissions,
          status: staff.status === 'enabled' ? 'disabled' : 'enabled',
          expected_version: staff.version,
        },
        proof
      ),
    onSuccess: () => setConfirming(null),
  })
  const remove = useVerifiedOwnerMutation<StaffAccount>({
    action: 'staff.delete',
    target: (staff) => staff.id,
    run: deleteStaffAccount,
    onSuccess: () => setConfirming(null),
  })
  const roleLabels = {
    support: t('Customer support'),
    finance: t('Finance'),
    operations: t('Operations'),
    manager: t('Management'),
  }
  let confirmLabel = t('Delete')
  if (confirming?.action === 'status') {
    confirmLabel =
      confirming.staff.status === 'enabled' ? t('Disable') : t('Enable')
  }
  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-lg font-medium'>{t('Staff accounts')}</h2>
        <Button onClick={() => setEditing({ mode: 'create', staff: null })}>
          {t('Create staff account')}
        </Button>
      </div>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Staff receive only the permissions selected here. Wallets, withdrawals, API keys and cross-site access are always blocked.'
        )}
      </p>
      {list.isPending && <Skeleton className='h-24' />}
      {list.isError && <ErrorState onRetry={() => void list.refetch()} />}
      {!list.isPending && !list.isError && (
        <>
          <StaticDataTable
            data={list.data.items ?? []}
            getRowKey={(staff) => staff.id}
            tableProps={{ 'aria-label': t('Staff accounts') }}
            emptyContent={<EmptyState title={t('No staff accounts')} />}
            columns={[
              {
                id: 'name',
                header: t('Display name'),
                cell: (staff) => (
                  <div>
                    <div>{staff.display_name}</div>
                    <div className='text-muted-foreground text-xs'>
                      {staff.username}
                    </div>
                  </div>
                ),
              },
              {
                id: 'role',
                header: t('Staff role'),
                cell: (staff) => roleLabels[staff.role],
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (staff) =>
                  staff.status === 'enabled' ? t('Enabled') : t('Disabled'),
              },
              {
                id: 'actions',
                header: t('Actions'),
                cell: (staff) => (
                  <div className='flex flex-wrap gap-1'>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setEditing({ mode: 'edit', staff })}
                    >
                      {t('Edit')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setEditing({ mode: 'password', staff })}
                    >
                      {t('Reset staff password')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setConfirming({ staff, action: 'status' })}
                    >
                      {staff.status === 'enabled' ? t('Disable') : t('Enable')}
                    </Button>
                    <Button
                      size='sm'
                      variant='destructive'
                      onClick={() => setConfirming({ staff, action: 'delete' })}
                    >
                      {t('Delete')}
                    </Button>
                  </div>
                ),
              },
            ]}
          />
          <PageSwitcher
            page={page}
            total={list.data.total}
            pageSize={STAFF_PAGE_SIZE}
            onPageChange={setPage}
          />
        </>
      )}
      <div className='flex flex-col gap-2'>
        <h2 className='text-lg font-medium'>{t('Balance requests')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'This request does not change the customer balance until the site owner or Root approves it.'
          )}
        </p>
        {requests.isPending && <Skeleton className='h-24' />}
        {requests.isError && (
          <ErrorState onRetry={() => void requests.refetch()} />
        )}
        {requests.isSuccess && (
          <StaticDataTable
            data={requests.data}
            getRowKey={(row) => row.id}
            tableProps={{ 'aria-label': t('Balance requests') }}
            emptyContent={<EmptyState title={t('No data')} />}
            columns={[
              {
                id: 'customer',
                header: t('Customer ID'),
                cell: (row: StaffBalanceRequest) => (
                  <span className='tabular-nums'>{row.customer_user_id}</span>
                ),
              },
              {
                id: 'direction',
                header: t('Direction'),
                cell: (row: StaffBalanceRequest) =>
                  row.direction === 'increase' ? t('Increase') : t('Decrease'),
              },
              {
                id: 'amount',
                header: t('Amount'),
                cell: (row: StaffBalanceRequest) => (
                  <span className='tabular-nums'>{row.amount_usd}</span>
                ),
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (row: StaffBalanceRequest) =>
                  row.status === 'pending'
                    ? t('Pending')
                    : row.status === 'approved'
                      ? t('Approved')
                      : t('Rejected'),
              },
              {
                id: 'actions',
                header: t('Actions'),
                cell: (row: StaffBalanceRequest) =>
                  row.status === 'pending' ? (
                    <div className='flex flex-wrap gap-1'>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={review.isPending}
                        onClick={() =>
                          review.mutate({ id: row.id, approve: true })
                        }
                      >
                        {t('Approve')}
                      </Button>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={review.isPending}
                        onClick={() =>
                          review.mutate({ id: row.id, approve: false })
                        }
                      >
                        {t('Reject')}
                      </Button>
                    </div>
                  ) : (
                    <span className='text-muted-foreground text-xs tabular-nums'>
                      {row.reviewed_at > 0
                        ? formatTimestamp(row.reviewed_at)
                        : '—'}
                    </span>
                  ),
              },
            ]}
          />
        )}
      </div>
      {editing && (
        <StaffDialog
          siteId={props.siteId}
          staff={editing.staff}
          mode={editing.mode}
          onClose={() => setEditing(null)}
        />
      )}
      <ConfirmDialog
        open={confirming !== null}
        onOpenChange={(open) => {
          if (!open) setConfirming(null)
        }}
        title={confirmLabel}
        confirmText={confirmLabel}
        destructive={
          confirming?.action === 'delete' ||
          confirming?.staff.status === 'enabled'
        }
        isLoading={changeStatus.isPending || remove.isPending}
        desc={t(
          'Existing staff sessions will stop working immediately. The staff member must sign in again after access is restored.'
        )}
        handleConfirm={() => {
          if (!confirming) return
          if (confirming.action === 'delete') remove.mutate(confirming.staff)
          else changeStatus.mutate(confirming.staff)
        }}
      />
      <StaffActivity />
    </div>
  )
}

function StaffActivity() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const audits = useQuery({
    queryKey: siteOwnerKeys.list('staff-audits', { p: page }),
    queryFn: () => getStaffAudits(page),
  })
  const actionLabels: Record<string, string> = {
    'staff.create': t('Create staff account'),
    'staff.update': t('Edit staff account'),
    'staff.password': t('Reset staff password'),
    'staff.delete': t('Delete staff account'),
    'staff.customers.read': t('View customer list'),
    'staff.customer.read': t('View customer list'),
    'staff.topups.read': t('View customer recharges'),
    'staff.usage.read': t('View customer usage'),
    'staff.customer.password.request': t('Send password reset instructions'),
    'staff.customer.disable': t('Disable customer'),
  }
  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-lg font-medium'>{t('Staff activity')}</h2>
      {audits.isPending && <Skeleton className='h-24' />}
      {audits.isError && <ErrorState onRetry={() => void audits.refetch()} />}
      {audits.data && !audits.isError && (
        <>
          <StaticDataTable
            data={audits.data.items ?? []}
            getRowKey={(audit) => audit.id}
            tableProps={{ 'aria-label': t('Staff activity') }}
            emptyContent={<EmptyState title={t('No activity yet')} />}
            columns={[
              {
                id: 'when',
                header: t('Time'),
                cell: (audit) => formatTimestamp(audit.created_at),
              },
              {
                id: 'operator',
                header: t('User ID'),
                cell: (audit) => audit.operator_id,
              },
              {
                id: 'action',
                header: t('Action'),
                cell: (audit) => actionLabels[audit.action] ?? audit.action,
              },
              {
                id: 'staff',
                header: t('Staff ID'),
                cell: (audit) => audit.staff_id || '—',
              },
              {
                id: 'customer',
                header: t('Customer ID'),
                cell: (audit) => audit.customer_id || '—',
              },
            ]}
          />
          <PageSwitcher
            page={page}
            total={audits.data.total}
            pageSize={STAFF_PAGE_SIZE}
            onPageChange={setPage}
          />
        </>
      )}
    </section>
  )
}
