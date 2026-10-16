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

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { clearFakeModel } from '../api'
import { LiveEstimateCell, RatioRateCell } from '../components/cost-cells'
import { EmptyHint } from '../components/empty-hint'
import { HalfOpenBadge } from '../components/half-open-badge'
import { ScoreBadge } from '../components/score-badge'
import {
  useBreakerEvents,
  useBreakerStages,
  useScores,
} from '../hooks/use-scores'

type ScoresPageProps = {
  initialChannelId?: string
}

export function ScoresPage(props: ScoresPageProps) {
  const { t } = useTranslation()
  const isRoot = useAuthStore(
    (s) => s.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const [model, setModel] = useState('')
  const scoresQuery = useScores(model.trim() || undefined)
  const stagesQuery = useBreakerStages()
  const eventsQuery = useBreakerEvents()

  const stageByChannel = new Map(
    (stagesQuery.data ?? []).map((s) => [s.channel_id, s])
  )

  const rows = scoresQuery.data ?? []
  const filtered =
    props.initialChannelId != null && props.initialChannelId !== ''
      ? rows.filter((r) => String(r.channel_id) === props.initialChannelId)
      : rows

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pb-4'>
      <div className='flex flex-wrap items-end gap-2'>
        <div className='flex flex-col gap-1'>
          <span className='text-muted-foreground text-xs'>{t('Filter by model')}</span>
          <Input
            className='max-w-xs'
            placeholder={t('Model name (optional)')}
            value={model}
            onChange={(e) => setModel(e.target.value)}
          />
        </div>
        <Button
          variant='outline'
          onClick={() => {
            void scoresQuery.refetch()
            void stagesQuery.refetch()
          }}
        >
          {t('Refresh')}
        </Button>
      </div>

      {filtered.length === 0 ? (
        <EmptyHint
          title={t('No score rows')}
          description={t(
            'Scores appear after real traffic. Columns stay ready for backend /scores.'
          )}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Overall score')}</TableHead>
              <TableHead>{t('Half-open')}</TableHead>
              <TableHead>{t('Multiplier × rate')}</TableHead>
              <TableHead>{t('Last-30-min estimate')}</TableHead>
              <TableHead>{t('Share')}</TableHead>
              <TableHead>{t('Samples')}</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((row) => {
              const stage = stageByChannel.get(row.channel_id)
              return (
                <TableRow key={`${row.channel_id}-${row.model ?? ''}`}>
                  <TableCell>
                    {row.channel_name || `#${row.channel_id}`}
                    {row.layer === 'official' ? (
                      <span className='text-muted-foreground ml-1 text-xs'>
                        ({t('Official')})
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {row.model || '—'}
                  </TableCell>
                  <TableCell>
                    <ScoreBadge
                      score={row.score}
                      dataInsufficient={row.data_insufficient}
                      noData={row.no_data}
                      isNew={row.is_new}
                    />
                  </TableCell>
                  <TableCell>
                    <HalfOpenBadge
                      stage={stage?.stage ?? row.half_open_stage}
                      successCount={
                        stage?.success_count ?? row.half_open_success
                      }
                      inflight={stage?.inflight ?? row.half_open_inflight}
                      cooldownRemainSec={
                        stage?.cooldown_remain_sec ?? row.cooldown_remain_sec
                      }
                      showInflight
                    />
                  </TableCell>
                  <TableCell className='tabular-nums text-xs'>
                    <RatioRateCell
                      missing={row.missing_cost}
                      value={row.cost_ratio_rate}
                    />
                  </TableCell>
                  <TableCell className='tabular-nums text-xs'>
                    <LiveEstimateCell value={row.cost_live_30m} />
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {row.share_pct != null
                      ? `${row.share_pct.toFixed(1)}%`
                      : '—'}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    {row.sample_30m ?? '—'}
                  </TableCell>
                  <TableCell>
                    {row.fake_model && isRoot ? (
                      <Button
                        size='sm'
                        variant='outline'
                        onClick={() => {
                          void clearFakeModel(row.channel_id).then((res) => {
                            if (!res.success) {
                              toast.error(res.message || t('Failed'))
                              return
                            }
                            toast.success(t('Fake-model flag cleared'))
                            void scoresQuery.refetch()
                          })
                        }}
                      >
                        {t('Clear fake-model')}
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      <div>
        <h3 className='mb-2 text-sm font-medium'>{t('Breaker timeline')}</h3>
        {(eventsQuery.data ?? []).length === 0 ? (
          <EmptyHint title={t('No breaker events')} />
        ) : (
          <ul className='space-y-1 text-sm'>
            {(eventsQuery.data ?? []).slice(0, 50).map((ev, i) => (
              <li
                key={ev.id ?? `${ev.channel_id}-${ev.at}-${i}`}
                className='text-muted-foreground'
              >
                <span className='text-foreground'>{ev.at}</span>
                {' · '}
                {ev.channel_name || `#${ev.channel_id}`}
                {' · '}
                {ev.event}
                {ev.detail ? ` — ${ev.detail}` : ''}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
