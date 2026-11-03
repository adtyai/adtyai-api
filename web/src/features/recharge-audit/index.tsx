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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import {
  methodLabel,
  ORDER_STATUS_LABELS,
  ORDER_STATUS_VARIANTS,
} from '@/features/wallet/adty/lib/methods'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import type { AdtyOrderStatus } from '@/features/wallet/adty/types'
import { toIntlLocale } from '@/i18n/languages'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatTimestamp } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import {
  getRechargeAudit,
  getRechargeAuditSummary,
  type RechargeAuditFilters,
  type RechargeAuditOrder,
} from './api'
import {
  ManualCompleteDialog,
  RefundDialog,
} from './components/order-action-dialogs'
import { PaymentOrphanPanel } from './components/payment-orphan-panel'

const PAGE_SIZE = 20
const STATUSES: AdtyOrderStatus[] = [
  'pending',
  'success',
  'expired',
  'canceled',
  'failed',
  'refunded',
]

/** Recharge reconciliation: orders, totals in USD, manual completion, refunds. */
export function RechargeAudit() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const canComplete = canAdmin(user, ADTY_PERMISSIONS.FINANCE_MANUAL_CREDIT)
  const canRefund = canAdmin(user, ADTY_PERMISSIONS.FINANCE_REFUND)
  const [keyword, setKeyword] = useState('')
  const [filters, setFilters] = useState<RechargeAuditFilters>({
    page: 1,
    pageSize: PAGE_SIZE,
  })
  const [completing, setCompleting] = useState<RechargeAuditOrder | null>(null)
  const [refunding, setRefunding] = useState<RechargeAuditOrder | null>(null)

  const ordersQuery = useQuery({
    queryKey: ['recharge-audit', 'orders', filters],
    queryFn: () => getRechargeAudit(filters),
  })
  const summaryQuery = useQuery({
    queryKey: ['recharge-audit', 'summary', { ...filters, page: 1 }],
    queryFn: () => getRechargeAuditSummary(filters),
  })
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['recharge-audit'] })
  const total = ordersQuery.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Recharge Audit')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <div className='grid gap-3 sm:grid-cols-3'>
            {[
              {
                label: t('Orders'),
                value: String(summaryQuery.data?.orders ?? 0),
              },
              {
                label: t('Credited face value'),
                value: formatUsd(
                  Number(summaryQuery.data?.credited_usd ?? 0),
                  locale
                ),
              },
              {
                label: t('Bonus given'),
                value: formatUsd(
                  Number(summaryQuery.data?.bonus_usd ?? 0),
                  locale
                ),
              },
            ].map((item) => (
              <Card key={item.label}>
                <CardContent className='flex flex-col gap-1 py-4'>
                  <span className='text-muted-foreground text-xs'>
                    {item.label}
                  </span>
                  <span className='text-xl font-semibold'>{item.value}</span>
                </CardContent>
              </Card>
            ))}
          </div>

          <Card>
            <CardContent className='flex flex-col gap-3 py-4'>
              <form
                className='flex flex-wrap items-end gap-2'
                onSubmit={(event) => {
                  event.preventDefault()
                  setFilters((prev) => ({
                    ...prev,
                    keyword: keyword.trim(),
                    page: 1,
                  }))
                }}
              >
                <Input
                  className='w-64'
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder={t('Order number, username or transaction hash')}
                  aria-label={t('Search')}
                />
                <NativeSelect
                  aria-label={t('Status')}
                  value={filters.status ?? ''}
                  onChange={(event) =>
                    setFilters((prev) => ({
                      ...prev,
                      status: event.target.value,
                      page: 1,
                    }))
                  }
                >
                  <option value=''>{t('All statuses')}</option>
                  {STATUSES.map((status) => (
                    <option key={status} value={status}>
                      {t(ORDER_STATUS_LABELS[status])}
                    </option>
                  ))}
                </NativeSelect>
                <NativeSelect
                  aria-label={t('Provider')}
                  value={filters.provider ?? ''}
                  onChange={(event) =>
                    setFilters((prev) => ({
                      ...prev,
                      provider: event.target.value,
                      page: 1,
                    }))
                  }
                >
                  <option value=''>{t('All providers')}</option>
                  <option value='bepusdt'>BEpusdt</option>
                  <option value='adty_epay'>{t('Epay channels')}</option>
                </NativeSelect>
                <Button type='submit'>{t('Search')}</Button>
              </form>

              <StaticDataTable
                tableClassName='min-w-[1100px]'
                tableProps={{ 'aria-label': t('Recharge Audit') }}
                data={ordersQuery.data?.items ?? []}
                getRowKey={(row) => row.trade_no}
                emptyContent={
                  <EmptyState className='min-h-32' title={t('No orders')} />
                }
                columns={[
                  {
                    id: 'time',
                    header: t('Time'),
                    cell: (row) => formatTimestamp(row.create_time),
                  },
                  {
                    id: 'user',
                    header: t('User'),
                    cell: (row) => `${row.username || '-'} (#${row.user_id})`,
                  },
                  {
                    id: 'method',
                    header: t('Payment method'),
                    cell: (row) => (
                      <div className='flex flex-col'>
                        <span>{t(methodLabel(row.payment_method))}</span>
                        {row.channel_name && (
                          <span className='text-muted-foreground text-xs'>
                            {row.channel_name}
                          </span>
                        )}
                      </div>
                    ),
                  },
                  {
                    id: 'face',
                    header: t('Amount'),
                    cell: (row) =>
                      formatUsd(
                        Number(row.has_ext ? row.face_usd : row.amount),
                        locale
                      ),
                  },
                  {
                    id: 'bonus',
                    header: t('Bonus'),
                    cell: (row) => formatUsd(Number(row.bonus_usd), locale),
                  },
                  {
                    id: 'pay',
                    header: t('Payable / paid'),
                    cell: (row) => (
                      <div className='flex flex-col text-xs'>
                        <span>
                          {row.has_ext
                            ? `${row.pay_amount} ${row.pay_currency}`
                            : row.money}
                        </span>
                        {Number(row.paid_amount) > 0 && (
                          <span className='text-muted-foreground'>
                            {row.paid_amount} {row.paid_currency}
                          </span>
                        )}
                        {row.paid_tx_hash && (
                          <code className='text-muted-foreground truncate font-mono'>
                            {row.paid_tx_hash}
                          </code>
                        )}
                      </div>
                    ),
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    cell: (row) => (
                      <div className='flex flex-col items-start gap-1'>
                        <StatusBadge
                          label={t(
                            ORDER_STATUS_LABELS[
                              row.status as AdtyOrderStatus
                            ] ?? row.status
                          )}
                          variant={
                            ORDER_STATUS_VARIANTS[
                              row.status as AdtyOrderStatus
                            ] ?? 'neutral'
                          }
                          copyable={false}
                        />
                        {row.anomaly && (
                          <StatusBadge
                            label={
                              row.anomaly === 'missing_ext'
                                ? t('Missing order details')
                                : t('Paid amount differs')
                            }
                            variant='danger'
                            copyable={false}
                          />
                        )}
                      </div>
                    ),
                  },
                  {
                    id: 'trade_no',
                    header: t('Order number'),
                    cell: (row) => (
                      <code className='font-mono text-xs'>{row.trade_no}</code>
                    ),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    className: 'text-right',
                    cellClassName: 'text-right',
                    cell: (row) => {
                      const ours =
                        row.payment_provider === 'bepusdt' ||
                        row.payment_provider === 'adty_epay'
                      if (!ours) return null
                      return (
                        <div className='flex justify-end gap-1'>
                          {canComplete &&
                            row.status !== 'success' &&
                            row.status !== 'refunded' && (
                              <Button
                                size='sm'
                                variant='outline'
                                onClick={() => setCompleting(row)}
                              >
                                {t('Complete')}
                              </Button>
                            )}
                          {canRefund && row.status === 'success' && (
                            <Button
                              size='sm'
                              variant='outline'
                              onClick={() => setRefunding(row)}
                            >
                              {t('Refund')}
                            </Button>
                          )}
                        </div>
                      )
                    },
                  },
                ]}
              />
              {pages > 1 && (
                <div className='flex items-center justify-end gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={filters.page <= 1}
                    onClick={() =>
                      setFilters((prev) => ({ ...prev, page: prev.page - 1 }))
                    }
                  >
                    {t('Previous')}
                  </Button>
                  <span className='text-muted-foreground text-sm'>
                    {filters.page} / {pages}
                  </span>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={filters.page >= pages}
                    onClick={() =>
                      setFilters((prev) => ({ ...prev, page: prev.page + 1 }))
                    }
                  >
                    {t('Next')}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          <PaymentOrphanPanel canCredit={canComplete} />
        </div>
        <ManualCompleteDialog
          order={completing}
          onOpenChange={(open) => !open && setCompleting(null)}
          onDone={refresh}
        />
        <RefundDialog
          order={refunding}
          onOpenChange={(open) => !open && setRefunding(null)}
          onDone={refresh}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
