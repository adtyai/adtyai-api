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
import { createHash, randomBytes } from 'node:crypto'
import { mkdir } from 'node:fs/promises'
import { createServer } from 'node:http'
import { join } from 'node:path'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
assert.ok(base && process.env.ADTY_BROWSER_SQL_DSN)
assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(base).hostname))
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
      { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }
    ).trim()
  } catch {
    throw new Error('Isolated PostgreSQL evidence query failed')
  }
}
const merchant = 'SCENE_BROWSER'
const merchantKey = randomBytes(24).toString('hex')
const orders = new Map()
let gatewayCalls = 0
function sign(params) {
  return createHash('md5')
    .update(
      Object.keys(params)
        .filter(
          (key) => !['sign', 'sign_type'].includes(key) && params[key] !== ''
        )
        .sort()
        .map((key) => `${key}=${params[key]}`)
        .join('&') + merchantKey
    )
    .digest('hex')
}
const gatewayServer = createServer(async (request, response) => {
  try {
    assert.equal(new URL(request.url, gateway).pathname, '/mapi.php')
    let raw = ''
    for await (const chunk of request) raw += chunk
    const params = Object.fromEntries(new URLSearchParams(raw))
    assert.equal(params.pid, merchant)
    assert.equal(params.sign, sign(params))
    const order = { ...params, gateway_id: `SCENE_${++gatewayCalls}` }
    orders.set(params.out_trade_no, order)
    response.setHeader('Content-Type', 'application/json')
    response.end(
      JSON.stringify({
        code: 1,
        trade_no: order.gateway_id,
        payurl: `${gateway}/checkout`,
      })
    )
  } catch {
    response.statusCode = 400
    response.end('Synthetic gateway rejected request')
  }
})
await new Promise((resolve, reject) => {
  gatewayServer.once('error', reject)
  gatewayServer.listen(0, '127.0.0.1', resolve)
})
const gateway = `http://127.0.0.1:${gatewayServer.address().port}`
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
let page, authorization
const pageErrors = []
async function api(method, path, data) {
  const response = await page.request[method](base + path, {
    headers: { authorization },
    ...(data === undefined ? {} : { data }),
  })
  const body = await response.json()
  assert.equal(response.status(), 200, path)
  assert.equal(body.success, true, JSON.stringify(body))
  return body.data
}
async function screenshot(name) {
  if (artifacts) {
    await page.screenshot({ path: join(artifacts, name), fullPage: true })
  }
}
async function saveEditor(dialog) {
  await dialog
    .getByRole('button', { name: 'Save payment scene', exact: true })
    .click()
  const confirm = page.getByRole('alertdialog', {
    name: 'Save payment scene',
    exact: true,
  })
  const saved = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/adty/admin/payment-scenes' &&
      response.request().method() === 'PUT'
  )
  await confirm.getByRole('button', { name: 'Continue', exact: true }).click()
  const response = await saved
  assert.equal(response.status(), 200)
  const result = await response.json()
  assert.equal(result.success, true, JSON.stringify(result))
  await dialog.waitFor({ state: 'hidden' })
  return result.data
}
async function editEnabled(enabled) {
  await page.goto(`${base}/system-settings/billing/payment?tab=general`)
  await page
    .getByRole('button', {
      name: 'Edit payment scene Browser checkout',
      exact: true,
    })
    .click()
  const dialog = page.getByRole('dialog', {
    name: 'Edit payment scene',
    exact: true,
  })
  const toggle = dialog.getByRole('switch', { name: 'Enabled', exact: true })
  assert.notEqual(await toggle.getAttribute('aria-checked'), String(enabled))
  await toggle.click()
  return saveEditor(dialog)
}
try {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  page = await context.newPage()
  page.on('pageerror', (error) => pageErrors.push(error.message))
  page.on('request', (request) => {
    if (
      new URL(request.url()).pathname.startsWith('/api/') &&
      request.headers().authorization
    ) {
      authorization = request.headers().authorization
    }
  })
  await page.goto(`${base}/sign-in`)
  await page
    .getByLabel('Username or Email')
    .fill(process.env.ADTY_BROWSER_USERNAME)
  await page
    .getByLabel('Password', { exact: true })
    .fill(process.env.ADTY_BROWSER_PASSWORD)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(authorization)
  for (const key of ['ServerAddress', 'CustomCallbackAddress']) {
    await api('put', '/api/option/', { key, value: base })
  }
  const channel = await api('post', '/api/adty/admin/epay/channels', {
    name: 'Scene browser channel',
    gateway_url: gateway,
    merchant_id: merchant,
    merchant_key: merchantKey,
    priority: 999,
    weight: 1,
    order_mode: 'api',
    probe_mode: 'off',
    unpaid_threshold: 100,
    methods: [
      {
        method: 'alipay',
        type_code: 'alipay',
        min_cny: '0',
        max_cny: '10000',
        daily_limit_cny: '0',
        enabled: true,
        return_type: 'payurl',
      },
    ],
  })
  await api('post', `/api/adty/admin/epay/channels/${channel.id}/status`, {
    status: 'enabled',
  })
  await page.goto(`${base}/system-settings/billing/payment?tab=general`)
  await page
    .getByRole('button', { name: 'New payment scene', exact: true })
    .click()
  const editor = page.getByRole('dialog', {
    name: 'New payment scene',
    exact: true,
  })
  await editor
    .getByLabel('Scene identifier', { exact: true })
    .fill('browser_checkout')
  await editor.getByLabel('Name', { exact: true }).fill('Browser checkout')
  await editor.getByLabel('Minimum amount (USD)', { exact: true }).fill('5')
  await editor
    .getByLabel('Maximum amount (USD, 0 = unlimited)', { exact: true })
    .fill('100')
  await editor.getByRole('switch', { name: 'Enabled', exact: true }).click()
  await editor.getByLabel('Payment providers', { exact: true }).click()
  await page.getByRole('option', { name: 'Epay channels', exact: true }).click()
  await page.keyboard.press('Escape')
  await editor.getByLabel('Allowed Epay channels', { exact: true }).click()
  await page
    .getByRole('option', { name: new RegExp('Scene browser channel') })
    .click()
  await page.keyboard.press('Escape')
  await editor.getByLabel('Site scope', { exact: true }).selectOption('main')
  await editor
    .getByRole('button', { name: 'Save payment scene', exact: true })
    .click()
  const firstConfirm = page.getByRole('alertdialog', {
    name: 'Save payment scene',
    exact: true,
  })
  await firstConfirm
    .getByRole('button', { name: 'Cancel', exact: true })
    .click()
  assert.equal((await api('get', '/api/adty/admin/payment-scenes')).version, 1)
  assert.equal(
    await editor.getByLabel('Name', { exact: true }).inputValue(),
    'Browser checkout'
  )
  const created = await saveEditor(editor)
  assert.equal(created.version, 2)
  await page
    .getByRole('button', {
      name: 'Delete payment scene Wallet top-up',
      exact: true,
    })
    .click()
  const deleting = page.getByRole('alertdialog', {
    name: 'Delete payment scene',
    exact: true,
  })
  const deleted = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/adty/admin/payment-scenes' &&
      response.request().method() === 'PUT'
  )
  await deleting.getByRole('button', { name: 'Continue', exact: true }).click()
  assert.equal((await (await deleted).json()).success, true)
  await deleting.waitFor({ state: 'hidden' })
  assert.equal((await api('get', '/api/adty/admin/payment-scenes')).version, 3)
  await screenshot('management-1440.png')
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    if (width === 390) await editEnabled(true)
    const config = await api('get', '/api/adty/admin/payment-scenes')
    const before = await api('get', '/api/user/self')
    await page.goto(`${base}/wallet`)
    await page.getByLabel('Payment scene', { exact: true }).waitFor()
    assert.equal(
      await page.getByLabel('Payment scene', { exact: true }).inputValue(),
      'browser_checkout'
    )
    assert.equal(
      await page
        .getByLabel('Payment scene', { exact: true })
        .locator('option')
        .count(),
      1
    )
    await page.getByLabel('Custom amount (USD)', { exact: true }).fill('10')
    const quoteResponse = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === '/api/adty/topup/quote' &&
        response.request().method() === 'POST'
    )
    await page.getByRole('button', { name: 'Alipay', exact: true }).click()
    const quoted = await quoteResponse
    assert.equal(
      quoted.request().postDataJSON().payment_scene,
      'browser_checkout'
    )
    assert.equal((await quoted.json()).data.available, true)
    const pay = page.getByRole('button', { name: 'Pay now', exact: true })
    await page.waitForFunction(() =>
      [...document.querySelectorAll('button')].some(
        (button) => button.textContent.trim() === 'Pay now' && !button.disabled
      )
    )
    await screenshot(`wallet-${width}.png`)
    const orderResponse = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === '/api/adty/epay/pay' &&
        response.request().method() === 'POST'
    )
    await pay.click()
    const placed = await orderResponse
    assert.equal(
      placed.request().postDataJSON().payment_scene,
      'browser_checkout'
    )
    const result = await placed.json()
    assert.equal(result.success, true, JSON.stringify(result))
    const order = result.data
    assert.match(order.trade_no, /^[A-Za-z0-9_-]+$/)
    assert.equal(order.ext.payment_scene, 'browser_checkout')
    assert.equal(order.ext.payment_scene_name, 'Browser checkout')
    assert.equal(order.ext.payment_scene_version, config.version)
    assert.equal(order.ext.channel_id, channel.id)
    assert.equal('payment_scene_snapshot' in order.ext, false)
    const frozen = postgres(
      `SELECT payment_scene_snapshot FROM adty_topup_orders WHERE trade_no='${order.trade_no}'`
    )
    assert.equal(JSON.parse(frozen).version, config.version)
    assert.equal(frozen.includes(merchantKey), false)
    await page.keyboard.press('Escape')
    const disabled = await editEnabled(false)
    assert.equal(disabled.version, config.version + 1)
    await screenshot(`disabled-${width}.png`)
    const count = postgres('SELECT count(*) FROM top_ups')
    for (const data of [
      { amount: 10, method: 'alipay', payment_scene: 'browser_checkout' },
      { amount: 10, method: 'alipay' },
    ]) {
      const response = await page.request.post(`${base}/api/adty/epay/pay`, {
        headers: { authorization },
        data,
      })
      assert.equal((await response.json()).success, false)
    }
    assert.equal(postgres('SELECT count(*) FROM top_ups'), count)
    assert.equal(gatewayCalls, width === 1440 ? 1 : 2)
    const info = await api('get', '/api/adty/topup/info')
    assert.deepEqual(info.scenes, [])
    assert.deepEqual(info.methods, [])
    const record = orders.get(order.trade_no)
    const params = {
      pid: merchant,
      type: 'alipay',
      out_trade_no: order.trade_no,
      trade_no: record.gateway_id,
      trade_status: 'TRADE_SUCCESS',
      money: record.money,
      name: record.name,
    }
    params.sign = sign(params)
    params.sign_type = 'MD5'
    for (let attempt = 0; attempt < 2; attempt++) {
      const callback = await fetch(
        `${base}/api/user/epay/notify/${channel.id}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams(params),
        }
      )
      assert.equal(await callback.text(), 'success')
      const after = await api('get', '/api/user/self')
      assert.equal(after.quota, before.quota + order.ext.credit_quota)
    }
    assert.equal(
      postgres(
        `SELECT payment_scene_snapshot FROM adty_topup_orders WHERE trade_no='${order.trade_no}'`
      ),
      frozen
    )
    assert.equal(
      Number(
        postgres(
          `SELECT count(*) FROM adty_balance_ledgers WHERE ref_id='${order.trade_no}'`
        )
      ),
      1
    )
    const detail = await api(
      'get',
      `/api/user/admin/finance/recharge-audit/${order.trade_no}`
    )
    assert.equal(detail.order.payment_scene, 'browser_checkout')
    assert.equal(detail.order.payment_scene_name, 'Browser checkout')
    assert.equal(detail.order.payment_scene_version, config.version)
    assert.equal('payment_scene_snapshot' in detail.order, false)
    await page.goto(`${base}/wallet`)
    await page
      .getByRole('table', { name: 'Top-up history', exact: true })
      .getByText(`Browser checkout (browser_checkout) · v${config.version}`, {
        exact: true,
      })
      .waitFor()
    assert.equal(
      await page.getByRole('button', { name: 'Alipay', exact: true }).count(),
      0
    )
    await screenshot(`credited-${width}.png`)
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 1
      ),
      true
    )
    console.log(
      `PASS ${width}: Root confirmed configuration; scene quote/order; frozen v${config.version}; disabled new orders; signed callback/replay credits once; private snapshot; history and finance projection`
    )
  }
  assert.deepEqual(pageErrors, [])
  console.log('PASS payment-scenes production browser; pageerrors=0')
} catch (error) {
  if (page) await screenshot('failure.png').catch(() => {})
  throw error
} finally {
  await browser.close()
  await new Promise((resolve) => gatewayServer.close(resolve))
}
