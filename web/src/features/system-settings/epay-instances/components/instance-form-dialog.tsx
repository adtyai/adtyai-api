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
import { Switch } from '@/components/ui/switch'
import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  useCreateEpayInstance,
  useUpdateEpayInstance,
} from '../hooks/use-epay-instances'
import { formatSecretFingerprint } from '../lib/display'
import type { EpayInstanceView } from '../types'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editing: EpayInstanceView | null
}

export function InstanceFormDialog({ open, onOpenChange, editing }: Props) {
  const { t } = useTranslation()
  const create = useCreateEpayInstance()
  const update = useUpdateEpayInstance()
  const [name, setName] = useState('')
  const [partnerId, setPartnerId] = useState('')
  const [payAddress, setPayAddress] = useState('')
  const [callbackBase, setCallbackBase] = useState('')
  const [dailyLimit, setDailyLimit] = useState('0')
  // Create-only: edit must not toggle enabled (enable/disable go through /enable|/disable + confirm).
  const [enabled, setEnabled] = useState(true)
  const [secret, setSecret] = useState('')
  const [localError, setLocalError] = useState<string | null>(null)

  const isEdit = Boolean(editing)
  const secretChanging = isEdit && secret.trim().length > 0
  const busy = create.isPending || update.isPending

  useEffect(() => {
    if (!open) return
    if (editing) {
      setName(editing.name)
      setPartnerId(editing.partner_id)
      setPayAddress(editing.pay_address)
      setCallbackBase(editing.callback_base || '')
      setDailyLimit(String(editing.daily_limit ?? 0))
    } else {
      setName('')
      setPartnerId('')
      setPayAddress('')
      setCallbackBase('')
      setDailyLimit('0')
      setEnabled(true)
    }
    setSecret('')
    setLocalError(null)
    create.reset()
    update.reset()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, editing?.id])

  async function handleSave() {
    setLocalError(null)
    const limit = Number(dailyLimit)
    if (!name.trim() || !partnerId.trim() || !payAddress.trim()) {
      setLocalError(t('Name, partner id and pay address are required'))
      return
    }
    if (!isEdit && !secret.trim()) {
      setLocalError(t('Secret is required'))
      return
    }
    if (Number.isNaN(limit) || limit < 0) {
      setLocalError(t('Daily limit must be a non-negative number'))
      return
    }

    try {
      if (!isEdit) {
        const res = await create.mutateAsync({
          name: name.trim(),
          partner_id: partnerId.trim(),
          secret: secret.trim(),
          pay_address: payAddress.trim(),
          callback_base: callbackBase.trim(),
          daily_limit: limit,
          enabled,
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

      // Edit: never PUT enabled - enable/disable only via list /enable / /disable + confirm.
      if (!editing) {
        setLocalError(t('Update failed'))
        return
      }
      const input: {
        name: string
        partner_id: string
        pay_address: string
        callback_base: string
        daily_limit: number
        secret?: string
        confirm?: boolean
      } = {
        name: name.trim(),
        partner_id: partnerId.trim(),
        pay_address: payAddress.trim(),
        callback_base: callbackBase.trim(),
        daily_limit: limit,
      }
      if (secretChanging) {
        input.secret = secret.trim()
        input.confirm = true
      }
      const res = await update.mutateAsync({ id: editing.id, input })
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
            {isEdit ? t('Edit epay instance') : t('Create epay instance')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Secrets are write-only. Responses only show secret_set and fingerprint - never plaintext.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='grid max-h-[60vh] gap-3 overflow-y-auto pr-1 text-sm'>
          {isEdit && editing ? (
            <div className='text-muted-foreground text-xs'>
              ID <span className='text-foreground font-mono'>{editing.id}</span>
              {' · '}
              {editing.secret_set ? t('Secret set') : t('Secret missing')}
              {editing.secret_fingerprint
                ? ` · ${t('Fingerprint')} ${formatSecretFingerprint(editing.secret_fingerprint)}`
                : ''}
              {' · '}
              {editing.status === 'enabled' ? t('Enabled') : t('Disabled')}
              <span className='mt-1 block'>
                {t(
                  'Enable or disable only via Enable / Disable on the list. Do not PUT enabled.'
                )}
              </span>
            </div>
          ) : null}

          <div className='grid gap-1.5'>
            <Label htmlFor='epay-name'>{t('Name')}</Label>
            <Input
              id='epay-name'
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='epay-pid'>{t('Partner ID')}</Label>
            <Input
              id='epay-pid'
              className='font-mono text-xs'
              value={partnerId}
              onChange={(e) => {
                setPartnerId(e.target.value)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='epay-addr'>{t('Pay address')}</Label>
            <Input
              id='epay-addr'
              className='font-mono text-xs'
              value={payAddress}
              onChange={(e) => {
                setPayAddress(e.target.value)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='epay-cb'>{t('Callback base')}</Label>
            <Input
              id='epay-cb'
              className='font-mono text-xs'
              value={callbackBase}
              onChange={(e) => {
                setCallbackBase(e.target.value)
              }}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='epay-limit'>{t('Daily limit')}</Label>
            <Input
              id='epay-limit'
              type='number'
              min={0}
              value={dailyLimit}
              onChange={(e) => {
                setDailyLimit(e.target.value)
              }}
            />
            <p className='text-muted-foreground text-[11.5px]'>
              {t('0 means unlimited')}
            </p>
          </div>
          {!isEdit ? (
            <div className='flex items-center justify-between rounded-lg border px-3 py-2'>
              <Label htmlFor='epay-enabled'>{t('Enabled')}</Label>
              <Switch
                id='epay-enabled'
                checked={enabled}
                onCheckedChange={(v) => {
                  setEnabled(v)
                }}
              />
            </div>
          ) : null}
          <div className='grid gap-1.5'>
            <Label htmlFor='epay-secret'>
              {isEdit ? t('New secret (optional)') : t('Secret')}
            </Label>
            <Input
              id='epay-secret'
              type='password'
              autoComplete='new-password'
              placeholder={
                isEdit
                  ? t('Leave blank to keep current secret')
                  : t('Merchant secret')
              }
              value={secret}
              onChange={(e) => {
                setSecret(e.target.value)
              }}
            />
            <p className='text-muted-foreground text-[11.5px]'>
              {t('Never re-displayed after save; only fingerprint is shown.')}
            </p>
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
            disabled={busy}
            onClick={() => void handleSave()}
          >
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
