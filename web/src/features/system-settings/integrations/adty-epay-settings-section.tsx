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

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export type AdtyEpaySettingsDefaults = {
  orderMinutes: number
  channelTimeoutSeconds: number
  totalTimeoutSeconds: number
  queryAfterSeconds: number
  expiredQuerySeconds: number
  unpaidThreshold: number
  probeEnabled: boolean
  probeIntervalMinutes: number
}

type NumberField = Exclude<keyof AdtyEpaySettingsDefaults, 'probeEnabled'>

const OPTION_KEYS: Record<keyof AdtyEpaySettingsDefaults, string> = {
  orderMinutes: 'adty_topup_setting.epay_order_minutes',
  channelTimeoutSeconds: 'adty_epay_setting.channel_timeout_seconds',
  totalTimeoutSeconds: 'adty_epay_setting.total_timeout_seconds',
  queryAfterSeconds: 'adty_epay_setting.query_after_seconds',
  expiredQuerySeconds: 'adty_epay_setting.expired_query_seconds',
  unpaidThreshold: 'adty_epay_setting.unpaid_threshold',
  probeEnabled: 'adty_epay_setting.probe_enabled',
  probeIntervalMinutes: 'adty_epay_setting.probe_interval_minutes',
}

const NUMBER_FIELDS: Array<{ field: NumberField; min: number }> = [
  { field: 'orderMinutes', min: 2 },
  { field: 'channelTimeoutSeconds', min: 1 },
  { field: 'totalTimeoutSeconds', min: 1 },
  { field: 'queryAfterSeconds', min: 30 },
  { field: 'expiredQuerySeconds', min: 0 },
  { field: 'unpaidThreshold', min: 1 },
  { field: 'probeIntervalMinutes', min: 1 },
]

function toDrafts(values: AdtyEpaySettingsDefaults) {
  const drafts = {} as Record<NumberField, string>
  for (const item of NUMBER_FIELDS) {
    drafts[item.field] = String(values[item.field])
  }
  return drafts
}

/** Alipay / WeChat ordering rules (adty task 3.6): validity, failover limits, follow-up query, maintenance. */
export function AdtyEpaySettingsSection(props: {
  defaultValues: AdtyEpaySettingsDefaults
}) {
  const { t } = useTranslation()
  const labels: Record<NumberField, string> = {
    orderMinutes: t('Order validity (minutes)'),
    channelTimeoutSeconds: t('Per-channel timeout (seconds)'),
    totalTimeoutSeconds: t('Total ordering timeout (seconds)'),
    queryAfterSeconds: t('Query unpaid orders after (seconds)'),
    expiredQuerySeconds: t('Keep querying expired orders for (seconds)'),
    unpaidThreshold: t('Unpaid orders in a row before maintenance'),
    probeIntervalMinutes: t('Probe interval (minutes)'),
  }
  const updateOption = useUpdateOption()
  const saved = props.defaultValues
  const [drafts, setDrafts] = useState(() => toDrafts(saved))
  const [probeEnabled, setProbeEnabled] = useState(saved.probeEnabled)

  // Reset when the saved values change (after a save refetches the options).
  const savedKey = JSON.stringify(saved)
  useEffect(() => {
    const next: AdtyEpaySettingsDefaults = JSON.parse(savedKey)
    setDrafts(toDrafts(next))
    setProbeEnabled(next.probeEnabled)
  }, [savedKey])

  const values = {} as Record<NumberField, number>
  const errors = {} as Partial<Record<NumberField, string>>
  for (const item of NUMBER_FIELDS) {
    const value = Number(drafts[item.field])
    values[item.field] = value
    if (!Number.isInteger(value) || value < item.min) {
      errors[item.field] = t('Enter a whole number of at least {{min}}', {
        min: item.min,
      })
    }
  }
  if (
    !errors.channelTimeoutSeconds &&
    !errors.totalTimeoutSeconds &&
    (values.channelTimeoutSeconds > values.totalTimeoutSeconds ||
      values.totalTimeoutSeconds > 30)
  ) {
    errors.totalTimeoutSeconds = t(
      'The total must be at least the per-channel timeout and at most 30 seconds'
    )
  }

  const save = async () => {
    if (Object.keys(errors).length > 0) return
    // The server checks per-channel ≤ total against the stored other value,
    // so save the total first when it grows and last when it shrinks.
    const order: NumberField[] =
      values.totalTimeoutSeconds >= saved.totalTimeoutSeconds
        ? ['totalTimeoutSeconds', 'channelTimeoutSeconds']
        : ['channelTimeoutSeconds', 'totalTimeoutSeconds']
    for (const item of NUMBER_FIELDS) {
      if (!order.includes(item.field)) order.push(item.field)
    }
    for (const field of order) {
      if (values[field] !== saved[field]) {
        await updateOption.mutateAsync({
          key: OPTION_KEYS[field],
          value: values[field],
        })
      }
    }
    if (probeEnabled !== saved.probeEnabled) {
      await updateOption.mutateAsync({
        key: OPTION_KEYS.probeEnabled,
        value: probeEnabled,
      })
    }
  }

  return (
    <SettingsSection title={t('Alipay and WeChat Pay')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <div className='grid max-w-2xl gap-5 sm:grid-cols-2'>
        {NUMBER_FIELDS.map((item) => (
          <div key={item.field} className='space-y-2'>
            <Label htmlFor={`adty-epay-${item.field}`}>
              {labels[item.field]}
            </Label>
            <Input
              id={`adty-epay-${item.field}`}
              type='number'
              min={item.min}
              step={1}
              className='w-32'
              value={drafts[item.field]}
              aria-invalid={Boolean(errors[item.field])}
              onChange={(event) =>
                setDrafts((prev) => ({
                  ...prev,
                  [item.field]: event.target.value,
                }))
              }
            />
            {errors[item.field] && (
              <p className='text-destructive text-xs'>{errors[item.field]}</p>
            )}
          </div>
        ))}
        <div className='flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2'>
          <Label htmlFor='adty-epay-probe'>
            {t('Probe channels in maintenance')}
          </Label>
          <Switch
            id='adty-epay-probe'
            checked={probeEnabled}
            onCheckedChange={setProbeEnabled}
          />
        </div>
      </div>
    </SettingsSection>
  )
}
