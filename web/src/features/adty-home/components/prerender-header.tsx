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
import { ADTY_BRAND_NAME } from '@/lib/adty-brand'

/**
 * Static header of the same height as the public header (h-16), with the
 * logo and the site name; the real header replaces it on mount.
 */
export function PrerenderHeader() {
  return (
    <header className='fixed inset-x-0 top-0 z-50'>
      <div className='mx-auto flex h-16 max-w-7xl items-center gap-2.5 px-6 md:px-8'>
        {/* The logo artwork already contains the ADTY wordmark; the brand name
            stays in the markup for assistive tech only. */}
        <img
          src='/logo.png'
          alt=''
          aria-hidden='true'
          className='h-6 w-auto max-w-none object-contain dark:hidden'
        />
        <img
          src='/logo-dark.png'
          alt=''
          aria-hidden='true'
          className='dark:block hidden h-6 w-auto max-w-none object-contain'
        />
        <span className='sr-only'>{ADTY_BRAND_NAME}</span>
      </div>
    </header>
  )
}
