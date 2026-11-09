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
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
assert.ok(base && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(base).hostname))
const width = Number(process.env.ADTY_BROWSER_OWNERSHIP_WIDTH)
assert.ok([1440, 390].includes(width))
const artifacts = process.env.ADTY_BROWSER_ARTIFACT_DIR
if (artifacts) await mkdir(artifacts, { recursive: true })
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
async function session(username, password) {
  const context = await browser.newContext({
    locale: 'en-US',
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
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(state.headers?.authorization)
  return state
}
async function api(state, method, path, data) {
  const response = await state.page.request[method](base + path, {
    headers: state.headers,
    ...(data === undefined ? {} : { data }),
  })
  return { status: response.status(), body: await response.json() }
}
async function success(state, method, path, data) {
  const result = await api(state, method, path, data)
  assert.equal(result.status, 200, path)
  assert.equal(result.body.success, true, result.body.message || path)
  return result.body.data
}
async function acceptAgreement(state) {
  await state.page
    .getByRole('link', { name: 'Site owner center', exact: true })
    .click()
  await state.page
    .getByRole('button', { name: 'Accept site agreement', exact: true })
    .click()
  await state.page
    .getByRole('tab', { name: 'Staff accounts', exact: true })
    .waitFor()
}

try {
  const root = await session(
    process.env.ADTY_BROWSER_USERNAME,
    process.env.ADTY_BROWSER_PASSWORD
  )
  await success(root, 'put', '/api/adty/admin/reseller/settings', {
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
      JSON.stringify({ en: 'Synthetic ownership agreement.' }),
    ],
    ['adty_legal_setting.site_agreement_version', '1'],
  ]) {
    await success(root, 'put', '/api/option/', { key, value })
  }
  {
    const password = `LocalHandover!${randomBytes(12).toString('hex')}`
    const oldName = `oldowner${width}`,
      nextName = `nextowner${width}`,
      buyerName = `buyer${width}`
    for (const username of [oldName, nextName, buyerName]) {
      await success(root, 'post', '/api/user/', { username, password, role: 1 })
    }
    const oldID = Number(
      postgres(`SELECT id FROM users WHERE username = '${oldName}'`)
    )
    const nextID = Number(
      postgres(`SELECT id FROM users WHERE username = '${nextName}'`)
    )
    const buyerID = Number(
      postgres(`SELECT id FROM users WHERE username = '${buyerName}'`)
    )
    const site = await success(root, 'post', '/api/adty/admin/sites', {
      user_id: oldID,
      subdomain: oldName,
      root_domain: 'sites.localhost',
    })
    const host = `${oldName}.sites.localhost`,
      path = `/api/adty/admin/sites/${site.id}`
    const old = await session(oldName, password)
    await acceptAgreement(old)
    const nextBefore = await session(nextName, password)
    // Representative historical pending records; business writes below use the
    // production APIs and transactions, with no external payment or mail.
    postgres(
      `UPDATE users SET site_id = ${site.id}, quota = 0 WHERE id = ${buyerID}; INSERT INTO adty_site_customers (site_id, user_id, reviewed) VALUES (${site.id}, ${buyerID}, true); INSERT INTO adty_site_earning_daily (site_id, site_owner_user_id, user_id, day, kind, earning, status) VALUES (${site.id}, 0, ${buyerID}, '2026-01-01', 'usage', 10, 'pending')`
    )
    const trade = `handover-order-${width}`
    postgres(
      `INSERT INTO top_ups (user_id, amount, money, trade_no, payment_provider, payment_method, create_time, status) VALUES (${buyerID}, 100, 100, '${trade}', 'bepusdt', 'usdt.trc20', 1000, 'pending'); INSERT INTO adty_topup_orders (trade_no, user_id, site_id, site_owner_user_id, provider, method, face_usd, bonus_usd, credit_quota, pay_amount, pay_currency, expire_at, created_at) VALUES ('${trade}', ${buyerID}, ${site.id}, ${oldID}, 'bepusdt', 'usdt.trc20', 100, 2, 51000000, 100, 'USDT', 4102444800, 1000)`
    )
    const page = root.page
    await page.setViewportSize({ width, height: 1000 })
    await page.goto(`${base}/site-admin/sites`)
    await page
      .getByRole('table', { name: 'Reseller sites' })
      .getByRole('row')
      .filter({ hasText: host })
      .getByRole('button', { name: 'Details', exact: true })
      .click()
    await page
      .getByRole('dialog', { name: host, exact: true })
      .getByRole('button', { name: 'Change site owner', exact: true })
      .click()
    const dialog = page.getByRole('dialog', {
      name: 'Change site owner',
      exact: true,
    })
    assert.equal(
      await dialog
        .getByRole('button', { name: 'Review handover', exact: true })
        .isDisabled(),
      true
    )
    await dialog.getByLabel('Find a main-site user').fill(nextName)
    await dialog
      .getByRole('button', { name: 'Search users', exact: true })
      .click()
    await dialog.getByRole('combobox', { name: 'New site owner' }).click()
    await page
      .getByRole('option', { name: `${nextName} (#${nextID})`, exact: true })
      .click()
    await dialog
      .getByLabel('Handover reason')
      .fill('Synthetic browser handover')
    await dialog
      .getByRole('button', { name: 'Review handover', exact: true })
      .click()
    const confirm = page.getByRole('alertdialog', {
      name: 'Confirm owner handover',
      exact: true,
    })
    await confirm.getByRole('button', { name: 'Cancel', exact: true }).click()
    assert.equal(
      postgres(`SELECT owner_user_id FROM adty_sites WHERE id = ${site.id}`),
      String(oldID)
    )
    await dialog
      .getByRole('button', { name: 'Review handover', exact: true })
      .click()
    if (artifacts) {
      await page.screenshot({
        path: join(artifacts, `confirm-${width}.png`),
        fullPage: true,
        animations: 'disabled',
      })
    }
    const saved = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === `${path}/owner` &&
        response.request().method() === 'POST'
    )
    await confirm
      .getByRole('button', { name: 'Change site owner', exact: true })
      .click()
    assert.equal((await (await saved).json()).success, true)
    await dialog.waitFor({ state: 'hidden' })
    assert.equal(
      postgres(
        `SELECT owner_user_id || ':' || ownership_version || ':' || agreement_ver FROM adty_sites WHERE id = ${site.id}`
      ),
      `${nextID}:1:`
    )
    assert.equal(
      postgres(
        `SELECT count(*) FROM adty_site_ownerships WHERE site_id = ${site.id} AND old_owner_user_id = ${oldID} AND new_owner_user_id = ${nextID}`
      ),
      '1'
    )
    assert.equal(
      postgres(
        `SELECT count(*) FROM adty_site_audits WHERE site_id = ${site.id} AND action = 'owner_handover'`
      ),
      '1'
    )
    for (const previous of [old, nextBefore]) {
      assert.equal((await api(previous, 'get', '/api/user/self')).status, 401)
      await previous.page.context().close()
    }
    const stale = await api(root, 'post', `${path}/owner`, {
      expected_owner_user_id: oldID,
      expected_ownership_version: 0,
      new_owner_user_id: nextID,
      reason: 'Stale retry',
    })
    assert.equal(stale.status, 200)
    assert.equal(stale.body.success, false)
    assert.equal(
      stale.body.message,
      'Site ownership changed; reload the site before trying again'
    )
    const oldAfter = await session(oldName, password)
    assert.equal(
      (await api(oldAfter, 'get', '/api/adty/site/self')).status,
      403
    )
    await oldAfter.page.context().close()
    const next = await session(nextName, password)
    assert.equal(
      (await api(next, 'get', '/api/adty/site/earnings')).status,
      403
    )
    await acceptAgreement(next)
    assert.equal(
      Number(
        (await success(next, 'get', '/api/adty/site/earnings')).balances
          .pending_usd
      ),
      0
    )
    await success(root, 'post', `/api/adty/admin/topup/${trade}/complete`, {
      proof: `isolated-proof-${width}`,
      note: 'Synthetic late completion after handover',
    })
    assert.equal(
      postgres(
        `SELECT site_owner_user_id FROM adty_topup_orders WHERE trade_no = '${trade}'`
      ),
      String(oldID)
    )
    assert.equal(
      Number(
        (await success(next, 'get', '/api/adty/site/earnings')).balances
          .pending_usd
      ),
      0
    )
    const refund = {
      reason: 'Synthetic old owner refund',
      idempotency_key: `handover-refund-${width}`,
      refund_quota: 50000000,
    }
    await success(root, 'post', `/api/adty/admin/topup/${trade}/refund`, refund)
    await success(root, 'post', `/api/adty/admin/topup/${trade}/refund`, refund)
    assert.equal(
      postgres(
        `SELECT count(*) FROM adty_balance_ledgers WHERE user_id = ${buyerID} AND type = 'refund'`
      ),
      '1'
    )
    postgres(
      `INSERT INTO adty_site_earning_daily (site_id, site_owner_user_id, user_id, day, kind, earning, status) VALUES (${site.id}, ${nextID}, ${buyerID}, '2026-01-01', 'usage', 5, 'pending'); UPDATE adty_sites SET settle_hold_reason = 'isolated settlement fixture' WHERE id = ${site.id}`
    )
    await success(root, 'post', `${path}/settlement/resume`)
    // The old order's -$2 bonus cost settles immediately, while its +$2
    // refund reversal keeps the existing freeze period. Both belong to oldID.
    for (const [owner, expected] of [
      [oldID, 8],
      [nextID, 5],
    ]) {
      assert.equal(
        Number(
          postgres(
            `SELECT available_amount FROM referral_commission_accounts WHERE user_id = ${owner}`
          )
        ),
        expected
      )
    }
    assert.equal(
      Number(
        postgres(
          `SELECT sum(earning) FROM adty_site_earning_daily WHERE site_id = ${site.id} AND site_owner_user_id = ${oldID} AND kind = 'bonus_reversal' AND status = 'pending'`
        )
      ),
      2
    )
    assert.equal(
      Number(
        (await success(next, 'get', '/api/adty/site/earnings')).balances
          .available_usd
      ),
      5
    )
    await page.reload()
    await page
      .getByRole('table', { name: 'Reseller sites' })
      .getByRole('row')
      .filter({ hasText: host })
      .getByRole('button', { name: 'Details', exact: true })
      .click()
    const detail = page.getByRole('dialog', { name: host, exact: true })
    await detail.getByText(nextName, { exact: true }).waitFor()
    if (artifacts) {
      await page.screenshot({
        path: join(artifacts, `completed-${width}.png`),
        fullPage: true,
        animations: 'disabled',
      })
    }
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      ),
      false
    )
    await success(root, 'put', `${path}/status`, {
      status: 'closed',
      immediate: true,
      transfer_to: 0,
      claw_back: true,
      reason: 'Synthetic per-owner clawback',
    })
    for (const owner of [oldID, nextID]) {
      assert.equal(
        Number(
          postgres(
            `SELECT available_amount FROM referral_commission_accounts WHERE user_id = ${owner}`
          )
        ),
        0
      )
    }
    console.log(
      `PASS ${width}: Root UI cancel/confirm, version conflict, old sessions 401, former owner 403, agreement gate, frozen late credit/refund replay, per-owner settlement/clawback and audit`
    )
  }
  assert.deepEqual(errors, [])
  console.log(
    `PASS production site ownership ${width}; zero pageerrors, no real money or external mail`
  )
} catch (error) {
  if (artifacts) {
    for (const [i, context] of browser.contexts().entries()) {
      for (const [j, page] of context.pages().entries()) {
        await page
          .screenshot({
            path: join(artifacts, `failure-${i}-${j}.png`),
            fullPage: true,
            animations: 'disabled',
          })
          .catch(() => {})
      }
    }
  }
  throw error
} finally {
  await browser.close()
}
