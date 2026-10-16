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
import type { SVGProps } from 'react'

import { cn } from '@/lib/utils'

/** ADTY brand mark — rounded square + lightning bolt (matches auth shell). */
export function Logo({ className, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      id='adty-logo'
      viewBox='0 0 24 24'
      xmlns='http://www.w3.org/2000/svg'
      height='24'
      width='24'
      fill='none'
      className={cn('size-6', className)}
      {...props}
    >
      <title>ADTY AI</title>
      <rect width='24' height='24' rx='5.5' className='fill-primary' />
      <path
        d='M13 2 3 14h9l-1 8 10-12h-9l1-8z'
        className='fill-primary-foreground'
      />
    </svg>
  )
}
