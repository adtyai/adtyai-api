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

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import { formatHalfOpenLabel, formatInflightHint } from '../lib/labels'
import type { HalfOpenStage } from '../types'

type HalfOpenBadgeProps = {
  stage?: HalfOpenStage | string | null
  successCount?: number | null
  inflight?: number | null
  cooldownRemainSec?: number | null
  showInflight?: boolean
  className?: string
}

export function HalfOpenBadge(props: HalfOpenBadgeProps) {
  const { t } = useTranslation()
  const key = formatHalfOpenLabel({
    stage: props.stage,
    successCount: props.successCount,
    inflight: props.inflight,
    cooldownRemainSec: props.cooldownRemainSec,
  })
  // Keys that are English source strings used as i18n keys
  const label = t(key)

  const stage = props.stage
  return (
    <span className={cn('inline-flex items-center gap-1', props.className)}>
      <Badge
        variant='outline'
        className={cn(
          stage === 'probe_2' && 'border-sky-500/40 text-sky-700 dark:text-sky-400',
          stage === 'ramp_10' && 'border-violet-500/40 text-violet-700 dark:text-violet-400',
          stage === 'full' && 'border-emerald-500/40 text-emerald-700 dark:text-emerald-400',
          stage === 'open' && 'border-red-500/40 text-red-700 dark:text-red-400'
        )}
      >
        {label}
      </Badge>
      {props.showInflight &&
      (stage === 'probe_2' || stage === 'ramp_10') ? (
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('In-flight')} {formatInflightHint(props.inflight)}
        </span>
      ) : null}
    </span>
  )
}
