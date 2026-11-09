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

import { verifyOrderTrace } from './browser-order-trace-checks.mjs'

const base = process.env.ADTY_BROWSER_BASE_URL
assert.ok(base && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
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
      {
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
      }
    ).trim()
  } catch {
    throw new Error('Isolated PostgreSQL fixture or evidence query failed')
  }
}
async function session(username, password) {
  const context = await browser.newContext({
    locale: 'en-US',
    timezoneId: 'UTC',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  const state = { page, headers: undefined }
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (request) => {
    if (request.headers().authorization) {
      state.headers = { authorization: request.headers().authorization }
    }
  })
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(state.headers?.authorization)
  return state
}
async function api(state, method, path, data) {
  return state.page.evaluate(
    async ({ method, path, data, headers }) => {
      const response = await fetch(path, {
        method,
        headers: { ...headers, 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        ...(data === undefined ? {} : { body: JSON.stringify(data) }),
      })
      return { status: response.status, body: await response.json() }
    },
    { method, path, data, headers: state.headers }
  )
}
async function success(state, method, path, data) {
  const result = await api(state, method, path, data)
  assert.equal(result.status, 200, path)
  assert.equal(result.body.success, true, result.body.message || path)
  return result.body.data
}
function orderRow(page, trade) {
  return page
    .getByRole('row')
    .filter({ has: page.getByText(trade, { exact: true }) })
}
try {
  const root = await session(
    process.env.ADTY_BROWSER_USERNAME,
    process.env.ADTY_BROWSER_PASSWORD
  )
  await success(root, 'PUT', '/api/adty/admin/reseller/settings', {
    enabled: true,
    root_domains: ['sites.localhost'],
    cost_factor: 0.8,
    min_cost_factor: 0.5,
    max_markup: 3,
    max_site_bonus_extra: 0,
    trial_grant_max_usd: 0,
    trial_grant_daily_usd: 0,
  })
  await success(root, 'PUT', '/api/option/', {
    key: 'adty_legal_setting.site_agreement',
    value: JSON.stringify({
      en: 'Synthetic order management owner agreement.',
    }),
  })
  await success(root, 'PUT', '/api/option/', {
    key: 'adty_legal_setting.site_agreement_version',
    value: '1',
  })
  const password = `LocalOrders!${randomBytes(12).toString('hex')}`
  const ids = new Map()
  for (const username of ['ordersowner', 'ordersother', 'ordersbuyer']) {
    await success(root, 'POST', '/api/user/', { username, password, role: 1 })
    const id = Number(
      postgres(`SELECT id FROM users WHERE username = '${username}'`)
    )
    assert.ok(Number.isSafeInteger(id) && id > 0)
    ids.set(username, id)
  }
  const north = await success(root, 'POST', '/api/adty/admin/sites', {
    user_id: ids.get('ordersowner'),
    subdomain: 'ordersnorth',
    root_domain: 'sites.localhost',
  })
  const south = await success(root, 'POST', '/api/adty/admin/sites', {
    user_id: ids.get('ordersother'),
    subdomain: 'orderssouth',
    root_domain: 'sites.localhost',
  })
  postgres(
    `UPDATE adty_sites SET brand = '{"name":"North Studio"}' WHERE id = ${north.id}; UPDATE adty_sites SET brand = '{"name":"South Studio"}' WHERE id = ${south.id}`
  )
  const buyer = ids.get('ordersbuyer')
  // Orders are historical fixtures. The buyer's current site intentionally differs.
  for (const [trade, siteID] of [
    ['m02-north-order', north.id],
    ['m02-south-order', south.id],
    ['m02-main-order', 0],
    ['m02-legacy-order', null],
  ]) {
    postgres(
      `INSERT INTO top_ups (user_id, amount, money, trade_no, payment_provider, payment_method, create_time, complete_time, status) VALUES (${buyer}, 2, 2, '${trade}', 'stripe', 'stripe', 1767225600, 1767225660, 'success')`
    )
    if (siteID !== null) {
      postgres(
        `INSERT INTO adty_topup_orders (trade_no, user_id, site_id, provider, method, face_usd, bonus_usd, credit_quota, pay_amount, pay_currency, paid_amount, paid_currency, provider_environment, created_at) VALUES ('${trade}', ${buyer}, ${siteID}, 'stripe', 'stripe', 2, 0, 1000000, 2, 'USD', 2, 'USD', 'test', 1767225600)`
      )
    }
  }
  postgres(`UPDATE users SET site_id = ${south.id} WHERE id = ${buyer}`)
  const owner = await session('ordersowner', password)
  await owner.page.goto(`${base}/site-owner/orders`)
  await owner.page
    .getByRole('button', { name: 'Accept site agreement', exact: true })
    .click()
  await orderRow(owner.page, 'm02-north-order').waitFor()
  for (const width of [1440, 390]) {
    for (const state of [root, owner]) {
      await state.page.setViewportSize({ width, height: 1000 })
    }
    await root.page.goto(`${base}/recharge-audit`)
    await root.page
      .getByRole('heading', { name: 'Order management', exact: true })
      .waitFor()
    await orderRow(root.page, 'm02-north-order').waitFor()
    const northRow = orderRow(root.page, 'm02-north-order')
    await northRow
      .getByRole('cell', { name: 'North Studio', exact: true })
      .waitFor()
    await northRow
      .getByRole('cell', { name: String(north.id), exact: true })
      .waitFor()
    await northRow.getByRole('cell', { name: 'Top-up', exact: true }).waitFor()
    await northRow
      .getByRole('cell', { name: '2026-01-01 00:01:00', exact: true })
      .waitFor()
    await orderRow(root.page, 'm02-legacy-order')
      .getByText('Unknown site', { exact: true })
      .waitFor()
    const site = root.page.getByRole('combobox', { name: 'Site', exact: true })
    await site
      .locator(`option[value="${north.id}"]`)
      .waitFor({ state: 'attached' })
    await site.selectOption('0')
    await orderRow(root.page, 'm02-main-order').waitFor()
    await northRow.waitFor({ state: 'hidden' })
    await site.selectOption(String(north.id))
    await northRow.waitFor()
    await orderRow(root.page, 'm02-main-order').waitFor({ state: 'hidden' })
    await root.page
      .getByRole('combobox', { name: 'Order type', exact: true })
      .selectOption('topup')
    const summary = await success(
      root,
      'GET',
      `/api/user/admin/finance/recharge-audit/summary?site_id=${north.id}&order_type=topup`
    )
    assert.equal(summary.orders, 1)
    assert.equal(summary.credited_usd, '2')
    await site.selectOption('')
    await orderRow(root.page, 'm02-south-order').waitFor()
    await owner.page.goto(`${base}/site-owner/orders`)
    await owner.page
      .getByRole('tab', { name: 'Order management', exact: true })
      .waitFor()
    await orderRow(owner.page, 'm02-north-order').waitFor()
    assert.equal(await orderRow(owner.page, 'm02-south-order').count(), 0)
    assert.equal(await orderRow(owner.page, 'm02-main-order').count(), 0)
    assert.equal(
      await owner.page
        .getByRole('combobox', { name: 'Site', exact: true })
        .count(),
      0
    )
    for (const action of ['Complete', 'Refund', 'Query payment']) {
      assert.equal(
        await owner.page
          .getByRole('button', { name: action, exact: true })
          .count(),
        0
      )
    }
    const scoped = await success(owner, 'GET', '/api/adty/site/orders')
    assert.equal(scoped.total, 1)
    assert.equal(scoped.items[0].site_id, north.id)
    assert.deepEqual(scoped.items[0].actions, {
      complete: false,
      refund: false,
      query: false,
    })
    const forged = await success(
      owner,
      'GET',
      `/api/adty/site/orders?site_id=${south.id}`
    )
    assert.equal(forged.total, 0)
    const forbidden = await api(
      owner,
      'GET',
      '/api/user/admin/finance/recharge-audit'
    )
    assert.equal(forbidden.status, 403)
    assert.equal(
      await owner.page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      ),
      false
    )
    console.log(
      `unified-orders: ${width}px global site/type filters, frozen ownership, legacy unknown and owner isolation passed`
    )
  }
  if (process.env.ADTY_BROWSER_ORDER_TRACE === '1') {
    await verifyOrderTrace({
      root,
      owner,
      buyer,
      north,
      south,
      base,
      success,
      api,
      postgres,
      orderRow,
    })
  }
  assert.deepEqual(errors, [])
  console.log(
    'unified-orders: PostgreSQL order snapshots main=0/north/south/unknown preserved, pageerrors=0'
  )
} finally {
  await browser.close()
}
