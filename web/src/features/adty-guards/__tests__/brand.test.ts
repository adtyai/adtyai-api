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
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

// Task 8.9: visitors, API callers and browser devtools no longer see the
// upstream product names. An upstream merge that brings one of these values
// back fails here; see task 8.9's table for the replacement and
// docs/adty/conventions.md for what is intentionally kept.
const FORBIDDEN_VALUES = [
  'new_api_error',
  'new_api_panic',
  'new_api_refresh',
  'new_api_has_session',
  'new-api-dashboard',
  'X-New-Api-Other-Ratios',
  'Calcium-Ion/new-api',
  'docs.newapi.pro',
  'newapi:',
  'new-api:',
  'NEW_API_KEY',
]

// Only files from the "kept" list belong here.
const ALLOWED: Record<string, string[]> = {}

const webRoot = resolve(import.meta.dirname, '../../../..')
const webSrc = resolve(webRoot, 'src')

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) {
      return name === '__tests__' ? [] : sourceFiles(path)
    }
    if (/\.test\.tsx?$/.test(name) || !/\.(ts|tsx|json|css)$/.test(name)) {
      return []
    }
    return [path]
  })
}

function violationsIn(path: string): string[] {
  const rel = relative(webRoot, path).replaceAll('\\', '/')
  const allowed = ALLOWED[rel] ?? []
  return readFileSync(path, 'utf8')
    .split('\n')
    .flatMap((line, index) =>
      FORBIDDEN_VALUES.filter(
        (value) => line.includes(value) && !allowed.includes(value)
      ).map((value) => `${rel}:${index + 1} ${value}`)
    )
}

describe('public-facing brand', () => {
  it('keeps the upstream names out of web/src and index.html', () => {
    const files = [...sourceFiles(webSrc), resolve(webRoot, 'index.html')]

    expect(files.length).toBeGreaterThan(500)
    expect(files.flatMap(violationsIn)).toEqual([])
  })

  it('titles the page and declares the ADTY icons in index.html', () => {
    const html = readFileSync(resolve(webRoot, 'index.html'), 'utf8')

    expect(html).toContain('<title>ADTY AI</title>')
    expect(html).toContain('<link rel="icon" href="/favicon.ico"')
    expect(html).toContain(
      '<link rel="apple-touch-icon" href="/adty-apple-touch-icon.png"'
    )
  })
})
