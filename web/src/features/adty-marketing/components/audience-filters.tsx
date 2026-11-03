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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { TagInput } from '@/components/tag-input'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useDebounce } from '@/hooks/use-debounce'
import { toIntlLocale } from '@/i18n/languages'
import { CONTENT_LANG_LABELS, CONTENT_LANGS } from '@/lib/adty-lang'
import { formatNumber } from '@/lib/format'

import { estimateAudience, marketingKeys, type MarketingAudience } from '../api'
import { dateInputToUnix, unixToDateInput } from '../lib/campaign'

type AudienceFiltersProps = {
  value: MarketingAudience
  onChange: (next: MarketingAudience) => void
  disabled?: boolean
}

/** Recipient filters with a live estimate of consented, sendable recipients (adty task 12.2). */
export function AudienceFilters(props: AudienceFiltersProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const audience = props.value
  const debounced = useDebounce(audience, 400)
  const estimate = useQuery({
    queryKey: marketingKeys.estimate(debounced),
    queryFn: () => estimateAudience(debounced),
  })
  const set = (patch: Partial<MarketingAudience>) =>
    props.onChange({ ...audience, ...patch })
  const langs = audience.langs ?? []
  const toppedUp =
    audience.topped_up === undefined || audience.topped_up === null
      ? 'any'
      : String(audience.topped_up)

  const dateFields: Array<{
    key:
      | 'registered_after'
      | 'registered_before'
      | 'active_after'
      | 'active_before'
    label: string
  }> = [
    { key: 'registered_after', label: t('Signed up on or after') },
    { key: 'registered_before', label: t('Signed up before') },
    { key: 'active_after', label: t('Last active on or after') },
    { key: 'active_before', label: t('Last active before') },
  ]

  return (
    <fieldset className='flex flex-col gap-4' disabled={props.disabled}>
      <legend className='mb-2 text-sm font-medium'>{t('Recipients')}</legend>
      <div className='space-y-2'>
        <Label>{t('Languages (none selected = all)')}</Label>
        <div className='flex flex-wrap gap-3'>
          {CONTENT_LANGS.map((lang) => (
            <label key={lang} className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={langs.includes(lang)}
                onCheckedChange={(checked) =>
                  set({
                    langs: checked
                      ? [...langs, lang]
                      : langs.filter((item) => item !== lang),
                  })
                }
              />
              {CONTENT_LANG_LABELS[lang]}
            </label>
          ))}
        </div>
      </div>
      <div className='grid gap-4 sm:grid-cols-2'>
        {dateFields.map((field) => (
          <div key={field.key} className='space-y-2'>
            <Label htmlFor={`audience-${field.key}`}>{field.label}</Label>
            <Input
              id={`audience-${field.key}`}
              type='date'
              value={unixToDateInput(audience[field.key])}
              onChange={(event) =>
                set({ [field.key]: dateInputToUnix(event.target.value) })
              }
            />
          </div>
        ))}
        <div className='space-y-2'>
          <Label htmlFor='audience-topped-up'>{t('Top-ups')}</Label>
          <NativeSelect
            id='audience-topped-up'
            value={toppedUp}
            onChange={(event) =>
              set({
                topped_up:
                  event.target.value === 'any'
                    ? null
                    : event.target.value === 'true',
              })
            }
          >
            <NativeSelectOption value='any'>{t('Any')}</NativeSelectOption>
            <NativeSelectOption value='true'>
              {t('Has topped up')}
            </NativeSelectOption>
            <NativeSelectOption value='false'>
              {t('Never topped up')}
            </NativeSelectOption>
          </NativeSelect>
        </div>
        <div className='space-y-2'>
          <Label>{t('Groups (empty = all)')}</Label>
          <TagInput
            value={audience.groups ?? []}
            onChange={(groups) => set({ groups })}
            placeholder={t('Type a group ID and press Enter')}
            disabled={props.disabled}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='audience-balance-min'>
            {t('Balance at least (USD)')}
          </Label>
          <Input
            id='audience-balance-min'
            type='number'
            min={0}
            step='0.01'
            value={audience.balance_min_usd ?? ''}
            onChange={(event) => set({ balance_min_usd: event.target.value })}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='audience-balance-max'>
            {t('Balance below (USD)')}
          </Label>
          <Input
            id='audience-balance-max'
            type='number'
            min={0}
            step='0.01'
            value={audience.balance_max_usd ?? ''}
            onChange={(event) => set({ balance_max_usd: event.target.value })}
          />
        </div>
      </div>
      <div className='space-y-2'>
        <label className='flex items-center gap-2 text-sm'>
          <Checkbox
            checked={audience.include_site_customers === true}
            onCheckedChange={(checked) =>
              set({ include_site_customers: checked === true })
            }
          />
          {t('Include sub-site customers (each site is sent separately)')}
        </label>
        {audience.include_site_customers && (
          <TagInput
            value={(audience.site_ids ?? []).map(String)}
            onChange={(ids) =>
              set({
                site_ids: ids
                  .map(Number)
                  .filter((id) => Number.isInteger(id) && id >= 0),
              })
            }
            placeholder={t('Site IDs, 0 is the main site (empty = all)')}
            disabled={props.disabled}
          />
        )}
      </div>
      <p className='text-sm' data-testid='audience-estimate' aria-live='polite'>
        {estimate.data
          ? t(
              'Estimated recipients who agreed to marketing emails: {{total}}',
              {
                total: formatNumber(estimate.data.total, locale),
              }
            )
          : t('Counting recipients…')}
      </p>
    </fieldset>
  )
}
