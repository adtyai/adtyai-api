import { Info, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
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
  riskReasonLabelKey,
  riskReviewKindLabelKey,
  riskReviewStatusLabelKey,
} from '@/features/system-settings/utils/status-labels'
import { cn } from '@/lib/utils'

import { ConfirmActionDialog } from './components/confirm-action-dialog'
import { useRiskReviewDetail, useRiskReviews } from './hooks/use-risk-reviews'
import { formatUsd, sortRiskReviewsForDisplay } from './lib/sort'
import type { RiskReview } from './types'

type Action = 'approve' | 'reject'

export function RiskReviewsSettings() {
  const { t } = useTranslation()
  const reasonText = (reason: string | undefined | null) => {
    if (!reason) return '-'
    const labelKey = riskReasonLabelKey(reason)
    return labelKey ? t(labelKey) : reason
  }
  const [status, setStatus] = useState('pending')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [actionRow, setActionRow] = useState<RiskReview | null>(null)
  const [action, setAction] = useState<Action | null>(null)

  const list = useRiskReviews(status === 'all' ? '' : status)
  const detail = useRiskReviewDetail(selectedId)

  const rows = useMemo(
    () => sortRiskReviewsForDisplay(list.data?.rows ?? []),
    [list.data?.rows]
  )

  const isForbidden = (list.error as { status?: number } | null)?.status === 403

  const selected = detail.data || rows.find((r) => r.id === selectedId) || null

  return (
    <>
      <SectionPageLayout>
      <SectionPageLayout.Title>{t('Risk reviews')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Select
          items={[
            { value: 'pending', label: t('Pending') },
            { value: 'approved', label: t('Approved') },
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
            <SelectItem value='approved'>{t('Approved')}</SelectItem>
            <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
            <SelectItem value='all'>{t('All statuses')}</SelectItem>
          </SelectContent>
        </Select>
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
              'Manual queue for first-topup extra and other risk hits. Approve credits bonus only; reject leaves paid amount settled without gift. Actions take effect on submit.'
            )}
          </p>

          <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
            <Info className='mt-0.5 size-4 shrink-0' />
            <div>
              {t(
                'Approve: credit pending bonus/gift. Reject: do not credit gift. Actions take effect on submit.'
              )}
            </div>
          </div>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'You do not have permission to view risk reviews. Ask a super admin.'
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
              title={t('Failed to load risk reviews')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <div className='grid gap-4 lg:grid-cols-[1fr_360px]'>
              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                  <div>
                    <div className='text-[15px] font-semibold'>
                      {t('Queue')}
                    </div>
                    <div className='text-muted-foreground mt-0.5 text-xs'>
                      {list.data.total} {t('items')}
                    </div>
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow className='bg-muted/40 hover:bg-muted/40'>
                      <TableHead>ID</TableHead>
                      <TableHead>{t('Kind')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('User')}</TableHead>
                      <TableHead>{t('Order')}</TableHead>
                      <TableHead>{t('Amount')}</TableHead>
                      <TableHead>{t('Reason')}</TableHead>
                      <TableHead className='w-[1%]' />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={8}
                          className='text-muted-foreground h-24 text-center text-sm'
                        >
                          {t('No risk reviews')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((row) => (
                        <TableRow
                          key={row.id}
                          className={cn(
                            'cursor-pointer',
                            selectedId === row.id && 'bg-muted/50'
                          )}
                          onClick={() => setSelectedId(row.id)}
                        >
                          <TableCell>
                            <code className='text-xs'>{row.id}</code>
                          </TableCell>
                          <TableCell>
                            <Badge variant='secondary' className='font-mono'>
                              {t(riskReviewKindLabelKey(row.kind))}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <code className='text-xs'>
                              {t(riskReviewStatusLabelKey(row.status))}
                            </code>
                          </TableCell>
                          <TableCell>{row.user_id}</TableCell>
                          <TableCell>
                            <span className='font-mono text-[11.5px]'>
                              {row.order_no || '-'}
                            </span>
                          </TableCell>
                          <TableCell className='font-semibold'>
                            {formatUsd(row.amount_usd)}
                          </TableCell>
                          <TableCell>
                            <span
                              className='text-muted-foreground line-clamp-2 text-xs'
                              data-table-text='secondary'
                            >
                              {reasonText(row.reason)}
                            </span>
                          </TableCell>
                          <TableCell>
                            {row.status === 'pending' ? (
                              <div
                                className='flex items-center gap-1.5'
                                onClick={(e) => e.stopPropagation()}
                              >
                                <Button
                                  type='button'
                                  variant='outline'
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
                                  variant='ghost'
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
                                -
                              </span>
                            )}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </section>

              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='border-b px-5 pt-4 pb-3'>
                  <div className='text-[15px] font-semibold'>
                    {selected
                      ? `${t('Review detail')} #${selected.id}`
                      : t('Review detail')}
                  </div>
                </div>
                <div className='space-y-3 p-5 text-sm'>
                  {!selected ? (
                    <p className='text-muted-foreground text-xs'>
                      {t('Select a row to view details.')}
                    </p>
                  ) : (
                    <>
                      {(
                        [
                          ['ID', String(selected.id)],
                          [t('Kind'), t(riskReviewKindLabelKey(selected.kind))],
                          [
                            t('Status'),
                            t(riskReviewStatusLabelKey(selected.status)),
                          ],
                          [t('User'), String(selected.user_id)],
                          [t('Order'), selected.order_no || '-'],
                          [t('Amount'), formatUsd(selected.amount_usd)],
                          [t('Reason'), reasonText(selected.reason)],
                          [
                            t('Resolved by'),
                            selected.resolved_by
                              ? String(selected.resolved_by)
                              : '-',
                          ],
                          [
                            t('Created at'),
                            selected.created_at
                              ? new Date(selected.created_at).toLocaleString()
                              : '-',
                          ],
                          [
                            t('Resolved at'),
                            selected.resolved_at
                              ? new Date(selected.resolved_at).toLocaleString()
                              : '-',
                          ],
                        ] as [string, string][]
                      ).map(([label, value]) => (
                        <div
                          key={label}
                          className='flex justify-between gap-3 border-b border-dashed pb-2 text-[13px]'
                        >
                          <span className='text-muted-foreground shrink-0'>
                            {label}
                          </span>
                          <span className='text-right font-medium break-all'>
                            {value}
                          </span>
                        </div>
                      ))}
                      {selected.payload ? (
                        <div>
                          <div className='text-muted-foreground mb-1 text-xs'>
                            payload
                          </div>
                          <pre className='bg-muted/40 max-h-40 overflow-auto rounded-lg border p-2 font-mono text-[11px] break-all whitespace-pre-wrap'>
                            {selected.payload}
                          </pre>
                        </div>
                      ) : null}
                      {selected.status === 'pending' ? (
                        <div className='flex flex-wrap gap-2 pt-1'>
                          <Button
                            type='button'
                            size='sm'
                            onClick={() => {
                              setActionRow(selected)
                              setAction('approve')
                            }}
                          >
                            {t('Approve')}
                          </Button>
                          <Button
                            type='button'
                            size='sm'
                            variant='outline'
                            onClick={() => {
                              setActionRow(selected)
                              setAction('reject')
                            }}
                          >
                            {t('Reject')}
                          </Button>
                        </div>
                      ) : null}
                    </>
                  )}
                </div>
              </section>
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>

      <ConfirmActionDialog
        row={actionRow}
        action={action}
        open={Boolean(actionRow && action)}
        onOpenChange={(open) => {
          if (!open) {
            setActionRow(null)
            setAction(null)
          }
        }}
        onDone={() => {
          if (actionRow) setSelectedId(actionRow.id)
        }}
      />
    </>
  )
}

export default RiskReviewsSettings
