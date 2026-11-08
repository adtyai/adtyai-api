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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useDraftNavigation } from '@/features/system-settings/hooks/use-draft-navigation'
import { ADTY_PERMISSIONS, canAdmin } from '@/lib/adty-permissions'
import { formatTimestamp } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  adminReferralKeys,
  getReferralSettings,
  saveReferralSettings,
  type ReferralSettingsUpdate,
} from '../api'
import {
  buildSettingsUpdate,
  MONEY_KEYS,
  RULE_KEYS,
  toDrafts,
  type Drafts,
  type MoneyKey,
  type RuleKey,
} from '../lib/settings'

export function SettingsPanel(props: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: adminReferralKeys.settings,
    queryFn: getReferralSettings,
  })
  const [drafts, setDrafts] = useState<Drafts | null>(null)
  const [enabled, setEnabled] = useState(false)
  const saved = query.data

  useEffect(() => {
    if (!saved) return
    setDrafts(toDrafts(saved))
    setEnabled(saved.enabled)
  }, [saved])

  const save = useMutation({
    mutationFn: (body: ReferralSettingsUpdate) => {
      if (
        !props.canWrite ||
        !canAdmin(
          useAuthStore.getState().auth.user,
          ADTY_PERMISSIONS.REFERRAL_SETTINGS
        )
      ) {
        throw new Error(t('No permission to perform this action'))
      }
      return saveReferralSettings(body)
    },
    onSuccess: (next) => {
      queryClient.setQueryData(adminReferralKeys.settings, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })

  useDraftNavigation(
    !!saved &&
      !!drafts &&
      (enabled !== saved.enabled ||
        JSON.stringify(drafts) !== JSON.stringify(toDrafts(saved)))
  )
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  if (!saved || !drafts) return <LoadingState />
  const disabled = !props.canWrite || save.isPending

  const labels: Record<MoneyKey | RuleKey, string> = {
    default_rate: t('Default commission rate (%)'),
    min_withdraw_usd: t('Minimum withdrawal (USD)'),
    usdt_withdraw_fee: t('USDT withdrawal fee (USDT)'),
    cookie_ttl_days: t('Referral cookie (days)'),
    freeze_days: t('Freeze period (days)'),
    cancel_window_minutes: t('Cancel window (minutes)'),
    self_invite_window_days: t('Self-invite check window (days)'),
    batch_register_threshold: t(
      'Sign-ups from one IP before freezing (0 = off)'
    ),
    batch_register_window_hours: t('Batch sign-up window (hours)'),
  }
  const set = (key: keyof Drafts, value: string) =>
    setDrafts((prev) => (prev ? { ...prev, [key]: value } : prev))
  const rateMissing = drafts.default_rate.trim() === ''

  return (
    <form
      className='flex max-w-3xl flex-col gap-6'
      onSubmit={(event) => {
        event.preventDefault()
        if (disabled) return
        save.mutate(buildSettingsUpdate(saved, drafts, enabled))
      }}
    >
      <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
        <div className='flex flex-col'>
          <Label htmlFor='referral-enabled'>{t('Referral program')}</Label>
          {rateMissing && (
            <span className='text-muted-foreground text-xs'>
              {t('Set the default rate first')}
            </span>
          )}
        </div>
        <Switch
          id='referral-enabled'
          checked={enabled}
          disabled={disabled || (rateMissing && !enabled)}
          onCheckedChange={setEnabled}
        />
      </div>
      <fieldset className='grid gap-4 sm:grid-cols-3'>
        <legend className='mb-2 text-sm font-medium'>
          {t('Money (no built-in values)')}
        </legend>
        {MONEY_KEYS.map((key) => (
          <div key={key} className='space-y-2'>
            <Label htmlFor={`referral-${key}`}>{labels[key]}</Label>
            <Input
              id={`referral-${key}`}
              disabled={disabled}
              type='number'
              min={0}
              step='0.01'
              value={drafts[key]}
              placeholder={t('Not set')}
              onChange={(event) => set(key, event.target.value)}
            />
            {drafts[key].trim() === '' && (
              <p className='text-warning text-xs'>{t('Not set')}</p>
            )}
          </div>
        ))}
      </fieldset>
      <div className='space-y-2'>
        <Label htmlFor='referral-cny-rate'>
          {t('CNY withdrawal rate (1 USD = ? CNY)')}
        </Label>
        <Input
          id='referral-cny-rate'
          disabled={disabled}
          className='w-40'
          type='number'
          min={0}
          step='0.0001'
          value={drafts.cny_rate}
          placeholder={t('Not set')}
          onChange={(event) => set('cny_rate', event.target.value)}
        />
        <p className='text-muted-foreground text-xs' data-testid='cny-updated'>
          {saved.withdraw_cny_rate_updated_at > 0
            ? t('Updated {{time}}', {
                time: formatTimestamp(saved.withdraw_cny_rate_updated_at),
              })
            : t('Not set')}
        </p>
      </div>
      <fieldset className='grid gap-4 sm:grid-cols-3'>
        <legend className='mb-2 text-sm font-medium'>{t('Rules')}</legend>
        {RULE_KEYS.map((key) => (
          <div key={key} className='space-y-2'>
            <Label htmlFor={`referral-${key}`}>{labels[key]}</Label>
            <Input
              id={`referral-${key}`}
              disabled={disabled}
              type='number'
              min={0}
              step={1}
              value={drafts[key]}
              onChange={(event) => set(key, event.target.value)}
            />
          </div>
        ))}
      </fieldset>
      <div>
        <Button type='submit' disabled={disabled}>
          {t('Save Changes')}
        </Button>
      </div>
    </form>
  )
}
