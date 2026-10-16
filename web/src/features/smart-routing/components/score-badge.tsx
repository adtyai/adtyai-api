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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { scoreTone } from '../lib/labels'

type ScoreBadgeProps = {
  score: number | null | undefined
  channelId?: number
  dataInsufficient?: boolean
  noData?: boolean
  isNew?: boolean
  className?: string
  linkToDetail?: boolean
}

export function ScoreBadge(props: ScoreBadgeProps) {
  const { t } = useTranslation()
  const tone = scoreTone(props.score)

  let label: string
  if (props.noData) {
    label = t('No data')
  } else if (props.dataInsufficient) {
    label = t('Insufficient data')
  } else if (props.score == null) {
    label = '—'
  } else {
    label = String(Math.round(props.score))
  }

  const badge = (
    <Badge
      variant='outline'
      className={cn(
        'tabular-nums',
        tone === 'green' &&
          'border-emerald-500/40 text-emerald-700 dark:text-emerald-400',
        tone === 'yellow' &&
          'border-amber-500/40 text-amber-700 dark:text-amber-400',
        tone === 'red' && 'border-red-500/40 text-red-700 dark:text-red-400',
        tone === 'muted' && 'text-muted-foreground',
        props.className
      )}
    >
      {label}
      {props.isNew ? ` · ${t('New channel')}` : null}
    </Badge>
  )

  if (props.linkToDetail && props.channelId != null) {
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <Link
              to='/smart-routing/$section'
              params={{ section: 'scores' }}
              search={{ channel: String(props.channelId) }}
              className='inline-flex'
            >
              {badge}
            </Link>
          }
        />
        <TooltipContent>
          <p>{t('Open smart routing detail')}</p>
        </TooltipContent>
      </Tooltip>
    )
  }

  return badge
}
