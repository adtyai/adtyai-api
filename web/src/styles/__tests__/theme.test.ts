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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { THEME_PRESETS } from '@/lib/theme-customization'

// jsdom does not load stylesheets or resolve var(), so the white theme
// contract (design draft section 6) is checked on the CSS source itself.
const webSrc = resolve(import.meta.dirname, '../..')

function readSource(path: string): string {
  return readFileSync(resolve(webSrc, path), 'utf8')
}

/** Declarations of the first rule whose selector line is exactly `selector`. */
function ruleBody(css: string, selector: string): Map<string, string> {
  const start = css.indexOf(`\n${selector} {\n`)
  expect(start, `rule ${selector}`).toBeGreaterThanOrEqual(0)
  const open = css.indexOf('{', start)
  const close = css.indexOf('\n}', open)
  const body = css
    .slice(open + 1, close)
    .replaceAll(/\/\*[\s\S]*?\*\//g, '')
    .replaceAll(/\s+/g, ' ')
  const declarations = new Map<string, string>()
  for (const part of body.split(';')) {
    const colon = part.indexOf(':')
    if (colon < 0) continue
    const name = part.slice(0, colon).trim()
    if (name.startsWith('--')) {
      declarations.set(
        name,
        part
          .slice(colon + 1)
          .trim()
          .toLowerCase()
      )
    }
  }
  return declarations
}

const themeCss = readSource('styles/theme.css')
const themeInline = ruleBody(themeCss, '@theme inline')
const light = ruleBody(themeCss, ':root')
const dark = ruleBody(themeCss, '.dark')

describe('white theme radius', () => {
  it('sets an 8px base radius so buttons and inputs (rounded-lg) are 8px', () => {
    expect(light.get('--radius')).toBe('0.5rem')
    expect(themeInline.get('--radius-lg')).toBe('var(--radius)')
  })

  it('scales rounded-xl by 1.5 so cards are 12px', () => {
    expect(themeInline.get('--radius-xl')).toBe('calc(var(--radius) * 1.5)')
  })

  it('keeps badges (rounded-4xl) pill shaped above 20px', () => {
    expect(themeInline.get('--radius-4xl')).toBe('calc(var(--radius) * 2.6)')
  })
})

describe('white theme colors', () => {
  it.each([
    ['--background', '#ffffff', '#0a0a0a'],
    ['--card', '#ffffff', '#141414'],
    ['--popover', '#ffffff', '#141414'],
    ['--sidebar', '#ffffff', '#0a0a0a'],
    ['--canvas', '#fafafa', '#0a0a0a'],
    ['--border', '#e5e5e5', '#262626'],
    ['--input', '#e5e5e5', '#262626'],
    ['--foreground', '#0a0a0a', '#fafafa'],
    ['--muted-foreground', '#737373', '#a3a3a3'],
    ['--primary', '#0a0a0a', '#fafafa'],
    ['--primary-foreground', '#ffffff', '#0a0a0a'],
    ['--secondary', '#f5f5f5', '#1f1f1f'],
    ['--muted', '#f5f5f5', '#1f1f1f'],
    ['--accent', '#f5f5f5', '#1f1f1f'],
    ['--sidebar-accent', '#f5f5f5', '#1f1f1f'],
    ['--secondary-foreground', '#0a0a0a', '#fafafa'],
    ['--sidebar-accent-foreground', '#0a0a0a', '#fafafa'],
    ['--sidebar-primary', '#0a0a0a', '#fafafa'],
    ['--ring', '#a3a3a3', '#525252'],
    ['--sidebar-ring', '#a3a3a3', '#525252'],
    ['--success', '#16a34a', '#4ade80'],
    ['--warning', '#b45309', '#fbbf24'],
    ['--destructive', '#dc2626', '#f87171'],
    ['--info', '#1d4ed8', '#60a5fa'],
    ['--chart-1', '#0a0a0a', '#e5e5e5'],
    ['--chart-2', '#525252', '#d4d4d4'],
    ['--chart-3', '#a3a3a3', '#a3a3a3'],
    ['--chart-4', '#d4d4d4', '#525252'],
    ['--chart-5', '#e5e5e5', '#0a0a0a'],
  ])(
    '%s is %s in light mode and %s in dark mode',
    (name, lightHex, darkHex) => {
      expect(light.get(name)).toBe(lightHex)
      expect(dark.get(name)).toBe(darkHex)
    }
  )

  it.each([
    ['success', '#f0fdf4'],
    ['warning', '#fffbeb'],
    ['destructive', '#fef2f2'],
    ['info', '#eff6ff'],
  ])(
    'the %s-soft tint is %s in light mode and a 15 percent mix in dark mode',
    (name, lightHex) => {
      expect(light.get(`--${name}-soft`)).toBe(lightHex)
      expect(dark.get(`--${name}-soft`)).toBe(
        `color-mix(in srgb, var(--${name}) 15%, transparent)`
      )
    }
  )

  it('keeps the overview accents gray in both modes', () => {
    for (const vars of [light, dark]) {
      for (const index of [1, 2, 3]) {
        expect(vars.get(`--overview-accent-${index}`)).toMatch(
          /^#([0-9a-f]{2})\1\1$/
        )
      }
    }
  })

  it('maps the new canvas and soft tokens to Tailwind color utilities', () => {
    expect(themeInline.get('--color-canvas')).toBe('var(--canvas)')
    for (const name of ['success', 'warning', 'destructive', 'info']) {
      expect(themeInline.get(`--color-${name}-soft`)).toBe(
        `var(--${name}-soft)`
      )
    }
  })

  it('paints the console content area with the canvas color', () => {
    expect(
      readSource('components/layout/components/authenticated-layout.tsx')
    ).toMatch(/<SidebarInset[\s\S]*?'bg-canvas'/)
  })

  it('previews the default preset as black and white in the theme drawer', () => {
    const preset = THEME_PRESETS.find((item) => item.value === 'default')
    expect(preset?.swatches).toEqual(['#0A0A0A', '#FFFFFF'])
  })
})

describe('white theme typography', () => {
  it('uses self-hosted Inter for the interface with CJK fallbacks', () => {
    expect(light.get('--font-ui-sans')).toBe(
      "'inter variable', 'pingfang sc', 'microsoft yahei', system-ui, sans-serif"
    )
    expect(light.get('--font-body')).toBe('var(--font-ui-sans)')
  })

  it('uses self-hosted JetBrains Mono for code', () => {
    expect(light.get('--font-mono')).toBe(
      "'jetbrains mono variable', consolas, monospace"
    )
  })

  it('imports both font packages instead of loading Google Fonts', () => {
    const indexCss = readSource('styles/index.css')
    expect(indexCss).toContain("@import '@fontsource-variable/inter';")
    expect(indexCss).toContain("@import '@fontsource-variable/jetbrains-mono';")
    expect(`${indexCss}\n${themeCss}`).not.toContain('fonts.googleapis.com')
  })

  // The default stack lists Chinese fonts before the generic family, so
  // Japanese pages would show kanji in Chinese glyph forms (task 9.2).
  it('puts Japanese fonts before the Chinese ones when the page is Japanese', () => {
    const japanese = ruleBody(
      themeCss,
      ":root:lang(ja):not([data-theme-font='sans'])"
    ).get('--font-ui-sans')
    expect(japanese).toBeDefined()
    const families = (japanese ?? '').split(',').map((name) => name.trim())
    expect(families[0]).toBe("'inter variable'")
    expect(families).toContain("'hiragino sans'")
    expect(families).toContain("'yu gothic ui'")
    expect(families).not.toContain("'pingfang sc'")
    expect(families).not.toContain("'microsoft yahei'")
    expect(families.at(-1)).toBe('sans-serif')
  })

  it('puts Korean fonts on Korean pages', () => {
    const korean = ruleBody(
      themeCss,
      ":root:lang(ko):not([data-theme-font='sans'])"
    ).get('--font-ui-sans')
    expect(korean).toBeDefined()
    const families = (korean ?? '').split(',').map((name) => name.trim())
    expect(families[0]).toBe("'inter variable'")
    expect(families).toContain("'apple sd gothic neo'")
    expect(families).toContain("'malgun gothic'")
    expect(families).toContain("'noto sans kr'")
    expect(families).not.toContain("'pingfang sc'")
    expect(families).not.toContain("'microsoft yahei'")
    expect(families.at(-1)).toBe('sans-serif')
  })
})

describe('white theme tables', () => {
  const tableSource = readSource('components/ui/table.tsx')

  it('renders 44px body rows', () => {
    expect(tableSource).toContain("'[&>tr]:h-11 [&_tr:last-child]:border-0'")
    expect(tableSource).not.toContain('[&>tr]:h-15')
  })

  it('renders header labels in the secondary text color', () => {
    expect(tableSource).toMatch(
      /data-slot='table-head'[\s\S]*?'text-muted-foreground h-10 /
    )
  })
})
