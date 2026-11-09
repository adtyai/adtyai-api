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
    throw new Error('Isolated PostgreSQL fixture or evidence query failed')
  }
}

try {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  let authorization
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (request) => {
    if (request.headers().authorization) {
      authorization = request.headers().authorization
    }
  })
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(authorization)
  async function api(method, path, data) {
    const response = await page.request[method](base + path, {
      headers: { authorization },
      ...(data === undefined ? {} : { data }),
    })
    assert.equal(response.status(), 200, `${method} ${path}`)
    const body = await response.json()
    assert.equal(body.success, true, body.message || path)
    return body.data
  }
  await api('put', '/api/adty/admin/reseller/settings', {
    enabled: true,
    root_domains: ['sites.example.test'],
    cost_factor: 0.8,
    min_cost_factor: 0.5,
    max_markup: 3,
    max_site_bonus_extra: 0,
    trial_grant_max_usd: 0,
    trial_grant_daily_usd: 0,
  })
  for (const name of ['m01owner', 'm01main', 'm01north']) {
    await api('post', '/api/user/', {
      username: name,
      password: `LocalGate!${randomBytes(12).toString('hex')}`,
      role: 1,
    })
  }
  const ownerID = Number(
    postgres("SELECT id FROM users WHERE username = 'm01owner'")
  )
  assert.ok(Number.isSafeInteger(ownerID) && ownerID > 0)
  const site = await api('post', '/api/adty/admin/sites', {
    user_id: ownerID,
    subdomain: 'm01north',
    root_domain: 'sites.example.test',
  })
  assert.ok(Number.isSafeInteger(site.id) && site.id > 0)
  // Fixture membership and branding are isolated from the user-query behavior under test.
  postgres(
    `UPDATE users SET site_id = ${site.id} WHERE username = 'm01north'; UPDATE adty_sites SET brand = '{"name":"North Studio"}' WHERE id = ${site.id}`
  )
  const choices = await api('get', '/api/user/sites')
  assert.ok(choices.some((choice) => choice.site_id === 0 && choice.site_name))
  assert.ok(
    choices.some(
      (choice) =>
        choice.site_id === site.id && choice.site_name === 'North Studio'
    )
  )
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.goto(`${base}/users/`)
    await page.getByText('m01north', { exact: true }).waitFor()
    await page.getByText('North Studio', { exact: true }).waitFor()
    await page.getByText(`Site ID: ${site.id}`, { exact: true }).waitFor()
    const filter = page
      .getByRole('button', { name: /^Site/ })
      .and(page.locator('[aria-haspopup="dialog"]'))
    await filter.click()
    const filtered = page.waitForResponse((response) => {
      const url = new URL(response.url())
      return (
        url.pathname === '/api/user/' &&
        url.searchParams.get('site_id') === String(site.id)
      )
    })
    await page
      .getByRole('option', { name: `North Studio · ${site.id}`, exact: true })
      .click()
    const body = await (await filtered).json()
    assert.equal(body.data.total, 1)
    assert.equal(body.data.items[0].username, 'm01north')
    await page
      .getByText('m01main', { exact: true })
      .waitFor({ state: 'detached' })
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: 'Edit', exact: true }).click()
    const detail = page.getByRole('dialog', {
      name: 'Update User',
      exact: true,
    })
    await detail.getByLabel('Username', { exact: true }).waitFor()
    assert.equal(
      await detail.getByLabel('Site', { exact: true }).textContent(),
      'North Studio'
    )
    assert.equal(
      await detail.getByLabel('Site ID', { exact: true }).textContent(),
      String(site.id)
    )
    await page.keyboard.press('Escape')
    await detail.waitFor({ state: 'hidden' })
    await page.reload()
    await page.getByText('m01north', { exact: true }).waitFor()
    assert.equal(await page.getByText('m01main', { exact: true }).count(), 0)
    await filter.click()
    const mainChoice = page.getByRole('option', { name: /· 0$/, exact: false })
    await mainChoice.click()
    await page.getByText('m01main', { exact: true }).waitFor()
    await page
      .getByText('m01north', { exact: true })
      .waitFor({ state: 'detached' })
    await page
      .getByRole('option', { name: 'Clear filters', exact: true })
      .click()
    await page.getByText('m01north', { exact: true }).waitFor()
    await page.keyboard.press('Escape')
    console.log(
      `PASS ${width}px site names/IDs, scoped query, detail, reload, main-site zero and clear filter`
    )
  }
  assert.deepEqual(errors, [])
  console.log('PASS user-site production browser checks; pageerrors=0')
} finally {
  await browser.close()
}
