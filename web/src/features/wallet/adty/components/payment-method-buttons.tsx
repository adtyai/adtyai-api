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

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { methodLabel, paymentMethodKey, paymentMethodLabels } from '../lib/methods'
import type { AdtyMethod } from '../types'

type PaymentMethodButtonsProps = {
  methods: AdtyMethod[]
  /** paymentMethodKey of the chosen button, or null. */
  selected: string | null
  onSelect: (key: string) => void
  disabled?: boolean
}

/** One button per payment method: each USDT chain and each Epay channel. */
export function PaymentMethodButtons(props: PaymentMethodButtonsProps) {
  const { t } = useTranslation()
  const labels = paymentMethodLabels(
    props.methods.map((item) => item.label || t(methodLabel(item.id)))
  )
  return (
    <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
      {props.methods.map((item, index) => {
        const key = paymentMethodKey(item)
        return (
          <Button
            key={key}
            type='button'
            variant='outline'
            disabled={props.disabled || !item.available}
            aria-pressed={props.selected === key}
            className={cn(
              'h-auto min-h-12 flex-col items-start px-3 py-2 text-left whitespace-normal',
              props.selected === key
                ? 'border-foreground bg-foreground/5'
                : 'border-muted'
            )}
            onClick={() => props.onSelect(key)}
          >
            <span className='text-sm font-medium'>{labels[index]}</span>
            {!item.available && (
              <span className='text-muted-foreground text-xs'>
                {t('Unavailable')}
              </span>
            )}
          </Button>
        )
      })}
    </div>
  )
}
