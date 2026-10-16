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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  useCreateEnterprisePricing,
  useUpdateEnterprisePricing,
} from '../hooks/use-enterprise-pricing'
import {
  discountToPercent,
  isValidDiscount,
  percentToDiscount,
} from '../lib/discount'
import type { EnterprisePriceRule } from '../types'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editing: EnterprisePriceRule | null
}

function toDatetimeLocal(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function fromDatetimeLocal(local: string): string | null {
  const v = local.trim()
  if (!v) return null
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString()
}

export function RuleFormDialog({ open, onOpenChange, editing }: Props) {
  const { t } = useTranslation()
  const create = useCreateEnterprisePricing()
  const update = useUpdateEnterprisePricing()
  const [userId, setUserId] = useState('')
  const [groupCode, setGroupCode] = useState('')
  const [modelName, setModelName] = useState('')
  const [discountPct, setDiscountPct] = useState('100')
  const [expiresLocal, setExpiresLocal] = useState('')
  const [confirmStep, setConfirmStep] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)

  const isEdit = Boolean(editing)
  const busy = create.isPending || update.isPending

  useEffect(() => {
    if (!open) return
    if (editing) {
      setUserId(String(editing.user_id))
      setGroupCode(editing.group_code ?? '')
      setModelName(editing.model_name ?? '')
      setDiscountPct(String(discountToPercent(editing.discount)))
      setExpiresLocal(toDatetimeLocal(editing.expires_at))
    } else {
      setUserId('')
      setGroupCode('')
      setModelName('')
      setDiscountPct('100')
      setExpiresLocal('')
    }
    setConfirmStep(false)
    setLocalError(null)
    create.reset()
    update.reset()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, editing?.id])

  async function handleSave() {
    setLocalError(null)
    const uid = Number(userId)
    if (!isEdit && (!Number.isInteger(uid) || uid <= 0)) {
      setLocalError(t('User ID is required'))
      return
    }
    const pct = Number(discountPct)
    if (!Number.isFinite(pct) || pct < 10 || pct > 100) {
      setLocalError(t('Discount must be between 10% and 100%'))
      return
    }
    const discount = percentToDiscount(pct)
    if (!isValidDiscount(discount)) {
      setLocalError(t('Discount must be between 10% and 100%'))
      return
    }
    if (expiresLocal.trim()) {
      const exp = fromDatetimeLocal(expiresLocal)
      if (!exp) {
        setLocalError(t('Invalid expires at'))
        return
      }
    }

    if (!confirmStep) {
      setConfirmStep(true)
      return
    }

    const group = groupCode.trim()
    const model = modelName.trim()
    const expiresIso = fromDatetimeLocal(expiresLocal)

    try {
      if (!isEdit) {
        const res = await create.mutateAsync({
          user_id: uid,
          group_code: group || null,
          model_name: model || null,
          discount,
          expires_at: expiresIso,
          confirm: true,
        })
        if (!res.success) {
          const msg = getServerErrorMessage(res, t('Create failed'))
          setLocalError(msg)
          toast.error(msg)
          return
        }
        toast.success(t('Created'))
        onOpenChange(false)
        return
      }

      if (!editing) return
      const hadExpires = Boolean(editing.expires_at)
      const clearingExpires = hadExpires && !expiresLocal.trim()
      const res = await update.mutateAsync({
        id: editing.id,
        input: {
          group_code: group,
          model_name: model,
          discount,
          expires_at: clearingExpires ? '' : expiresIso,
          confirm: true,
        },
      })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Update failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Saved'))
      onOpenChange(false)
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Save failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('Edit enterprise pricing rule')
              : t('Create enterprise pricing rule')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Empty group/model means wildcard (all). Discount 10%–100% maps to API 0.1–1. Writes require a second confirmation.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid max-h-[60vh] gap-3 overflow-y-auto pr-1 text-sm'>
          {isEdit && editing ? (
            <div className='text-muted-foreground text-xs'>
              ID <span className='text-foreground font-mono'>{editing.id}</span>
              {' · '}
              {t('User')}{' '}
              <span className='text-foreground font-mono'>
                {editing.user_id}
              </span>
            </div>
          ) : (
            <div className='grid gap-1.5'>
              <Label htmlFor='ep-user'>{t('User ID')}</Label>
              <Input
                id='ep-user'
                type='number'
                min={1}
                className='font-mono text-xs'
                value={userId}
                onChange={(e) => {
                  setUserId(e.target.value)
                  setConfirmStep(false)
                }}
              />
            </div>
          )}

          <div className='grid gap-1.5'>
            <Label htmlFor='ep-group'>{t('Group code')}</Label>
            <Input
              id='ep-group'
              className='font-mono text-xs'
              placeholder={t('Empty = all groups')}
              value={groupCode}
              onChange={(e) => {
                setGroupCode(e.target.value)
                setConfirmStep(false)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='ep-model'>{t('Model name')}</Label>
            <Input
              id='ep-model'
              className='font-mono text-xs'
              placeholder={t('Empty = all models')}
              value={modelName}
              onChange={(e) => {
                setModelName(e.target.value)
                setConfirmStep(false)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='ep-discount'>{t('Discount (%)')}</Label>
            <Input
              id='ep-discount'
              type='number'
              min={10}
              max={100}
              step={0.1}
              value={discountPct}
              onChange={(e) => {
                setDiscountPct(e.target.value)
                setConfirmStep(false)
              }}
            />
            <p className='text-muted-foreground text-[11.5px]'>
              {t('10–100%; submitted as 0.1–1 multiplier after group ratio.')}
              {Number.isFinite(Number(discountPct))
                ? ` → API ${percentToDiscount(Number(discountPct))}`
                : ''}
            </p>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='ep-exp'>{t('Expires at')}</Label>
            <Input
              id='ep-exp'
              type='datetime-local'
              value={expiresLocal}
              onChange={(e) => {
                setExpiresLocal(e.target.value)
                setConfirmStep(false)
              }}
            />
            <p className='text-muted-foreground text-[11.5px]'>
              {t('Leave empty for no expiry. Clear on edit sends "" to API.')}
            </p>
          </div>

          {confirmStep ? (
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
            disabled={busy}
            onClick={() => void handleSave()}
          >
            {confirmStep ? t('Confirm save') : t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
