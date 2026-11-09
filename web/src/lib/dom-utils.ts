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
import type { SystemConfig } from '@/stores/system-config-store'

export function applyFaviconToDom(url: string) {
  if (typeof document === 'undefined' || !url) return
  try {
    const next = new URL(url, window.location.href).href
    const existing =
      document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]')
    if (existing.length === 1 && existing[0].href === next) return
    const link = document.createElement('link')
    link.rel = 'icon'
    link.href = url
    existing.forEach((l) => l.remove())
    document.head.appendChild(link)
  } catch {
    // Ignore malformed URLs
  }
}

export function resolveBrandThemeColor(value: unknown): string {
  return typeof value === 'string' && /^#[0-9a-f]{6}$/i.test(value) ? value : ''
}

/** Apply only validated brand fields; empty fields restore the platform defaults. */
export function applySystemBrandingToDom(config: Partial<SystemConfig>) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const title = config.siteTitle || config.systemName
  if (title && !root.hasAttribute('data-adty-seo')) {
    document.title = title
    document.querySelector('meta[name="title"]')?.setAttribute('content', title)
  }
  applyFaviconToDom(config.favicon || config.logo || '/favicon.ico')
  const color = resolveBrandThemeColor(config.themeColor)
  let foreground = ''
  if (color) {
    const channels = [1, 3, 5].map((offset) => {
      const channel = Number.parseInt(color.slice(offset, offset + 2), 16) / 255
      return channel <= 0.04045
        ? channel / 12.92
        : ((channel + 0.055) / 1.055) ** 2.4
    })
    const luminance =
      0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
    foreground =
      (luminance + 0.05) / 0.05 >= 1.05 / (luminance + 0.05)
        ? '#000000'
        : '#ffffff'
  }
  const styles = {
    '--primary': color,
    '--primary-foreground': foreground,
    '--ring': color,
    '--sidebar-primary': color,
    '--sidebar-primary-foreground': foreground,
    '--sidebar-ring': color,
  }
  for (const [key, value] of Object.entries(styles)) {
    if (value) root.style.setProperty(key, value)
    else root.style.removeProperty(key)
  }
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute(
      'content',
      color || (root.classList.contains('dark') ? '#020817' : '#fff')
    )
}
