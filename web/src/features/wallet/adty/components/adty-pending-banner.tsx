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
import { Clock } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import { methodLabel } from '../lib/methods'
import type { AdtyOrder } from '../types'

type AdtyPendingBannerProps = {
  order: AdtyOrder | null | undefined
  onResume: (order: AdtyOrder) => void
}

/** Shown while an unpaid order is still open; reopens its payment dialog. */
export function AdtyPendingBanner(props: AdtyPendingBannerProps) {
  const { t } = useTranslation()
  const order = props.order
  if (!order?.ext) return null
  return (
    <Alert>
      <Clock aria-hidden='true' />
      <AlertTitle>{t('You have an order waiting for payment')}</AlertTitle>
      <AlertDescription className='flex flex-wrap items-center justify-between gap-3'>
        <span>
          {t('{{method}}, ${{amount}}, order {{tradeNo}}', {
            method: t(methodLabel(order.payment_method)),
            amount: order.ext.face_usd,
            tradeNo: order.trade_no,
          })}
        </span>
        <Button size='sm' onClick={() => props.onResume(order)}>
          {t('Continue payment')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
