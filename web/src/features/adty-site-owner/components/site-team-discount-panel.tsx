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
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import { getOwnerSiteTeams, saveOwnerSiteTeamDiscount, siteOwnerKeys } from '../api'

/** Owner sets a site team discount. The saved discount is shown; no rate is suggested. */
export function SiteTeamDiscountPanel() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const teams = useQuery({
    queryKey: siteOwnerKeys.list('site-teams', {}),
    queryFn: getOwnerSiteTeams,
  })
  const [drafts, setDrafts] = useState<Record<number, string>>({})
  const save = useMutation({
    mutationFn: (teamId: number) => saveOwnerSiteTeamDiscount(teamId, drafts[teamId] ?? ''),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (error) => handleServerError(error),
  })
  return (
    <section className='flex flex-col gap-3'>
      <div className='flex flex-col gap-1'>
        <h2 className='text-sm font-medium'>{t('Team discount')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t('A site team discount cannot go below the site supply price. Shared balance is not changed.')}
        </p>
      </div>
      {teams.isPending && <LoadingState />}
      {teams.isError && <ErrorState onRetry={() => void teams.refetch()} />}
      {teams.isSuccess && teams.data.length === 0 && <EmptyState title={t('No data')} />}
      {teams.isSuccess &&
        teams.data.map((team) => {
          const value = drafts[team.id] ?? ''
          return (
            <div key={team.id} className='flex flex-col gap-2'>
              <p className='text-sm'>
                {team.name} · <span className='tabular-nums'>{team.discount}</span>
              </p>
              <div className='flex flex-col gap-1'>
                <Label htmlFor={`team-discount-${team.id}`}>{t('Discount')}</Label>
                <Input
                  id={`team-discount-${team.id}`}
                  value={value}
                  inputMode='decimal'
                  autoComplete='off'
                  onChange={(event) =>
                    setDrafts((current) => ({ ...current, [team.id]: event.target.value }))
                  }
                />
              </div>
              <Button
                type='button'
                variant='outline'
                disabled={save.isPending || value.trim() === ''}
                onClick={() => save.mutate(team.id)}
              >
                {t('Save')}
              </Button>
            </div>
          )
        })}
    </section>
  )
}
