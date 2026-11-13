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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/** Root sets a main-site team discount. The form does not suggest a floor. */
export function MainTeamPricePanel() {
  const { t } = useTranslation()
  const isRoot = useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const [teamId, setTeamId] = useState('')
  const [discount, setDiscount] = useState('')
  const save = useMutation({
    mutationFn: async () => {
      const res = await api.put(`/api/adty/admin/teams/${teamId.trim()}/agreement-price`, {
        discount: discount.trim(),
      })
      requireServerSuccess(res.data)
    },
    onSuccess: () => {
      setTeamId('')
      setDiscount('')
    },
    onError: (error) => handleServerError(error),
  })
  if (!isRoot) return null
  return (
    <section className='flex flex-col gap-3'>
      <h2 className='text-sm font-medium'>{t('Agreement price')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t('A main-site team discount has no supply floor. A subsite team is refused.')}
      </p>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='main-team-id'>{t('Team')}</Label>
          <Input id='main-team-id' value={teamId} inputMode='numeric' onChange={(event) => setTeamId(event.target.value)} />
        </div>
        <div className='flex flex-col gap-1'>
          <Label htmlFor='main-team-discount'>{t('Discount')}</Label>
          <Input id='main-team-discount' value={discount} inputMode='decimal' onChange={(event) => setDiscount(event.target.value)} />
        </div>
      </div>
      <Button
        type='button'
        variant='outline'
        disabled={save.isPending || teamId.trim() === '' || discount.trim() === ''}
        onClick={() => save.mutate()}
      >
        {t('Save')}
      </Button>
    </section>
  )
}
