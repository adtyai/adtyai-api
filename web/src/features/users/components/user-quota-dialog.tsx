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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import { adjustUserQuota } from '../api'
import type { QuotaAdjustMode } from '../types'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentQuota: number
  onSuccess: () => void
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(false)
  // adty task 7.2: a reason is required; one idempotency key per dialog so a
  // retry after a network failure cannot apply the change twice.
  const [reason, setReason] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState(() =>
    crypto.randomUUID()
  )
  const reasonLength = [...reason.trim()].length
  const reasonValid = reasonLength > 0 && reasonLength <= 200

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const amountValue = Number.parseFloat(amount) || 0
  const quotaValue = parseQuotaFromDollars(Math.abs(amountValue))
  const overrideQuota = parseQuotaFromDollars(amountValue)
  const floorBlocked =
    (mode === 'override' && overrideQuota < 0) ||
    (mode === 'subtract' && props.currentQuota - quotaValue < 0)

  const getPreviewText = () => {
    if (floorBlocked) return t('Adjustment cannot make the balance negative')
    const current = props.currentQuota
    const val = quotaValue
    switch (mode) {
      case 'add':
        return `${t('Current quota')}: ${formatQuota(current)}  +${formatQuota(val)} = ${formatQuota(current + val)}`
      case 'subtract':
        return `${t('Current quota')}: ${formatQuota(current)}  -${formatQuota(val)} = ${formatQuota(current - val)}`
      case 'override': {
        const overrideQuota = parseQuotaFromDollars(amountValue)
        return `${t('Current quota')}: ${formatQuota(current)} → ${formatQuota(overrideQuota)}`
      }
      default:
        return ''
    }
  }

  const resetForm = () => {
    setAmount('')
    setMode('add')
    setReason('')
    setIdempotencyKey(crypto.randomUUID())
  }

  const handleConfirm = async () => {
    if (!reasonValid || floorBlocked) return
    if (!amount && mode !== 'override') return
    if (quotaValue <= 0 && mode !== 'override') return

    setLoading(true)
    try {
      const value =
        mode === 'override' ? parseQuotaFromDollars(amountValue) : quotaValue
      const result = await adjustUserQuota({
        id: props.userId,
        action: 'add_quota',
        mode,
        value: mode === 'override' ? value : Math.abs(value),
        reason: reason.trim(),
        idempotency_key: idempotencyKey,
      })
      if (result.success) {
        toast.success(t('Quota adjusted successfully'))
        resetForm()
        props.onOpenChange(false)
        props.onSuccess()
      } else {
        handleServerError(result, t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      handleServerError(e, t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const handleCancel = () => {
    resetForm()
    props.onOpenChange(false)
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Adjust Quota')}
      description={t('Select an operation mode and enter the amount')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={loading || !reasonValid || floorBlocked}
          >
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='text-muted-foreground text-sm'>{getPreviewText()}</div>

        <div className='space-y-2'>
          <Label>{t('Mode')}</Label>
          <div className='flex gap-1'>
            {(['add', 'subtract', 'override'] as const).map((m) => (
              <Button
                key={m}
                type='button'
                variant='outline'
                size='sm'
                className={cn(
                  mode === m &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => {
                  setMode(m)
                  setAmount('')
                }}
              >
                {m === 'add' && t('Add')}
                {!(m === 'add') && m === 'subtract' && t('Subtract')}
                {!(m === 'add') && !(m === 'subtract') && t('Override')}
              </Button>
            ))}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>
            {t('Amount')} ({currencyLabel})
          </Label>
          <Input
            type='number'
            step={tokensOnly ? 1 : 0.000001}
            min={0}
            placeholder={placeholder}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleConfirm()
            }}
          />
        </div>

        <div className='space-y-2'>
          <Label htmlFor='quota-reason'>{t('Reason (admins only)')}</Label>
          <Textarea
            id='quota-reason'
            value={reason}
            maxLength={200}
            aria-invalid={reason !== '' && !reasonValid}
            onChange={(e) => setReason(e.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t('{{count}}/200, not shown to the user', { count: reasonLength })}
          </p>
        </div>
      </div>
    </Dialog>
  )
}
