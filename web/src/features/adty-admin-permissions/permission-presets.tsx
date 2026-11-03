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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import type {
  AdminPermissionMatrix,
  PermissionCatalog,
} from '@/lib/admin-permissions'
import {
  ADTY_PERMISSION_PRESETS,
  applyPermissionPreset,
} from '@/lib/adty-permissions'

type PermissionPresetsProps = {
  catalog: PermissionCatalog
  onApply: (matrix: AdminPermissionMatrix) => void
  disabled?: boolean
}

/** Support / operations / finance presets that replace the whole matrix. */
export function PermissionPresets(props: PermissionPresetsProps) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-center gap-2'>
      <span className='text-muted-foreground text-xs'>
        {t('Apply a preset')}
      </span>
      {ADTY_PERMISSION_PRESETS.map((preset) => (
        <Button
          key={preset.id}
          type='button'
          variant='outline'
          size='sm'
          disabled={props.disabled}
          onClick={() =>
            props.onApply(applyPermissionPreset(props.catalog, preset))
          }
        >
          {t(preset.labelKey)}
        </Button>
      ))}
    </div>
  )
}
