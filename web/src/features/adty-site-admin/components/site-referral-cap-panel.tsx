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

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminSiteCommissionPolicy,
  getAdminSiteReferralCap,
  saveAdminSiteCommissionPolicy,
  saveAdminSiteReferralCap,
  siteAdminKeys,
} from '../api'

/** Root cap for site invite rates. The field starts empty and does not suggest a rate. */
export function SiteReferralCapPanel() {
  const { t } = useTranslation()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const client = useQueryClient()
  const [maxRate, setMaxRate] = useState('')
  const [coverageCap, setCoverageCap] = useState('')
  const [observationDays, setObservationDays] = useState('')
  const cap = useQuery({
    queryKey: siteAdminKeys.list('site-referral-cap', {}),
    queryFn: getAdminSiteReferralCap,
    enabled: isRoot,
  })
  const policy = useQuery({
    queryKey: siteAdminKeys.list('site-commission-policy', {}),
    queryFn: getAdminSiteCommissionPolicy,
    enabled: isRoot,
  })
  const savePolicy = useMutation({
    mutationFn: () =>
      saveAdminSiteCommissionPolicy(coverageCap.trim(), observationDays.trim()),
    onSuccess: () => {
      setCoverageCap('')
      setObservationDays('')
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  const save = useMutation({
    mutationFn: () => saveAdminSiteReferralCap(maxRate.trim()),
    onSuccess: () => {
      setMaxRate('')
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Maximum invite rate')}</h2>
      {cap.isPending && <LoadingState />}
      {cap.isError && <ErrorState onRetry={() => void cap.refetch()} />}
      {cap.data && !cap.data.cap_configured && (
        <p className='text-sm'>{t('The referral cap is not configured')}</p>
      )}
      {cap.data?.cap_configured && (
        <p className='text-sm tabular-nums'>{cap.data.max_rate}</p>
      )}
      <div className='flex flex-col gap-1'>
        <Label htmlFor='site-referral-cap'>{t('Maximum invite rate')}</Label>
        <Input
          id='site-referral-cap'
          value={maxRate}
          inputMode='decimal'
          autoComplete='off'
          onChange={(event) => setMaxRate(event.target.value)}
        />
      </div>
      <Button
        type='button'
        variant='outline'
        disabled={save.isPending || maxRate.trim() === ''}
        onClick={() => save.mutate()}
      >
        {t('Save')}
      </Button>
      <h2 className='text-sm font-medium'>{t('Commission coverage')}</h2>
      {policy.isPending && <LoadingState />}
      {policy.isError && <ErrorState onRetry={() => void policy.refetch()} />}
      {policy.data && !policy.data.configured && (
        <p className='text-sm'>{t('Commission coverage is not configured')}</p>
      )}
      {policy.data?.configured && (
        <p className='text-sm tabular-nums'>
          {policy.data.pending_coverage_cap} / {policy.data.observation_days}
        </p>
      )}
      <div className='flex flex-col gap-1'>
        <Label htmlFor='site-coverage-cap'>{t('Pending coverage cap')}</Label>
        <Input
          id='site-coverage-cap'
          value={coverageCap}
          inputMode='decimal'
          autoComplete='off'
          onChange={(event) => setCoverageCap(event.target.value)}
        />
      </div>
      <div className='flex flex-col gap-1'>
        <Label htmlFor='site-observation-days'>{t('Observation days')}</Label>
        <Input
          id='site-observation-days'
          value={observationDays}
          inputMode='numeric'
          autoComplete='off'
          onChange={(event) => setObservationDays(event.target.value)}
        />
      </div>
      <Button
        type='button'
        variant='outline'
        disabled={
          savePolicy.isPending ||
          coverageCap.trim() === '' ||
          observationDays.trim() === ''
        }
        onClick={() => savePolicy.mutate()}
      >
        {t('Save')}
      </Button>
    </section>
  )
}
