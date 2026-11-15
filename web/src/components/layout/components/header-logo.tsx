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

interface HeaderLogoProps {
  src: string
  /**
   * Optional dark-theme artwork (white wordmark). Swapped in through the
   * class-based `dark` variant, so it also works in the prerendered HTML.
   */
  darkSrc?: string
  alt?: string
  loading: boolean
  logoLoaded: boolean
  className?: string
  /**
   * Which artwork `src` points at:
   * - `full` (default): the complete horizontal lockup, which already contains
   *   the ADTY wordmark. Fixed height, automatic width, never cropped or
   *   rounded — never render a text site name next to it.
   * - `mark`: square glyph-only artwork for icon slots, also uncropped.
   */
  variant?: 'full' | 'mark'
}

/**
 * Logo component for header with loading state
 * Shows image only when fully loaded for smooth UX
 */
export function HeaderLogo({
  src,
  darkSrc,
  alt = 'logo',
  loading,
  logoLoaded,
  className,
  variant = 'full',
}: HeaderLogoProps) {
  const classes = cn(
    'object-contain transition-opacity duration-200',
    variant === 'full' ? 'h-7 w-auto max-w-none' : 'size-7',
    // A site-supplied logo keeps its own colours, so on a dark theme it may
    // carry a dark wordmark. Rather than recolouring someone else's artwork,
    // park it on a light chip. The bundled ADTY artwork swaps to a white
    // wordmark instead and needs no backing.
    !darkSrc && 'dark:bg-white/10 dark:rounded-md dark:px-1 dark:py-0.5',
    !loading && logoLoaded ? 'opacity-100' : 'opacity-0',
    className
  )

  if (!darkSrc) {
    return <img src={src} alt={alt} className={classes} />
  }

  return (
    <>
      <img src={src} alt={alt} className={cn(classes, 'dark:hidden')} />
      <img
        src={darkSrc}
        alt=''
        aria-hidden='true'
        className={cn(classes, 'dark:block hidden')}
      />
    </>
  )
}
