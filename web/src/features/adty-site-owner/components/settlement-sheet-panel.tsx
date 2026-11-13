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
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { formatUsd } from '@/features/wallet/adty/lib/money'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestamp } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  getOwnerSettlementSheets,
  siteOwnerKeys,
  type SettlementSheet,
} from '../api'
import { isNegativeAmount } from '../lib/site-owner'

type Line = {
  id: string
  label: string
  value: string
  raw?: string
}

export function settlementSheetLines(
  t: TFunction,
  sheet: SettlementSheet,
  locale: string | undefined
): Line[] {
  const money = (value: string) => formatUsd(Number(value || 0), locale)
  const usd = (id: string, label: string, raw: string): Line => ({
    id,
    label,
    value: money(raw),
    raw,
  })
  const lines: Line[] = [
    { id: 'day', label: t('Date'), value: sheet.business_day },
    { id: 'zone', label: t('Timezone'), value: sheet.timezone },
    { id: 'start', label: t('Day start'), value: formatTimestamp(sheet.day_start) },
    { id: 'end', label: t('Day end'), value: formatTimestamp(sheet.day_end) },
    usd('topup', t('Top-up'), sheet.topup_face_usd),
    usd('paid', t('Paid in USD'), sheet.paid_usd),
  ]
  let paid: Record<string, string> = {}
  try {
    paid = JSON.parse(sheet.paid_by_currency || '{}') as Record<string, string>
  } catch {
    paid = {}
  }
  for (const [currency, amount] of Object.entries(paid)) {
    lines.push({
      id: `paid-${currency || 'other'}`,
      label: t('Other paid amounts'),
      value: `${amount} ${currency}`.trim(),
    })
  }
  lines.push(
    usd('consumption', t('Customer consumption'), sheet.consumption_usd),
    usd('cost', t('Supply cost'), sheet.supply_cost_usd),
    usd('spread', t('Settlement spread'), sheet.spread_usd),
    usd('reserve', t('Gift reserve'), sheet.gift_reserve_usd),
    usd('funding', t('Gift funding'), sheet.gift_funding_usd),
    usd('release', t('Gift release'), sheet.gift_release_usd),
    usd('trial', t('Trial expense'), sheet.trial_expense_usd),
    usd('commission', t('Commission'), sheet.commission_usd),
    usd('pending', t('Pending coverage'), sheet.pending_coverage_usd),
    usd('margin', t('Margin change'), sheet.margin_delta_usd),
    usd('earnings', t('Earnings change'), sheet.earnings_delta_usd),
    usd('available', t('Available change'), sheet.earnings_available_delta_usd),
    usd('frozen', t('Frozen change'), sheet.earnings_frozen_delta_usd),
    usd('withdrawal', t('Withdrawal amount'), sheet.withdrawal_usd),
    usd('fee', t('Withdrawal fee'), sheet.withdrawal_fee_usd),
    usd('adjustment', t('Financial adjustment'), sheet.adjustment_usd),
    {
      id: 'anomalies',
      label: t('Anomalies'),
      value: String(sheet.anomaly_count),
    },
    { id: 'batch', label: t('Batch'), value: sheet.batch_id },
    {
      id: 'watermark',
      label: t('Event watermark'),
      value: String(sheet.event_watermark),
    }
  )
  return lines
}

export function SettlementSheetLines({ sheet }: { sheet: SettlementSheet }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const lines = settlementSheetLines(t, sheet, locale)
  return (
    <dl className='grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2 text-sm'>
      {lines.map((line) => (
        <div key={line.id} className='contents'>
          <dt className='text-muted-foreground'>{line.label}</dt>
          <dd
            className={cn(
              'text-right tabular-nums',
              isNegativeAmount(line.raw) && 'text-destructive'
            )}
            data-negative={isNegativeAmount(line.raw) ? 'true' : undefined}
          >
            {line.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

/** Previous-day check sheet. It lists recorded lines and does not credit earnings. */
export function SettlementSheetPanel() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: siteOwnerKeys.list('settlement-sheets', {}),
    queryFn: getOwnerSettlementSheets,
  })
  const sheet = query.data?.[0]

  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Settlement sheet')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t(
          'This sheet checks the previous business day. It does not add earnings.'
        )}
      </p>
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isSuccess && !sheet && (
        <EmptyState className='min-h-24' title={t('No settlement sheet yet')} />
      )}
      {sheet && (
        <>
          <SettlementSheetLines sheet={sheet} />
          <p className='text-muted-foreground text-sm'>
            {t(
              'Settlement spread is the recorded usage difference. It is not earnings after gifts or commissions.'
            )}
          </p>
        </>
      )}
    </section>
  )
}
