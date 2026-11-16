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
import { spawnSync } from 'node:child_process'
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

import { afterEach, beforeEach, expect, test } from 'vitest'

let fixture: string

beforeEach(() => {
  fixture = mkdtempSync(join(tmpdir(), 'adty-i18n-check-'))
  mkdirSync(join(fixture, 'scripts/adty'), { recursive: true })
  mkdirSync(join(fixture, 'src/i18n/locales'), { recursive: true })
  copyFileSync(
    resolve('scripts/adty/i18n-check.mjs'),
    join(fixture, 'scripts/adty/i18n-check.mjs')
  )
  writeFileSync(join(fixture, 'scripts/adty/upstream-en-keys.json'), '[]')
})

afterEach(() => rmSync(fixture, { recursive: true, force: true }))

function checkTranslations(
  en: Record<string, string>,
  zh: Record<string, string>
) {
  for (const [name, translation] of Object.entries({ en, zh })) {
    writeFileSync(
      join(fixture, `src/i18n/locales/${name}.json`),
      JSON.stringify({ translation })
    )
  }
  return spawnSync(
    process.execPath,
    [join(fixture, 'scripts/adty/i18n-check.mjs')],
    {
      encoding: 'utf8',
    }
  )
}

test('formal translations pass with semantic English labels and no unloaded legacy packs', () => {
  const result = checkTranslations(
    { 'Ticket status new': 'New', 'Refund order': 'Manual refund' },
    { 'Ticket status new': '新建', 'Refund order': '人工退款' }
  )
  expect(result.stderr).toBe('')
  expect(result.status).toBe(0)
})

test('a missing Chinese translation fails with the key named', () => {
  const result = checkTranslations(
    { 'New notification': 'New notification' },
    {}
  )
  expect(result.status).toBe(1)
  expect(result.stderr).toContain('zh: missing "New notification"')
})

test('English fallback text in a Chinese translation fails', () => {
  const result = checkTranslations(
    { 'New notification': 'New notification' },
    { 'New notification': 'New notification' }
  )
  expect(result.status).toBe(1)
  expect(result.stderr).toContain('zh: same as English "New notification"')
})

test.each(['Ticket status new', ' Ticket status new ', ' New '])(
  'semantic English labels reject the Chinese fallback %j',
  (fallback) => {
    const result = checkTranslations(
      { 'Ticket status new': 'New' },
      { 'Ticket status new': fallback }
    )
    expect(result.status).toBe(1)
    expect(result.stderr).toContain('zh: same as English "Ticket status new"')
  }
)

test('literal technical names may stay the same in both formal languages', () => {
  const result = checkTranslations({ USDT: 'USDT' }, { USDT: 'USDT' })
  expect(result.stderr).toBe('')
  expect(result.status).toBe(0)
})

test.each(['', '   '])(
  'a blank literal translation %j still fails',
  (blank) => {
    const result = checkTranslations({ USDT: 'USDT' }, { USDT: blank })
    expect(result.status).toBe(1)
    expect(result.stderr).toContain('zh: missing "USDT"')
  }
)

test('a Chinese key without an English translation fails with the key named', () => {
  const result = checkTranslations({}, { 'New notification': '新通知' })
  expect(result.status).toBe(1)
  expect(result.stderr).toContain('en: missing "New notification"')
})
