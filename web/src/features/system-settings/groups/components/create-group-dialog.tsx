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

import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useCreateGroupMeta } from '../hooks/use-groups'
import { groupApiErrorToastKey, validateCreateInput } from '../lib/form'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated?: () => void
}

export function CreateGroupDialog({ open, onOpenChange, onCreated }: Props) {
  const { t } = useTranslation()
  const create = useCreateGroupMeta()
  const [code, setCode] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [sort, setSort] = useState('0')

  useEffect(() => {
    if (!open) return
    setCode('')
    setDisplayName('')
    setSort('0')
    create.reset()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  async function handleCreate() {
    const validated = validateCreateInput({
      code,
      display_name: displayName,
      sort,
    })
    if (!validated.ok) {
      if (validated.error === 'invalid_code') {
        toast.error(
          t('Invalid group code (use a-z, 0-9, _, -; 2–32 chars)')
        )
        return
      }
      if (validated.error === 'invalid_sort') {
        toast.error(t('Sort must be an integer'))
        return
      }
      toast.error(t('Display name is required'))
      return
    }

    try {
      const res = await create.mutateAsync(validated.body)
      if (!res.success) {
        const key = groupApiErrorToastKey(res.code)
        toast.error(key ? t(key) : getServerErrorMessage(res, t('Save failed')))
        return
      }
      toast.success(t('Saved'))
      onOpenChange(false)
      onCreated?.()
    } catch (err) {
      const status = (err as { status?: number })?.status
      if (status === 403) {
        toast.error(
          t('Root permission is required to manage group display names.')
        )
        return
      }
      toast.error(getServerErrorMessage(err, t('Save failed')))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Create group display name')}</DialogTitle>
          <DialogDescription>
            {t(
              'Code is a fixed English identifier. Display name can be Chinese; leaving it empty uses the code.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-3 py-1'>
          <div className='space-y-1.5'>
            <Label htmlFor='group-code'>{t('Group code')}</Label>
            <Input
              id='group-code'
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder={t('e.g. {{example}}', { example: 'vip' })}
              autoComplete='off'
              className='font-mono'
              disabled={create.isPending}
            />
            <p className='text-muted-foreground text-xs'>
              {t('a-z, 0-9, underscore, hyphen; 2–32 characters. Cannot change later.')}
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='group-dn'>{t('Display name')}</Label>
            <Input
              id='group-dn'
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t('Optional; defaults to code')}
              autoComplete='off'
              disabled={create.isPending}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='group-sort'>{t('Sort')}</Label>
            <Input
              id='group-sort'
              value={sort}
              onChange={(e) => setSort(e.target.value)}
              inputMode='numeric'
              autoComplete='off'
              disabled={create.isPending}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={create.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={() => void handleCreate()}
            disabled={create.isPending || !code.trim()}
          >
            {create.isPending ? t('Saving...') : t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
