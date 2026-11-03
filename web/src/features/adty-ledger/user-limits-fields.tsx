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

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

type UserLimits = { rpm: number; tpm: number; concurrency: number }
type LimitKey = keyof UserLimits

const KEYS: LimitKey[] = ['rpm', 'tpm', 'concurrency']

async function getUserLimits(userId: number): Promise<UserLimits> {
  const res = await api.get<{ success: boolean; data: UserLimits }>(
    `/api/adty/admin/users/${userId}/limits`
  )
  return requireServerSuccess(res.data).data
}

async function putUserLimits(
  userId: number,
  limits: UserLimits
): Promise<UserLimits> {
  const res = await api.put<{ success: boolean; data: UserLimits }>(
    `/api/adty/admin/users/${userId}/limits`,
    limits
  )
  return requireServerSuccess(res.data).data
}

/** One user's RPM, TPM and concurrency (adty task 7.4); empty follows the default, 0 is no limit. */
export function UserLimitsFields(props: { userId: number; canEdit: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const queryKey = ['adty-user-limits', props.userId]
  const query = useQuery({
    queryKey,
    queryFn: () => getUserLimits(props.userId),
  })
  const [drafts, setDrafts] = useState<Record<LimitKey, string>>({
    rpm: '',
    tpm: '',
    concurrency: '',
  })
  const loaded = query.data
  useEffect(() => {
    if (!loaded) return
    setDrafts({
      rpm: loaded.rpm < 0 ? '' : String(loaded.rpm),
      tpm: loaded.tpm < 0 ? '' : String(loaded.tpm),
      concurrency: loaded.concurrency < 0 ? '' : String(loaded.concurrency),
    })
  }, [loaded])

  const toValue = (draft: string) => (draft.trim() === '' ? -1 : Number(draft))
  const invalid = KEYS.some((key) => {
    const value = toValue(drafts[key])
    return !Number.isInteger(value) || value < -1
  })
  const save = useMutation({
    mutationFn: () =>
      putUserLimits(props.userId, {
        rpm: toValue(drafts.rpm),
        tpm: toValue(drafts.tpm),
        concurrency: toValue(drafts.concurrency),
      }),
    onSuccess: (next) => {
      queryClient.setQueryData(queryKey, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  const labels: Record<LimitKey, string> = {
    rpm: 'RPM',
    tpm: 'TPM',
    concurrency: t('Concurrent requests'),
  }

  return (
    <fieldset className='space-y-2'>
      <legend className='text-sm font-medium'>{t('Request limits')}</legend>
      <div className='grid grid-cols-3 gap-2'>
        {KEYS.map((key) => (
          <div key={key} className='space-y-1'>
            <Label htmlFor={`user-limit-${key}`} className='text-xs'>
              {labels[key]}
            </Label>
            <Input
              id={`user-limit-${key}`}
              type='number'
              min={0}
              step={1}
              value={drafts[key]}
              placeholder={t('Default')}
              disabled={!props.canEdit}
              onChange={(event) =>
                setDrafts((prev) => ({ ...prev, [key]: event.target.value }))
              }
            />
          </div>
        ))}
      </div>
      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-xs'>
          {t('Empty follows the default, 0 means no limit.')}
        </p>
        {props.canEdit && (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={invalid || save.isPending}
            onClick={() => save.mutate()}
          >
            {t('Save limits')}
          </Button>
        )}
      </div>
    </fieldset>
  )
}
