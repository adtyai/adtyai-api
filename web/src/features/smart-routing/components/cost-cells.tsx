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

type RatioRateCellProps = {
  missing?: boolean
  value?: number | null
}

export function RatioRateCell(props: RatioRateCellProps) {
  const { t } = useTranslation()
  if (props.missing) {
    return (
      <span className='text-amber-600 dark:text-amber-400'>
        {t('Fill rate & multiplier')}
      </span>
    )
  }
  if (props.value != null) {
    return <>{props.value.toFixed(4)}</>
  }
  return <>—</>
}

type LiveEstimateCellProps = {
  value?: number | null
}

export function LiveEstimateCell(props: LiveEstimateCellProps) {
  const { t } = useTranslation()
  if (props.value == null) {
    return <>—</>
  }
  return (
    <>
      {props.value.toFixed(4)}
      <span className='text-muted-foreground ml-1'>
        ({t('local ledger, not upstream invoice')})
      </span>
    </>
  )
}
