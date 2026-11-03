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
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getResellerSettings,
  saveResellerSettings,
  siteAdminKeys,
} from '../api'
import {
  buildResellerUpdate,
  cloudflareReady,
  missingMoneyKeys,
  newRowKey,
  MONEY_KEYS,
  RULE_KEYS,
  SWITCH_KEYS,
  toResellerDrafts,
  VISIBILITY_KEYS,
  type MoneyKey,
  type ResellerDrafts,
  type RuleKey,
  type SwitchKey,
  type VisibilityKey,
} from '../lib/settings'
import type { ResellerSettingsUpdate } from '../types'
import { SecretField, SwitchRow, TextField } from './setting-fields'

/**
 * Every adty_reseller_setting key (task 11.9, convention I). Money keys show
 * "Not configured" until set and keep the master switch off; custom and
 * sender domains stay off until Cloudflare and Resend are filled in. The
 * server checks the same rules; this only explains them up front.
 */
export function SettingsPanel(props: { canWrite: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: siteAdminKeys.settings,
    queryFn: getResellerSettings,
  })
  const saved = query.data
  const [drafts, setDrafts] = useState<ResellerDrafts | null>(null)

  useEffect(() => {
    if (saved) setDrafts(toResellerDrafts(saved))
  }, [saved])

  const save = useMutation({
    mutationFn: (body: ResellerSettingsUpdate) => saveResellerSettings(body),
    onSuccess: (next) => {
      queryClient.setQueryData(siteAdminKeys.settings, next)
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })

  if (!saved || !drafts) return <LoadingState />

  const disabled = !props.canWrite || save.isPending
  const set = <K extends keyof ResellerDrafts>(
    key: K,
    value: ResellerDrafts[K]
  ) => setDrafts((prev) => (prev ? { ...prev, [key]: value } : prev))

  const missingMoney = missingMoneyKeys(drafts)
  const rootsSet = drafts.root_domains.trim() !== ''
  const canEnable = missingMoney.length === 0 && rootsSet
  const cfReady = cloudflareReady(saved, drafts)

  const moneyLabels: Record<MoneyKey, string> = {
    cost_factor: t('Cost factor d (0 to 1)'),
    min_cost_factor: t('Minimum cost factor'),
    max_markup: t('Markup limit k (1 to 100)'),
    max_site_bonus_extra: t('Extra top-up bonus over the main site (points)'),
    trial_grant_max_usd: t('Trial credit per grant (USD)'),
    trial_grant_daily_usd: t('Trial credit per day (USD)'),
  }
  const ruleLabels: Record<RuleKey, string> = {
    markup_increase_delay_hours: t('Markup increase delay (hours)'),
    settle_hour: t('Settlement hour (0 to 23, server time)'),
    freeze_days: t('Freeze days'),
    close_notice_days: t('Close notice days'),
    fingerprint_window_days: t('Fingerprint window (days)'),
    customer_lookup_per_hour: t('Customer lookups per hour'),
    max_custom_domains_per_site: t('Custom domains per site'),
    custom_domain_creates_per_hour: t('Custom domains added per hour'),
  }
  const switchLabels: Record<SwitchKey, string> = {
    site_login_only: t('Site customers sign in only on their site'),
    invite_code_enabled: t('Site opening invite codes'),
    self_use_check_enabled: t('Check for self-use'),
    migration_check_enabled: t('Check for main site migration'),
  }
  const visibilityLabels: Record<VisibilityKey, string> = {
    customer_email: t('Customer email'),
    customer_balance: t('Customer balance'),
    customer_topups: t('Customer top-ups'),
    customer_usage: t('Customer usage'),
    request_logs: t('Request logs'),
    customer_tokens: t('Customer API keys'),
  }

  let enableHint: string | undefined
  if (missingMoney.length > 0) {
    enableHint = t('Set every money setting marked Not configured first.')
  } else if (!rootsSet) {
    enableHint = t('Add at least one root domain first.')
  }

  return (
    <form
      className='flex max-w-3xl flex-col gap-6'
      onSubmit={(event) => {
        event.preventDefault()
        save.mutate(buildResellerUpdate(saved, drafts))
      }}
    >
      <SwitchRow
        label={t('Reseller sites')}
        checked={drafts.enabled}
        disabled={disabled || (!canEnable && !drafts.enabled)}
        hint={enableHint}
        onCheckedChange={(value) => set('enabled', value)}
      />

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>{t('Money')}</legend>
        {MONEY_KEYS.map((key) => (
          <TextField
            key={key}
            type='number'
            required
            label={moneyLabels[key]}
            value={drafts[key]}
            disabled={disabled}
            onChange={(value) => set(key, value)}
          />
        ))}
        <div className='space-y-2 sm:col-span-2'>
          <Label>{t('Cost factor tiers by last month customer spend')}</Label>
          {drafts.tiers.map((tier, index) => (
            <div key={tier.key} className='flex items-end gap-2'>
              <TextField
                type='number'
                label={t('From (USD per month)')}
                value={tier.min_monthly_usd}
                disabled={disabled}
                onChange={(value) =>
                  set(
                    'tiers',
                    drafts.tiers.map((item, i) =>
                      i === index ? { ...item, min_monthly_usd: value } : item
                    )
                  )
                }
              />
              <TextField
                type='number'
                label={t('Cost factor')}
                value={tier.cost_factor}
                disabled={disabled}
                onChange={(value) =>
                  set(
                    'tiers',
                    drafts.tiers.map((item, i) =>
                      i === index ? { ...item, cost_factor: value } : item
                    )
                  )
                }
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                disabled={disabled}
                aria-label={t('Remove tier {{n}}', { n: index + 1 })}
                onClick={() =>
                  set(
                    'tiers',
                    drafts.tiers.filter((_, i) => i !== index)
                  )
                }
              >
                <Trash2 aria-hidden />
              </Button>
            </div>
          ))}
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={disabled}
            onClick={() =>
              set('tiers', [
                ...drafts.tiers,
                { key: newRowKey(), min_monthly_usd: '', cost_factor: '' },
              ])
            }
          >
            <Plus aria-hidden />
            {t('Add tier')}
          </Button>
          <p className='text-muted-foreground text-xs'>
            {t('Leave empty to use the global cost factor for every site.')}
          </p>
        </div>
      </fieldset>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>{t('Domains')}</legend>
        <div className='space-y-2'>
          <Label htmlFor='reseller-roots'>
            {t('Root domains (one per line)')}
          </Label>
          <Textarea
            id='reseller-roots'
            rows={3}
            value={drafts.root_domains}
            disabled={disabled}
            onChange={(event) => set('root_domains', event.target.value)}
          />
          {!rootsSet && (
            <p className='text-warning text-xs'>{t('Not configured')}</p>
          )}
        </div>
        <div className='space-y-2'>
          <Label htmlFor='reseller-reserved'>
            {t('Reserved subdomains (one per line)')}
          </Label>
          <Textarea
            id='reseller-reserved'
            rows={3}
            value={drafts.reserved_subdomains}
            disabled={disabled}
            onChange={(event) => set('reserved_subdomains', event.target.value)}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='reseller-unknown-host'>{t('Unknown host')}</Label>
          <NativeSelect
            id='reseller-unknown-host'
            value={drafts.unknown_host_mode}
            disabled={disabled}
            onChange={(event) =>
              set(
                'unknown_host_mode',
                event.target.value as ResellerDrafts['unknown_host_mode']
              )
            }
          >
            <NativeSelectOption value='main'>
              {t('Serve the main site')}
            </NativeSelectOption>
            <NativeSelectOption value='reject'>
              {t('Return 404')}
            </NativeSelectOption>
          </NativeSelect>
        </div>
      </fieldset>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>
          {t('Custom domains (Cloudflare for SaaS)')}
        </legend>
        <SecretField
          label={t('Cloudflare API token')}
          value={drafts.cloudflare_token}
          configured={
            saved.cloudflare_token_configured && !drafts.clear_cloudflare_token
          }
          disabled={disabled}
          onChange={(value) => set('cloudflare_token', value)}
        />
        <TextField
          label={t('Cloudflare zone ID')}
          required
          value={drafts.cloudflare_zone_id}
          disabled={disabled}
          onChange={(value) => set('cloudflare_zone_id', value)}
        />
        <TextField
          label={t('Cloudflare account ID')}
          required
          value={drafts.cloudflare_account_id}
          disabled={disabled}
          onChange={(value) => set('cloudflare_account_id', value)}
        />
        <TextField
          label={t('CNAME target')}
          required
          placeholder='customers.example.com'
          value={drafts.cname_target}
          disabled={disabled}
          onChange={(value) => set('cname_target', value)}
        />
        <TextField
          label={t('Fallback origin')}
          required
          value={drafts.fallback_origin}
          disabled={disabled}
          onChange={(value) => set('fallback_origin', value)}
        />
        {saved.cloudflare_token_configured && (
          <div className='flex items-center gap-2 self-end'>
            <Checkbox
              id='reseller-clear-token'
              checked={drafts.clear_cloudflare_token}
              disabled={disabled}
              onCheckedChange={(value) =>
                set('clear_cloudflare_token', value === true)
              }
            />
            <Label htmlFor='reseller-clear-token'>
              {t('Remove the saved token')}
            </Label>
          </div>
        )}
        <div className='space-y-2 sm:col-span-2'>
          <Label>{t('Turnstile widgets')}</Label>
          {drafts.widgets.map((widget, index) => (
            <div
              key={widget.key}
              className='grid items-end gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]'
            >
              <TextField
                label={t('Widget ID of widget {{n}}', { n: index + 1 })}
                value={widget.id}
                disabled={disabled}
                onChange={(value) =>
                  set(
                    'widgets',
                    drafts.widgets.map((item, i) =>
                      i === index ? { ...item, id: value } : item
                    )
                  )
                }
              />
              <TextField
                label={t('Site key of widget {{n}}', { n: index + 1 })}
                value={widget.site_key}
                disabled={disabled}
                onChange={(value) =>
                  set(
                    'widgets',
                    drafts.widgets.map((item, i) =>
                      i === index ? { ...item, site_key: value } : item
                    )
                  )
                }
              />
              <SecretField
                label={t('Secret of widget {{n}}', { n: index + 1 })}
                value={widget.secret}
                configured={
                  saved.turnstile_secret_configured?.[widget.id] === true
                }
                disabled={disabled}
                onChange={(value) =>
                  set(
                    'widgets',
                    drafts.widgets.map((item, i) =>
                      i === index ? { ...item, secret: value } : item
                    )
                  )
                }
              />
              <Button
                type='button'
                variant='ghost'
                size='icon'
                disabled={disabled}
                aria-label={t('Remove widget {{n}}', { n: index + 1 })}
                onClick={() =>
                  set(
                    'widgets',
                    drafts.widgets.filter((_, i) => i !== index)
                  )
                }
              >
                <Trash2 aria-hidden />
              </Button>
            </div>
          ))}
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={disabled}
            onClick={() =>
              set('widgets', [
                ...drafts.widgets,
                { key: newRowKey(), id: '', site_key: '', secret: '' },
              ])
            }
          >
            <Plus aria-hidden />
            {t('Add widget')}
          </Button>
          {drafts.widgets.length === 0 && (
            <p className='text-warning text-xs'>{t('Not configured')}</p>
          )}
        </div>
      </fieldset>

      <fieldset className='flex flex-col gap-3'>
        <legend className='mb-2 text-sm font-medium'>{t('Features')}</legend>
        <SwitchRow
          label={t('Custom domains')}
          checked={drafts.custom_domain_enabled}
          disabled={disabled || (!cfReady && !drafts.custom_domain_enabled)}
          hint={
            cfReady
              ? undefined
              : t(
                  'Fill in the Cloudflare API token, zone ID and CNAME target first.'
                )
          }
          onCheckedChange={(value) => set('custom_domain_enabled', value)}
        />
        <SwitchRow
          label={t('Site owner sender domains')}
          checked={drafts.sender_domain_enabled}
          disabled={
            disabled ||
            (!saved.resend_configured && !drafts.sender_domain_enabled)
          }
          hint={
            saved.resend_configured
              ? undefined
              : t(
                  'Fill in the Resend API key under System Settings > Email first.'
                )
          }
          onCheckedChange={(value) => set('sender_domain_enabled', value)}
        />
        {SWITCH_KEYS.map((key) => (
          <SwitchRow
            key={key}
            label={switchLabels[key]}
            checked={drafts[key]}
            disabled={disabled}
            onCheckedChange={(value) => set(key, value)}
          />
        ))}
      </fieldset>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>{t('Rules')}</legend>
        {RULE_KEYS.map((key) => (
          <TextField
            key={key}
            type='number'
            label={ruleLabels[key]}
            value={drafts[key]}
            disabled={disabled}
            onChange={(value) => set(key, value)}
          />
        ))}
      </fieldset>

      <fieldset className='grid gap-4 sm:grid-cols-2'>
        <legend className='mb-2 text-sm font-medium'>
          {t('What site owners can see by default')}
        </legend>
        {VISIBILITY_KEYS.map((key) => (
          <div key={key} className='space-y-2'>
            <Label htmlFor={`reseller-visibility-${key}`}>
              {visibilityLabels[key]}
            </Label>
            <NativeSelect
              id={`reseller-visibility-${key}`}
              value={drafts.visibility[key]}
              disabled={disabled}
              onChange={(event) =>
                set('visibility', {
                  ...drafts.visibility,
                  [key]: event.target.value,
                })
              }
            >
              {key === 'customer_email' ? (
                <>
                  <NativeSelectOption value='masked'>
                    {t('Masked')}
                  </NativeSelectOption>
                  <NativeSelectOption value='full'>
                    {t('Full')}
                  </NativeSelectOption>
                </>
              ) : (
                <>
                  <NativeSelectOption value='on'>
                    {t('Shown')}
                  </NativeSelectOption>
                  <NativeSelectOption value='off'>
                    {t('Hidden')}
                  </NativeSelectOption>
                </>
              )}
            </NativeSelect>
          </div>
        ))}
      </fieldset>

      <p className='text-muted-foreground text-xs'>
        {t(
          'Site agreement version {{agreement}}, site customer terms version {{terms}}. Edit them under System Settings > Content.',
          {
            agreement: saved.site_agreement_version,
            terms: saved.site_customer_terms_version,
          }
        )}
      </p>

      {props.canWrite && (
        <div>
          <Button type='submit' disabled={save.isPending}>
            {t('Save')}
          </Button>
        </div>
      )}
    </form>
  )
}
