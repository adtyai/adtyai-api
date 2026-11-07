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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { AdtySiteDefaults } from './adty-site-defaults'

type TextField = Exclude<keyof AdtySiteDefaults, 'refund_policy_enabled'>

const TEXT_FIELDS: Array<{ field: TextField; kind: 'url' | 'email' | 'text' }> =
  [
    { field: 'api_base_url', kind: 'url' },
    { field: 'support_email', kind: 'email' },
    { field: 'telegram_url', kind: 'url' },
    { field: 'support_hours', kind: 'text' },
    { field: 'tawk_property_id', kind: 'text' },
    { field: 'tawk_widget_id', kind: 'text' },
  ]

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function toDrafts(values: AdtySiteDefaults): Record<TextField, string> {
  const drafts = {} as Record<TextField, string>
  for (const item of TEXT_FIELDS) drafts[item.field] = values[item.field]
  return drafts
}

/** Public site settings (adty task 8.3): API address, support contacts, live chat, refund policy. */
export function AdtySiteSection(props: { defaultValues: AdtySiteDefaults }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const savedRef = useRef(props.defaultValues)
  const [drafts, setDrafts] = useState(() => toDrafts(props.defaultValues))
  const [refundPolicy, setRefundPolicy] = useState(
    props.defaultValues.refund_policy_enabled
  )
  const [showErrors, setShowErrors] = useState(false)

  // Reset drafts when the server-provided saved values change.
  const savedKey = JSON.stringify(props.defaultValues)
  useEffect(() => {
    const next: AdtySiteDefaults = JSON.parse(savedKey)
    savedRef.current = next
    setDrafts(toDrafts(next))
    setRefundPolicy(next.refund_policy_enabled)
    setShowErrors(false)
  }, [savedKey])

  const labels: Record<TextField, string> = {
    api_base_url: t('API base URL'),
    support_email: t('Support email'),
    telegram_url: t('Telegram link'),
    support_hours: t('Support hours'),
    tawk_property_id: t('Tawk.to property ID'),
    tawk_widget_id: t('Tawk.to widget ID'),
  }
  const hints: Partial<Record<TextField, string>> = {
    api_base_url: t(
      'The address customers put in their clients, without /v1. Empty uses the server address.'
    ),
    support_hours: t('Shown on the home page next to the payment methods.'),
    tawk_widget_id: t('Live chat appears when both Tawk.to IDs are set.'),
  }

  const values = {} as Record<TextField, string>
  const errors: Partial<Record<TextField, string>> = {}
  for (const item of TEXT_FIELDS) {
    const value = drafts[item.field].trim()
    values[item.field] = value
    if (value === '') continue
    if (item.kind === 'url' && !isHttpUrl(value)) {
      errors[item.field] = t(
        'Enter a full URL starting with http:// or https://'
      )
    }
    if (item.kind === 'email' && !/^[^\s@]+@[^\s@]+$/.test(value)) {
      errors[item.field] = t('Enter a valid email address')
    }
  }

  const save = async () => {
    if (Object.keys(errors).length > 0) {
      setShowErrors(true)
      return
    }
    const saved = savedRef.current
    for (const item of TEXT_FIELDS) {
      if (values[item.field] !== saved[item.field]) {
        await updateOption.mutateAsync({
          key: `adty_site_setting.${item.field}`,
          value: values[item.field],
        })
        savedRef.current = {
          ...savedRef.current,
          [item.field]: values[item.field],
        }
      }
    }
    if (refundPolicy !== saved.refund_policy_enabled) {
      await updateOption.mutateAsync({
        key: 'adty_site_setting.refund_policy_enabled',
        value: refundPolicy,
      })
      savedRef.current = {
        ...savedRef.current,
        refund_policy_enabled: refundPolicy,
      }
    }
  }

  return (
    <SettingsSection title={t('ADTY AI Site')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <div className='grid max-w-3xl gap-5 sm:grid-cols-2'>
        {TEXT_FIELDS.map((item) => {
          const error = showErrors ? errors[item.field] : undefined
          return (
            <div key={item.field} className='space-y-2'>
              <Label htmlFor={`adty-site-${item.field}`}>
                {labels[item.field]}
              </Label>
              <Input
                id={`adty-site-${item.field}`}
                type={item.kind === 'text' ? 'text' : item.kind}
                value={drafts[item.field]}
                aria-invalid={Boolean(error)}
                onChange={(event) =>
                  setDrafts((prev) => ({
                    ...prev,
                    [item.field]: event.target.value,
                  }))
                }
              />
              {error && <p className='text-destructive text-xs'>{error}</p>}
              {!error && hints[item.field] && (
                <p className='text-muted-foreground text-xs'>
                  {hints[item.field]}
                </p>
              )}
            </div>
          )
        })}
        <div className='flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2'>
          <div className='space-y-1'>
            <Label htmlFor='adty-site-refund-policy'>
              {t('Show the refund policy')}
            </Label>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Adds the refund policy page, its footer link and the sign-up consent.'
              )}
            </p>
          </div>
          <Switch
            id='adty-site-refund-policy'
            checked={refundPolicy}
            onCheckedChange={setRefundPolicy}
          />
        </div>
      </div>
    </SettingsSection>
  )
}
