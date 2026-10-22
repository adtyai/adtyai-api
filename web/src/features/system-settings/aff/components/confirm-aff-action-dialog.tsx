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
  withdrawMethodLabel,
  withdrawStatusLabel,
  type AffAdminWithdrawAction,
} from '@/features/affiliate/lib/status'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { usdtChainById } from '@/lib/usdt-chains'

import {
  useAdminAffWithdrawAction,
  useApproveSelfInvite,
  useRejectSelfInvite,
  useReopenSelfInvite,
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
  | {
      kind: 'self-invite'
      row: AffRiskReview
      action: 'approve' | 'reject' | 'reopen'
    }

type Props = {
  target: AffConfirmKind | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: () => void
}

function titleFor(t: (k: string) => string, target: AffConfirmKind): string {
  if (target.kind === 'void') return t('Void commission')
  if (target.kind === 'self-invite') {
    if (target.action === 'approve') return t('Approve self-invite review')
    if (target.action === 'reject') return t('Reject self-invite review')
    return t('Restore self-invite review')
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
  const reopenInvite = useReopenSelfInvite()
  const [reason, setReason] = useState('')
  const [payoutRef, setPayoutRef] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  const busy =
    voidMut.isPending ||
    wdMut.isPending ||
    approveInvite.isPending ||
    rejectInvite.isPending ||
    reopenInvite.isPending

  useEffect(() => {
    if (open) {
      setReason('')
      setPayoutRef('')
      setLocalError(null)
      voidMut.reset()
      wdMut.reset()
      approveInvite.reset()
      rejectInvite.reset()
      reopenInvite.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, target])

  if (!target) return null

  async function handleConfirm() {
    if (!target) return
    setLocalError(null)
    if (
      target.kind === 'withdraw' &&
      target.action === 'reject' &&
      !reason.trim()
    ) {
      const msg = t(
        'Reason is required before confirming this withdrawal action'
      )
      setLocalError(msg)
      return
    }
    try {
      let res: { success: boolean; message?: string; code?: string }
      if (target.kind === 'void') {
        res = await voidMut.mutateAsync(target.row.id)
      } else if (target.kind === 'self-invite') {
        if (target.action === 'approve') {
          res = await approveInvite.mutateAsync(target.row.id)
        } else if (target.action === 'reject') {
          res = await rejectInvite.mutateAsync(target.row.id)
        } else {
          res = await reopenInvite.mutateAsync(target.row.id)
        }
      } else {
        res = await wdMut.mutateAsync({
          id: target.row.id,
          action: target.action,
          reason: reason.trim() || undefined,
          payout_ref:
            target.action === 'mark-paid'
              ? payoutRef.trim() || undefined
              : undefined,
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
            {target.kind === 'withdraw'
              ? t(
                  'Invite earnings change after this action. Paid balance and bonus balance are not changed here.'
                )
              : t(
                  'Please confirm this sensitive action. Submit takes effect immediately.'
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
                {t('Payout method')}: {t(withdrawMethodLabel(target.row))}
                {target.row.usdt_chain
                  ? ` · ${usdtChainById(target.row.usdt_chain)?.label || target.row.usdt_chain}`
                  : ''}
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
                {t('Order')}:{' '}
                <span className='font-mono'>{target.row.order_no || '-'}</span>
              </div>
              <div>
                {t('Inviter')}:{' '}
                {target.row.inviter_name || target.row.inviter_id || '-'}
              </div>
              <div>
                {t('Invitee')}:{' '}
                {target.row.invitee_name ||
                  target.row.invitee_id ||
                  target.row.user_id ||
                  '-'}
              </div>
              <div>
                {t('Amount')}:{' '}
                {formatBillingCurrencyFromUSD(target.row.amount_usd || 0, {
                  digitsLarge: 2,
                  digitsSmall: 2,
                  abbreviate: false,
                })}
              </div>
              <div>
                {t('Commission')}:{' '}
                {formatAffUsd(target.row.commission_quota || 0)}
              </div>
              <div className='col-span-2'>{target.row.reason || '-'}</div>
            </div>
          ) : null}
          {target.kind === 'withdraw' && target.action === 'mark-paid' ? (
            <div className='grid gap-1'>
              <Input
                value={payoutRef}
                onChange={(e) => setPayoutRef(e.target.value)}
                placeholder={t('Payout reference')}
              />
              <div className='text-muted-foreground text-[11px]'>
                {t(
                  'Payout reference is optional. Leave it blank if you do not have one.'
                )}
              </div>
            </div>
          ) : null}
          {target.kind === 'withdraw' && target.action !== 'mark-paid' ? (
            <Input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={
                target.action === 'reject'
                  ? t(
                      'Reason is required before confirming this withdrawal action'
                    )
                  : t('Reason')
              }
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
