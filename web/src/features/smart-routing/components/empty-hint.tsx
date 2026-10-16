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

import { cn } from '@/lib/utils'

type EmptyHintProps = {
  title?: string
  description?: string
  className?: string
}

export function EmptyHint(props: EmptyHintProps) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'text-muted-foreground flex flex-col items-center justify-center gap-1 rounded-lg border border-dashed p-8 text-center text-sm',
        props.className
      )}
    >
      <p className='font-medium text-foreground'>
        {props.title ?? t('No data yet')}
      </p>
      <p>
        {props.description ??
          t(
            'Backend APIs may still be unavailable. The page stays usable; data will appear once scoring is online.'
          )}
      </p>
    </div>
  )
}
