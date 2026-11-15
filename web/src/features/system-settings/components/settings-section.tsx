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
import { cn } from '@/lib/utils'

import {
  useSettingsPageTitle,
  useSuppressSettingsSectionHeader,
} from './settings-page-context'

type SettingsSectionProps = {
  title: string
  /** Optional one-line explanation shown under the card title. */
  description?: string
  titleProps?: React.HTMLAttributes<HTMLHeadingElement>
  children: React.ReactNode
  className?: string
}

/**
 * A group of related settings, drawn as one bordered card (design system §3:
 * 14px radius, 1px border, no shadow). Cards are separated by 16px of page
 * rhythm, and the title is skipped when the page heading already says it.
 */
export function SettingsSection({
  title,
  description,
  titleProps,
  children,
  className,
}: SettingsSectionProps) {
  const suppressHeader = useSuppressSettingsSectionHeader()
  const pageTitle = useSettingsPageTitle()
  const showHeader = !suppressHeader || pageTitle !== title

  return (
    <section
      data-slot='settings-section'
      className={cn(
        'border-border bg-card min-w-0 overflow-hidden rounded-[14px] border',
        className
      )}
    >
      {showHeader && (
        <div className='border-border flex flex-col gap-1 border-b px-5 py-4 sm:px-6'>
          <h3
            {...titleProps}
            className={cn(
              'text-[15px] leading-snug font-semibold tracking-tight [word-break:keep-all]',
              titleProps?.className
            )}
          >
            {title}
          </h3>
          {description ? (
            <p className='text-dim text-[12.5px] leading-relaxed'>
              {description}
            </p>
          ) : null}
        </div>
      )}
      <div className='min-w-0 px-5 py-5 sm:px-6 sm:py-6'>{children}</div>
    </section>
  )
}
