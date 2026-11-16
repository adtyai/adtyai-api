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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsSection } from '../components/settings-section'

/** GET /api/adty/admin/group-health: success_min is a 0–1 decimal string. */
type GroupHealthConfig = {
  success_min: string
  stale_seconds: number
  heartbeat_at: number
}

/** "0.95" ↔ "95" without floating-point noise. */
function toPercent(fraction: string) {
  const value = Number(fraction)
  return Number.isFinite(value) ? String(Math.round(value * 10000) / 100) : ''
}

function toFraction(percent: string) {
  return String(Math.round(Number(percent) * 100) / 10000)
}

/**
 * Root tunes how the API key group picker colours group health: the success
 * rate below which a group is flagged, and how long without samples counts as
 * an interrupted monitor. Monitoring never changes a key's group.
 */
export function GroupHealthSection() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const isRoot =
    useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const config = useQuery({
    queryKey: ['adty-group-health-config'],
    queryFn: async () => {
      const res = await api.get('/api/adty/admin/group-health')
      return requireServerSuccess(res.data).data as GroupHealthConfig
    },
    enabled: isRoot,
  })
  const [percent, setPercent] = useState('')
  const [minutes, setMinutes] = useState('')
  useEffect(() => {
    if (!config.data) return
    setPercent(toPercent(config.data.success_min))
    setMinutes(String(Math.round(config.data.stale_seconds / 60)))
  }, [config.data])
  const save = useMutation({
    mutationFn: async () => {
      const res = await api.put('/api/adty/admin/group-health', {
        success_min: toFraction(percent),
        stale_seconds: Number(minutes) * 60,
      })
      requireServerSuccess(res.data)
    },
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void client.invalidateQueries({ queryKey: ['adty-group-health-config'] })
    },
    onError: (error) => handleServerError(error),
  })
  const percentValue = Number(percent)
  const minutesValue = Number(minutes)
  const ready =
    percent.trim() !== '' &&
    percentValue >= 0 &&
    percentValue <= 100 &&
    Number.isInteger(minutesValue) &&
    minutesValue >= 1

  return (
    <SettingsSection
      title={t('Group health')}
      description={t(
        'Shown next to each group when customers create or edit an API key. Monitoring never moves a key to another group.'
      )}
    >
      {!isRoot && (
        <p className='text-sm'>
          {t('Only Root can change group health settings.')}
        </p>
      )}
      {isRoot && config.isPending && <LoadingState />}
      {isRoot && config.isError && (
        <ErrorState onRetry={() => void config.refetch()} />
      )}
      {isRoot && config.isSuccess && (
        <div className='flex flex-col gap-4'>
          <div className='grid max-w-xl gap-3 sm:grid-cols-2'>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='group-health-success'>
                {t('Healthy at or above (%)')}
              </Label>
              <Input
                id='group-health-success'
                inputMode='decimal'
                value={percent}
                onChange={(event) => setPercent(event.target.value)}
              />
            </div>
            <div className='flex flex-col gap-1'>
              <Label htmlFor='group-health-stale'>
                {t('Monitor interrupted after (minutes without samples)')}
              </Label>
              <Input
                id='group-health-stale'
                inputMode='numeric'
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
              />
            </div>
          </div>
          <p className='text-muted-foreground text-sm tabular-nums'>
            {t('Last sample')}:{' '}
            {config.data?.heartbeat_at
              ? formatTimestamp(config.data.heartbeat_at)
              : t('Never')}
          </p>
          <div>
            <Button
              type='button'
              variant='brand'
              disabled={!ready || save.isPending}
              onClick={() => save.mutate()}
            >
              {t('Save')}
            </Button>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}
