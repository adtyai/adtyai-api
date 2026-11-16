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
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getSiteTiers, setSiteTierAssignment, siteAdminKeys } from '../api'
import type { AdminSite } from '../types'

const AUTOMATIC = ''

/**
 * Root pins one site to an enabled level or hands it back to the hourly
 * evaluation. A pinned site keeps its level until Root changes it.
 */
export function SiteTierControl(props: { site: AdminSite }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const isRoot =
    useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const pinned = props.site.tier_mode === 'manual'
  const current = pinned ? (props.site.tier_code ?? '') : AUTOMATIC
  const [choice, setChoice] = useState(current)
  const tiers = useQuery({
    queryKey: siteAdminKeys.list('site-tiers', {}),
    queryFn: getSiteTiers,
    enabled: isRoot,
  })
  const save = useMutation({
    mutationFn: () =>
      choice === AUTOMATIC
        ? setSiteTierAssignment(props.site.id, '', 'auto')
        : setSiteTierAssignment(props.site.id, choice, 'manual'),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  const enabled = (Array.isArray(tiers.data) ? tiers.data : []).filter(
    (tier) => tier.enabled
  )
  const currentName =
    (Array.isArray(tiers.data) ? tiers.data : []).find(
      (tier) => tier.code === props.site.tier_code
    )?.name ?? props.site.tier_code
  return (
    <section className='flex flex-col gap-2'>
      <h3 className='text-sm font-medium'>{t('Site level')}</h3>
      <p className='text-muted-foreground text-sm'>
        {currentName
          ? `${t('Current level')}: ${currentName}`
          : t('No level yet; default terms apply.')}{' '}
        ·{' '}
        {pinned ? t('Pinned by Root') : t('Evaluated automatically every hour')}
      </p>
      {tiers.isError && <ErrorState onRetry={() => void tiers.refetch()} />}
      <div className='flex flex-wrap items-end gap-2'>
        <div className='flex flex-col gap-1'>
          <Label htmlFor={`site-tier-${props.site.id}`}>
            {t('Level assignment')}
          </Label>
          <NativeSelect
            id={`site-tier-${props.site.id}`}
            value={choice}
            onChange={(event) => setChoice(event.target.value)}
          >
            <NativeSelectOption value={AUTOMATIC}>
              {t('Automatic (by thresholds)')}
            </NativeSelectOption>
            {enabled.map((tier) => (
              <NativeSelectOption key={tier.code} value={tier.code}>
                {t('Pin to {{name}}', { name: tier.name })}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={save.isPending || choice === current}
          onClick={() => save.mutate()}
        >
          {t('Save')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Switching to automatic clears the level until the next hourly evaluation picks one.'
        )}
      </p>
    </section>
  )
}
