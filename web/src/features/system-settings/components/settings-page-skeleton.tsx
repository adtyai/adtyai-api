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
import { Skeleton } from '@/components/ui/skeleton'

type SettingsPageSkeletonProps = {
  /** Accessible label announced while the settings options load. */
  label: string
}

type SkeletonCardSpec = {
  id: string
  /** Number of label/control rows drawn inside the card body. */
  rows: number
  withDescription: boolean
}

/**
 * Mirrors the real settings sections: a 15px card title over a hairline
 * border, a 20–24px padded body, and 20px of rhythm between rows. Every
 * width is a percentage of its cell, so the placeholder never pushes the
 * layout wider than the viewport.
 */
const SKELETON_CARDS: readonly SkeletonCardSpec[] = [
  { id: 'primary', rows: 4, withDescription: true },
  { id: 'behaviour', rows: 3, withDescription: false },
  { id: 'advanced', rows: 2, withDescription: true },
]

const SKELETON_ROW_KEYS = ['first', 'second', 'third', 'fourth'] as const

export function SettingsPageSkeleton(props: SettingsPageSkeletonProps) {
  return (
    <div
      role='status'
      aria-busy='true'
      aria-label={props.label}
      className='flex min-w-0 flex-col gap-4'
    >
      {SKELETON_CARDS.map((card) => (
        <div
          key={card.id}
          className='border-border bg-card min-w-0 overflow-hidden rounded-[14px] border'
        >
          <div className='border-border flex flex-col gap-1 border-b px-5 py-4 sm:px-6'>
            <Skeleton className='h-[15px] w-[34%] max-w-full' />
            {card.withDescription ? (
              <Skeleton className='h-3 w-[58%] max-w-full' />
            ) : null}
          </div>
          <div className='min-w-0 px-5 py-5 sm:px-6 sm:py-6'>
            <div className='flex min-w-0 flex-col gap-5'>
              {SKELETON_ROW_KEYS.slice(0, card.rows).map((rowKey) => (
                <div
                  key={rowKey}
                  className='grid min-w-0 gap-x-6 gap-y-2 sm:grid-cols-[minmax(0,5fr)_minmax(0,6fr)] sm:items-center'
                >
                  <div className='flex min-w-0 flex-col gap-1.5'>
                    <Skeleton className='h-3.5 w-[62%] max-w-full' />
                    {rowKey === 'first' || rowKey === 'third' ? (
                      <Skeleton className='h-3 w-[86%] max-w-full' />
                    ) : null}
                  </div>
                  <Skeleton className='h-9 w-full min-w-0 rounded-[0.5rem]' />
                </div>
              ))}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
