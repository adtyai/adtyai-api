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

import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getOrderTrace, type OrderTraceSection } from '../api'
import { ORDER_TRACE_SECTIONS } from '../trace-labels'
import { OrderDetailSummary } from './order-detail-summary'
import { OrderTraceTable } from './order-trace-table'

export function OrderDetailDialog(props: {
  tradeNo: string
  userId: number
  ownerMode?: boolean
  ownerSiteId?: number
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [section, setSection] = useState<OrderTraceSection>('payments')
  const [page, setPage] = useState(1)
  const query = useQuery({
    queryKey: [
      'recharge-audit',
      props.userId,
      props.ownerMode === true,
      props.ownerSiteId,
      'detail',
      props.tradeNo,
      section,
      page,
    ],
    queryFn: () => getOrderTrace(props.tradeNo, section, page, props.ownerMode),
  })
  const detail = query.data
  const pages = Math.max(1, Math.ceil((detail?.total ?? 0) / 20))
  const sections = (
    Object.keys(ORDER_TRACE_SECTIONS) as OrderTraceSection[]
  ).filter(
    (value) =>
      !props.ownerMode ||
      (value !== 'commissions' && value !== 'commission_movements')
  )
  return (
    <Dialog
      open
      title={t('Order details')}
      description={props.tradeNo}
      onOpenChange={(open) => !open && props.onClose()}
      contentClassName='sm:max-w-5xl'
    >
      <div className='flex min-w-0 flex-col gap-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Earlier payment attempts may not have been recorded. The order snapshot is not a complete callback history.'
          )}
        </p>
        {query.isPending && <LoadingState />}
        {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
        {detail && <OrderDetailSummary detail={detail} />}
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <NativeSelect
            aria-label={t('Trace records')}
            value={section}
            onChange={(event) => {
              setSection(event.target.value as OrderTraceSection)
              setPage(1)
            }}
          >
            {sections.map((value) => (
              <option key={value} value={value}>
                {t(ORDER_TRACE_SECTIONS[value])}
              </option>
            ))}
          </NativeSelect>
          <Button
            size='sm'
            variant='outline'
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {t('Refresh')}
          </Button>
        </div>
        {detail && (
          <>
            <OrderTraceTable entries={detail.items} />
            <div className='flex items-center justify-end gap-2'>
              {pages > 1 && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={page <= 1 || query.isFetching}
                  onClick={() => setPage(page - 1)}
                >
                  {t('Previous')}
                </Button>
              )}
              <span className='text-sm'>
                {page} / {pages} · {formatNumber(detail.total, locale)}
              </span>
              {pages > 1 && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={page >= pages || query.isFetching}
                  onClick={() => setPage(page + 1)}
                >
                  {t('Next')}
                </Button>
              )}
            </div>
          </>
        )}
      </div>
    </Dialog>
  )
}
