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

import type { WalletLotProjection } from '../types'

export type { WalletLotProjection }

/** Renders the official USD string from WalletUSDPartsFromQuota without a second rounding rule. */
export function formatOfficialUsd(amount: string): string {
  return `$${amount}`
}

export function WalletLotProjectionLines(props: {
  projection?: WalletLotProjection | null
  compact?: boolean
  showNote?: boolean
}) {
  const { t } = useTranslation()
  if (!props.projection) return null
  const principal = formatOfficialUsd(props.projection.principal_usd)
  const bonus = formatOfficialUsd(props.projection.bonus_usd)
  if (props.compact) {
    return (
      <div className='text-muted-foreground text-xs'>
        <div>
          {t('Principal')}: <span className='tabular-nums'>{principal}</span>
        </div>
        <div>
          {t('Permanent bonus')}: <span className='tabular-nums'>{bonus}</span>
        </div>
      </div>
    )
  }
  return (
    <div className='text-muted-foreground mt-1 space-y-0.5 text-xs'>
      <div>
        {t('Remaining principal')}:{' '}
        <span className='text-foreground tabular-nums'>{principal}</span>
      </div>
      <div>
        {t('Remaining permanent bonus')}:{' '}
        <span className='text-foreground tabular-nums'>{bonus}</span>
      </div>
      {props.showNote !== false && (
        <div className='text-muted-foreground/70'>
          {t(
            'Lot remaining is a projection of funding lots, not a second wallet.'
          )}
        </div>
      )}
    </div>
  )
}
