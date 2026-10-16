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
import { ShieldAlert } from 'lucide-react'
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

import {
  useDisableEpayInstance,
  useEnableEpayInstance,
  useSetEpayPrimary,
} from '../hooks/use-epay-instances'
import type { EpayInstanceView } from '../types'

type Action = 'set-primary' | 'disable' | 'enable'

type Props = {
  row: EpayInstanceView | null
  action: Action | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ConfirmEpayActionDialog({
  row,
  action,
  open,
  onOpenChange,
}: Props) {
  const { t } = useTranslation()
  const setPrimary = useSetEpayPrimary()
  const disable = useDisableEpayInstance()
  const enable = useEnableEpayInstance()
  const [step, setStep] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)
  const busy = setPrimary.isPending || disable.isPending || enable.isPending

  useEffect(() => {
    if (open) {
      setStep(false)
      setLocalError(null)
      setPrimary.reset()
      disable.reset()
      enable.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, action, row?.id])

  if (!row || !action) return null

  async function handleConfirm() {
    if (!row || !action) return
    if (!step) {
      setStep(true)
      return
    }
    setLocalError(null)
    try {
      let res
      if (action === 'set-primary') {
        res = await setPrimary.mutateAsync(row.id)
      } else if (action === 'enable') {
        res = await enable.mutateAsync(row.id)
      } else {
        res = await disable.mutateAsync(row.id)
      }
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Action failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      let okMsg: string
      if (action === 'set-primary') {
        okMsg = t('Set as primary')
      } else if (action === 'enable') {
        okMsg = t('Instance enabled')
      } else {
        okMsg = t('Instance disabled')
      }
      toast.success(okMsg)
      onOpenChange(false)
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Action failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  let title: string
  let body: string
  let confirmLabel: string
  if (action === 'set-primary') {
    title = t('Set as primary')
    body = t(
      'Only one primary instance is active. Setting this one clears the previous primary and resets its failure streak. Requires a second confirmation.'
    )
    confirmLabel = t('Set as primary')
  } else if (action === 'enable') {
    title = t('Enable epay instance')
    body = t(
      'Re-enable this instance so it can accept new orders. Does not set it as primary. Requires a second confirmation.'
    )
    confirmLabel = t('Enable')
  } else {
    title = t('Disable epay instance')
    body = t(
      'Disabled instances stop accepting new orders but still handle historical callbacks. If it is primary, it will also be demoted. Requires a second confirmation.'
    )
    confirmLabel = t('Disable')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{body}</DialogDescription>
        </DialogHeader>
        <div className='grid gap-2 text-sm'>
          <div className='text-muted-foreground text-xs'>
            <span className='text-foreground font-medium'>{row.name}</span>
            {' · '}
            <span className='font-mono'>#{row.id}</span>
            {' · '}
            <span className='font-mono'>{row.partner_id}</span>
          </div>
          {step ? (
            <div className='rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/40 dark:text-amber-100'>
              <div className='mb-1 flex items-center gap-1 font-medium'>
                <ShieldAlert className='size-3.5' />
                {t('Requires second confirmation')}
              </div>
              {t('Click again to submit this sensitive action.')}
            </div>
          ) : (
            <div className='text-muted-foreground text-xs'>
              {t('Requires second confirmation')}
            </div>
          )}
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
            variant={action === 'disable' ? 'destructive' : 'default'}
            disabled={busy}
            onClick={() => void handleConfirm()}
          >
            {step ? t('Confirm') : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
