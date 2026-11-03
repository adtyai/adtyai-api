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
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * A titled block of the landing page. The heading names the region, so each
 * block is reachable as a landmark and in tests by its title.
 */
export function LandingSection(props: {
  id: string
  title: string
  description?: string
  action?: ReactNode
  className?: string
  children: ReactNode
}) {
  const headingId = `${props.id}-title`
  return (
    <section
      id={props.id}
      aria-labelledby={headingId}
      className={cn('border-t py-16 md:py-20', props.className)}
    >
      <div className='mx-auto max-w-6xl px-4 md:px-6'>
        <div className='mb-8 flex flex-wrap items-end justify-between gap-4'>
          <div className='space-y-2'>
            <h2
              id={headingId}
              className='text-2xl font-semibold tracking-tight md:text-3xl'
            >
              {props.title}
            </h2>
            {props.description && (
              <p className='text-muted-foreground max-w-2xl text-sm'>
                {props.description}
              </p>
            )}
          </div>
          {props.action}
        </div>
        {props.children}
      </div>
    </section>
  )
}
