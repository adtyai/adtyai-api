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
import { riskReviewStatusLabelKey } from '@/features/system-settings/utils/status-labels'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  useApproveRiskReview,
  useRejectRiskReview,
} from '../hooks/use-risk-reviews'
import { formatUsd } from '../lib/sort'
import type { RiskReview } from '../types'

type Action = 'approve' | 'reject'

type Props = {
  row: RiskReview | null
  action: Action | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: () => void
}

export function ConfirmActionDialog({
  row,
  action,
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const approve = useApproveRiskReview()
  const reject = useRejectRiskReview()
  const [localError, setLocalError] = useState<string | null>(null)
  const busy = approve.isPending || reject.isPending

  useEffect(() => {
    if (open) {
      setLocalError(null)
      approve.reset()
      reject.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, action, row?.id])

  if (!row || !action) return null

  async function handleConfirm() {
    if (!row || !action) return
    setLocalError(null)
    try {
      const res =
        action === 'approve'
          ? await approve.mutateAsync(row.id)
          : await reject.mutateAsync(row.id)
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Action failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(action === 'approve' ? t('Approved') : t('Rejected'))
      onOpenChange(false)
      onDone?.()
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Action failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  const title =
    action === 'approve' ? t('Approve risk review') : t('Reject risk review')
  const body =
    action === 'approve'
      ? t(
          'Approve credits the pending bonus/gift into the bonus wallet. Paid amount is already settled. Requires a second confirmation.'
        )
      : t(
          'Reject does not credit the bonus/gift. Paid amount stays settled. Requires a second confirmation.'
        )

  const confirmButtonLabel = action === 'approve' ? t('Approve') : t('Reject')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{body}</DialogDescription>
        </DialogHeader>
        <div className='grid gap-2 text-sm'>
          <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
            <div>
              <span className='text-foreground font-medium'>{t('ID')}: </span>
              <span className='font-mono'>{row.id}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Status')}:{' '}
              </span>
              <span className='font-mono'>
                {t(riskReviewStatusLabelKey(row.status))}
              </span>
            </div>
            <div>
              <span className='text-foreground font-medium'>{t('User')}: </span>
              {row.user_id}
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Amount')}:{' '}
              </span>
              {formatUsd(row.amount_usd)}
            </div>
            {row.order_no ? (
              <div className='col-span-2'>
                <span className='text-foreground font-medium'>
                  {t('Order')}:{' '}
                </span>
                <span className='font-mono text-[12px]'>{row.order_no}</span>
              </div>
            ) : null}
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
            variant={action === 'reject' ? 'destructive' : 'default'}
            disabled={busy || row.status !== 'pending'}
            onClick={() => void handleConfirm()}
          >
            {confirmButtonLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
