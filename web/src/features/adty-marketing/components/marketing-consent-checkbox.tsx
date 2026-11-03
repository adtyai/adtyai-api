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
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

import { useMarketingConsentDefault } from '../hooks/use-marketing-consent-default'

type MarketingConsentCheckboxProps = {
  checked: boolean
  onCheckedChange: (value: boolean) => void
  className?: string
}

/**
 * Optional "product updates and offers" checkbox on the sign-up form (adty task 12.1).
 * Hidden while marketing emails are off. The server picks the default from
 * CF-IPCountry: unticked for the EU and unknown countries.
 */
export function MarketingConsentCheckbox(props: MarketingConsentCheckboxProps) {
  const { t } = useTranslation()
  const query = useMarketingConsentDefault()
  const applied = useRef(false)
  const onCheckedChange = props.onCheckedChange
  const defaultChecked = query.data?.checked

  useEffect(() => {
    if (applied.current || defaultChecked === undefined) return
    applied.current = true
    onCheckedChange(defaultChecked)
  }, [defaultChecked, onCheckedChange])

  if (!query.data?.show) return null

  return (
    <div className={cn('flex items-start gap-3 px-1', props.className)}>
      <Checkbox
        id='marketing-consent'
        checked={props.checked}
        onCheckedChange={(value) => props.onCheckedChange(value === true)}
        className='mt-0.5'
      />
      <Label
        htmlFor='marketing-consent'
        className='text-muted-foreground text-left text-xs leading-5 font-normal'
      >
        {t(
          'Send me product updates and offers by email. I can unsubscribe at any time.'
        )}
      </Label>
    </div>
  )
}
