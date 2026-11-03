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

import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getMarketingSettings,
  marketingKeys,
  saveMarketingSecrets,
  saveMarketingSettings,
  type MarketingSettings,
  type MarketingSettingsUpdate,
} from '../api'
import {
  BOOL_KEYS,
  buildMarketingSettingsUpdate,
  NUMBER_KEYS,
  REQUIRED_TEXT_KEYS,
  TEXT_KEYS,
  toMarketingDrafts,
  type MarketingDrafts,
} from '../lib/settings'

type SettingsPanelProps = { canWrite: boolean }

/** Every adty_marketing_setting key (convention I), with "Not configured" next to required blanks. */
export function SettingsPanel(props: SettingsPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: marketingKeys.settings,
    queryFn: getMarketingSettings,
  })
  const [drafts, setDrafts] = useState<MarketingDrafts | null>(null)
  const saved = query.data

  useEffect(() => {
    if (saved) setDrafts(toMarketingDrafts(saved))
  }, [saved])

  const root =
    useAuthStore((state) => state.auth.user?.role) === ROLE.SUPER_ADMIN
  const save = useMutation({
    mutationFn: (body: MarketingSettingsUpdate) => saveMarketingSettings(body),
    onSuccess: (next) => {
      queryClient.setQueryData(marketingKeys.settings, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  const saveSecret = useMutation({
    mutationFn: () =>
      saveMarketingSecrets({
        webhook_secret: drafts?.webhook_secret.trim(),
      }),
    onSuccess: (next) => {
      queryClient.setQueryData(marketingKeys.settings, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })

  if (!saved || !drafts) return <LoadingState />

  const labels: Record<string, string> = {
    from_address: t('Marketing sender address'),
    footer_company: t('Sender name in the email footer'),
    footer_address: t('Contact address in the email footer'),
    resend_segment_prefix: t('Resend segment name prefix'),
    consent_version: t('Consent wording version'),
    eu_countries: t('Countries treated as EU (ISO codes, comma separated)'),
    sync_batch_size: t('Contacts per sync batch'),
    sync_interval_seconds: t('Sync interval (seconds)'),
    api_rate_per_second: t('Resend requests per second'),
    consent_default_non_eu: t('Tick the sign-up checkbox outside the EU'),
    open_tracking: t('Open tracking'),
    click_tracking: t('Click tracking'),
    include_site_customers_default: t(
      'Include sub-site customers in campaigns by default'
    ),
    promo_enabled: t('Promo codes'),
  }
  const placeholders: Record<string, string> = {
    from_address: 'ADTY AI <news@news.example.com>',
  }
  const set = <K extends keyof MarketingDrafts>(
    key: K,
    value: MarketingDrafts[K]
  ) => setDrafts((prev) => (prev ? { ...prev, [key]: value } : prev))

  // The switch stays off until every required field has a value.
  const missing = missingAfterDrafts(saved, drafts)
  const canEnable = missing.length === 0
  const disabled = !props.canWrite || save.isPending

  return (
    <form
      className='flex max-w-3xl flex-col gap-6'
      onSubmit={(event) => {
        event.preventDefault()
        save.mutate(buildMarketingSettingsUpdate(saved, drafts))
      }}
    >
      <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
        <div className='flex flex-col'>
          <Label htmlFor='marketing-enabled'>{t('Marketing emails')}</Label>
          {!canEnable && (
            <span className='text-muted-foreground text-xs'>
              {t('Fill in the items marked Not configured first')}
            </span>
          )}
        </div>
        <Switch
          id='marketing-enabled'
          checked={drafts.enabled}
          disabled={disabled || (!canEnable && !drafts.enabled)}
          onCheckedChange={(value) => set('enabled', value)}
        />
      </div>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>
          {t('Sender and footer')}
        </legend>
        {TEXT_KEYS.map((key) => (
          <div key={key} className='space-y-2'>
            <Label htmlFor={`marketing-${key}`}>{labels[key]}</Label>
            <Input
              id={`marketing-${key}`}
              value={drafts[key]}
              disabled={disabled}
              placeholder={placeholders[key]}
              onChange={(event) => set(key, event.target.value)}
            />
            {REQUIRED_TEXT_KEYS.includes(key) && drafts[key].trim() === '' && (
              <p className='text-warning text-xs'>{t('Not configured')}</p>
            )}
          </div>
        ))}
        <div className='space-y-2'>
          <Label htmlFor='marketing-webhook-secret'>
            {t('Resend webhook signing secret')}
          </Label>
          <Input
            id='marketing-webhook-secret'
            type='password'
            autoComplete='off'
            value={drafts.webhook_secret}
            disabled={disabled || !root}
            placeholder={
              saved.webhook_secret_configured
                ? t('Saved. Enter a new value to replace it')
                : 'whsec_…'
            }
            onChange={(event) => set('webhook_secret', event.target.value)}
          />
          {!root && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Only a super admin can change the webhook signing secret'
              )}
            </p>
          )}
          {root && props.canWrite && (
            <Button
              type='button'
              variant='outline'
              disabled={
                saveSecret.isPending || drafts.webhook_secret.trim() === ''
              }
              onClick={() => saveSecret.mutate()}
            >
              {t('Save signing secret')}
            </Button>
          )}
          {!saved.webhook_secret_configured &&
            drafts.webhook_secret.trim() === '' && (
              <p className='text-warning text-xs'>{t('Not configured')}</p>
            )}
        </div>
        <div className='space-y-2'>
          <Label>{t('Resend API key')}</Label>
          <p
            className='text-muted-foreground text-xs'
            data-testid='resend-key-state'
          >
            {saved.resend_key_configured
              ? t('Configured in System Settings → Email')
              : t('Not configured')}
          </p>
        </div>
      </fieldset>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>{t('Rules')}</legend>
        {NUMBER_KEYS.map((key) => (
          <div key={key} className='space-y-2'>
            <Label htmlFor={`marketing-${key}`}>{labels[key]}</Label>
            <Input
              id={`marketing-${key}`}
              type='number'
              min={1}
              step={1}
              className='w-32'
              value={drafts[key]}
              disabled={disabled}
              onChange={(event) => set(key, event.target.value)}
            />
          </div>
        ))}
        {BOOL_KEYS.map((key) => (
          <div
            key={key}
            className='flex items-center justify-between gap-4 rounded-lg border p-3'
          >
            <Label htmlFor={`marketing-${key}`}>{labels[key]}</Label>
            <Switch
              id={`marketing-${key}`}
              checked={drafts[key]}
              disabled={disabled}
              onCheckedChange={(value) => set(key, value)}
            />
          </div>
        ))}
      </fieldset>

      {props.canWrite && (
        <div>
          <Button type='submit' disabled={save.isPending}>
            {t('Save Changes')}
          </Button>
        </div>
      )}
    </form>
  )
}

function missingAfterDrafts(
  saved: MarketingSettings,
  drafts: MarketingDrafts
): string[] {
  const missing = REQUIRED_TEXT_KEYS.filter((key) => drafts[key].trim() === '')
  // 签名密钥不随普通设置保存，没点「保存签名密钥」就不能当成已配置。
  return [
    ...missing,
    ...(saved.webhook_secret_configured ? [] : ['webhook_secret']),
    ...(saved.resend_key_configured ? [] : ['resend_api_key']),
  ]
}
