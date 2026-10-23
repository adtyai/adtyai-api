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

import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { createRouteTag, deleteRouteTag, listRouteTags } from '../api'
import { routingErrorText } from '../lib/error-text'

const TAGS = [
  { id: 'excluded', label: 'Do not use this channel' },
  {
    id: 'fallback_only',
    label: 'Use this channel only when nothing else is available',
  },
] as const

function tagLabel(tag: string) {
  return TAGS.find((item) => item.id === tag)?.label ?? tag
}

export function ChannelsPage({
  initialChannelId,
}: {
  initialChannelId?: string
}) {
  const { t } = useTranslation()
  const isRoot = useAuthStore((s) => s.auth.user?.role === ROLE.SUPER_ADMIN)
  const qc = useQueryClient()
  const [channelId, setChannelId] = useState(initialChannelId ?? '')
  const [model, setModel] = useState('*')
  const [note, setNote] = useState('')
  const [tag, setTag] = useState<(typeof TAGS)[number]['id']>('excluded')
  const parsed = Number(channelId)
  const filterId = Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
  const tagsQuery = useQuery({
    queryKey: ['smart-routing', 'tags', filterId ?? 0],
    queryFn: () => listRouteTags(filterId),
  })

  const save = async () => {
    if (!isRoot || !filterId) {
      toast.error(t('Enter a channel ID'))
      return
    }
    const res = await createRouteTag({
      channel_id: filterId,
      model: model.trim() || '*',
      tag,
      note: note.trim(),
    })
    if (!res.success) {
      toast.error(routingErrorText(t, res, t('Save failed')))
      return
    }
    setNote('')
    toast.success(t('Saved successfully'))
    await qc.invalidateQueries({ queryKey: ['smart-routing', 'tags'] })
  }

  const remove = async (id: number) => {
    if (!isRoot) return
    const res = await deleteRouteTag(id)
    if (!res.success) {
      toast.error(routingErrorText(t, res, t('Save failed')))
      return
    }
    await qc.invalidateQueries({ queryKey: ['smart-routing', 'tags'] })
  }

  return (
    <div className='flex flex-col gap-4 overflow-auto pb-4'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Channel usage')}</CardTitle>
          <CardDescription>
            {t(
              'Mark a channel as unused, or as a last resort. Other routing rules stay in the backend.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='grid gap-3 sm:grid-cols-2'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='route-channel'>{t('Channel ID')}</Label>
            <Input
              id='route-channel'
              value={channelId}
              onChange={(e) => setChannelId(e.target.value)}
              inputMode='numeric'
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='route-model'>{t('Model')}</Label>
            <Input
              id='route-model'
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder='*'
            />
          </div>
          <div className='flex flex-col gap-1.5 sm:col-span-2'>
            <Label htmlFor='route-note'>{t('Reason')}</Label>
            <Input
              id='route-note'
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <div className='flex flex-wrap gap-2 sm:col-span-2'>
            {TAGS.map((item) => (
              <Button
                key={item.id}
                type='button'
                size='sm'
                variant={tag === item.id ? 'default' : 'outline'}
                onClick={() => setTag(item.id)}
              >
                {t(item.label)}
              </Button>
            ))}
            <Button
              type='button'
              size='sm'
              disabled={!isRoot}
              onClick={() => void save()}
            >
              {t('Save')}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('Current marks')}</CardTitle>
        </CardHeader>
        <CardContent>
          {tagsQuery.isError ? (
            <p className='text-destructive text-sm'>{t('Failed to load')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel ID')}</TableHead>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('Mark')}</TableHead>
                  <TableHead>{t('Reason')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {(tagsQuery.data ?? []).length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5}>{t('No data yet')}</TableCell>
                  </TableRow>
                ) : (
                  (tagsQuery.data ?? []).map((row) => (
                    <TableRow key={row.id}>
                      <TableCell>{row.channel_id}</TableCell>
                      <TableCell>{row.model}</TableCell>
                      <TableCell>{t(tagLabel(row.tag))}</TableCell>
                      <TableCell>{row.note || '—'}</TableCell>
                      <TableCell>
                        {isRoot &&
                        (row.tag === 'excluded' ||
                          row.tag === 'fallback_only') ? (
                          <Button
                            type='button'
                            size='sm'
                            variant='ghost'
                            onClick={() => void remove(row.id)}
                          >
                            {t('Remove')}
                          </Button>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
