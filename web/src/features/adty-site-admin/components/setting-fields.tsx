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
import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

/** A switch with its label and an optional reason it cannot be turned on. */
export function SwitchRow(props: {
  label: string
  checked: boolean
  disabled?: boolean
  /** Shown under the label when set; usually why the switch is disabled. */
  hint?: string
  onCheckedChange: (value: boolean) => void
}) {
  const id = useId()
  return (
    <div className='flex items-center justify-between gap-4 rounded-lg border p-3'>
      <div className='flex flex-col gap-0.5'>
        <Label htmlFor={id}>{props.label}</Label>
        {props.hint && (
          <span id={`${id}-hint`} className='text-muted-foreground text-xs'>
            {props.hint}
          </span>
        )}
      </div>
      <Switch
        id={id}
        checked={props.checked}
        disabled={props.disabled}
        aria-describedby={props.hint ? `${id}-hint` : undefined}
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  )
}

/** A text or number field; "Not configured" under it while it is empty and required. */
export function TextField(props: {
  label: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  type?: 'text' | 'number'
  required?: boolean
  placeholder?: string
  children?: ReactNode
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className='space-y-2'>
      <Label htmlFor={id}>{props.label}</Label>
      <Input
        id={id}
        type={props.type ?? 'text'}
        step={props.type === 'number' ? 'any' : undefined}
        value={props.value}
        disabled={props.disabled}
        placeholder={props.placeholder}
        onChange={(event) => props.onChange(event.target.value)}
      />
      {props.required && props.value.trim() === '' && (
        <p className='text-warning text-xs'>{t('Not configured')}</p>
      )}
      {props.children}
    </div>
  )
}

/**
 * A write-only secret: the saved value never comes back, so the box is empty
 * and the placeholder says whether one is saved.
 */
export function SecretField(props: {
  label: string
  value: string
  configured: boolean
  onChange: (value: string) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className='space-y-2'>
      <Label htmlFor={id}>{props.label}</Label>
      <Input
        id={id}
        type='password'
        autoComplete='off'
        value={props.value}
        disabled={props.disabled}
        placeholder={
          props.configured ? t('Saved. Enter a new value to replace it') : ''
        }
        onChange={(event) => props.onChange(event.target.value)}
      />
      {!props.configured && props.value.trim() === '' && (
        <p className='text-warning text-xs'>{t('Not configured')}</p>
      )}
    </div>
  )
}
