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
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useDeleteEnterprisePricing } from '../hooks/use-enterprise-pricing'
import { formatDiscount, scopeLabel } from '../lib/discount'
import type { EnterprisePriceRule } from '../types'

type Props = {
  row: EnterprisePriceRule | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ConfirmDeleteDialog({ row, open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const del = useDeleteEnterprisePricing()
  const [localError, setLocalError] = useState<string | null>(null)
  const busy = del.isPending

  useEffect(() => {
    if (open) {
      setLocalError(null)
      del.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, row?.id])

  if (!row) return null

  async function handleConfirm() {
    if (!row) return
    setLocalError(null)
    try {
      const res = await del.mutateAsync(row.id)
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Delete failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Deleted'))
      onOpenChange(false)
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Delete failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  const wildcard = t('All (wildcard)')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Delete enterprise pricing rule')}</DialogTitle>
          <DialogDescription>
            {t(
              'Deletes this agreement-price rule. Takes effect on submit. Matching falls back to a lower-priority rule or full price.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-2 text-sm'>
          <div className='text-muted-foreground text-xs'>
            <span className='text-foreground font-mono'>#{row.id}</span>
            {' · '}
            {t('User')} {row.user_id}
            {' · '}
            {scopeLabel(row.group_code, wildcard)} /{' '}
            {scopeLabel(row.model_name, wildcard)}
            {' · '}
            {formatDiscount(row.discount)}
          </div>
          {localError ? (
            <div className='text-destructive border-destructive/30 bg-destructive/5 rounded-lg border px-3 py-2 text-xs'>
              {localError}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={busy}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            disabled={busy}
            onClick={() => void handleConfirm()}
          >
            {t('Delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
