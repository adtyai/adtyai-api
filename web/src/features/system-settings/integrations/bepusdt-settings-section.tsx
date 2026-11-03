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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { CHAIN_NAMES } from '@/features/wallet/adty/lib/methods'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

// Ported from the previous fork (adty task 4.3): keys moved to adty_bepusdt.*,
// chains are the payment method IDs that BEpusdt also uses as trade_type.
const BEPUSDT_CHAINS = Object.entries(CHAIN_NAMES).map(([id, label]) => ({
  id,
  label,
}))

type BEpusdtSettingsDefaults = {
  enabled: boolean
  baseUrl: string
  tradeTypes: string
  orderMinutes: number
}

type BEpusdtSettingsSectionProps = {
  defaultValues: BEpusdtSettingsDefaults
}

function parseTradeTypes(raw: string): string[] {
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed)
      ? parsed.filter((v): v is string => typeof v === 'string')
      : []
  } catch {
    return []
  }
}

export function BEpusdtSettingsSection(props: BEpusdtSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initialChains = useMemo(
    () => parseTradeTypes(props.defaultValues.tradeTypes),
    [props.defaultValues.tradeTypes]
  )
  const [enabled, setEnabled] = useState(props.defaultValues.enabled)
  const [baseUrl, setBaseUrl] = useState(props.defaultValues.baseUrl)
  const [secret, setSecret] = useState('')
  const [chains, setChains] = useState<string[]>(initialChains)
  const [minutes, setMinutes] = useState(
    String(props.defaultValues.orderMinutes)
  )

  // Reset when the saved values change (after a save refetches the options).
  const savedEnabled = props.defaultValues.enabled
  const savedBaseUrl = props.defaultValues.baseUrl
  const savedMinutes = props.defaultValues.orderMinutes
  useEffect(() => {
    setEnabled(savedEnabled)
    setBaseUrl(savedBaseUrl)
    setChains(initialChains)
    setMinutes(String(savedMinutes))
  }, [savedEnabled, savedBaseUrl, savedMinutes, initialChains])

  const minutesValue = Number.parseInt(minutes, 10)
  const chainError = chains.length === 0 ? t('Choose at least one chain') : ''
  const minutesError =
    !Number.isInteger(minutesValue) || minutesValue < 2
      ? t('At least 2 minutes')
      : ''

  const save = async () => {
    if (chainError || minutesError) return
    const updates: Array<{ key: string; value: string | boolean | number }> = []
    if (enabled !== savedEnabled) {
      updates.push({ key: 'adty_bepusdt.enabled', value: enabled })
    }
    if (baseUrl.trim() !== savedBaseUrl) {
      updates.push({ key: 'adty_bepusdt.base_url', value: baseUrl.trim() })
    }
    // The secret is write-only: an empty field keeps the stored one.
    if (secret.trim() !== '') {
      updates.push({ key: 'adty_bepusdt.secret', value: secret.trim() })
    }
    const serializedChains = JSON.stringify(chains)
    if (serializedChains !== JSON.stringify(initialChains)) {
      updates.push({ key: 'adty_bepusdt.trade_types', value: serializedChains })
    }
    if (minutesValue !== savedMinutes) {
      updates.push({
        key: 'adty_topup_setting.usdt_order_minutes',
        value: minutesValue,
      })
    }
    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
    setSecret('')
  }

  return (
    <SettingsSection title={t('USDT (BEpusdt)')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
      />
      <div className='flex max-w-2xl flex-col gap-5'>
        <Alert>
          <AlertDescription>
            {t(
              'In BEpusdt set the amount match mode to has_prefix, the order timeout to the same minutes as here, the confirmations you need, and allow callbacks to this site.'
            )}
          </AlertDescription>
        </Alert>
        <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
          <Label htmlFor='bepusdt-enabled'>{t('Enable USDT payments')}</Label>
          <Switch
            id='bepusdt-enabled'
            checked={enabled}
            onCheckedChange={setEnabled}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='bepusdt-base-url'>{t('Gateway address')}</Label>
          <Input
            id='bepusdt-base-url'
            value={baseUrl}
            placeholder='https://pay.example.com'
            onChange={(event) => setBaseUrl(event.target.value)}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='bepusdt-secret'>{t('API token')}</Label>
          <Input
            id='bepusdt-secret'
            type='password'
            autoComplete='new-password'
            value={secret}
            placeholder={t('Leave empty to keep the current token')}
            onChange={(event) => setSecret(event.target.value)}
          />
        </div>
        <fieldset className='space-y-2'>
          <legend className='text-sm font-medium'>{t('Enabled chains')}</legend>
          <div className='flex flex-wrap gap-4'>
            {BEPUSDT_CHAINS.map((chain) => (
              <label key={chain.id} className='flex items-center gap-2 text-sm'>
                <Checkbox
                  checked={chains.includes(chain.id)}
                  onCheckedChange={(checked) =>
                    setChains((prev) =>
                      checked === true
                        ? [...prev, chain.id]
                        : prev.filter((id) => id !== chain.id)
                    )
                  }
                />
                {chain.label}
              </label>
            ))}
          </div>
          {chainError && (
            <p className='text-destructive text-xs'>{chainError}</p>
          )}
        </fieldset>
        <div className='space-y-2'>
          <Label htmlFor='bepusdt-minutes'>
            {t('Order validity (minutes)')}
          </Label>
          <Input
            id='bepusdt-minutes'
            type='number'
            min={2}
            step={1}
            className='w-32'
            value={minutes}
            onChange={(event) => setMinutes(event.target.value)}
            aria-invalid={Boolean(minutesError)}
          />
          {minutesError && (
            <p className='text-destructive text-xs'>{minutesError}</p>
          )}
        </div>
      </div>
    </SettingsSection>
  )
}
