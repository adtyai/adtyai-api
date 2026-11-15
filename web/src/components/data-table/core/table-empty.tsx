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
import { Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { TableRow, TableCell } from '@/components/ui/table'

interface TableEmptyProps {
  /**
   * Number of columns to span
   */
  colSpan: number
  /**
   * Custom title for empty state
   * @default 'No Data'
   */
  title?: string
  /**
   * Custom description for empty state
   * @default 'No records found. Try adjusting your filters.'
   */
  description?: string
  /**
   * Custom icon component
   * @default Database icon
   */
  icon?: React.ReactNode
  /**
   * Additional content to display (e.g., buttons)
   */
  children?: React.ReactNode
}

/**
 * Generic table empty state component
 * Displays a centered empty state message when table has no data
 */
export function TableEmpty({
  colSpan,
  title,
  description,
  icon,
  children,
}: TableEmptyProps) {
  const { t } = useTranslation()
  const resolvedTitle = title ?? t('No Data')
  const resolvedDescription =
    description ?? t('No records found. Try adjusting your filters.')
  return (
    <TableRow>
      {/* Design system §4: empty states sit directly on the card surface —
          a 24px faint glyph, a 14px/600 title and 13px dim copy. No gray
          block behind the icon. */}
      <TableCell colSpan={colSpan} className='h-[300px] p-0'>
        <Empty className='border-0'>
          <EmptyHeader>
            <EmptyMedia>
              {icon || <Database className='text-faint size-6' />}
            </EmptyMedia>
            <EmptyTitle className='text-[14px] font-semibold'>
              {resolvedTitle}
            </EmptyTitle>
            <EmptyDescription className='text-dim text-[13px]'>
              {resolvedDescription}
            </EmptyDescription>
          </EmptyHeader>
          {children}
        </Empty>
      </TableCell>
    </TableRow>
  )
}
