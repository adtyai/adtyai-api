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
import { memo } from 'react'

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

type SettingsCardProps = {
  title: string
  description?: string
  children: React.ReactNode
  className?: string
}

/** Card shell for a group of settings: 14px radius, hairline border, and a
 *  15px/600 title over a 20–24px padded body (design system §3). */
export const SettingsCard = memo(function SettingsCard({
  title,
  description,
  children,
  className,
}: SettingsCardProps) {
  return (
    <Card
      data-card-hover='false'
      className={cn('gap-0 overflow-hidden py-0', className)}
    >
      <CardHeader className='border-border gap-1 border-b px-5 py-4 sm:px-6'>
        <CardTitle className='text-[15px] leading-snug font-semibold tracking-tight [word-break:keep-all]'>
          {title}
        </CardTitle>
        {description ? (
          <CardDescription className='text-dim text-[12.5px] leading-relaxed'>
            {description}
          </CardDescription>
        ) : null}
      </CardHeader>
      <CardContent className='px-5 py-5 sm:px-6'>{children}</CardContent>
    </Card>
  )
})
