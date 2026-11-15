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
import { useEffect, useState } from 'react'

import { useTheme } from '@/context/theme-provider'

import { resolveThemeColor } from './theme-color'

/**
 * The shared chart palette. Blue is the only accent colour in the design
 * system, so every chart reads the same blue → grey ramp instead of the
 * rainbow default palettes of the chart library. The values live in
 * `styles/theme.css` as `--chart-1` … `--chart-5` (light and dark).
 */
export const CHART_PALETTE_VARS = [
  '--chart-1',
  '--chart-2',
  '--chart-3',
  '--chart-4',
  '--chart-5',
] as const

/** Mirrors theme.css, used when the stylesheet is not available (SSR, jsdom). */
export const CHART_PALETTE_FALLBACK = {
  light: ['#0070ec', '#4fa3ff', '#8fc2ff', '#a3a3a3', '#d4d4d4'],
  dark: ['#3d94ff', '#1a6fd4', '#9a9a9a', '#525252', '#2a2a2a'],
} as const

const HEX_COLOUR = /^#(?:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i

function isDarkTheme(): boolean {
  if (typeof document === 'undefined') return false
  return document.documentElement.classList.contains('dark')
}

/**
 * One token, as a colour the canvas charts accept. Hex values are used as
 * written; other colour syntaxes (a theme preset may ship `oklch()`) are
 * normalized through a canvas by `resolveThemeColor`.
 */
function readPaletteEntry(token: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(token)
    .trim()
  if (HEX_COLOUR.test(raw)) return raw
  if (!raw) return fallback
  return resolveThemeColor(token) ?? fallback
}

/** The palette for the theme currently on `<html>`, read from CSS variables. */
export function readChartPalette(): string[] {
  const fallback = isDarkTheme()
    ? CHART_PALETTE_FALLBACK.dark
    : CHART_PALETTE_FALLBACK.light
  return CHART_PALETTE_VARS.map((token, index) =>
    readPaletteEntry(token, fallback[index] ?? fallback[0])
  )
}

/** Repeats the palette so an ordinal scale of `length` series stays in-brand. */
export function chartPaletteRange(
  length: number,
  palette: readonly string[] = readChartPalette()
): string[] {
  if (length <= 0 || palette.length === 0) return []
  return Array.from(
    { length },
    (_, index) => palette[index % palette.length] as string
  )
}

/* Opacity lost per lap round the palette, and the floor it stops at. */
const REPEAT_ALPHA_STEP = 0.2
const MIN_REPEAT_ALPHA = 0.4

function withAlpha(colour: string, alpha: number): string {
  const match = /^#([0-9a-f]{6})$/i.exec(colour.trim())
  if (!match?.[1]) return colour
  const value = Number.parseInt(match[1], 16)
  const r = (value >> 16) & 0xff
  const g = (value >> 8) & 0xff
  const b = value & 0xff
  return `rgba(${r}, ${g}, ${b}, ${Number(alpha.toFixed(2))})`
}

/**
 * `chartPaletteRange` for leaderboards, where the series count routinely runs
 * past the five slots (the vendor chart lists up to twelve). Each lap round the
 * ramp drops the opacity one step, so series 6-10 read as a muted version of
 * 1-5 instead of an exact duplicate of them. Non-hex entries (a theme preset
 * may ship `oklch()`) are not converted, so those simply repeat.
 */
export function chartPaletteRangeFaded(
  length: number,
  palette: readonly string[] = readChartPalette()
): string[] {
  if (length <= 0 || palette.length === 0) return []
  const size = palette.length
  return Array.from({ length }, (_, index) => {
    const colour = palette[index % size] as string
    const lap = Math.floor(index / size)
    if (lap === 0) return colour
    const alpha = Math.max(MIN_REPEAT_ALPHA, 1 - lap * REPEAT_ALPHA_STEP)
    return withAlpha(colour, alpha)
  })
}

/**
 * The palette, re-read when the light/dark class or the theme preset changes.
 * Chart components remount on theme change, so this keeps the series colours
 * in sync with the CSS variables they came from.
 */
export function useChartPalette(): string[] {
  const { resolvedTheme } = useTheme()
  const [palette, setPalette] = useState<string[]>(readChartPalette)

  useEffect(() => {
    const update = () => setPalette(readChartPalette())
    update()

    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
    observer.observe(document.body, {
      attributes: true,
      attributeFilter: ['data-theme-preset'],
    })
    return () => observer.disconnect()
  }, [resolvedTheme])

  return palette
}
