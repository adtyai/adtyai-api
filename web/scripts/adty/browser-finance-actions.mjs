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
const username = process.env.ADTY_BROWSER_USERNAME
const password = process.env.ADTY_BROWSER_PASSWORD
assert.ok(base && username && password && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
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
const providers = [
  'bepusdt',
  'adty_epay',
  'stripe',
  'creem',
  'waffo',
  'waffo_pancake',
]
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const errors = []
const artifacts = process.env.ADTY_BROWSER_ARTIFACT_DIR
if (artifacts) await mkdir(artifacts, { recursive: true })
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
  const created = await page.request.post(`${base}/api/user/`, {
    headers: { authorization },
    data: {
      username: 'm04buyer',
      password: `LocalGate!${randomBytes(12).toString('hex')}`,
      role: 1,
    },
  })
  assert.equal(created.status(), 200)
  assert.equal((await created.json()).success, true)
  const userID = Number(
    postgres("SELECT id FROM users WHERE username = 'm04buyer'")
  )
  assert.ok(Number.isSafeInteger(userID) && userID > 0)
  const initialQuota = Number(
    postgres(`SELECT quota FROM users WHERE id = ${userID}`)
  )
  async function checkResponse(promise) {
    const response = await promise
    assert.equal(response.status(), 200)
    const body = await response.json()
    assert.equal(body.success, true, body.message)
    return body.data
  }
  for (const width of [1440, 390]) {
    // SQL prepares unpaid order fixtures only; all financial writes go through the real UI/API.
    for (const provider of providers) {
      const tradeNo = `m04-${width}-${provider}`
      const method =
        { bepusdt: 'usdt.trc20', adty_epay: 'alipay' }[provider] || provider
      postgres(
        `INSERT INTO top_ups (user_id, amount, money, trade_no, payment_provider, payment_method, create_time, status) VALUES (${userID}, 2, 2, '${tradeNo}', '${provider}', '${method}', extract(epoch from now())::bigint, 'pending'); INSERT INTO adty_topup_orders (trade_no, user_id, site_id, provider, method, face_usd, bonus_usd, credit_quota, pay_amount, pay_currency, provider_environment, created_at) VALUES ('${tradeNo}', ${userID}, 0, '${provider}', '${method}', 2, 0, 1000000, 2, 'USD', 'test', extract(epoch from now())::bigint)`
      )
    }
    await page.setViewportSize({ width, height: 1000 })
    await page.goto(`${base}/recharge-audit`)
    const select = page.getByRole('combobox', { name: 'Provider', exact: true })
    await select
      .locator('option[value="waffo_pancake"]')
      .waitFor({ state: 'attached' })
    assert.deepEqual(
      (
        await select
          .locator('option')
          .evaluateAll((options) => options.map((option) => option.value))
      )
        .filter(Boolean)
        .sort(),
      [...providers].sort()
    )
    for (const provider of providers) {
      const tradeNo = `m04-${width}-${provider}`
      await select.selectOption(provider)
      const row = page
        .getByRole('row')
        .filter({ has: page.getByText(tradeNo, { exact: true }) })
      await row.waitFor()
      assert.equal(
        await row
          .getByRole('button', { name: 'Query payment', exact: true })
          .count(),
        1
      )
      if (provider === 'creem' || provider === 'bepusdt') {
        await row
          .getByRole('button', { name: 'Query payment', exact: true })
          .click()
        const queryDialog = page.getByRole('alertdialog', {
          name: 'Query payment',
          exact: true,
        })
        const response = page.waitForResponse(
          (candidate) =>
            new URL(candidate.url()).pathname ===
            `/api/adty/admin/topup/${tradeNo}/query`
        )
        await queryDialog
          .getByRole('button', { name: 'Query payment', exact: true })
          .click()
        const body = await (await response).json()
        assert.equal(body.success, false)
        await queryDialog
          .getByText(
            'Payment query failed. Check the provider console or try again.',
            { exact: true }
          )
          .waitFor()
        if (provider === 'bepusdt' && artifacts) {
          await page.screenshot({
            path: join(artifacts, `bepusdt-query-${width}.png`),
            fullPage: true,
            animations: 'disabled',
          })
        }
        assert.equal(
          Number(postgres(`SELECT quota FROM users WHERE id = ${userID}`)),
          initialQuota
        )
        assert.equal(
          postgres(
            `SELECT count(*) FROM adty_balance_ledgers WHERE ref_id = '${tradeNo}'`
          ),
          '0'
        )
        await queryDialog
          .getByRole('button', { name: 'Cancel', exact: true })
          .click()
        await queryDialog.waitFor({ state: 'hidden' })
      }
      await row.getByRole('button', { name: 'Complete', exact: true }).click()
      const complete = page.getByRole('dialog', {
        name: 'Complete order manually',
        exact: true,
      })
      await complete
        .getByLabel('Proof (transaction hash or platform order number)', {
          exact: true,
        })
        .fill(`fixture-proof-${tradeNo}`)
      await complete
        .getByLabel('Note', { exact: true })
        .fill('Isolated browser manual-credit verification')
      const completed = page.waitForResponse(
        (response) =>
          new URL(response.url()).pathname ===
          `/api/adty/admin/topup/${tradeNo}/complete`
      )
      await complete
        .getByRole('button', { name: 'Complete', exact: true })
        .click()
      await checkResponse(completed)
      await complete.waitFor({ state: 'hidden' })
      await row.getByRole('button', { name: 'Refund', exact: true }).waitFor()
      assert.equal(
        Number(postgres(`SELECT quota FROM users WHERE id = ${userID}`)),
        initialQuota + 1000000
      )
      assert.equal(
        postgres(
          `SELECT count(*) FROM adty_balance_ledgers WHERE ref_id = '${tradeNo}' AND type = 'topup' AND delta_quota = 1000000`
        ),
        '1'
      )
      assert.equal(
        await row
          .getByRole('button', { name: 'Query payment', exact: true })
          .count(),
        0
      )
      await row.getByRole('button', { name: 'Refund', exact: true }).click()
      const refund = page.getByRole('dialog', {
        name: 'Refund order',
        exact: true,
      })
      await refund
        .getByLabel('Reason (required)', { exact: true })
        .fill('Isolated browser accounting refund verification')
      const refunded = page.waitForResponse(
        (response) =>
          new URL(response.url()).pathname ===
          `/api/adty/admin/topup/${tradeNo}/refund`
      )
      await refund.getByRole('button', { name: 'Refund', exact: true }).click()
      const result = await checkResponse(refunded)
      assert.equal(result.deducted_quota, 1000000)
      assert.equal(result.refund_quota, 1000000)
      assert.equal(Number(result.refund_amount), 2)
      const confirmation = page.getByRole('dialog', {
        name: 'Order refunded',
        exact: true,
      })
      await confirmation.getByText(/to the user offline/).waitFor()
      await page.keyboard.press('Escape')
      await confirmation.waitFor({ state: 'hidden' })
      assert.equal(
        Number(postgres(`SELECT quota FROM users WHERE id = ${userID}`)),
        initialQuota
      )
      assert.equal(
        postgres(`SELECT status FROM top_ups WHERE trade_no = '${tradeNo}'`),
        'refunded'
      )
      assert.equal(
        postgres(
          `SELECT count(*) FROM adty_balance_ledgers WHERE ref_id = '${tradeNo}' AND type = 'refund' AND delta_quota = -1000000`
        ),
        '1'
      )
      for (const action of ['Complete', 'Refund', 'Query payment']) {
        await row
          .getByRole('button', { name: action, exact: true })
          .waitFor({ state: 'detached' })
      }
      console.log(
        `PASS ${width}px ${provider}: filter, capabilities, manual credit, offline accounting refund, terminal actions`
      )
    }
  }
  assert.deepEqual(errors, [])
  console.log(
    'PASS six-provider production finance actions; pageerrors=0; query success covered by authenticated provider HTTP boundary tests'
  )
} finally {
  await browser.close()
}
