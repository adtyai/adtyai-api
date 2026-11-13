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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { getServerErrorMessage } from '@/lib/server-error-message'

type TeamMember = {
  user_id: number
  username: string
  role: string
  status: string
}

type TeamKey = {
  token_id: number
  name: string
  user_id: number
  status: string
}

type TeamMine = {
  team_id: number
  name: string
  role: string
  discount: string
  shared_quota: number
  status: string
  members?: TeamMember[]
  keys?: TeamKey[]
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
  const [rename, setRename] = useState('')
  const [topupAmount, setTopupAmount] = useState('')
  const [discount, setDiscount] = useState('1')
  const [invite, setInvite] = useState('')
  const [issued, setIssued] = useState('')
  const [tokenId, setTokenId] = useState('')
  const [confirmClose, setConfirmClose] = useState(false)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['adty-team'] })
  const team = mine.data
  useEffect(() => {
    setRename(team?.name ?? '')
  }, [team?.name])

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

  const run = async (action: () => Promise<void>) => {
    setPending(true)
    setError('')
    try {
      await action()
      refresh()
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
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-rename'>{t('Team name')}</Label>
              <Input id='team-rename' value={rename} onChange={(event) => setRename(event.target.value)} />
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={pending || rename.trim() === '' || rename.trim() === team.name}
              onClick={() =>
                void run(async () => {
                  const response = await api.post(`/api/adty/teams/${team.team_id}/profile`, { name: rename.trim() })
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                })
              }
            >
              {t('Rename')}
            </Button>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-topup'>{t('Team top-up')}</Label>
              <Input id='team-topup' value={topupAmount} inputMode='decimal' autoComplete='off' onChange={(event) => setTopupAmount(event.target.value)} />
            </div>
            <p className='text-muted-foreground text-sm'>
              {t('A team top-up does not include a payment-method gift. The order stays pending until payment completion credits the shared balance.')}
            </p>
            <Button
              type='button'
              variant='outline'
              disabled={pending || topupAmount.trim() === ''}
              onClick={() =>
                void run(async () => {
                  const response = await api.post(
                    `/api/adty/teams/${team.team_id}/topups`,
                    { amount_usd: topupAmount.trim() },
                    { headers: { 'Idempotency-Key': crypto.randomUUID() } }
                  )
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                  setTopupAmount('')
                })
              }
            >
              {t('Team top-up')}
            </Button>
            <Button type='button' variant='outline' disabled={pending} onClick={() => void issueInvite()}>
              {t('Create invite')}
            </Button>
            {issued && (
              <p className='text-sm break-all'>
                {t('Invite code')}: {issued}
              </p>
            )}
            <p className='text-muted-foreground text-sm'>{t('The invite code is shown once')}</p>
            <div className='flex flex-col gap-2'>
              <h3 className='text-sm font-medium'>{t('Members')}</h3>
              {(team.members ?? []).map((member) => (
                <div key={member.user_id} className='flex items-center justify-between gap-3 text-sm'>
                  <span className='min-w-0 truncate'>
                    {member.username || member.user_id} · {member.role === 'leader' ? t('Leader') : t('Member')} · {member.status}
                  </span>
                  {member.role !== 'leader' && member.status === 'active' && (
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={pending}
                      onClick={() =>
                        void run(async () => {
                          const response = await api.post(`/api/adty/teams/${team.team_id}/members/${member.user_id}/disable`)
                          if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                        })
                      }
                    >
                      {t('Disable member')}
                    </Button>
                  )}
                </div>
              ))}
            </div>
            <div className='flex flex-col gap-2'>
              <h3 className='text-sm font-medium'>{t('Team keys')}</h3>
              {(team.keys ?? []).length === 0 && <p className='text-muted-foreground text-sm'>{t('No team keys')}</p>}
              {(team.keys ?? []).map((key) => (
                <div key={key.token_id} className='flex items-center justify-between gap-3 text-sm'>
                  <span className='min-w-0 truncate tabular-nums'>
                    {key.name || key.token_id} · {key.status}
                  </span>
                  {key.status === 'active' && (
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={pending}
                      onClick={() =>
                        void run(async () => {
                          const response = await api.post(`/api/adty/teams/${team.team_id}/keys/${key.token_id}/revoke`)
                          if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                        })
                      }
                    >
                      {t('Revoke key')}
                    </Button>
                  )}
                </div>
              ))}
            </div>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='leader-token'>{t('Token id')}</Label>
              <Input id='leader-token' value={tokenId} inputMode='numeric' onChange={(event) => setTokenId(event.target.value)} />
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={pending || tokenId.trim() === ''}
              onClick={() =>
                void run(async () => {
                  const response = await api.post(`/api/adty/teams/${team.team_id}/keys`, { token_id: Number(tokenId) })
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                  setTokenId('')
                })
              }
            >
              {t('Bind key')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={pending}
              onClick={() => {
                if (!confirmClose) {
                  setConfirmClose(true)
                  return
                }
                void run(async () => {
                  const response = await api.post(`/api/adty/teams/${team.team_id}/close`)
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                  setConfirmClose(false)
                })
              }}
            >
              {confirmClose ? t('Confirm close') : t('Close team')}
            </Button>
          </div>
        )}
        {team && team.role !== 'leader' && (
          <div className='flex flex-col gap-2'>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='team-token'>{t('Token id')}</Label>
              <Input id='team-token' value={tokenId} inputMode='numeric' onChange={(event) => setTokenId(event.target.value)} />
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={pending || tokenId.trim() === ''}
              onClick={() =>
                void run(async () => {
                  const response = await api.post(`/api/adty/teams/${team.team_id}/keys`, { token_id: Number(tokenId) })
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                  setTokenId('')
                })
              }
            >
              {t('Bind key')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={pending}
              onClick={() =>
                void run(async () => {
                  const response = await api.post(`/api/adty/teams/${team.team_id}/leave`)
                  if (!response.data?.success) throw new Error(response.data?.message || 'Failed to save')
                })
              }
            >
              {t('Leave team')}
            </Button>
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
