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
import { Link } from '@tanstack/react-router'
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
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { useUpdateBusinessParam } from '../hooks/use-business-params'
import {
  BUSINESS_PARAM_DESCRIPTIONS,
  settingKeyLabel,
  formatRange,
  formatSnapshot,
  parsePutValue,
} from '../lib/groups'
import type { BusinessParamRow } from '../types'

type EditParamDialogProps = {
  row: BusinessParamRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EditParamDialog({
  row,
  open,
  onOpenChange,
}: EditParamDialogProps) {
  const { t } = useTranslation()
  const update = useUpdateBusinessParam()
  const [draft, setDraft] = useState('')
  const [confirmStep, setConfirmStep] = useState(false)
  const [localError, setLocalError] = useState<string | null>(null)

  useEffect(() => {
    if (open && row) {
      if (row.type === 'json') {
        try {
          setDraft(JSON.stringify(JSON.parse(row.value), null, 2))
        } catch {
          setDraft(row.value)
        }
      } else {
        setDraft(row.value)
      }
      setConfirmStep(false)
      setLocalError(null)
      update.reset()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset only when dialog opens for a row
  }, [open, row?.key])

  if (!row) return null

  const description =
    BUSINESS_PARAM_DESCRIPTIONS[row.key] || row.description || row.notes || ''

  async function handleSave() {
    const current = row
    if (!current) return
    setLocalError(null)
    let value: string | number | unknown
    try {
      value = parsePutValue(current, draft)
    } catch (err) {
      // parsePutValue only throws i18n keys
      const msg = t(err instanceof Error ? err.message : 'Invalid value')
      setLocalError(msg)
      return
    }

    if (!confirmStep) {
      setConfirmStep(true)
      return
    }

    try {
      const res = await update.mutateAsync({ key: current.key, value })
      if (!res.success) {
        const msg = getServerErrorMessage(res, t('Update failed'))
        setLocalError(msg)
        toast.error(msg)
        return
      }
      toast.success(t('Saved'))
      onOpenChange(false)
    } catch (err) {
      const msg = getServerErrorMessage(err, t('Update failed'))
      setLocalError(msg)
      toast.error(msg)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='text-base'>
            {t(settingKeyLabel(row.key))}
          </DialogTitle>
          <DialogDescription>{t(description) || description}</DialogDescription>
        </DialogHeader>

        <div className='grid gap-3 text-sm'>
          <div className='text-muted-foreground grid grid-cols-2 gap-2 text-xs'>
            <div>
              <span className='text-foreground font-medium'>
                {t('Default')}:{' '}
              </span>
              <span className='font-mono'>{row.default || '—'}</span>
            </div>
            <div>
              <span className='text-foreground font-medium'>
                {t('Range')}:{' '}
              </span>
              <span className='font-mono'>{t(formatRange(row))}</span>
            </div>
            <div className='col-span-2'>
              <span className='text-foreground font-medium'>
                {t('Effect')}:{' '}
              </span>
              {t(formatSnapshot(row.snapshot))}
            </div>
            {row.page ? (
              <div>
                <span className='text-foreground font-medium'>
                  {t('Page')}:{' '}
                </span>
                {row.page}
              </div>
            ) : null}
          </div>

          <div className='grid gap-1.5'>
            <Label htmlFor='bp-value'>{t('New value')}</Label>
            <ParamValueEditor
              row={row}
              draft={draft}
              onDraftChange={(next) => {
                setDraft(next)
                setConfirmStep(false)
                setLocalError(null)
              }}
            />
            {row.key === 'DailyReportTime' ? (
              <p className='text-muted-foreground text-xs'>
                {t('Timezone is Asia/Shanghai. Format HH:mm (e.g. 09:00).')}{' '}
                {t(
                  'Daily report is sent via the Telegram pending alert channel.'
                )}{' '}
                <Link
                  to='/system-settings/alerts'
                  className='text-foreground font-medium underline underline-offset-2'
                >
                  {t('Open Telegram alerts')}
                </Link>
              </p>
            ) : null}
            {row.key === 'WalletAccountReconcileFrom' ? (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Once a date is set, daily reconcile also checks paid and bonus accounts separately. Report only; does not affect whether it balances. Leave empty to turn off.'
                )}
              </p>
            ) : null}
          </div>

          {confirmStep ? (
            <div className='rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/40 dark:text-amber-100'>
              {t(
                'Second confirmation required before saving. Money-related values only affect new orders.'
              )}
              <div className='mt-1 font-mono'>
                {row.value || '—'} → {draft.trim() || '—'}
              </div>
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
            disabled={update.isPending}
            onClick={() => {
              if (confirmStep) {
                setConfirmStep(false)
                return
              }
              onOpenChange(false)
            }}
          >
            {confirmStep ? t('Back') : t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={update.isPending || draft.trim() === ''}
            onClick={() => void handleSave()}
          >
            {saveButtonLabel(update.isPending, confirmStep, t)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function saveButtonLabel(
  pending: boolean,
  confirmStep: boolean,
  t: (key: string) => string
) {
  if (pending) return t('Saving...')
  if (confirmStep) return t('Confirm save')
  return t('Continue')
}

function ParamValueEditor({
  row,
  draft,
  onDraftChange,
}: {
  row: BusinessParamRow
  draft: string
  onDraftChange: (next: string) => void
}) {
  if (row.type === 'enum' && row.enum?.length) {
    return (
      <Select
        items={row.enum.map((v) => ({ value: v, label: v }))}
        value={draft}
        onValueChange={(v) => {
          if (v !== null) onDraftChange(v)
        }}
      >
        <SelectTrigger id='bp-value' className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {row.enum.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    )
  }
  if (row.type === 'json') {
    return (
      <Textarea
        id='bp-value'
        className='min-h-40 font-mono text-xs'
        value={draft}
        onChange={(e) => onDraftChange(e.target.value)}
      />
    )
  }
  return (
    <Input
      id='bp-value'
      className='font-mono'
      value={draft}
      onChange={(e) => onDraftChange(e.target.value)}
    />
  )
}
