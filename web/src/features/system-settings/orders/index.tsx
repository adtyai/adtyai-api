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

import { Link } from '@tanstack/react-router'
import { Info, RefreshCw, Search, Undo2 } from 'lucide-react'
import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { ConfirmCompleteDialog } from './components/confirm-complete-dialog'
import { useAdminOrderDetail, useAdminOrders } from './hooks/use-orders'
import {
  DEFAULT_ORDERS_LIMIT,
  canCompleteOrder,
  formatUnixTime,
  formatUsd,
  orderStatusLabelKey,
  orderStatusVariant,
  parseUserIdFilter,
  paymentLabel,
  refundStatusLabelKey,
  refundStatusVariant,
  orderUserDisplay,
} from './lib/orders'
import type { AdminOrdersListParams, TopupOrder } from './types'

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant={orderStatusVariant(status)}>
      {t(orderStatusLabelKey(status))}
    </Badge>
  )
}

function RefundBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  const s = status || 'none'
  return (
    <Badge variant={refundStatusVariant(s)}>{t(refundStatusLabelKey(s))}</Badge>
  )
}

function DetailRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className='flex justify-between gap-3 border-b border-dashed pb-2 text-[13px]'>
      <span className='text-muted-foreground shrink-0'>{label}</span>
      <span className='text-right font-medium break-all'>{value}</span>
    </div>
  )
}

export function OrdersSettings() {
  const { t } = useTranslation()
  // Audit P1-1: manual complete is root-only on the backend.
  const isRoot = useAuthStore((s) => s.auth.user?.role === ROLE.SUPER_ADMIN)
  const [statusDraft, setStatusDraft] = useState('all')
  const [orderNoDraft, setOrderNoDraft] = useState('')
  const [userIdDraft, setUserIdDraft] = useState('')
  const [page, setPage] = useState(0)
  const [applied, setApplied] = useState<{
    status: string
    order_no: string
    user_id?: number
  }>({ status: '', order_no: '' })
  const [selectedNo, setSelectedNo] = useState<string | null>(null)
  const [completeOrder, setCompleteOrder] = useState<TopupOrder | null>(null)

  const listParams: AdminOrdersListParams = useMemo(
    () => ({
      status: applied.status || undefined,
      order_no: applied.order_no || undefined,
      user_id: applied.user_id,
      limit: DEFAULT_ORDERS_LIMIT,
      offset: page * DEFAULT_ORDERS_LIMIT,
    }),
    [applied, page]
  )

  const list = useAdminOrders(listParams)
  const detail = useAdminOrderDetail(selectedNo)

  const rows = list.data?.rows ?? []
  const total = list.data?.total ?? 0
  const isForbidden = (list.error as { status?: number } | null)?.status === 403

  const selected =
    detail.data || rows.find((r) => r.order_no === selectedNo) || null

  function onSearch(e?: FormEvent) {
    e?.preventDefault()
    setPage(0)
    setApplied({
      status: statusDraft === 'all' ? '' : statusDraft,
      order_no: orderNoDraft.trim(),
      user_id: parseUserIdFilter(userIdDraft),
    })
  }

  function filterByUser(userId: number) {
    setUserIdDraft(String(userId))
    setPage(0)
    setApplied((prev) => ({ ...prev, user_id: userId }))
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Topup orders')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
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
              'Admin list of top-up orders. Filter by status, order number, or user. Detail loads via GET /api/admin/orders/:no. Complete is for pending only and needs a second confirmation.'
            )}
          </p>

          <div className='grid gap-3'>
            <div className='flex gap-2.5 rounded-lg border border-indigo-100 bg-indigo-50/70 px-3.5 py-2.5 text-[13px] leading-5 text-indigo-950 dark:border-indigo-900/40 dark:bg-indigo-950/30 dark:text-indigo-100'>
              <Info className='mt-0.5 size-4 shrink-0' />
              <div>
                {t(
                  'Refunds stay on the Order refunds page — use the link in detail. Do not complete paid or failed orders from this UI.'
                )}
              </div>
            </div>
          </div>

          <form className='flex flex-wrap items-end gap-2' onSubmit={onSearch}>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Status')}
              </span>
              <Select
                items={[
                  { value: 'all', label: t('All statuses') },
                  { value: 'pending', label: t('Pending') },
                  { value: 'paid', label: t('Paid') },
                  { value: 'failed', label: t('Failed') },
                  { value: 'need_manual', label: t('Needs manual review') },
                ]}
                value={statusDraft}
                onValueChange={(v) => {
                  if (v) setStatusDraft(v)
                }}
              >
                <SelectTrigger className='h-9 w-36 text-sm'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>{t('All statuses')}</SelectItem>
                  <SelectItem value='pending'>{t('Pending')}</SelectItem>
                  <SelectItem value='paid'>{t('Paid')}</SelectItem>
                  <SelectItem value='failed'>{t('Failed')}</SelectItem>
                  <SelectItem value='need_manual'>
                    {t('Needs manual review')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>
                {t('Order No.')}
              </span>
              <Input
                value={orderNoDraft}
                onChange={(e) => setOrderNoDraft(e.target.value)}
                placeholder={t('Exact order number')}
                className='h-9 w-56 font-mono text-sm'
              />
            </div>
            <div className='grid gap-1'>
              <span className='text-muted-foreground text-xs'>{t('User')}</span>
              <Input
                value={userIdDraft}
                onChange={(e) => setUserIdDraft(e.target.value)}
                placeholder={t('User ID')}
                className='h-9 w-28 font-mono text-sm'
                inputMode='numeric'
              />
            </div>
            <Button type='submit' size='sm' className='h-9'>
              <Search className='size-3.5' />
              {t('Search')}
            </Button>
          </form>

          {isForbidden ? (
            <ErrorState
              title={t('Insufficient permissions')}
              description={t(
                'Admin permission is required to view topup orders.'
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
              title={t('Failed to load orders')}
              onRetry={() => void list.refetch()}
            />
          ) : null}

          {!list.isLoading && list.data && !isForbidden ? (
            <div className='grid gap-4 lg:grid-cols-[1fr_360px]'>
              <section className='bg-card overflow-hidden rounded-xl border shadow-xs'>
                <div className='flex items-center justify-between border-b px-5 pt-4 pb-3'>
                  <div>
                    <div className='text-[15px] font-semibold'>
                      {t('Orders')}
                    </div>
                    <div className='text-muted-foreground mt-0.5 text-xs'>
                      {total} {t('items')}
                    </div>
                  </div>
                  <div className='text-muted-foreground flex items-center gap-2 text-xs'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='h-7 px-2 text-xs'
                      disabled={page <= 0}
                      onClick={() => setPage((p) => Math.max(0, p - 1))}
                    >
                      {t('Prev')}
                    </Button>
                    <span>
                      {t('Page')} {page + 1}
                    </span>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='h-7 px-2 text-xs'
                      disabled={(page + 1) * DEFAULT_ORDERS_LIMIT >= total}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      {t('Next')}
                    </Button>
                  </div>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow className='bg-muted/40 hover:bg-muted/40'>
                      <TableHead>{t('Order No.')}</TableHead>
                      <TableHead>{t('User')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Refund')}</TableHead>
                      <TableHead>{t('Paid amount')}</TableHead>
                      <TableHead>{t('Payment method')}</TableHead>
                      <TableHead>{t('Created at')}</TableHead>
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
                          {t('No orders')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      rows.map((row) => (
                        <TableRow
                          key={row.order_no}
                          className={cn(
                            'cursor-pointer',
                            selectedNo === row.order_no && 'bg-muted/50'
                          )}
                          onClick={() => setSelectedNo(row.order_no)}
                        >
                          <TableCell>
                            <span className='font-mono text-[11.5px]'>
                              {row.order_no}
                            </span>
                          </TableCell>
                          <TableCell>
                            <button
                              type='button'
                              title={`ID ${row.user_id}`}
                              className='text-primary hover:underline'
                              onClick={(e) => {
                                e.stopPropagation()
                                filterByUser(row.user_id)
                              }}
                            >
                              {orderUserDisplay(row)}
                            </button>
                          </TableCell>
                          <TableCell>
                            <StatusBadge status={row.status} />
                          </TableCell>
                          <TableCell>
                            <RefundBadge status={row.refund_status || 'none'} />
                          </TableCell>
                          <TableCell className='font-semibold'>
                            {formatUsd(row.paid_amount_usd)}
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground text-xs'>
                              {paymentLabel(row, t)}
                            </span>
                          </TableCell>
                          <TableCell>
                            <span className='text-muted-foreground text-xs'>
                              {formatUnixTime(row.create_time)}
                            </span>
                          </TableCell>
                          <TableCell>
                            {isRoot && canCompleteOrder(row) ? (
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                className='h-7 px-2.5 text-xs'
                                onClick={(e) => {
                                  e.stopPropagation()
                                  setSelectedNo(row.order_no)
                                  setCompleteOrder(row)
                                }}
                              >
                                {t('Complete')}
                              </Button>
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
                    {t('Order detail')}
                  </div>
                </div>
                <div className='space-y-3 p-5 text-sm'>
                  <OrderDetailBody
                    selectedNo={selectedNo}
                    selected={selected}
                    detail={detail}
                    onComplete={(o) => setCompleteOrder(o)}
                  />
                </div>
              </section>
            </div>
          ) : null}
        </div>
      </SectionPageLayout.Content>

      <ConfirmCompleteDialog
        order={completeOrder}
        open={Boolean(completeOrder)}
        onOpenChange={(open) => {
          if (!open) setCompleteOrder(null)
        }}
        onDone={() => {
          if (completeOrder) setSelectedNo(completeOrder.order_no)
        }}
      />
    </SectionPageLayout>
  )
}

export default OrdersSettings

function OrderDetailBody({
  selectedNo,
  selected,
  detail,
  onComplete,
}: {
  selectedNo: string | null
  selected: TopupOrder | null | undefined
  detail: { isLoading: boolean; error: unknown; refetch: () => unknown }
  onComplete: (order: TopupOrder) => void
}) {
  const { t } = useTranslation()
  // Audit P1-1: manual complete is root-only on the backend.
  const isRoot = useAuthStore((s) => s.auth.user?.role === ROLE.SUPER_ADMIN)
  if (!selectedNo) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('Select a row to view details.')}
      </p>
    )
  }
  if (detail.isLoading && !selected) {
    return <p className='text-muted-foreground text-xs'>{t('Loading...')}</p>
  }
  if ((detail.error as { status?: number } | null)?.status === 404) {
    return <ErrorState title={t('Order not found')} />
  }
  if (!selected) {
    return (
      <ErrorState
        title={t('Failed to load order')}
        onRetry={() => void detail.refetch()}
      />
    )
  }
  return (
    <>
      <DetailRow
        label={t('Order No.')}
        value={<span className='font-mono text-xs'>{selected.order_no}</span>}
      />
      <DetailRow
        label={t('User')}
        value={`${orderUserDisplay(selected)} (ID ${selected.user_id})`}
      />
      <DetailRow
        label={t('Status')}
        value={<StatusBadge status={selected.status} />}
      />
      <DetailRow
        label={t('Refund')}
        value={<RefundBadge status={selected.refund_status || 'none'} />}
      />
      <DetailRow
        label={t('Paid amount')}
        value={formatUsd(selected.paid_amount_usd)}
      />
      <DetailRow
        label={t('Credit amount')}
        value={formatUsd(selected.credit_amount_usd)}
      />
      <DetailRow
        label={t('Bonus amount')}
        value={formatUsd(selected.bonus_amount_usd)}
      />
      <DetailRow
        label={t('Already refunded')}
        value={formatUsd(selected.refunded_amount)}
      />
      <DetailRow label={t('Payment method')} value={paymentLabel(selected, t)} />
      <DetailRow
        label={t('Provider trade no')}
        value={selected.provider_trade_no || '—'}
      />
      <DetailRow
        label={t('Created at')}
        value={formatUnixTime(selected.create_time)}
      />
      <DetailRow
        label={t('Completed at')}
        value={formatUnixTime(selected.complete_time)}
      />
      <div className='flex flex-wrap items-center gap-2 pt-1'>
        {isRoot && canCompleteOrder(selected) ? (
          <Button type='button' size='sm' onClick={() => onComplete(selected)}>
            {t('Complete')}
          </Button>
        ) : null}
        <Link
          to='/system-settings/order-refunds'
          className='border-border bg-background hover:bg-muted inline-flex h-8 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium'
        >
          <Undo2 className='size-3.5' />
          {t('Go to refunds')}
        </Link>
      </div>
    </>
  )
}
