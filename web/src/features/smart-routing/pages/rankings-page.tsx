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

import { EmptyHint } from '../components/empty-hint'
import { HalfOpenBadge } from '../components/half-open-badge'
import { ScoreBadge } from '../components/score-badge'
import { useRankings } from '../hooks/use-scores'

export function RankingsPage() {
  const { t } = useTranslation()
  const [model, setModel] = useState('')
  const rankingsQuery = useRankings(model.trim() || undefined)
  const rows = [...(rankingsQuery.data ?? [])].sort((a, b) => {
    const sa = a.score ?? -1
    const sb = b.score ?? -1
    return sb - sa
  })

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pb-4'>
      <div className='flex flex-wrap items-end gap-2'>
        <Input
          className='max-w-xs'
          placeholder={t('Model name')}
          value={model}
          onChange={(e) => setModel(e.target.value)}
        />
        <Button variant='outline' onClick={() => void rankingsQuery.refetch()}>
          {t('Refresh')}
        </Button>
      </div>

      {rows.length === 0 ? (
        <EmptyHint
          title={t('No rankings')}
          description={t('Pick a model after scoring has data.')}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>#</TableHead>
              <TableHead>{t('Channel')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Overall score')}</TableHead>
              <TableHead>{t('Connectivity')}</TableHead>
              <TableHead>{t('Price')}</TableHead>
              <TableHead>{t('Speed')}</TableHead>
              <TableHead>{t('Stability')}</TableHead>
              <TableHead>{t('Authenticity')}</TableHead>
              <TableHead>{t('Share')}</TableHead>
              <TableHead>{t('Half-open')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, idx) => (
              <TableRow key={`${row.channel_id}-${row.model ?? ''}`}>
                <TableCell className='tabular-nums'>{idx + 1}</TableCell>
                <TableCell>
                  {row.channel_name || `#${row.channel_id}`}
                </TableCell>
                <TableCell className='font-mono text-xs'>
                  {row.model || '—'}
                </TableCell>
                <TableCell>
                  <ScoreBadge score={row.score} />
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.connectivity ?? '—'}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.price ?? '—'}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.speed ?? '—'}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.stability ?? '—'}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.authenticity ?? '—'}
                </TableCell>
                <TableCell className='tabular-nums'>
                  {row.share_pct != null
                    ? `${row.share_pct.toFixed(1)}%`
                    : '—'}
                </TableCell>
                <TableCell>
                  <HalfOpenBadge stage={row.half_open_stage} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
