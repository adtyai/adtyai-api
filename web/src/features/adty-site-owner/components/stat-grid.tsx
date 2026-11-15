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

export type OwnerStat = {
  label: string
  value: string
  /** Shown in the destructive colour, e.g. a negative balance. */
  negative?: boolean
}

/** Row of headline numbers drawn as connected cells (1px hairlines, 14px
 *  radius); negative amounts are marked with data-negative for styling and tests. */
export function OwnerStatGrid(props: { items: OwnerStat[]; label: string }) {
  return (
    <section
      aria-label={props.label}
      className='border-border bg-border grid grid-cols-2 gap-px overflow-hidden rounded-[14px] border lg:grid-cols-4'
    >
      {props.items.map((item) => (
        <div key={item.label} className='bg-card flex flex-col gap-1 px-5 py-4'>
          <span className='text-dim text-[12px] leading-none'>{item.label}</span>
          <span
            data-negative={item.negative ? 'true' : undefined}
            className={cn(
              'text-[19px] leading-tight font-semibold tabular-nums',
              item.negative && 'text-destructive'
            )}
          >
            {item.value}
          </span>
        </div>
      ))}
      {/* Keep the trailing cells white when a row is not full; no copy. */}
      {Array.from({ length: (4 - (props.items.length % 4)) % 4 }).map(
        (_, index) => (
          <div
            key={`stat-filler-${index}`}
            aria-hidden
            className='bg-card px-5 py-4'
          />
        )
      )}
    </section>
  )
}
