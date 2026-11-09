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
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const errors = []
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

try {
  const root = await session(
    process.env.ADTY_BROWSER_USERNAME,
    process.env.ADTY_BROWSER_PASSWORD
  )
  await success(root, 'PUT', '/api/user/admin/referral/settings', {
    enabled: true,
    default_rate: 10,
    min_withdraw_usd: 20,
    usdt_withdraw_fee: 2,
    cancel_window_minutes: 30,
  })
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
    value: JSON.stringify({ en: 'Synthetic withdrawal owner agreement.' }),
  })
  await success(root, 'PUT', '/api/option/', {
    key: 'adty_legal_setting.site_agreement_version',
    value: '1',
  })
  const password = `LocalWithdraw!${randomBytes(12).toString('hex')}`
  await success(root, 'POST', '/api/user/', {
    username: 'withdrawowner',
    password,
    role: 1,
  })
  const ownerId = Number(
    postgres("SELECT id FROM users WHERE username = 'withdrawowner'")
  )
  assert.ok(Number.isSafeInteger(ownerId) && ownerId > 0)
  await success(root, 'POST', '/api/adty/admin/sites', {
    user_id: ownerId,
    subdomain: 'withdrawnorth',
    root_domain: 'sites.localhost',
  })
  const owner = await session('withdrawowner', password)
  const profile = await success(owner, 'GET', '/api/user/referral/profile')
  assert.equal(profile.site_owner, true)
  await success(
    root,
    'POST',
    `/api/user/admin/referral/affiliates/${ownerId}/update`,
    {
      action: 'adjust',
      amount: '400',
      reason: 'Synthetic isolated browser balance, no external transfer',
      idempotency_key: 'browser-network-seed',
    }
  )
  let withdrawn = 0
  for (const width of [1440, 390]) {
    const address =
      width === 1440
        ? '0x52908400098527886E0F7030069857D2E4169EE7'
        : '0x52908400098527886E0F7030069857D2E4169EE8'
    await owner.page.setViewportSize({ width, height: 1000 })
    await root.page.setViewportSize({ width, height: 1000 })
    await owner.page.goto(`${base}/referral`)
    await owner.page
      .getByRole('button', { name: 'Withdraw', exact: true })
      .click()
    const drawer = owner.page.getByRole('dialog', {
      name: 'Withdraw commission',
      exact: true,
    })
    await drawer.getByRole('button', { name: 'USDT', exact: true }).click()
    await drawer.getByLabel('Amount (USD)', { exact: true }).fill('50')
    const network = drawer.getByLabel('Network', { exact: true })
    assert.deepEqual(await network.locator('option').allTextContents(), [
      'Choose a network',
      'TRC20',
      'BEP20',
      'Polygon',
      'X Layer',
    ])
    await network.selectOption('TRC20')
    const addressInput = drawer.getByLabel('USDT address', { exact: true })
    await addressInput.fill('TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t')
    await network.selectOption('XLAYER')
    assert.equal(await addressInput.inputValue(), '')
    await addressInput.fill('TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t')
    assert.equal(await addressInput.getAttribute('aria-invalid'), 'true')
    assert.equal(
      await drawer
        .getByRole('button', { name: 'Submit', exact: true })
        .isDisabled(),
      true
    )
    await addressInput.fill(address)
    await drawer.getByRole('button', { name: 'Submit', exact: true }).click()
    const confirmation = owner.page.getByRole('alertdialog', {
      name: 'Confirm the USDT withdrawal',
      exact: true,
    })
    await confirmation
      .getByText(`X Layer · ${address}`, { exact: true })
      .waitFor()
    await confirmation
      .getByText('You receive 48.00 USDT', { exact: true })
      .waitFor()
    if (artifacts) {
      await owner.page.screenshot({
        animations: 'disabled',
        path: join(artifacts, `owner-${width}-confirm.png`),
      })
    }
    const requestPromise = owner.page.waitForRequest(
      (request) =>
        request.url().endsWith('/api/user/referral/withdrawals') &&
        request.method() === 'POST'
    )
    const responsePromise = owner.page.waitForResponse(
      (response) =>
        response.url().endsWith('/api/user/referral/withdrawals') &&
        response.request().method() === 'POST'
    )
    await confirmation
      .getByRole('button', { name: 'Confirm', exact: true })
      .click()
    const request = await requestPromise
    const response = await responsePromise
    const result = await response.json()
    assert.equal(result.success, true, result.message)
    const row = result.data
    assert.equal(row.account_network, 'XLAYER')
    assert.equal(row.net_amount, 48)
    const replay = await success(
      owner,
      'POST',
      '/api/user/referral/withdrawals',
      request.postDataJSON()
    )
    assert.equal(replay.id, row.id)
    const frozen = await success(owner, 'GET', '/api/user/referral/profile')
    assert.deepEqual(
      [
        frozen.account.available_amount,
        frozen.account.frozen_amount,
        frozen.account.withdrawn_amount,
      ],
      [350 - withdrawn, 50, withdrawn]
    )
    assert.equal(
      (
        await api(
          owner,
          'POST',
          `/api/user/admin/referral/withdrawals/${row.id}/review`,
          { approve: true }
        )
      ).status,
      403
    )
    await drawer.waitFor({ state: 'hidden' })
    await owner.page
      .getByRole('tab', { name: 'Withdrawal records', exact: true })
      .click()
    await owner.page
      .getByRole('table', { name: 'Withdrawal records', exact: true })
      .getByText(`X Layer ${address}`, { exact: true })
      .waitFor()
    await root.page.goto(`${base}/admin-referral`)
    await root.page
      .getByRole('combobox', { name: 'Status', exact: true })
      .selectOption('')
    const adminRow = root.page
      .getByRole('row')
      .filter({ has: root.page.getByText(address, { exact: true }) })
    await adminRow.getByText('X Layer', { exact: true }).waitFor()
    await adminRow.getByText('48.00 USDT', { exact: true }).waitFor()
    await adminRow.getByRole('button', { name: 'Approve', exact: true }).click()
    await adminRow
      .getByRole('button', { name: 'Mark as paid', exact: true })
      .waitFor()
    const noTxn = await api(
      root,
      'POST',
      `/api/user/admin/referral/withdrawals/${row.id}/pay`,
      { txn_no: '', proof_url: '', note: '' }
    )
    assert.equal(noTxn.body.success, false)
    await adminRow
      .getByRole('button', { name: 'Mark as paid', exact: true })
      .click()
    const payout = root.page.getByRole('dialog', {
      name: 'Mark as paid',
      exact: true,
    })
    assert.equal(
      await payout
        .getByRole('button', { name: 'Confirm', exact: true })
        .isDisabled(),
      true
    )
    const txid = `synthetic-xlayer-txid-${width}`
    await payout.getByLabel('Transaction hash', { exact: true }).fill(txid)
    const uploadPromise = root.page.waitForResponse(
      (uploadResponse) =>
        uploadResponse.url().endsWith('/api/user/admin/referral/upload') &&
        uploadResponse.request().method() === 'POST'
    )
    await payout.getByLabel('Payment proof', { exact: true }).setInputFiles({
      name: 'synthetic-proof.png',
      mimeType: 'image/png',
      buffer: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=',
        'base64'
      ),
    })
    const uploaded = await (await uploadPromise).json()
    assert.equal(uploaded.success, true, uploaded.message)
    if (artifacts) {
      await root.page.screenshot({
        animations: 'disabled',
        path: join(artifacts, `root-${width}-payout.png`),
      })
    }
    await payout.getByRole('button', { name: 'Confirm', exact: true }).click()
    await payout.waitFor({ state: 'hidden' })
    await adminRow.getByText('Paid', { exact: true }).waitFor()
    withdrawn += 50
    const evidence = JSON.parse(
      postgres(
        `SELECT json_build_object('network',account_network,'address',account_no,'status',status,'amount',amount,'fee',fee_amount,'net',net_amount,'txid',payment_txn_no,'proof',payment_proof_url) FROM referral_withdrawals WHERE id = ${row.id}`
      )
    )
    assert.deepEqual(evidence, {
      network: 'XLAYER',
      address,
      status: 'paid',
      amount: 50,
      fee: 2,
      net: 48,
      txid,
      proof: uploaded.data.url,
    })
    const account = JSON.parse(
      postgres(
        `SELECT json_build_object('available',available_amount,'frozen',frozen_amount,'withdrawn',withdrawn_amount) FROM referral_commission_accounts WHERE user_id = ${ownerId}`
      )
    )
    assert.deepEqual(account, {
      available: 400 - withdrawn,
      frozen: 0,
      withdrawn,
    })
    assert.equal(
      Number(
        postgres(
          `SELECT count(*) FROM referral_commission_ledgers WHERE withdrawal_id = ${row.id} AND type = 'withdraw_paid'`
        )
      ),
      1
    )
    assert.equal(
      await root.page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      ),
      false
    )
    assert.equal(
      await owner.page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      ),
      false
    )
    console.log(
      `withdraw-networks: ${width}px X Layer confirmation, idempotent freeze, owner records, authorization, Root review, TxID/proof and SQL payout passed`
    )
  }
  assert.deepEqual(errors, [])
  console.log(
    'withdraw-networks: production PostgreSQL/Redis, synthetic funds only, pageerrors=0'
  )
} finally {
  await browser.close()
}
