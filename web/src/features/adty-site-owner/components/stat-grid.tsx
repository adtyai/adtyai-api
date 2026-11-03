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
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

export type OwnerStat = {
  label: string
  value: string
  /** Shown in the destructive colour, e.g. a negative balance. */
  negative?: boolean
}

/** Row of headline numbers; negative amounts are marked with data-negative for styling and tests. */
export function OwnerStatGrid(props: { items: OwnerStat[]; label: string }) {
  return (
    <section
      aria-label={props.label}
      className='grid grid-cols-2 gap-3 lg:grid-cols-4'
    >
      {props.items.map((item) => (
        <Card key={item.label}>
          <CardContent className='flex flex-col gap-1 py-4'>
            <span className='text-muted-foreground text-xs'>{item.label}</span>
            <span
              data-negative={item.negative ? 'true' : undefined}
              className={cn(
                'text-xl font-semibold',
                item.negative && 'text-destructive'
              )}
            >
              {item.value}
            </span>
          </CardContent>
        </Card>
      ))}
    </section>
  )
}
