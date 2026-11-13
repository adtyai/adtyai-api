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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  bindOwnerSiteInvite,
  getOwnerSiteInvites,
  getOwnerSiteReferral,
  resumeOwnerSiteCoverage,
  saveOwnerSiteReferral,
  saveOwnerSiteReferralEnabled,
  siteOwnerKeys,
} from '../api'

/** Owner configuration for one-level site invites. Amounts come from Root and the owner, never from this page. */
export function SiteReferralPanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const rules = useQuery({
    queryKey: siteOwnerKeys.list('site-referral', {}),
    queryFn: getOwnerSiteReferral,
  })
  const invites = useQuery({
    queryKey: siteOwnerKeys.list('site-invites', {}),
    queryFn: getOwnerSiteInvites,
  })
  const [rate, setRate] = useState('')
  const [inviterId, setInviterId] = useState('')
  const [inviteeId, setInviteeId] = useState('')
  const save = useMutation({
    mutationFn: () => saveOwnerSiteReferral(rate.trim()),
    onSuccess: () => {
      setRate('')
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const enable = useMutation({
    mutationFn: (enabled: boolean) => saveOwnerSiteReferralEnabled(enabled),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const resume = useMutation({
    mutationFn: resumeOwnerSiteCoverage,
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const bind = useMutation({
    mutationFn: () =>
      bindOwnerSiteInvite(Number(inviterId), Number(inviteeId)),
    onSuccess: () => {
      setInviterId('')
      setInviteeId('')
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <section className='flex flex-col gap-4'>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('One-level invites')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t('Saving a rate does not pay commission.')}
        </p>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Uses the current earnings and authorized margin cycle on the original commission rows.'
          )}
        </p>
        <Button
          type='button'
          variant='outline'
          disabled={resume.isPending}
          onClick={() => resume.mutate()}
        >
          {t('Cover pending commissions')}
        </Button>
      </div>
      {rules.isPending && <LoadingState />}
      {rules.isError && <ErrorState onRetry={() => void rules.refetch()} />}
      {rules.data && !rules.data.cap_configured && (
        <p className='text-sm'>{t('The referral cap is not configured')}</p>
      )}
      {rules.data?.cap_configured && (
        <div className='flex flex-col gap-3'>
          <p className='text-sm tabular-nums'>
            {t('Maximum invite rate')} {rules.data.max_rate}
          </p>
          {rules.data.rate_configured && (
            <p className='text-sm tabular-nums'>
              {t('Invite rate')} {rules.data.rate}
            </p>
          )}
          {rules.data.rate_configured && (
            <div className='flex items-center justify-between gap-4'>
              <div className='flex flex-col gap-1'>
                <Label htmlFor='site-referral-enabled'>{t('Invite commission')}</Label>
                <span className='text-muted-foreground text-sm'>
                  {t('New top-ups do not create commission while this is off.')}
                </span>
              </div>
              <Switch
                id='site-referral-enabled'
                checked={rules.data.enabled === true}
                disabled={enable.isPending}
                onCheckedChange={(value) => enable.mutate(value)}
              />
            </div>
          )}
          <div className='flex flex-col gap-1'>
            <Label htmlFor='site-invite-rate'>{t('Invite rate')}</Label>
            <Input
              id='site-invite-rate'
              value={rate}
              inputMode='decimal'
              autoComplete='off'
              onChange={(event) => setRate(event.target.value)}
            />
          </div>
          <Button
            type='button'
            variant='outline'
            disabled={save.isPending || rate.trim() === ''}
            onClick={() => save.mutate()}
          >
            {t('Save')}
          </Button>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='site-inviter'>{t('Inviter')}</Label>
              <Input
                id='site-inviter'
                value={inviterId}
                inputMode='numeric'
                onChange={(event) => setInviterId(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='site-invitee'>{t('Invitee')}</Label>
              <Input
                id='site-invitee'
                value={inviteeId}
                inputMode='numeric'
                onChange={(event) => setInviteeId(event.target.value)}
              />
            </div>
          </div>
          <Button
            type='button'
            variant='outline'
            disabled={
              bind.isPending ||
              !rules.data.rate_configured ||
              inviterId.trim() === '' ||
              inviteeId.trim() === ''
            }
            onClick={() => bind.mutate()}
          >
            {t('Bind invite')}
          </Button>
        </div>
      )}
      {invites.isError && <ErrorState onRetry={() => void invites.refetch()} />}
      {invites.isSuccess && (
        <StaticDataTable
          data={invites.data}
          getRowKey={(row) => row.id}
          tableProps={{ 'aria-label': t('One-level invites') }}
          emptyContent={<EmptyState title={t('No data')} />}
          columns={[
            {
              id: 'inviter',
              header: t('Inviter'),
              cell: (row) => <span className='tabular-nums'>{row.inviter_id}</span>,
            },
            {
              id: 'invitee',
              header: t('Invitee'),
              cell: (row) => <span className='tabular-nums'>{row.invitee_id}</span>,
            },
            {
              id: 'rate',
              header: t('Invite rate'),
              cell: (row) => <span className='tabular-nums'>{row.rate}</span>,
            },
          ]}
        />
      )}
    </section>
  )
}
