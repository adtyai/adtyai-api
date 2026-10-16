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
import { formatAffUsd } from '@/features/affiliate/lib/quota'
import {
  commissionStatusLabel,
  withdrawStatusLabel,
  type AffAdminWithdrawAction,
} from '@/features/affiliate/lib/status'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  useAdminAffWithdrawAction,
  useApproveSelfInvite,
  useRejectSelfInvite,
  useVoidAffCommission,
} from '../hooks/use-admin-aff'
import type { AffCommission, AffRiskReview, AffWithdrawal } from '../types'

export type AffConfirmKind =
  | { kind: 'void'; row: AffCommission }
  | {
      kind: 'withdraw'
      row: AffWithdrawal
      action: AffAdminWithdrawAction
    }
  | { kind: 'self-invite'; row: AffRiskReview; action: 'approve' | 'reject' }

type Props = {
  target: AffConfirmKind | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: () => void
}

function titleFor(t: (k: string) => string, target: AffConfirmKind): string {
  if (target.kind === 'void') return t('Void commission')
  if (target.kind === 'self-invite') {
    return target.action === 'approve'
      ? t('Approve self-invite review')
      : t('Reject self-invite review')
  }
  const map: Record<AffAdminWithdrawAction, string> = {
    approve: t('Approve withdrawal'),
    reject: t('Reject withdrawal'),
    'mark-paid': t('Mark paid'),
    'mark-failed': t('Mark failed'),
    'mark-unknown': t('Mark unknown'),
    retry: t('Retry payout'),
  }
  return map[target.action]
}

export function ConfirmAffActionDialog({
  target,
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const voidMut = useVoidAffCommission()
  const wdMut = useAdminAffWithdrawAction()
  const approveInvite = useApproveSelfInvite()
  const rejectInvite = useRejectSelfInvite()
  const [reason, setReason] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  const busy =
    voidMut.isPending ||
    wdMut.isPending ||
    approveInvite.isPending ||
    rejectInvite.isPending

  useEffect(() => {
    if (open) {
      setReason('')
      setLocalError(null)
      voidMut.reset()
      wdMut.reset()
      approveInvite.reset()
      rejectInvite.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, target])

  if (!target) return null

  async function handleConfirm() {
    if (!target) return
    setLocalError(null)
    try {
      let res: { success: boolean; message?: string; code?: string }
      if (target.kind === 'void') {
        res = await voidMut.mutateAsync(target.row.id)
      } else if (target.kind === 'self-invite') {
        res =
          target.action === 'approve'
            ? await approveInvite.mutateAsync(target.row.id)
            : await rejectInvite.mutateAsync(target.row.id)
      } else {
        res = await wdMut.mutateAsync({
          id: target.row.id,
          action: target.action,
          reason: target.action === 'reject' ? reason.trim() : undefined,
        })
      }
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Action failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Done'))
      onOpenChange(false)
      onDone?.()
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Action failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{titleFor(t, target)}</DialogTitle>
          <DialogDescription>
            {t(
              'Sensitive action requires a second confirmation. Click twice to submit.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-2 text-sm'>
          {target.kind === 'void' ? (
            <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
              <div>
                ID: <span className='font-mono'>{target.row.id}</span>
              </div>
              <div>
                {t('Status')}:{' '}
                <span className='font-mono'>
                  {t(commissionStatusLabel(target.row.status))}
                </span>
              </div>
              <div>
                {t('Inviter')}: {target.row.inviter_id}
              </div>
              <div>
                {t('Commission')}: {formatAffUsd(target.row.commission_quota)}
              </div>
            </div>
          ) : null}
          {target.kind === 'withdraw' ? (
            <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
              <div>
                ID: <span className='font-mono'>{target.row.id}</span>
              </div>
              <div>
                {t('Status')}:{' '}
                <span className='font-mono'>
                  {t(withdrawStatusLabel(target.row.status))}
                </span>
              </div>
              <div>
                {t('User')}: {target.row.user_id}
              </div>
              <div>
                {t('Amount')}: {formatAffUsd(target.row.amount_quota)}
              </div>
              <div className='col-span-2 break-all'>
                {t('Payout account')}: {target.row.pay_account || '-'}
              </div>
            </div>
          ) : null}
          {target.kind === 'self-invite' ? (
            <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
              <div>
                ID: <span className='font-mono'>{target.row.id}</span>
              </div>
              <div>
                {t('User')}: {target.row.user_id}
              </div>
              <div className='col-span-2'>{target.row.reason || '-'}</div>
            </div>
          ) : null}
          {target.kind === 'withdraw' && target.action === 'reject' ? (
            <Input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t('Reject reason (optional)')}
            />
          ) : null}
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
            variant={
              target.kind === 'void' ||
              ('action' in target &&
                (target.action === 'reject' || target.action === 'mark-failed'))
                ? 'destructive'
                : 'default'
            }
            disabled={busy}
            onClick={() => void handleConfirm()}
          >
            {t('Confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
