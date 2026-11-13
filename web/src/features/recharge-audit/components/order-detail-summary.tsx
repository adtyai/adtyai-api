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
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestamp } from '@/lib/format'

import {
  officialCommissionStateLabel,
} from '@/features/wallet/adty/commission-states'

import type { OrderTrace } from '../api'
import { ORDER_TRACE_LABELS } from '../trace-labels'

export function OrderDetailSummary(props: { detail: OrderTrace }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const order = props.detail.order
  return (
    <>
      <dl className='grid grid-cols-1 gap-3 text-sm break-words sm:grid-cols-3'>
        {[
          [t('User'), `${order.username || '-'} (#${order.user_id})`],
          [
            t('Site'),
            `${order.site_name || t('Unknown site')} · ${order.site_id ?? '-'}`,
          ],
          [t('Status'), t(ORDER_TRACE_LABELS[order.status] ?? order.status)],
          [t('Provider'), order.payment_provider],
          [
            t('Payment scene'),
            order.payment_scene
              ? `${order.payment_scene_name || order.payment_scene} (${order.payment_scene}) · v${order.payment_scene_version}`
              : '-',
          ],
          [
            t('Payable / paid'),
            order.has_ext
              ? `${order.pay_amount} ${order.pay_currency} / ${order.paid_amount} ${order.paid_currency}`
              : t('Missing order details'),
          ],
          [
            t('Order bonus snapshot'),
            order.has_ext ? `${order.bonus_usd} USD` : '-',
          ],
          [
            t('Promo bonus'),
            order.has_ext ? `${props.detail.promo_bonus_usd ?? '0'} USD` : '-',
          ],
          [t('Created at'), formatTimestamp(order.create_time)],
          [
            t('Completed at'),
            order.complete_time > 0
              ? formatTimestamp(order.complete_time)
              : '-',
          ],
        ].map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground'>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      {order.paid_tx_hash && (
        <p className='text-sm break-all'>
          {t('Payment proof')}: {order.paid_tx_hash}
        </p>
      )}
      {props.detail.refund.refunded_at > 0 && (
        <div className='rounded-md border p-3 text-sm'>
          <p>
            {t('Refund')}: {props.detail.refund.refund_amount}{' '}
            {props.detail.refund.currency} ·{' '}
            {formatTimestamp(props.detail.refund.refunded_at)}
          </p>
          <p>
            {t('Refund principal quota')}:{' '}
            {formatNumber(props.detail.refund.refund_quota, locale)}
          </p>
          {props.detail.refund.reason && (
            <p className='break-words'>{props.detail.refund.reason}</p>
          )}
          {props.detail.refund.commission?.issued_usd && (
            <p className='tabular-nums'>
              {t('Issued commission')}: {props.detail.refund.commission.issued_usd} USD
            </p>
          )}
          {props.detail.refund.commission?.states?.map((item) => (
            <p key={item.state} className='tabular-nums'>
              {t(officialCommissionStateLabel(item.state))}: {item.amount_usd} USD
            </p>
          ))}
          {positiveUsd(
            props.detail.refund.commission?.recovered_usd,
            props.detail.refund.commission_recovered_usd
          ) && (
            <p className='tabular-nums'>
              {t('Returned to site wallets')}:{' '}
              {positiveUsd(
                props.detail.refund.commission?.recovered_usd,
                props.detail.refund.commission_recovered_usd
              )}{' '}
              USD
            </p>
          )}
          {positiveUsd(props.detail.refund.commission?.converted_clawed_usd) && (
            <p className='tabular-nums'>
              {t('Clawback from balance')}:{' '}
              {props.detail.refund.commission?.converted_clawed_usd} USD
            </p>
          )}
          {positiveUsd(props.detail.refund.commission_recovery_usd) && (
            <p className='tabular-nums'>
              {t('Recovery liability')}: {props.detail.refund.commission_recovery_usd} USD
            </p>
          )}
          {!props.detail.refund.commission &&
            positiveUsd(props.detail.refund.commission_clawed_usd) && (
              <p className='tabular-nums'>
                {t('Commission clawback')}: {props.detail.refund.commission_clawed_usd} USD
              </p>
            )}
          {!props.detail.refund.commission &&
            positiveUsd(props.detail.refund.commission_withdrawn_usd) && (
              <p className='tabular-nums'>
                {t('Withdrawn commission')}: {props.detail.refund.commission_withdrawn_usd} USD
              </p>
            )}
          <p className='text-muted-foreground'>
            {t(
              'This records the wallet refund. Return the payment offline through the original payment provider.'
            )}
          </p>
        </div>
      )}
    </>
  )
}

function positiveUsd(...values: (string | undefined)[]) {
  for (const value of values) {
    if (value && value !== '0') return value
  }
  return undefined
}

