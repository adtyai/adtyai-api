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

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type TeamMine = {
  team_id: number
  name: string
  role: string
  discount: string
  shared_quota: number
  status: string
}

export function TeamPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mine = useQuery({
    queryKey: ['adty-team', 'mine'],
    queryFn: async () => {
      const response = await api.get('/api/adty/teams/mine')
      const body = response.data as { success?: boolean; data?: { team?: TeamMine | null } }
      if (!body.success) throw new Error('Failed to load data')
      return body.data?.team ?? null
    },
  })
  const [name, setName] = useState('')
  const [discount, setDiscount] = useState('1')
  const [invite, setInvite] = useState('')
  const [issued, setIssued] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['adty-team', 'mine'] })
  const team = mine.data

  const create = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post('/api/adty/teams', { name: name.trim(), discount })
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) {
        setError(t(body.message || 'Failed to save'))
        return
      }
      setName('')
      refresh()
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  const issueInvite = async () => {
    if (!team) return
    setPending(true)
    setError('')
    try {
      const response = await api.post(`/api/adty/teams/${team.team_id}/invites`)
      const body = response.data as { success?: boolean; message?: string; data?: { code?: string } }
      if (!body.success || !body.data?.code) {
        setError(t(body.message || 'Failed to save'))
        return
      }
      setIssued(body.data.code)
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Failed to save')))
    } finally {
      setPending(false)
    }
  }

  const accept = async () => {
    setPending(true)
    setError('')
    try {
      const response = await api.post('/api/adty/teams/accept', { code: invite.trim() })
      const body = response.data as { success?: boolean; message?: string }
      if (!body.success) {
        setError(t(body.message || 'Invalid parameters'))
        return
      }
      setInvite('')
      refresh()
    } catch (cause) {
      setError(getServerErrorMessage(cause, t('Invalid parameters')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardContent className='flex flex-col gap-4 py-4'>
        <div className='flex flex-col gap-1'>
          <h2 className='text-sm font-medium'>{t('Team')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('A team key spends the shared balance. The discount applies only to that key.')}
          </p>
        </div>
        {team && (
          <dl className='grid grid-cols-2 gap-3 text-sm'>
            <div className='flex flex-col gap-1'>
              <dt className='text-muted-foreground'>{t('Name')}</dt>
              <dd>{team.name}</dd>
            </div>
            <div className='flex flex-col gap-1'>
              <dt className='text-muted-foreground'>{t('Shared quota')}</dt>
              <dd className='tabular-nums'>{team.shared_quota}</dd>
            </div>
            <div className='flex flex-col gap-1'>
              <dt className='text-muted-foreground'>{t('Discount')}</dt>
              <dd className='tabular-nums'>{team.discount}</dd>
            </div>
            <div className='flex flex-col gap-1'>
              <dt className='text-muted-foreground'>{t('Role')}</dt>
              <dd>{team.role === 'leader' ? t('Leader') : t('Member')}</dd>
            </div>
          </dl>
        )}
        {team?.role === 'leader' && (
          <div className='flex flex-col gap-2'>
            <Button type='button' variant='outline' disabled={pending} onClick={() => void issueInvite()}>
              {t('Create invite')}
            </Button>
            {issued && <p className='text-sm break-all'>{t('Invite code')}: {issued}</p>}
          </div>
        )}
        {!team && (
          <div className='flex flex-col gap-3'>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-name'>{t('Team name')}</Label>
              <Input id='team-name' value={name} onChange={(event) => setName(event.target.value)} />
            </div>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-discount'>{t('Discount')}</Label>
              <Input id='team-discount' value={discount} inputMode='decimal' onChange={(event) => setDiscount(event.target.value)} />
            </div>
            <Button type='button' disabled={pending || name.trim() === ''} onClick={() => void create()}>
              {t('Create team')}
            </Button>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-invite'>{t('Invite code')}</Label>
              <Input id='team-invite' value={invite} autoComplete='off' onChange={(event) => setInvite(event.target.value)} />
            </div>
            <Button type='button' variant='outline' disabled={pending || invite.trim() === ''} onClick={() => void accept()}>
              {t('Join team')}
            </Button>
          </div>
        )}
        {error && <p className='text-destructive text-sm'>{error}</p>}
      </CardContent>
    </Card>
  )
}
