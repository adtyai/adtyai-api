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

function postgresEvidence(query) {
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

async function signIn(name, secret) {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  const session = { page, headers: undefined }
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (request) => {
    const authorization = request.headers().authorization
    if (authorization) session.headers = { authorization }
  })
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(name)
  await page.getByLabel('Password', { exact: true }).fill(secret)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(session.headers?.authorization)
  return session
}

try {
  const root = await signIn(username, password)
  async function api(method, path, data, extraHeaders = {}) {
    const response = await root.page.request[method](base + path, {
      headers: { ...root.headers, ...extraHeaders },
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
  await api('post', '/api/user/', {
    username: 'closeowner',
    password: `LocalGate!${randomBytes(12).toString('hex')}`,
    role: 1,
  })
  const ownerID = Number(
    postgresEvidence("SELECT id FROM users WHERE username = 'closeowner'")
  )
  assert.ok(Number.isSafeInteger(ownerID) && ownerID > 0)
  const site = await api('post', '/api/adty/admin/sites', {
    user_id: ownerID,
    subdomain: 'close-source',
    root_domain: 'sites.example.test',
  })
  assert.ok(Number.isSafeInteger(site.id) && site.id > 0)
  const path = `/api/adty/admin/sites/${site.id}`
  const host = 'close-source.sites.example.test'
  const adminPassword = `LocalGate!${randomBytes(12).toString('hex')}`
  const proof = await api('post', '/api/verify', {
    method: 'password',
    scope: 'admin.user.create',
    context: { role: 10 },
    password,
  })
  await api(
    'post',
    '/api/user/',
    {
      username: 'closeadmin',
      password: adminPassword,
      role: 10,
      admin_permissions: { adty_site: { read: true, adjust: true } },
    },
    { 'X-Security-Proof': proof.proof_token }
  )
  const admin = await signIn('closeadmin', adminPassword)
  for (const width of [1440, 390]) {
    await admin.page.setViewportSize({ width, height: 1000 })
    await admin.page.goto(`${base}/site-admin/sites`)
    const row = admin.page
      .getByRole('table', { name: 'Reseller sites' })
      .getByRole('row')
      .filter({ hasText: host })
    await row.getByRole('button', { name: 'Details', exact: true }).click()
    const detail = admin.page.getByRole('dialog', { name: host, exact: true })
    await detail.getByRole('button', { name: 'Pause', exact: true }).waitFor()
    assert.equal(
      await detail
        .getByRole('button', { name: 'Close site', exact: true })
        .count(),
      0
    )
    await detail.getByRole('button', { name: 'Pause', exact: true }).click()
    await detail.getByRole('button', { name: 'Resume', exact: true }).waitFor()
    assert.equal((await api('get', path)).site.status, 'paused')
    await detail.getByRole('button', { name: 'Resume', exact: true }).click()
    await detail.getByRole('button', { name: 'Pause', exact: true }).waitFor()
    assert.equal((await api('get', path)).site.status, 'active')
    await detail.getByRole('button', { name: 'Close', exact: true }).click()
  }
  for (const immediate of [false, true]) {
    const rejected = await admin.page.request.put(`${base}${path}/status`, {
      headers: admin.headers,
      data: { status: 'closed', transfer_to: 0, immediate, role: 100 },
    })
    assert.equal(rejected.status(), 403)
    assert.equal((await rejected.json()).success, false)
  }
  assert.equal((await api('get', path)).site.status, 'active')
  assert.equal(
    postgresEvidence(
      `SELECT count(*) FROM adty_site_audits WHERE site_id = ${site.id} AND action IN ('close_schedule', 'close')`
    ),
    '0'
  )
  console.log(
    'PASS delegated admin desktop/mobile pause/resume, hidden closure and direct immediate/scheduled close 403 without side effects'
  )
  await root.page.goto(`${base}/site-admin/sites`)
  await root.page
    .getByRole('table', { name: 'Reseller sites' })
    .getByRole('row')
    .filter({ hasText: host })
    .getByRole('button', { name: 'Details', exact: true })
    .click()
  const detail = root.page.getByRole('dialog', { name: host, exact: true })
  await detail.getByRole('button', { name: 'Close site', exact: true }).click()
  const dialog = root.page.getByRole('dialog', {
    name: `Close ${host}`,
    exact: true,
  })
  assert.equal(
    await dialog
      .getByRole('button', { name: 'Close site', exact: true })
      .isDisabled(),
    true
  )
  await dialog
    .getByRole('radio', { name: 'Move to the main site', exact: true })
    .click()
  await dialog
    .getByRole('checkbox', {
      name: 'Close now without the notice period',
      exact: true,
    })
    .click()
  const [response] = await Promise.all([
    root.page.waitForResponse(
      (result) =>
        new URL(result.url()).pathname === `${path}/status` &&
        result.request().method() === 'PUT'
    ),
    dialog.getByRole('button', { name: 'Close site', exact: true }).click(),
  ])
  assert.equal(response.status(), 200)
  assert.equal((await response.json()).success, true)
  await dialog.waitFor({ state: 'hidden' })
  assert.equal((await api('get', path)).site.status, 'closed')
  assert.equal(
    postgresEvidence(`SELECT status FROM adty_sites WHERE id = ${site.id}`),
    'closed'
  )
  assert.equal(
    postgresEvidence(
      `SELECT count(*) FROM adty_site_audits WHERE site_id = ${site.id} AND action = 'close_schedule' AND operator_id = (SELECT id FROM users WHERE role = 100)`
    ),
    '1'
  )
  assert.deepEqual(errors, [])
  console.log(
    'PASS real Root UI closure, destination requirement, PostgreSQL state/audit and zero pageerrors; no third-party mocks'
  )
} finally {
  await browser.close()
}
