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
import { Textarea } from '@/components/ui/textarea'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  useApproveBalanceAdjustment,
  useRejectBalanceAdjustment,
} from '../hooks/use-balance-adjustments'
import {
  adjustQuotaToUsd,
  balanceAdjustErrorMessage,
  formatAdjustUsd,
} from '../lib/amount'
import type { ApproveBalanceAdjustResult, BalanceAdjustRequest } from '../types'

type Action = 'approve' | 'reject'

type Props = {
  row: BalanceAdjustRequest | null
  action: Action | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: (result?: ApproveBalanceAdjustResult) => void
}

function balanceConfirmLabel(
  action: 'approve' | 'reject',
  t: (key: string) => string
): string {
  return action === 'approve' ? t('Approve') : t('Reject')
}

export function ConfirmBalanceAdjustDialog({
  row,
  action,
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const userId = useAuthStore((s) => s.auth.user?.id)
  const approve = useApproveBalanceAdjustment()
  const reject = useRejectBalanceAdjustment()
  const [selfReason, setSelfReason] = useState('')
  const [rejectReason, setRejectReason] = useState('')
  const [amountConfirm, setAmountConfirm] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)
  const busy = approve.isPending || reject.isPending

  const isSelf =
    row != null && userId != null && Number(userId) === Number(row.applicant_id)

  useEffect(() => {
    if (open) {
      setSelfReason('')
      setRejectReason('')
      setAmountConfirm('')
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

    if (action === 'approve' && isSelf && !selfReason.trim()) {
      const msg = t('Self-approve reason is required')
      setLocalError(msg)
      return
    }
    if (action === 'reject' && !rejectReason.trim()) {
      const msg = t('Reject reason is required')
      setLocalError(msg)
      return
    }
    const usd = adjustQuotaToUsd(row.amount)
    if (
      action === 'approve' &&
      Math.abs(usd) >= 100 &&
      Math.abs(Number(amountConfirm) - usd) > 1e-6
    ) {
      const msg = t('Retype the USD amount to confirm this large change')
      setLocalError(msg)
      return
    }

    try {
      if (action === 'approve') {
        const res = await approve.mutateAsync({
          id: row.id,
          self_approve_reason: isSelf ? selfReason.trim() : undefined,
        })
        if (!res.success || !res.data) {
          const msg = getServerErrorMessage(res, t('Action failed'))
          setLocalError(msg)
          toast.error(msg)
          if (res.code === 'self_approve_forbidden') {
            toast.error(t('Self-approve forbidden when multiple admins'))
          }
          return
        }
        toast.success(
          `${t('Approved')} · ${formatAdjustUsd(
            res.data.balance_before
          )} → ${formatAdjustUsd(res.data.balance_after)}`
        )
        onOpenChange(false)
        onDone?.(res.data)
        return
      }

      const res = await reject.mutateAsync({
        id: row.id,
        reason: rejectReason.trim(),
      })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Action failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Rejected'))
      onOpenChange(false)
      onDone?.()
    } catch (err) {
      const code = (
        err as { code?: string; response?: { data?: { code?: string } } }
      )?.response?.data?.code
      const mapped = balanceAdjustErrorMessage(
        code,
        getServerErrorMessage(err, t('Action failed'))
      )
      const msg = t(mapped)
      setLocalError(msg)
      toast.error(msg)
      if (code === 'self_approve_forbidden') {
        toast.error(t('Self-approve forbidden when multiple admins'))
      }
    }
  }

  const title =
    action === 'approve'
      ? t('Approve balance adjustment')
      : t('Reject balance adjustment')
  const body =
    action === 'approve'
      ? t(
          'Approve executes AdjustQuota in the same transaction (pending→executed). Takes effect on submit. Self-approve needs a reason when you are the only admin.'
        )
      : t(
          'Reject leaves balances unchanged (pending→rejected). Takes effect on submit. Requires a reject reason.'
        )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{body}</DialogDescription>
        </DialogHeader>
        <div className='grid gap-3 text-sm'>
          <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
            <div>
              <span className='text-foreground font-medium'>ID: </span>
              <span className='font-mono'>{row.id}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Status')}:{' '}
              </span>
              <span className='font-mono'>{row.status}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>{t('User')}: </span>
              {row.user_id}
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Account')}:{' '}
              </span>
              <span className='font-mono'>{row.account}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Amount')}:{' '}
              </span>
              {formatAdjustUsd(row.amount)}
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Applicant')}:{' '}
              </span>
              {row.applicant_id}
              {isSelf ? (
                <span className='text-amber-700 dark:text-amber-300'>
                  {' '}
                  ({t('you')})
                </span>
              ) : null}
            </div>
            <div className='col-span-2 break-all'>
              <span className='text-foreground font-medium'>
                {t('Reason')}:{' '}
              </span>
              {row.reason}
            </div>
          </div>
          <p className='text-muted-foreground text-xs'>
            {row.account === 'bonus'
              ? t(
                  'This request changes bonus balance after another admin approves. Bonus is not withdrawable.'
                )
              : t(
                  'This request changes paid balance after another admin approves. It does not change bonus or invite earnings.'
                )}
          </p>
          {action === 'approve' && Math.abs(adjustQuotaToUsd(row.amount)) >= 100 ? (
            <div className='grid gap-1.5'>
              <Label htmlFor='ba-amount-confirm'>
                {t('Retype the USD amount to confirm this large change')}
              </Label>
              <Input
                id='ba-amount-confirm'
                type='number'
                step='0.01'
                value={amountConfirm}
                onChange={(e) => setAmountConfirm(e.target.value)}
                disabled={busy}
              />
            </div>
          ) : null}

          {action === 'approve' && isSelf ? (
            <div className='grid gap-1.5'>
              <Label htmlFor='ba-self-reason'>{t('Self-approve reason')}</Label>
              <Textarea
                id='ba-self-reason'
                value={selfReason}
                onChange={(e) => {
                  setSelfReason(e.target.value)
                }}
                rows={2}
                disabled={busy}
                placeholder={t('Required for sole-admin self-approve')}
              />
            </div>
          ) : null}

          {action === 'reject' ? (
            <div className='grid gap-1.5'>
              <Label htmlFor='ba-reject-reason'>{t('Reject reason')}</Label>
              <Textarea
                id='ba-reject-reason'
                value={rejectReason}
                onChange={(e) => {
                  setRejectReason(e.target.value)
                }}
                rows={2}
                disabled={busy}
                placeholder={t('Reject reason is required')}
              />
            </div>
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
            variant={action === 'reject' ? 'destructive' : 'default'}
            disabled={busy || row.status !== 'pending'}
            onClick={() => void handleConfirm()}
          >
            {balanceConfirmLabel(action, t)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
