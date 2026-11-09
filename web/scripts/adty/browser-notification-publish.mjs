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
assert.ok(base && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
  args: ['--host-resolver-rules=MAP *.sites.localhost 127.0.0.1'],
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const errors = []
function postgres(query) {
  try {
    return execFileSync(
      'psql',
      [
        process.env.ADTY_BROWSER_SQL_DSN,
        '-XAt',
        '-v',
        'ON_ERROR_STOP=1',
        '-c',
        query,
      ],
      { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }
    ).trim()
  } catch {
    throw new Error('Isolated PostgreSQL evidence query failed')
  }
}
async function session(name, password, origin = base) {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  const state = { page, origin, headers: undefined }
  page.on('pageerror', (error) => {
    errors.push(error.message)
    console.error('notification-publish: page error:', error.message)
  })
  page.on('console', async (message) => {
    if (message.type() !== 'error') return
    for (const argument of message.args()) {
      const failure = await argument.evaluate((value) =>
        value instanceof Error
          ? { name: value.name, message: value.message }
          : null
      )
      if (failure) console.error('notification-publish: render error:', failure)
    }
  })
  page.on('request', (request) => {
    if (request.headers().authorization) {
      state.headers = { authorization: request.headers().authorization }
    }
  })
  await page.goto(`${origin}/sign-in`)
  try {
    await page.getByLabel('Username or Email').waitFor({ timeout: 10000 })
  } catch {
    console.error(
      'notification-publish: sign-in page:',
      new URL(page.url()).pathname,
      (await page.locator('body').innerText()).slice(0, 600)
    )
    throw new Error('Isolated sign-in form did not load')
  }
  await page.getByLabel('Username or Email').fill(name)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  const overview = page.getByRole('heading', { name: 'Overview', exact: true })
  const terms = page.getByRole('button', {
    name: 'I agree to the site customer terms',
    exact: true,
  })
  await overview.or(terms).waitFor()
  if (await terms.isVisible()) await terms.click()
  await overview.waitFor()
  assert.ok(state.headers?.authorization)
  return state
}
async function api(state, method, path, data) {
  assert.equal(new URL(state.page.url()).origin, state.origin)
  const response = await state.page.evaluate(
    async ({ method, path, data, headers }) => {
      const result = await fetch(path, {
        method: method.toUpperCase(),
        headers: { ...headers, 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        ...(data === undefined ? {} : { body: JSON.stringify(data) }),
      })
      return { status: result.status, body: await result.json() }
    },
    { method, path, data, headers: state.headers }
  )
  assert.equal(response.status, 200, path)
  const body = response.body
  assert.equal(body.success, true, body.message || path)
  return body.data
}
async function refresh(state) {
  const response = state.page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === '/api/adty/notifications' &&
      r.request().method() === 'GET',
    { timeout: 10000 }
  )
  await state.page.evaluate(() =>
    window.dispatchEvent(new Event('visibilitychange'))
  )
  assert.equal((await response).status(), 200)
}
async function count(page, expected) {
  const bell = page.getByRole('button', { name: 'Notifications', exact: true })
  await bell.waitFor()
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
async function publish(state, title, audience, siteId) {
  await state.page
    .getByRole('button', { name: 'New notification', exact: true })
    .click()
  const dialog = state.page.getByRole('dialog', {
    name: 'New notification',
    exact: true,
  })
  await dialog.getByLabel('Title', { exact: true }).fill(title)
  await dialog.getByLabel('Content', { exact: true }).fill(`Body ${title}`)
  await dialog
    .getByLabel('Related business', { exact: true })
    .fill('Synthetic service maintenance')
  if (audience) {
    await dialog
      .getByLabel('Recipients', { exact: true })
      .selectOption(audience)
    await dialog
      .getByLabel('Site', { exact: true })
      .selectOption(String(siteId))
    await dialog
      .getByLabel('Notification type', { exact: true })
      .selectOption('maintenance')
  } else {
    assert.equal(
      await dialog.getByLabel('Recipients', { exact: true }).isDisabled(),
      true
    )
  }
  const pending = state.page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith('/notifications') &&
      r.request().method() === 'POST'
  )
  await dialog
    .getByRole('button', { name: 'Publish notification', exact: true })
    .click()
  const result = await (await pending).json()
  assert.equal(result.success, true, result.message)
  await dialog.waitFor({ state: 'hidden' })
  return result.data
}
try {
  const root = await session(
    process.env.ADTY_BROWSER_USERNAME,
    process.env.ADTY_BROWSER_PASSWORD
  )
  await api(root, 'put', '/api/adty/admin/reseller/settings', {
    enabled: true,
    root_domains: ['sites.localhost'],
    cost_factor: 0.8,
    min_cost_factor: 0.5,
    max_markup: 3,
    max_site_bonus_extra: 0,
    trial_grant_max_usd: 0,
    trial_grant_daily_usd: 0,
  })
  for (const [key, value] of [
    [
      'adty_legal_setting.site_agreement',
      JSON.stringify({ en: 'Synthetic owner agreement.' }),
    ],
    ['adty_legal_setting.site_agreement_version', '1'],
    [
      'adty_legal_setting.site_customer_terms',
      JSON.stringify({ en: 'Synthetic customer terms.' }),
    ],
    ['adty_legal_setting.site_customer_terms_version', '1'],
  ]) {
    await api(root, 'put', '/api/option/', { key, value })
  }
  const password = `LocalNotice!${randomBytes(12).toString('hex')}`
  const ids = new Map()
  for (const name of [
    'noticeowner',
    'noticeother',
    'noticecustomer',
    'noticeoutsider',
  ]) {
    await api(root, 'post', '/api/user/', { username: name, password, role: 1 })
    const id = Number(
      postgres(`SELECT id FROM users WHERE username = '${name}'`)
    )
    assert.ok(Number.isSafeInteger(id) && id > 0)
    ids.set(name, id)
  }
  const shop = await api(root, 'post', '/api/adty/admin/sites', {
    user_id: ids.get('noticeowner'),
    subdomain: 'noticeshop',
    root_domain: 'sites.localhost',
  })
  const other = await api(root, 'post', '/api/adty/admin/sites', {
    user_id: ids.get('noticeother'),
    subdomain: 'noticeother',
    root_domain: 'sites.localhost',
  })
  for (const [name, id] of [
    ['noticecustomer', shop.id],
    ['noticeoutsider', other.id],
  ]) {
    postgres(`UPDATE users SET site_id = ${id} WHERE id = ${ids.get(name)}`)
  }
  const owner = await session('noticeowner', password)
  await owner.page.goto(`${base}/site-owner/settings`)
  await owner.page
    .getByRole('button', { name: 'Accept site agreement', exact: true })
    .click()
  await owner.page
    .getByRole('button', { name: 'New notification', exact: true })
    .waitFor()
  const customer = await session(
    'noticecustomer',
    password,
    `http://noticeshop.sites.localhost:${new URL(base).port}`
  )
  const outsider = await session(
    'noticeoutsider',
    password,
    `http://noticeother.sites.localhost:${new URL(base).port}`
  )
  await root.page.goto(`${base}/system-settings/content/announcements`)
  await root.page
    .getByRole('button', { name: 'New notification', exact: true })
    .waitFor()
  for (const width of [1440, 390]) {
    for (const state of [root, owner, customer, outsider]) {
      await state.page.setViewportSize({ width, height: 1000 })
    }
    const platform = await publish(
      root,
      `Scoped maintenance ${width}`,
      'site',
      shop.id
    )
    const local = await publish(owner, `Shop announcement ${width}`)
    assert.equal(platform.target_site_id, shop.id)
    assert.equal(local.issuer_site_id, shop.id)
    await refresh(customer)
    await refresh(outsider)
    await count(customer.page, 2)
    await count(outsider.page, 0)
    const outsiderInbox = await api(outsider, 'get', '/api/adty/notifications')
    assert.equal(outsiderInbox.announcements.length, 0)
    const mainInbox = await api(root, 'get', '/api/adty/notifications')
    assert.equal(mainInbox.announcements.length, 0)
    await customer.page
      .getByRole('button', { name: 'Notifications', exact: true })
      .click()
    await customer.page
      .getByRole('tab', { name: 'Timeline', exact: true })
      .click()
    const timeline = customer.page.getByRole('tabpanel', {
      name: 'Timeline',
      exact: true,
    })
    await timeline.getByText(platform.title, { exact: true }).waitFor()
    await timeline.getByText(local.title, { exact: true }).waitFor()
    await timeline.getByText(/Publisher.*browserroot/).waitFor()
    await count(customer.page, 0)
    await customer.page
      .getByRole('button', { name: 'Close', exact: true })
      .click()
    const row = root.page
      .getByRole('table', { name: 'Published notifications', exact: true })
      .getByRole('row')
      .filter({ hasText: platform.title })
    await row.getByRole('button', { name: 'Edit', exact: true }).click()
    const editor = root.page.getByRole('dialog', {
      name: 'Edit notification',
      exact: true,
    })
    await editor.getByLabel('Content', { exact: true }).fill(`Edited ${width}`)
    const editResponse = root.page.waitForResponse(
      (r) =>
        r.request().method() === 'PUT' &&
        new URL(r.url()).pathname ===
          `/api/adty/admin/notifications/${platform.id}`
    )
    await editor
      .getByRole('button', { name: 'Save changes', exact: true })
      .click()
    assert.equal((await (await editResponse).json()).success, true)
    await editor.waitFor({ state: 'hidden' })
    await refresh(customer)
    await count(customer.page, 1)
    await row
      .getByRole('button', { name: 'Withdraw notification', exact: true })
      .click()
    const withdrawResponse = root.page.waitForResponse(
      (r) =>
        r.request().method() === 'DELETE' &&
        new URL(r.url()).pathname ===
          `/api/adty/admin/notifications/${platform.id}`
    )
    await root.page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Continue', exact: true })
      .click()
    assert.equal((await (await withdrawResponse).json()).success, true)
    await row.getByText('Withdrawn', { exact: true }).waitFor()
    await refresh(customer)
    await count(customer.page, 0)
    const receiptCount = Number(
      postgres(
        `SELECT count(*) FROM adty_notification_reads WHERE user_id = ${ids.get('noticecustomer')}`
      )
    )
    assert.equal(receiptCount, width === 1440 ? 2 : 4)
    assert.equal(
      Number(
        postgres(
          `SELECT count(*) FROM adty_notification_reads WHERE user_id = ${ids.get('noticeoutsider')}`
        )
      ),
      0
    )
    console.log(
      `notification-publish: ${width}px Root and owner publishing, Host/account isolation, attribution, receipts, editing and withdrawal passed`
    )
  }
  assert.deepEqual(errors, [])
  console.log(
    'notification-publish: PostgreSQL receipts customer=4, outsider=0, pageerrors=0'
  )
} finally {
  await browser.close()
}
