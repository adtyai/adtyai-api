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

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  issueTeamRegistration,
  listTeamRegistrations,
  revokeTeamRegistration,
  teamRegistrationUrl,
  TEAM_REGISTRATION_MAX_TTL_HOURS,
  type TeamRegistrationActor,
} from '../api'

const statusLabel: Record<string, string> = {
  outstanding: 'Outstanding',
  expired: 'Expired',
  used: 'Used',
  revoked: 'Revoked',
}

function ttlSecondsFromHours(value: string): number | null {
  const hours = Number(value)
  if (!Number.isFinite(hours) || hours <= 0 || hours > TEAM_REGISTRATION_MAX_TTL_HOURS) {
    return null
  }
  return Math.round(hours * 3600)
}

export function TeamRegistrationLinksPanel(props: {
  actor: TeamRegistrationActor
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = ['adty-team-registrations', props.actor] as const
  const links = useQuery({
    queryKey,
    queryFn: () => listTeamRegistrations(props.actor),
  })
  const [ttlHours, setTtlHours] = useState('24')
  const [issuedUrl, setIssuedUrl] = useState('')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const refresh = () => void queryClient.invalidateQueries({ queryKey })

  const issue = async () => {
    const ttlSeconds = ttlSecondsFromHours(ttlHours)
    if (ttlSeconds == null) {
      setError(t('Invalid parameters'))
      return
    }
    setPending(true)
    setError('')
    setCopied(false)
    try {
      const issued = await issueTeamRegistration(props.actor, ttlSeconds)
      setIssuedUrl(teamRegistrationUrl(issued.code))
      refresh()
    } catch (cause) {
      setError(t(getServerErrorMessage(cause, t('Failed to save'))))
    } finally {
      setPending(false)
    }
  }

  const revoke = async (id: number) => {
    setPending(true)
    setError('')
    try {
      await revokeTeamRegistration(props.actor, id)
      refresh()
    } catch (cause) {
      setError(t(getServerErrorMessage(cause, t('Failed to save'))))
    } finally {
      setPending(false)
    }
  }

  const copyIssued = async () => {
    if (!issuedUrl || !navigator.clipboard?.writeText) {
      setError(t('Failed to copy'))
      return
    }
    try {
      await navigator.clipboard.writeText(issuedUrl)
      setCopied(true)
    } catch {
      setCopied(false)
      setError(t('Failed to copy'))
    }
  }

  return (
    <section className='flex flex-col gap-4'>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Team registration')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Issue a team registration link so the recipient can set a username, password, and team name.'
          )}
        </p>
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor={`team-registration-ttl-${props.actor}`}>
          {t('Validity (hours)')}
        </Label>
        <Input
          id={`team-registration-ttl-${props.actor}`}
          value={ttlHours}
          inputMode='decimal'
          autoComplete='off'
          onChange={(event) => setTtlHours(event.target.value)}
        />
      </div>
      <Button
        type='button'
        variant='outline'
        disabled={pending}
        onClick={() => void issue()}
      >
        {t('Issue registration link')}
      </Button>
      {issuedUrl && (
        <div className='flex flex-col gap-2'>
          <p className='text-sm break-all'>
            {t('Registration link')}: {issuedUrl}
          </p>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void copyIssued()}
          >
            {copied ? t('Copied to clipboard') : t('Copy link')}
          </Button>
          <p className='text-muted-foreground text-sm'>
            {t('The registration link is shown once')}
          </p>
        </div>
      )}
      {error && <p className='text-destructive text-sm'>{error}</p>}
      {links.isPending && <LoadingState />}
      {links.isError && <ErrorState onRetry={() => void links.refetch()} />}
      {links.isSuccess && links.data.length === 0 && (
        <EmptyState title={t('No registration links yet')} />
      )}
      {links.isSuccess && links.data.length > 0 && (
        <div className='flex flex-col gap-2'>
          {links.data.map((row) => (
            <div
              key={row.id}
              className='flex items-center justify-between gap-3 text-sm'
            >
              <span className='min-w-0 truncate'>
                #{row.id} · {t(statusLabel[row.status] ?? row.status)} ·{' '}
                {t('Validity')}:{' '}
                {row.expires_at
                  ? new Date(row.expires_at * 1000).toLocaleString()
                  : '—'}
              </span>
              {row.status === 'outstanding' && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={pending}
                  onClick={() => void revoke(row.id)}
                >
                  {t('Revoke registration link')}
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
