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
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'

import { VISIBILITY_KEYS, type VisibilityKey } from '../lib/settings'
import type { SiteVisibilityDraft } from '../lib/site-visibility'

export function SiteVisibilityEditor(props: {
  value: SiteVisibilityDraft
  disabled: boolean
  onChange: (value: SiteVisibilityDraft) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const labels: Record<VisibilityKey, string> = {
    customer_email: t('Customer email'),
    customer_balance: t('Customer balance'),
    customer_topups: t('Customer top-ups'),
    customer_usage: t('Customer usage'),
    request_logs: t('Request logs'),
    customer_tokens: t('Customer API keys'),
  }
  return (
    <fieldset disabled={props.disabled} className='grid gap-3 sm:grid-cols-2'>
      <legend className='mb-2 text-sm font-medium'>
        {t('What this site owner can see')}
      </legend>
      {VISIBILITY_KEYS.map((key) => (
        <div key={key} className='space-y-1'>
          <Label htmlFor={`${id}-${key}`}>{labels[key]}</Label>
          <NativeSelect
            id={`${id}-${key}`}
            className='w-full'
            value={props.value[key]}
            onChange={(event) =>
              props.onChange({ ...props.value, [key]: event.target.value })
            }
          >
            <option value=''>{t('Inherit global default')}</option>
            {key === 'customer_email' ? (
              <>
                <option value='masked'>{t('Masked')}</option>
                <option value='full'>{t('Full')}</option>
              </>
            ) : (
              <>
                <option value='on'>{t('Shown')}</option>
                <option value='off'>{t('Hidden')}</option>
              </>
            )}
          </NativeSelect>
        </div>
      ))}
    </fieldset>
  )
}
