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
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
const username = process.env.ADTY_BROWSER_USERNAME
const password = process.env.ADTY_BROWSER_PASSWORD
assert.ok(base && username && password && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const errors = []
async function signIn(page, name, secret) {
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(name)
  await page.getByLabel('Password', { exact: true }).fill(secret)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
}
async function session(name, secret) {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  page.on('pageerror', (error) => errors.push(error.message))
  const result = { page, headers: undefined }
  page.on('request', (request) => {
    if (request.headers().authorization) {
      result.headers = { authorization: request.headers().authorization }
    }
  })
  await signIn(page, name, secret)
  assert.ok(result.headers?.authorization)
  return result
}
async function refreshInbox(page) {
  const responsePromise = page.waitForResponse(
    (response) => {
      const request = response.request()
      return (
        new URL(response.url()).pathname === '/api/adty/notifications' &&
        request.method() === 'GET'
      )
    },
    { timeout: 10_000 }
  )
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')))
  const response = await responsePromise
  assert.equal(response.status(), 200)
}
async function count(page, expected) {
  const bell = page.getByRole('button', { name: 'Notifications', exact: true })
  await bell.waitFor({ state: 'visible' })
  if (expected) {
    await bell.getByText(String(expected), { exact: true }).waitFor()
  } else {
    await page.waitForFunction(() => {
      const button = document.querySelector(
        'button[aria-label="Notifications"]'
      )
      return button !== null && button.textContent.trim() === ''
    })
  }
}
async function signOut(page) {
  await page
    .locator('button')
    .filter({ has: page.locator('[data-slot="avatar"]') })
    .click()
  await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
  await page
    .getByRole('alertdialog')
    .getByRole('button', { name: 'Sign out', exact: true })
    .click()
  await page.getByLabel('Username or Email').waitFor()
}
try {
  const root = await session(username, password)
  async function option(key, value) {
    const response = await root.page.request.put(`${base}/api/option/`, {
      headers: root.headers,
      data: { key, value },
    })
    assert.equal(response.status(), 200)
    const body = await response.json()
    assert.equal(body.success, true, body.message)
  }
  const secret = `LocalReceipt!${randomBytes(12).toString('hex')}`
  for (const name of ['receipt-a', 'receipt-b']) {
    const response = await root.page.request.post(`${base}/api/user/`, {
      headers: root.headers,
      data: { username: name, password: secret, role: 1 },
    })
    assert.equal(response.status(), 200)
    const body = await response.json()
    assert.equal(body.success, true, body.message)
  }
  await option('console_setting.announcements_enabled', 'true')
  const a = await session('receipt-a', secret)
  const device = await session('receipt-a', secret)
  for (const width of [1440, 390]) {
    for (const current of [a, device]) {
      await current.page.setViewportSize({ width, height: 1000 })
    }
    const notice = `Receipt notice ${width}`
    const timeline = `Receipt timeline ${width}`
    await option('Notice', notice)
    await option(
      'console_setting.announcements',
      JSON.stringify([
        {
          id: 7,
          content: timeline,
          publishDate: '2026-11-09T00:00:00Z',
          type: 'default',
        },
      ])
    )
    for (const current of [a, device]) {
      await refreshInbox(current.page)
      await count(current.page, 2)
    }
    await a.page
      .getByRole('button', { name: 'Notifications', exact: true })
      .click()
    await a.page
      .getByRole('tabpanel', { name: 'Notice', exact: true })
      .getByText(notice, { exact: true })
      .waitFor()
    await count(a.page, 1)
    await a.page.getByRole('button', { name: 'Close', exact: true }).click()
    await refreshInbox(device.page)
    await count(device.page, 1)
    await a.page
      .getByRole('button', { name: 'Notifications', exact: true })
      .click()
    await a.page.getByRole('tab', { name: 'Timeline', exact: true }).click()
    await a.page
      .getByRole('tabpanel', { name: 'Timeline', exact: true })
      .getByText(timeline, { exact: true })
      .waitFor()
    await count(a.page, 0)
    await a.page.getByRole('button', { name: 'Close', exact: true }).click()
    await refreshInbox(device.page)
    await count(device.page, 0)
    if (width === 1440) {
      await signOut(a.page)
      await signIn(a.page, 'receipt-b', secret)
      await count(a.page, 2)
      await signOut(a.page)
      await signIn(a.page, 'receipt-a', secret)
      await count(a.page, 0)
    }
    await option(
      'console_setting.announcements',
      JSON.stringify([
        {
          id: 7,
          content: `${timeline} edited`,
          publishDate: '2026-11-09T00:00:00Z',
          type: 'default',
        },
      ])
    )
    await refreshInbox(device.page)
    await count(device.page, 1)
    console.log(
      `PASS ${width}px real notice/timeline receipts, independent account, separate device, edited same-ID content`
    )
  }
  let blockInbox = true
  await device.page.route('**/api/adty/notifications', (route) =>
    blockInbox ? route.abort() : route.continue()
  )
  await device.page.evaluate(() =>
    window.dispatchEvent(new Event('visibilitychange'))
  )
  await device.page
    .getByRole('button', { name: 'Notifications', exact: true })
    .click()
  await device.page
    .getByRole('button', { name: 'Retry', exact: true })
    .waitFor({ timeout: 45000 })
  blockInbox = false
  await device.page.getByRole('button', { name: 'Retry', exact: true }).click()
  await device.page
    .getByRole('tabpanel', { name: 'Notice', exact: true })
    .getByText('Receipt notice 390', { exact: true })
    .waitFor()
  let rows
  try {
    rows = execFileSync(
      'psql',
      [
        process.env.ADTY_BROWSER_SQL_DSN,
        '-XAt',
        '-v',
        'ON_ERROR_STOP=1',
        '-c',
        "SELECT u.username || ':' || COUNT(r.read_key) FROM users u LEFT JOIN adty_notification_reads r ON r.user_id = u.id WHERE u.username IN ('receipt-a','receipt-b') GROUP BY u.username ORDER BY u.username",
      ],
      { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }
    ).trim()
  } catch {
    throw new Error('Isolated receipt SQL evidence query failed')
  }
  assert.equal(rows, 'receipt-a:4\nreceipt-b:0')
  assert.deepEqual(errors, [])
  console.log(
    'PASS real sign-out/account switch, network failure/retry, durable SQL receipts; pageerrors=0'
  )
} finally {
  await browser.close()
}
