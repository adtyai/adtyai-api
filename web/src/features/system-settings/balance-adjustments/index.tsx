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

import { Info, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { ConfirmBalanceAdjustDialog } from './components/confirm-action-dialog'
import { CreateBalanceAdjustDialog } from './components/create-dialog'
import { useBalanceAdjustments } from './hooks/use-balance-adjustments'
import { formatAdjustUsd } from './lib/amount'
import type { ApproveBalanceAdjustResult, BalanceAdjustRequest } from './types'

type Action = 'approve' | 'reject'

function statusBadgeClass(status: string): string {
  if (status === 'pending') {
    return 'border-amber-200 bg-amber-50 text-amber-900'
  }
  if (status === 'executed') {
    return 'border-emerald-200 bg-emerald-50 text-emerald-900'
  }
  if (status === 'rejected') {
    return 'border-rose-200 bg-rose-50 text-rose-800'
  }
  return 'border-slate-200 bg-slate-50 text-slate-700'
}

export function BalanceAdjustmentsSettings() {
  const { t } = useTranslation()
  const currentUserId = useAuthStore((s) => s.auth.user?.id)
  const [status, setStatus] = useState('pending')
  const [createOpen, setCreateOpen] = useState(false)
  const [actionRow, setActionRow] = useState<BalanceAdjustRequest | null>(null)
  const [action, setAction] = useState<Action | null>(null)
  const [lastApprove, setLastApprove] =
    useState<ApproveBalanceAdjustResult | null>(null)

  const list = useBalanceAdjustments(status === 'all' ? '' : status)
  const rows = list.data?.rows ?? []
  const isForbidden = (list.error as { status?: number } | null)?.status === 403

  return (
    <>
      <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Balance adjustments')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Select
          items={[
            { value: 'pending', label: t('Pending') },
            { value: 'executed', label: t('Executed') },
            { value: 'rejected', label: t('Rejected') },
            { value: 'all', label: t('All statuses') },
          ]}
          value={status}
          onValueChange={(v) => {
            if (v) setStatus(v)
          }}
        >
          <SelectTrigger className='h-8 w-40 text-sm'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='pending'>{t('Pending')}</SelectItem>
            <SelectItem value='executed'>{t('Executed')}</SelectItem>
            <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
          </SelectContent>
        </Select>
        <Button
          type='button'
          size='sm'
          className='h-8'
          onClick={() => setCreateOpen(true)}
        >
          <Plus className='size-3.5' />
          {t('New request')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-8'
          onClick={() => void list.refetch()}
        >
          <RefreshCw className='size-3.5' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 w-full flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Dual-admin balance adjustment approvals. Create pending requests; approve executes AdjustQuota; reject leaves balances unchanged. Decisions take effect on submit.'
            )}
          </p>

          <div className='flex gap-2.5 rounded-lg border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-[13px] leading-5 text-amber-950 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-100'>
            <Info className='mt-0.5 size-4 shrink-0' />
            <div>
              {t(
                'Multiple admins cannot self-approve; sole admin self-approve needs a reason and is alerted.'
              )}
            </div>
          </div>

          {lastApprove ? (
            <div className='rounded-xl border border-emerald-200 bg-emerald-50/70 p-4 text-sm text-emerald-950 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-100'>
              <div className='mb-1 font-medium'>
                {t('Last approve result')} #{lastApprove.request?.id}
              </div>
              <div className='grid gap-1 text-xs sm:grid-cols-2 lg:grid-cols-4'>
                <div>
                  {t('Balance before')}:{' '}
                  {formatAdjustUsd(lastApprove.balance_before)}
                </div>
                <div>
                  {t('Balance after')}:{' '}
                  {formatAdjustUsd(lastApprove.balance_after)}
                </div>
                <div>ledger #{lastApprove.ledger_entry_id}</div>
                <div>
                  {lastApprove.self_approved
                    ? t('Self-approved')
                    : t('Peer-approved')}{' '}
                  · admins={lastApprove.admin_count}
                </div>
              </div>
            </div>
          ) : null}

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to view balance adjustments. Ask a super admin.'
              )}
            />
          ) : null}

          {list.isLoading ? (
            <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
              {t('Loading...')}
            </div>
          ) : null}

          {list.isError && !isForbidden && !list.data ? (
            <ErrorState
              title={t('Failed to load balance adjustments')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
              <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                <div className='text-[15px] font-semibold'>
                  {t('Adjustment requests')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {list.data.total} {t('items')}
                </div>
              </div>
              <Table>
                <TableHeader>
                  <TableRow className='bg-muted/40 hover:bg-muted/40'>
                    <TableHead>ID</TableHead>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Account')}</TableHead>
                    <TableHead>{t('Amount')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Applicant')}</TableHead>
                    <TableHead>{t('Reason')}</TableHead>
                    <TableHead>{t('Created at')}</TableHead>
                    <TableHead className='w-[1%]' />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t('No balance adjustments')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const isSelf =
                        currentUserId != null &&
                        Number(currentUserId) === Number(row.applicant_id)
                      return (
                        <TableRow key={row.id}>
                          <TableCell>
                            <code className='text-xs'>{row.id}</code>
                          </TableCell>
                          <TableCell>{row.user_id}</TableCell>
                          <TableCell>
                            <Badge variant='secondary' className='font-mono'>
                              {row.account}
                            </Badge>
                          </TableCell>
                          <TableCell
                            className={cn(
                              'font-semibold',
                              row.amount < 0 &&
                                'text-rose-700 dark:text-rose-300'
                            )}
                          >
                            {formatAdjustUsd(row.amount)}
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant='outline'
                              className={cn(
                                'font-mono text-[11px]',
                                statusBadgeClass(row.status)
                              )}
                            >
                              {row.status}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            {row.applicant_id}
                            {isSelf ? (
                              <span className='text-muted-foreground text-[11px]'>
                                {' '}
                                ({t('you')})
                              </span>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground line-clamp-2 text-xs'>
                              {row.reason || '-'}
                              {row.reject_reason
                                ? ` · ${row.reject_reason}`
                                : ''}
                            </span>
                          </TableCell>
                          <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                            {row.created_at
                              ? new Date(row.created_at).toLocaleString()
                              : '-'}
                          </TableCell>
                          <TableCell>
                            {row.status === 'pending' ? (
                              <div className='flex flex-wrap items-center gap-1.5'>
                                <Button
                                  type='button'
                                  size='sm'
                                  className='h-7 px-2.5 text-xs'
                                  onClick={() => {
                                    setActionRow(row)
                                    setAction('approve')
                                  }}
                                >
                                  {t('Approve')}
                                </Button>
                                <Button
                                  type='button'
                                  variant='outline'
                                  size='sm'
                                  className='h-7 px-2.5 text-xs'
                                  onClick={() => {
                                    setActionRow(row)
                                    setAction('reject')
                                  }}
                                >
                                  {t('Reject')}
                                </Button>
                              </div>
                            ) : (
                              <span className='text-muted-foreground text-xs'>
                                {t('Read only')}
                              </span>
                            )}
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </section>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>

      <CreateBalanceAdjustDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
      <ConfirmBalanceAdjustDialog
        row={actionRow}
        action={action}
        open={Boolean(actionRow && action)}
        onOpenChange={(open) => {
          if (!open) {
            setActionRow(null)
            setAction(null)
          }
        }}
        onDone={(result) => {
          if (result) setLastApprove(result)
        }}
      />
    </>
  )
}

export default BalanceAdjustmentsSettings
