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

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

type NumberKey = 'default_rpm' | 'default_tpm' | 'default_concurrency'

export type AdtyUserLimitDefaults = {
  enabled: boolean
  default_rpm: number
  default_tpm: number
  default_concurrency: number
}

const NUMBER_KEYS: NumberKey[] = [
  'default_rpm',
  'default_tpm',
  'default_concurrency',
]

/** Per-user RPM, TPM and concurrency defaults (adty task 7.4); 0 means no limit. */
export function AdtyUserLimitSection(props: {
  defaultValues: AdtyUserLimitDefaults
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const saved = props.defaultValues
  const labels: Record<NumberKey, string> = {
    default_rpm: t('Requests per minute (RPM)'),
    default_tpm: t('Tokens per minute (TPM)'),
    default_concurrency: t('Concurrent requests'),
  }
  const [enabled, setEnabled] = useState(saved.enabled)
  const [drafts, setDrafts] = useState<Record<NumberKey, string>>({
    default_rpm: String(saved.default_rpm),
    default_tpm: String(saved.default_tpm),
    default_concurrency: String(saved.default_concurrency),
  })
  const savedKey = JSON.stringify(saved)
  useEffect(() => {
    const next: AdtyUserLimitDefaults = JSON.parse(savedKey)
    setEnabled(next.enabled)
    setDrafts({
      default_rpm: String(next.default_rpm),
      default_tpm: String(next.default_tpm),
      default_concurrency: String(next.default_concurrency),
    })
  }, [savedKey])

  const invalid = NUMBER_KEYS.filter((key) => {
    const value = Number(drafts[key])
    return !Number.isInteger(value) || value < 0
  })

  const save = async () => {
    if (invalid.length > 0) return
    for (const key of NUMBER_KEYS) {
      const value = Number(drafts[key])
      if (value !== saved[key]) {
        await updateOption.mutateAsync({
          key: `adty_limit_setting.${key}`,
          value,
        })
      }
    }
    if (enabled !== saved.enabled) {
      await updateOption.mutateAsync({
        key: 'adty_limit_setting.enabled',
        value: enabled,
      })
    }
  }

  return (
    <SettingsSection title={t('Per-user limits')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <div className='flex max-w-2xl flex-col gap-5'>
        <Alert>
          <AlertDescription>
            {t(
              'Defaults for every user; 0 means no limit. A user can have their own values in the user editor. TPM is counted after each request finishes, so large requests started in the same minute are not stopped.'
            )}
          </AlertDescription>
        </Alert>
        <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
          <Label htmlFor='adty-limit-enabled'>
            {t('Enable per-user limits')}
          </Label>
          <Switch
            id='adty-limit-enabled'
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        </div>
        <div className='grid gap-4 sm:grid-cols-3'>
          {NUMBER_KEYS.map((key) => (
            <div key={key} className='space-y-2'>
              <Label htmlFor={`adty-limit-${key}`}>{labels[key]}</Label>
              <Input
                id={`adty-limit-${key}`}
                type='number'
                min={0}
                step={1}
                value={drafts[key]}
                aria-invalid={invalid.includes(key)}
                onChange={(event) =>
                  setDrafts((prev) => ({ ...prev, [key]: event.target.value }))
                }
              />
            </div>
          ))}
        </div>
      </div>
    </SettingsSection>
  )
}
