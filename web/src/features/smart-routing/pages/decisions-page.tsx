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
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { EmptyHint } from '../components/empty-hint'
import { useDecisions } from '../hooks/use-decisions'

export function DecisionsPage() {
  const { t } = useTranslation()
  const [requestId, setRequestId] = useState('')
  const [logId, setLogId] = useState('')
  const [submitted, setSubmitted] = useState<{
    request_id?: string
    log_id?: string
  }>({})

  const decisionsQuery = useDecisions({
    request_id: submitted.request_id,
    log_id: submitted.log_id,
  })

  const rows = decisionsQuery.data ?? []

  return (
    <div className='flex h-full min-h-0 flex-col gap-4 overflow-auto pb-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='sr-req'>{t('Request ID')}</Label>
          <Input
            id='sr-req'
            className='max-w-sm font-mono text-xs'
            value={requestId}
            onChange={(e) => setRequestId(e.target.value)}
            placeholder='request_id'
          />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='sr-log'>{t('Log ID')}</Label>
          <Input
            id='sr-log'
            className='max-w-xs font-mono text-xs'
            value={logId}
            onChange={(e) => setLogId(e.target.value)}
            placeholder='log_id'
          />
        </div>
        <Button
          onClick={() =>
            setSubmitted({
              request_id: requestId.trim() || undefined,
              log_id: logId.trim() || undefined,
            })
          }
        >
          {t('Replay')}
        </Button>
        <Button
          variant='outline'
          onClick={() => {
            setRequestId('')
            setLogId('')
            setSubmitted({})
          }}
        >
          {t('Recent list')}
        </Button>
      </div>

      <p className='text-muted-foreground text-xs'>
        {t(
          'Read-only replay of logs.other.route. Does not re-hit upstream. Shows candidates, tried, exclude reasons, half-open flags.'
        )}
      </p>

      {rows.length === 0 ? (
        <EmptyHint
          title={t('No routing decisions')}
          description={t(
            'Search by request_id / log_id, or wait for /decisions once the backend ships.'
          )}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Request ID')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Chosen channel')}</TableHead>
              <TableHead>{t('Pinned')}</TableHead>
              <TableHead>{t('Tried')}</TableHead>
              <TableHead>{t('Exclude reasons')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row, i) => (
              <TableRow key={row.request_id ?? row.log_id ?? i}>
                <TableCell className='text-xs'>{row.at || '—'}</TableCell>
                <TableCell className='font-mono text-xs'>
                  {row.request_id || row.log_id || '—'}
                </TableCell>
                <TableCell className='font-mono text-xs'>
                  {row.model || '—'}
                </TableCell>
                <TableCell>
                  {row.channel_name ||
                    (row.channel_id != null ? `#${row.channel_id}` : '—')}
                  {row.half_open ? (
                    <span className='text-muted-foreground ml-1 text-xs'>
                      ({t('half-open')})
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>{row.pinned ? t('Yes') : t('No')}</TableCell>
                <TableCell className='font-mono text-xs'>
                  {(row.tried ?? []).join(', ') || '—'}
                </TableCell>
                <TableCell className='text-xs'>
                  {(row.exclude_reasons ?? []).join('; ') || '—'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
