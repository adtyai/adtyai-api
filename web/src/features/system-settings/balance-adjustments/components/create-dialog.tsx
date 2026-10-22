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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useCreateBalanceAdjustment } from '../hooks/use-balance-adjustments'
import { adjustUsdToQuota } from '../lib/amount'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone?: () => void
}

export function CreateBalanceAdjustDialog({
  open,
  onOpenChange,
  onDone,
}: Props) {
  const { t } = useTranslation()
  const mut = useCreateBalanceAdjustment()
  const [userId, setUserId] = useState('')
  const [account, setAccount] = useState<'quota' | 'bonus'>('quota')
  const [amountUsd, setAmountUsd] = useState('')
  const [reason, setReason] = useState('')
  const [chatwoot, setChatwoot] = useState('')
  const [amountConfirm, setAmountConfirm] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setUserId('')
      setAccount('quota')
      setAmountUsd('')
      setReason('')
      setChatwoot('')
      setAmountConfirm('')
      setLocalError(null)
      mut.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  async function handleSubmit() {
    setLocalError(null)
    const uid = Number(userId)
    if (!Number.isInteger(uid) || uid <= 0) {
      const msg = t('User ID is required')
      setLocalError(msg)
      return
    }
    const usd = Number(amountUsd)
    if (!Number.isFinite(usd) || usd === 0) {
      const msg = t('Amount must be non-zero')
      setLocalError(msg)
      return
    }
    if (!reason.trim()) {
      const msg = t('Reason is required')
      setLocalError(msg)
      return
    }
    if (Math.abs(usd) >= 100 && Math.abs(Number(amountConfirm) - usd) > 1e-6) {
      const msg = t('Retype the USD amount to confirm this large change')
      setLocalError(msg)
      return
    }
    const amount = adjustUsdToQuota(usd)
    if (amount === 0) {
      const msg = t('Amount must be non-zero')
      setLocalError(msg)
      return
    }
    try {
      const res = await mut.mutateAsync({
        user_id: uid,
        account,
        amount,
        reason: reason.trim(),
        chatwoot_conversation_id: chatwoot.trim() || undefined,
      })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Failed to create request'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Balance adjust request created'))
      onOpenChange(false)
      onDone?.()
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Failed to create request'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('New balance adjustment')}</DialogTitle>
          <DialogDescription>
            {t(
              'Creates a pending request only; balance changes after approve. Amount is entered in USD and converted with quotaPerUnit.'
            )}{' '}
            {account === 'bonus'
              ? t(
                  'This request changes bonus balance after another admin approves. Bonus is not withdrawable.'
                )
              : t(
                  'This request changes paid balance after another admin approves. It does not change bonus or invite earnings.'
                )}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3 text-sm'>
          <div className='grid gap-1.5'>
            <Label htmlFor='ba-user'>{t('User ID')}</Label>
            <Input
              id='ba-user'
              type='number'
              min={1}
              value={userId}
              onChange={(e) => setUserId(e.target.value)}
              disabled={mut.isPending}
              className='font-mono'
            />
          </div>
          <div className='grid gap-1.5'>
            <Label>{t('Account')}</Label>
            <Select
              items={[
                { value: 'quota', label: 'quota' },
                { value: 'bonus', label: 'bonus' },
              ]}
              value={account}
              onValueChange={(v) => {
                if (v === 'quota' || v === 'bonus') setAccount(v)
              }}
            >
              <SelectTrigger className='h-9'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='quota'>quota</SelectItem>
                <SelectItem value='bonus'>bonus</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='ba-amount'>
              {t('Amount (USD, negative to deduct)')}
            </Label>
            <Input
              id='ba-amount'
              type='number'
              step='0.01'
              value={amountUsd}
              onChange={(e) => setAmountUsd(e.target.value)}
              disabled={mut.isPending}
            />
          </div>
          {Math.abs(Number(amountUsd)) >= 100 ? (
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
                disabled={mut.isPending}
              />
            </div>
          ) : null}
          <div className='grid gap-1.5'>
            <Label htmlFor='ba-reason'>{t('Reason')}</Label>
            <Textarea
              id='ba-reason'
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              disabled={mut.isPending}
              placeholder={t('Reason (required)')}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='ba-cw'>{t('Chatwoot conversation ID')}</Label>
            <Input
              id='ba-cw'
              value={chatwoot}
              onChange={(e) => setChatwoot(e.target.value)}
              disabled={mut.isPending}
              placeholder={t('Optional')}
            />
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
            disabled={mut.isPending}
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={mut.isPending}
            onClick={() => void handleSubmit()}
          >
            {t('Create request')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
