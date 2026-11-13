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

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { SettlementSheetLines } from '@/features/adty-site-owner/components/settlement-sheet-panel'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminBusinessTimezone,
  getAdminSettlementSheets,
  saveAdminBusinessTimezone,
  siteAdminKeys,
} from '../api'

/** Root business-day clock and the check sheets it produces. */
export function BusinessTimezonePanel() {
  const { t } = useTranslation()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const client = useQueryClient()
  const [timezone, setTimezone] = useState('')
  const [selected, setSelected] = useState('')
  const zone = useQuery({
    queryKey: siteAdminKeys.list('business-timezone', {}),
    queryFn: getAdminBusinessTimezone,
    enabled: isRoot,
  })
  const sheets = useQuery({
    queryKey: siteAdminKeys.list('settlement-sheets', {}),
    queryFn: () => getAdminSettlementSheets(),
    enabled: isRoot,
  })
  const save = useMutation({
    mutationFn: (value: string) => saveAdminBusinessTimezone(value),
    onSuccess: () => {
      setTimezone('')
      void client.invalidateQueries({ queryKey: siteAdminKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  const rows = sheets.data ?? []
  const current = rows.find((row) => String(row.id) === selected) ?? rows[0]

  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Business day timezone')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Unset uses UTC. A saved sheet keeps the timezone it was generated with.'
        )}
      </p>
      {zone.isError && <ErrorState onRetry={() => void zone.refetch()} />}
      {zone.data && (
        <p className='text-sm tabular-nums'>
          {zone.data.configured ? zone.data.timezone : t('Not configured')}
        </p>
      )}
      <div className='flex flex-col gap-1'>
        <Label htmlFor='business-timezone'>{t('Timezone')}</Label>
        <Input
          id='business-timezone'
          value={timezone}
          autoComplete='off'
          spellCheck={false}
          onChange={(event) => setTimezone(event.target.value)}
        />
      </div>
      <div className='flex flex-wrap gap-2'>
        <Button
          type='button'
          variant='outline'
          disabled={save.isPending || timezone.trim() === ''}
          onClick={() => save.mutate(timezone.trim())}
        >
          {t('Save')}
        </Button>
        <Button
          type='button'
          variant='outline'
          disabled={save.isPending || zone.data?.configured === false}
          onClick={() => save.mutate('')}
        >
          {t('Use UTC')}
        </Button>
      </div>
      <h2 className='text-sm font-medium'>{t('Settlement sheet')}</h2>
      {sheets.isError && <ErrorState onRetry={() => void sheets.refetch()} />}
      {sheets.isSuccess && rows.length === 0 && (
        <EmptyState className='min-h-24' title={t('No settlement sheet yet')} />
      )}
      {rows.length > 0 && current && (
        <>
          <NativeSelect
            aria-label={t('Settlement sheet')}
            value={String(current.id)}
            onChange={(event) => setSelected(event.target.value)}
          >
            {rows.map((row) => (
              <NativeSelectOption key={row.id} value={String(row.id)}>
                {t('Site ID')} {row.site_id} {row.business_day}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <SettlementSheetLines sheet={current} />
        </>
      )}
    </section>
  )
}
