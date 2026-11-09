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
import { once } from 'node:events'
import { mkdir } from 'node:fs/promises'
import { createServer } from 'node:net'
import { join } from 'node:path'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
assert.ok(base && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const viewportWidth = Number(process.env.ADTY_BROWSER_STAFF_WIDTH)
assert.ok([1440, 390].includes(viewportWidth))
const staffCase = process.env.ADTY_BROWSER_STAFF_CASE ?? 'lifecycle'
assert.ok(['lifecycle', 'customer-security'].includes(staffCase))
const customerSecurity = staffCase === 'customer-security'
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const errors = []
// Capture only the external mail transport on loopback; recovery, verification
// and customer password selection still run through the production backend.
const mailbox = createServer((socket) => {
  socket.setEncoding('utf8')
  socket.write('220 localhost SMTP capture\r\n')
  let buffer = ''
  let message = ''
  let inData = false
  socket.on('error', () => errors.push('Local SMTP capture failed'))
  socket.on('data', (chunk) => {
    buffer += chunk
    while (buffer.includes('\r\n')) {
      const end = buffer.indexOf('\r\n')
      const line = buffer.slice(0, end)
      buffer = buffer.slice(end + 2)
      if (inData) {
        if (line === '.') {
          inData = false
          socket.write('250 Accepted\r\n')
          mailbox.emit('mail', message)
        } else {
          message += `${line}\r\n`
        }
      } else if (/^(EHLO|HELO) /i.test(line)) {
        socket.write('250 localhost\r\n')
      } else if (line === 'DATA') {
        message = ''
        inData = true
        socket.write('354 Send message\r\n')
      } else if (line === 'QUIT') {
        socket.end('221 Bye\r\n')
      } else {
        socket.write('250 OK\r\n')
      }
    }
  })
})
mailbox.listen(0, '127.0.0.1')
await once(mailbox, 'listening')
const artifacts = process.env.ADTY_BROWSER_ARTIFACT_DIR
if (artifacts) await mkdir(artifacts, { recursive: true })

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

async function session(
  username,
  password,
  heading = 'Overview',
  sessionBase = base
) {
  const context = await browser.newContext({
    locale: 'en-US',
    timezoneId: 'UTC',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  const state = { page, headers: undefined }
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('request', (request) => {
    if (
      new URL(request.url()).pathname.startsWith('/api/') &&
      request.headers().authorization
    ) {
      state.headers = { authorization: request.headers().authorization }
    }
  })
  await page.goto(`${sessionBase}/sign-in`)
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  const [response] = await Promise.all([
    page.waitForResponse(
      (item) =>
        new URL(item.url()).pathname === '/api/user/login' &&
        item.request().method() === 'POST'
    ),
    page.getByRole('button', { name: 'Sign in', exact: true }).click(),
  ])
  assert.equal(response.status(), 200)
  const result = await response.json()
  assert.equal(result.success, true, result.message || 'Login failed')
  await page.getByRole('heading', { name: heading, exact: true }).waitFor()
  assert.ok(state.headers?.authorization)
  return state
}

async function api(state, method, path, data, extra = {}) {
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
    { method, path, data, headers: { ...state.headers, ...extra } }
  )
}

async function success(state, method, path, data) {
  const result = await api(state, method, path, data)
  assert.equal(result.status, 200, path)
  assert.equal(result.body.success, true, result.body.message || path)
  return result.body.data
}

function row(page, name) {
  return page
    .getByRole('row')
    .filter({ has: page.getByText(name, { exact: true }) })
}

async function verifiedSave(page, password, method, path) {
  const verification = page.getByRole('dialog', {
    name: 'Confirm it is you',
    exact: true,
  })
  await verification
    .getByLabel('Password', { exact: true })
    .and(verification.locator('input[type="password"]'))
    .fill(password)
  const [result] = await Promise.all([
    page.waitForResponse(
      (item) =>
        new URL(item.url()).pathname === path &&
        item.request().method() === method
    ),
    verification.getByRole('button', { name: 'Verify', exact: true }).click(),
  ])
  assert.equal(result.status(), 200, path)
  const body = await result.json()
  assert.equal(body.success, true, body.message || path)
  return body.data
}

try {
  const root = await session(
    process.env.ADTY_BROWSER_USERNAME,
    process.env.ADTY_BROWSER_PASSWORD
  )
  for (const [key, value] of Object.entries({
    EmailProvider: 'smtp',
    SMTPServer: '127.0.0.1',
    SMTPPort: String(mailbox.address().port),
    SMTPFrom: 'capture@example.test',
    SMTPSSLEnabled: 'false',
    SMTPStartTLSEnabled: 'false',
  })) {
    await success(root, 'PUT', '/api/option/', { key, value })
  }
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
    value: JSON.stringify({ en: 'Synthetic staff owner agreement.' }),
  })
  await success(root, 'PUT', '/api/option/', {
    key: 'adty_legal_setting.site_agreement_version',
    value: '1',
  })
  await success(root, 'POST', '/api/user/', {
    username: 'staffoutside',
    password: `Outside!${randomBytes(16).toString('hex')}`,
    role: 1,
  })
  const outsideID = Number(
    postgres("SELECT id FROM users WHERE username = 'staffoutside'")
  )
  await success(root, 'POST', '/api/adty/admin/sites', {
    user_id: outsideID,
    subdomain: 'staffoutside',
    root_domain: 'sites.localhost',
  })
  for (const width of [viewportWidth]) {
    const password = `LocalStaff!${randomBytes(12).toString('hex')}`
    const ownerName = `staffowner${width}`
    const buyerName = `staffbuyer${width}`
    const staffBase = `http://${ownerName}.sites.localhost:${new URL(base).port}`
    for (const username of [ownerName, buyerName]) {
      await success(root, 'POST', '/api/user/', { username, password, role: 1 })
    }
    const ownerID = Number(
      postgres(`SELECT id FROM users WHERE username = '${ownerName}'`)
    )
    const buyerID = Number(
      postgres(`SELECT id FROM users WHERE username = '${buyerName}'`)
    )
    const site = await success(root, 'POST', '/api/adty/admin/sites', {
      user_id: ownerID,
      subdomain: ownerName,
      root_domain: 'sites.localhost',
    })
    postgres(
      `UPDATE users SET site_id = ${site.id}, email = 'private${width}@example.com', quota = 1000000 WHERE id = ${buyerID}; INSERT INTO adty_site_customers (user_id, site_id, joined_at) VALUES (${buyerID}, ${site.id}, 1000); UPDATE adty_sites SET visibility = '{"customer_email":"full","customer_balance":"on","customer_usage":"on","customer_topups":"on","request_logs":"on"}' WHERE id = ${site.id}`
    )
    postgres(
      `INSERT INTO logs (user_id, created_at, type, model_name, quota, token_name, content, other, ip) VALUES (${buyerID}, 1001, 2, 'staff-safe-model', 250000, 'staff-secret-key', 'staff-secret-body', '{"key":"sk-secret"}', '192.0.2.23'); INSERT INTO logs (user_id, created_at, type, model_name, quota) VALUES (${buyerID}, 999, 2, 'before-join-model', 100)`
    )
    postgres(
      `INSERT INTO top_ups (user_id, amount, money, trade_no, payment_provider, payment_method, create_time, complete_time, status) VALUES (${buyerID}, 2, 2, 'staff-order-${width}', 'stripe', 'stripe', 1001, 1001, 'success'); INSERT INTO adty_topup_orders (trade_no, user_id, site_id, provider, method, face_usd, bonus_usd, credit_quota, pay_amount, pay_currency, paid_amount, paid_currency, provider_environment, created_at, paid_tx_hash) VALUES ('staff-order-${width}', ${buyerID}, ${site.id}, 'stripe', 'stripe', 2, 0, 1000000, 2, 'USD', 2, 'USD', 'test', 1001, 'staff-secret-payment')`
    )
    const owner = await session(ownerName, password)
    const page = owner.page
    await page
      .getByRole('link', { name: 'Site owner center', exact: true })
      .click()
    await page
      .getByRole('button', { name: 'Accept site agreement', exact: true })
      .click()
    await page.getByRole('tab', { name: 'Staff accounts', exact: true }).click()
    await page.setViewportSize({ width, height: 1000 })
    assert.equal(
      await page.getByRole('tablist').evaluate((list) => {
        const bounds = list.getBoundingClientRect()
        return [...list.querySelectorAll('[role="tab"]')].every(
          (tab) => tab.getBoundingClientRect().bottom <= bounds.bottom
        )
      }),
      true,
      'Wrapped owner tabs must remain inside the tab list'
    )
    await page
      .getByRole('button', { name: 'Create staff account', exact: true })
      .click()
    const create = page.getByRole('dialog', {
      name: 'Create staff account',
      exact: true,
    })
    const staffName = `staff${width}`
    await create.getByLabel('Username', { exact: true }).fill(staffName)
    await create
      .getByLabel('Display name', { exact: true })
      .fill(`Support ${width}`)
    await create.getByLabel('Password', { exact: true }).fill(password)
    await create.getByRole('button', { name: 'Save', exact: true }).click()
    let member = await verifiedSave(
      page,
      password,
      'POST',
      '/api/adty/site/staff'
    )
    await row(page, staffName).waitFor()
    assert.deepEqual(member.permissions, ['customers.read'])
    assert.equal(
      postgres(`SELECT quota FROM users WHERE id = ${member.user_id}`),
      '0'
    )
    const mainLogin = await api(root, 'POST', '/api/user/login', {
      username: staffName,
      password,
    })
    assert.equal(mainLogin.body.success, false)
    let employee = await session(
      staffName,
      password,
      'Staff workspace',
      staffBase
    )
    await employee.page.setViewportSize({ width, height: 1000 })
    await employee.page.getByText(buyerName, { exact: true }).waitFor()
    assert.equal(
      await employee.page
        .getByRole('columnheader', { name: 'Balance', exact: true })
        .count(),
      0
    )
    assert.equal(
      await employee.page
        .getByText(`private${width}@example.com`, { exact: true })
        .count(),
      0
    )
    assert.equal(
      await employee.page
        .getByRole('link', { name: 'API Keys', exact: true })
        .count(),
      0
    )
    for (const label of [
      'Send password reset instructions',
      'Disable customer',
    ]) {
      assert.equal(
        await employee.page
          .getByRole('button', { name: label, exact: true })
          .count(),
        0
      )
    }
    await employee.page.goto(`${staffBase}/wallet`)
    await employee.page
      .getByRole('heading', { name: 'Staff workspace', exact: true })
      .waitFor()
    for (const path of [
      '/api/adty/site/staff',
      '/api/adty/site/earnings',
      '/api/token/',
      '/api/user/topup/info',
    ]) {
      assert.equal((await api(employee, 'GET', path)).status, 403, path)
    }
    const foreign = await api(employee, 'GET', '/api/adty/staff/customers/1')
    assert.equal(foreign.status, 404)
    const otherHost = await employee.page.context().newPage()
    otherHost.on('pageerror', (error) => errors.push(error.message))
    await otherHost.goto(
      `http://staffoutside.sites.localhost:${new URL(base).port}/api/status`,
      { waitUntil: 'domcontentloaded' }
    )
    assert.equal(
      (
        await api(
          { page: otherHost, headers: employee.headers },
          'GET',
          '/api/adty/staff/self'
        )
      ).status,
      403
    )
    await otherHost.close()
    const oldSupport = { ...employee, headers: { ...employee.headers } }
    await row(page, staffName)
      .getByRole('button', { name: 'Edit', exact: true })
      .click()
    const edit = page.getByRole('dialog', {
      name: 'Edit staff account',
      exact: true,
    })
    await edit.getByLabel('Staff role', { exact: true }).selectOption('finance')
    await edit
      .getByRole('checkbox', {
        name: 'Send password reset instructions',
        exact: true,
      })
      .check()
    await edit
      .getByRole('checkbox', { name: 'Disable customer', exact: true })
      .check()
    await edit.getByRole('button', { name: 'Save', exact: true }).click()
    member = await verifiedSave(
      page,
      password,
      'PUT',
      `/api/adty/site/staff/${member.id}`
    )
    assert.equal(
      (await api(oldSupport, 'GET', '/api/adty/staff/self')).status,
      401
    )
    await employee.page.context().close()
    employee = await session(staffName, password, 'Staff workspace', staffBase)
    await employee.page.setViewportSize({ width, height: 1000 })
    await employee.page
      .getByRole('columnheader', { name: 'Balance', exact: true })
      .waitFor()
    await employee.page
      .getByRole('button', { name: 'Usage records', exact: true })
      .click()
    const usage = employee.page.getByRole('dialog', {
      name: 'Usage records',
      exact: true,
    })
    await usage.getByText('staff-safe-model', { exact: true }).waitFor()
    assert.equal(
      await usage.getByText('before-join-model', { exact: true }).count(),
      0
    )
    if (artifacts) {
      await employee.page.screenshot({
        path: join(artifacts, `staff-usage-${width}.png`),
        fullPage: true,
        animations: 'disabled',
      })
    }
    await usage.getByRole('button', { name: 'Close', exact: true }).click()
    await employee.page
      .getByRole('button', { name: 'Recharge records', exact: true })
      .click()
    await employee.page
      .getByRole('dialog')
      .getByText(`staff-order-${width}`, { exact: true })
      .waitFor()
    for (const kind of ['logs', 'topups']) {
      const data = await success(
        employee,
        'GET',
        `/api/adty/staff/customers/${buyerID}/${kind}`
      )
      assert.equal(data.total, 1)
      const text = JSON.stringify(data)
      for (const secret of [
        'staff-secret',
        'sk-secret',
        '192.0.2.23',
        'before-join',
      ]) {
        assert.ok(!text.includes(secret))
      }
    }
    await employee.page
      .getByRole('dialog', { name: 'Recharge records', exact: true })
      .getByRole('button', { name: 'Close', exact: true })
      .click()
    const customerSecrets = []
    if (customerSecurity) {
      let customer = await session(buyerName, password, 'Overview', staffBase)
      await success(customer, 'POST', '/api/token/', {
        name: 'staff-customer-key',
        expired_time: -1,
        unlimited_quota: true,
      })
      const customerKey = postgres(
        `SELECT key FROM tokens WHERE user_id=${buyerID} AND name='staff-customer-key'`
      )
      assert.ok(customerKey)
      assert.equal(
        (
          await api(customer, 'GET', '/v1/models', undefined, {
            authorization: `Bearer sk-${customerKey}`,
          })
        ).status,
        200
      )
      const originalHash = postgres(
        `SELECT password FROM users WHERE id=${buyerID}`
      )
      await employee.page
        .getByRole('button', {
          name: 'Send password reset instructions',
          exact: true,
        })
        .click()
      const resetConfirm = employee.page.getByRole('alertdialog', {
        name: 'Send password reset instructions',
        exact: true,
      })
      assert.equal(
        await resetConfirm.getByText(buyerName, { exact: true }).count(),
        1
      )
      if (artifacts) {
        await employee.page.screenshot({
          path: join(artifacts, `staff-customer-reset-${width}.png`),
          fullPage: true,
          animations: 'disabled',
        })
      }
      await resetConfirm
        .getByRole('button', { name: 'Continue', exact: true })
        .click()
      const received = once(mailbox, 'mail', {
        signal: AbortSignal.timeout(15000),
      })
      const [receivedMail] = await Promise.all([
        received,
        verifiedSave(
          employee.page,
          password,
          'POST',
          `/api/adty/staff/customers/${buyerID}/password-reset`
        ),
      ])
      const [mail] = receivedMail
      assert.ok(mail.includes(`To: private${width}@example.com\r\n`))
      const resetLink = /href='([^']+\/user\/reset#[^']+)'/.exec(mail)?.[1]
      assert.ok(resetLink)
      assert.equal(new URL(resetLink).hostname, `${ownerName}.sites.localhost`)
      assert.equal(
        postgres(`SELECT password FROM users WHERE id=${buyerID}`),
        originalHash
      )
      assert.equal((await api(customer, 'GET', '/api/user/self')).status, 200)
      const newCustomerPassword = `CustomerChosen!${randomBytes(12).toString('hex')}`
      const resetPage = await browser.newPage({
        viewport: { width, height: 1000 },
        locale: 'en-US',
      })
      resetPage.on('pageerror', (error) => errors.push(error.message))
      // Public site URLs use the standard port; this isolated runtime uses an
      // ephemeral loopback port. Keep the real hostname, path and credential.
      const localResetURL = new URL(resetLink)
      localResetURL.port = new URL(base).port
      await resetPage.goto(localResetURL.href)
      await resetPage
        .getByLabel('New password', { exact: true })
        .fill(newCustomerPassword)
      const [changed] = await Promise.all([
        resetPage.waitForResponse(
          (item) =>
            new URL(item.url()).pathname === '/api/user/reset' &&
            item.request().method() === 'POST'
        ),
        resetPage
          .getByRole('button', { name: 'Confirm reset password', exact: true })
          .click(),
      ])
      assert.equal(changed.status(), 200)
      assert.equal((await changed.json()).success, true)
      assert.equal((await api(customer, 'GET', '/api/user/self')).status, 401)
      await customer.page.context().close()
      await resetPage.context().close()
      customer = await session(
        buyerName,
        newCustomerPassword,
        'Overview',
        staffBase
      )
      await employee.page
        .getByRole('button', { name: 'Disable customer', exact: true })
        .click()
      const disableConfirm = employee.page.getByRole('alertdialog', {
        name: 'Disable customer',
        exact: true,
      })
      await disableConfirm
        .getByRole('button', { name: 'Cancel', exact: true })
        .click()
      assert.equal(
        postgres(`SELECT status FROM users WHERE id=${buyerID}`),
        '1'
      )
      await employee.page
        .getByRole('button', { name: 'Disable customer', exact: true })
        .click()
      await disableConfirm
        .getByRole('button', { name: 'Continue', exact: true })
        .click()
      await verifiedSave(
        employee.page,
        password,
        'POST',
        `/api/adty/staff/customers/${buyerID}/disable`
      )
      await employee.page
        .getByRole('button', { name: 'Disable customer', exact: true })
        .waitFor({ state: 'hidden' })
      assert.equal((await api(customer, 'GET', '/api/user/self')).status, 401)
      assert.equal(
        (
          await api(customer, 'GET', '/v1/models', undefined, {
            authorization: `Bearer sk-${customerKey}`,
          })
        ).status,
        403
      )
      await customer.page.context().close()
      assert.equal(
        postgres(
          `SELECT status || ':' || quota FROM users WHERE id=${buyerID}`
        ),
        '2:1000000'
      )
      if (artifacts) {
        await employee.page.screenshot({
          path: join(artifacts, `staff-customer-disabled-${width}.png`),
          fullPage: true,
          animations: 'disabled',
        })
      }
      customerSecrets.push(
        customerKey,
        resetLink,
        newCustomerPassword,
        originalHash,
        `private${width}@example.com`
      )
    }
    let newPassword = ''
    if (!customerSecurity) {
      const finance = { ...employee, headers: { ...employee.headers } }
      for (const action of ['Disable', 'Enable']) {
        await row(page, staffName)
          .getByRole('button', { name: action, exact: true })
          .click()
        await page
          .getByRole('alertdialog')
          .getByRole('button', { name: action, exact: true })
          .click()
        member = await verifiedSave(
          page,
          password,
          'PUT',
          `/api/adty/site/staff/${member.id}`
        )
        assert.equal(
          (await api(finance, 'GET', '/api/adty/staff/self')).status,
          401
        )
      }
      await employee.page.context().close()
      employee = await session(
        staffName,
        password,
        'Staff workspace',
        staffBase
      )
      newPassword = `ChangedStaff!${randomBytes(12).toString('hex')}`
      await row(page, staffName)
        .getByRole('button', { name: 'Reset staff password', exact: true })
        .click()
      const reset = page.getByRole('dialog', {
        name: 'Reset staff password',
        exact: true,
      })
      await reset.getByLabel('Password', { exact: true }).fill(newPassword)
      await reset.getByRole('button', { name: 'Save', exact: true }).click()
      member = await verifiedSave(
        page,
        password,
        'PUT',
        `/api/adty/site/staff/${member.id}/password`
      )
      assert.equal(
        (await api(employee, 'GET', '/api/adty/staff/self')).status,
        401
      )
      await employee.page.context().close()
      employee = await session(
        staffName,
        newPassword,
        'Staff workspace',
        staffBase
      )
      await row(page, staffName)
        .getByRole('button', { name: 'Delete', exact: true })
        .click()
      await page
        .getByRole('alertdialog')
        .getByRole('button', { name: 'Delete', exact: true })
        .click()
      await verifiedSave(
        page,
        password,
        'DELETE',
        `/api/adty/site/staff/${member.id}`
      )
      assert.equal(
        (await api(employee, 'GET', '/api/adty/staff/self')).status,
        401
      )
      await employee.page.context().close()
      await row(page, staffName).waitFor({ state: 'hidden' })
    }
    const audits = await success(
      owner,
      'GET',
      '/api/adty/site/staff/audits?page_size=100'
    )
    assert.ok(audits.items.some((event) => event.action === 'staff.usage.read'))
    assert.equal(
      audits.items.filter(
        (event) => event.action === 'staff.customer.password.request'
      ).length,
      customerSecurity ? 1 : 0
    )
    assert.equal(
      audits.items.filter((event) => event.action === 'staff.customer.disable')
        .length,
      customerSecurity ? 1 : 0
    )
    for (const secret of customerSecrets) {
      assert.ok(!JSON.stringify(audits).includes(secret))
    }
    assert.ok(!JSON.stringify(audits).includes(password))
    if (newPassword) assert.ok(!JSON.stringify(audits).includes(newPassword))
    const evidence = JSON.parse(
      postgres(
        `SELECT json_build_object('status', s.status, 'version', s.version, 'auth_version', u.auth_version, 'quota', u.quota, 'staff_site_id', u.staff_site_id) FROM adty_site_staff s JOIN users u ON u.id=s.user_id WHERE s.id=${member.id}`
      )
    )
    assert.deepEqual(evidence, {
      status: customerSecurity ? 'enabled' : 'deleted',
      version: customerSecurity ? 2 : 6,
      auth_version: customerSecurity ? 2 : 6,
      quota: 0,
      staff_site_id: site.id,
    })
    if (artifacts) {
      await page.screenshot({
        path: join(artifacts, `staff-audits-${width}.png`),
        fullPage: true,
        animations: 'disabled',
      })
    }
    console.log(
      `site-staff: ${width}px ${staffCase} creation, login, grants, records, privacy, denylist and SQL passed`
    )
  }
  assert.deepEqual(errors, [])
  console.log('site-staff: pageerrors=0')
} finally {
  await browser.close()
  await new Promise((resolve) => mailbox.close(resolve))
}
