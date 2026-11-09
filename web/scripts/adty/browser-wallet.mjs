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
import { createServer } from 'node:http'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
const username = process.env.ADTY_BROWSER_USERNAME
const password = process.env.ADTY_BROWSER_PASSWORD
assert.ok(
  base && username && password,
  'ADTY_BROWSER_BASE_URL, ADTY_BROWSER_USERNAME and ADTY_BROWSER_PASSWORD are required'
)
const origin = new URL(base)
assert.ok(
  ['127.0.0.1', 'localhost', '[::1]'].includes(origin.hostname),
  'The wallet fixture requires an isolated loopback backend'
)
const merchant = 'ADTY_BROWSER_GATE'
const merchantKey = randomBytes(24).toString('hex')
const gatewayOrders = new Map()
let gatewayCreateCount = 0,
  gatewayQueryCount = 0
function sign(params) {
  return createHash('md5')
    .update(
      Object.keys(params)
        .filter((k) => k !== 'sign' && k !== 'sign_type' && params[k] !== '')
        .sort()
        .map((k) => k + '=' + params[k])
        .join('&') + merchantKey
    )
    .digest('hex')
}
const mock = createServer(async (req, res) => {
  try {
    const url = new URL(req.url, gateway)
    if (url.pathname === '/mapi.php') {
      assert.equal(req.method, 'POST')
      let raw = ''
      for await (const chunk of req) raw += chunk
      const params = Object.fromEntries(new URLSearchParams(raw))
      assert.equal(params.pid, merchant)
      assert.equal(params.sign, sign(params))
      const trade = params.out_trade_no
      const record = {
        params,
        tradeID: `LOCAL_GATEWAY_${++gatewayCreateCount}`,
        paid: false,
      }
      gatewayOrders.set(trade, record)
      res.setHeader('Content-Type', 'application/json')
      res.end(
        JSON.stringify({
          code: 1,
          trade_no: record.tradeID,
          payurl: `${gateway}/checkout?order=${trade}`,
        })
      )
      return
    }
    if (url.pathname === '/api.php') {
      assert.equal(req.method, 'GET')
      assert.equal(url.searchParams.get('act'), 'order')
      assert.equal(url.searchParams.get('pid'), merchant)
      assert.equal(url.searchParams.get('key'), merchantKey)
      gatewayQueryCount++
      const record = gatewayOrders.get(url.searchParams.get('out_trade_no'))
      res.setHeader('Content-Type', 'application/json')
      res.end(
        JSON.stringify(
          record
            ? {
                code: 1,
                status: record.paid ? 1 : 0,
                money: record.params.money,
                type: record.params.type,
                trade_no: record.tradeID,
              }
            : { code: -1, msg: 'not found' }
        )
      )
      return
    }
    if (url.pathname === '/checkout') {
      const record = gatewayOrders.get(url.searchParams.get('order'))
      assert.ok(record)
      res.setHeader('Content-Type', 'text/html')
      res.end(
        `<h1>Local mock checkout</h1><p>${record.params.out_trade_no}</p>`
      )
      return
    }
    res.statusCode = 404
    res.end('not found')
  } catch (error) {
    res.statusCode = 400
    res.end(String(error))
  }
})
await new Promise((resolve, reject) => {
  mock.once('error', reject)
  mock.listen(0, '127.0.0.1', resolve)
})
const gateway = `http://127.0.0.1:${mock.address().port}`
let browser
let channelID,
  originalCallback,
  originalServerAddress,
  originalChats,
  originalPromoEnabled,
  originalNotice,
  authHeaders,
  page
async function api(method, path, data) {
  const response = await page.request[method](base + path, {
    headers: authHeaders,
    ...(data === undefined ? {} : { data }),
  })
  const body = await response.json()
  assert.equal(response.status(), 200, `${method} ${path}`)
  assert.equal(body.success, true, JSON.stringify(body))
  return body.data
}
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
      {
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
      }
    )
  } catch {
    // A child-process error includes its argv, which may contain a DSN password.
    throw new Error('Isolated PostgreSQL evidence query failed')
  }
}
try {
  browser = await chromium.launch({
    headless: true,
    ...(process.env.ADTY_CHROMIUM_PATH
      ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
      : {}),
  })
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  page = await context.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('request', (r) => {
    const h = r.headers()
    if (h.authorization) {
      authHeaders = {
        authorization: h.authorization,
        ...(h['new-api-user'] ? { 'new-api-user': h['new-api-user'] } : {}),
      }
    }
  })
  page.on('response', (r) => {
    if (r.status() >= 400) {
      console.log('HTTP', r.status(), new URL(r.url()).pathname)
    }
  })
  await page.goto(`${base}/sign-in`)
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.locator('input[name=username]').waitFor({ state: 'hidden' })
  await page.getByRole('link', { name: 'Wallet', exact: true }).click()
  await page.getByLabel('Custom amount (USD)', { exact: true }).waitFor()
  assert.ok(authHeaders?.authorization)
  const options = await api('get', '/api/option/')
  originalCallback =
    options.find((o) => o.key === 'CustomCallbackAddress')?.value ?? ''
  originalServerAddress =
    options.find((o) => o.key === 'ServerAddress')?.value ?? ''
  originalChats = options.find((o) => o.key === 'Chats')?.value ?? '[]'
  originalPromoEnabled =
    options.find((o) => o.key === 'adty_marketing_setting.promo_enabled')
      ?.value ?? 'false'
  originalNotice = options.find((o) => o.key === 'Notice')?.value ?? ''
  await api('put', '/api/option/', {
    key: 'Chats',
    value: JSON.stringify([{ 'Local browser chat': `${base}/about` }]),
  })
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.goto(`${base}/chat/0`)
    if (width < 768) {
      await page
        .getByRole('button', { name: 'Toggle Sidebar', exact: true })
        .click()
    }
    const chatMenu = page.getByRole('button', { name: 'Chat', exact: true })
    await chatMenu.waitFor()
    assert.equal(
      await page.getByRole('link', { name: 'Playground', exact: true }).count(),
      0
    )
    assert.equal(await chatMenu.getAttribute('aria-expanded'), 'false')
    assert.equal(
      await page.getByRole('link', { name: 'Marketing', exact: true }).count(),
      0
    )
    const mutations = []
    const trackMutation = (request) => {
      if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(request.method())) {
        mutations.push(new URL(request.url()).pathname)
      }
    }
    page.on('request', trackMutation)
    await chatMenu.getByText('Chat', { exact: true }).click()
    assert.equal(await chatMenu.getAttribute('aria-expanded'), 'true')
    const presetLink = page.getByRole('link', {
      name: 'Local browser chat',
      exact: true,
    })
    await presetLink.waitFor()
    assert.equal(await presetLink.getAttribute('data-active'), '')
    await chatMenu.locator('svg').last().click()
    assert.equal(await chatMenu.getAttribute('aria-expanded'), 'false')
    await chatMenu.focus()
    await page.keyboard.press('Enter')
    assert.equal(await chatMenu.getAttribute('aria-expanded'), 'true')
    await page.keyboard.press('Enter')
    assert.equal(await chatMenu.getAttribute('aria-expanded'), 'false')
    page.off('request', trackMutation)
    assert.equal(new URL(page.url()).pathname, '/chat/0')
    assert.deepEqual(
      mutations,
      [],
      'submenu toggles cannot submit business mutations'
    )
  }
  console.log(
    'PASS desktop/mobile active submenu defaults folded, label/arrow/keyboard toggles, active link and no business mutations'
  )
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto(base + '/playground')
  await page.getByRole('heading', { name: '404', exact: true }).waitFor()
  assert.equal(
    await page.getByRole('link', { name: 'Playground', exact: true }).count(),
    0
  )
  await page.goto(base + '/console/playground')
  await page.waitForURL('**/dashboard')
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.equal(await page.locator('a[href="/playground"]').count(), 0)
  for (const headers of [{}, authHeaders]) {
    const retired = await fetch(base + '/pg/chat/completions', {
      method: 'POST',
      headers: { ...headers, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model: 'gpt-4o',
        messages: [{ role: 'user', content: 'hi' }],
      }),
    })
    assert.equal(retired.status, 404)
    assert.ok(retired.headers.get('content-type')?.includes('application/json'))
  }
  console.log(
    'PASS retired Playground page 404, legacy bookmark dashboard fallback, absent menu/shortcuts and anonymous/session API 404'
  )
  await page.goto(base + '/marketing')
  await page.getByRole('heading', { name: '404', exact: true }).waitFor()
  for (const headers of [{}, authHeaders]) {
    for (const path of [
      '/api/adty/marketing/self',
      '/api/adty/admin/marketing/settings',
      '/api/adty/admin/marketing/campaigns',
      '/api/adty/admin/marketing/promo-codes',
    ]) {
      const retired = await fetch(base + path, { headers })
      assert.equal(retired.status, 404, path)
      assert.ok(
        retired.headers.get('content-type')?.includes('application/json')
      )
    }
  }
  await page.goto(base + '/system-settings/billing/promo-codes')
  const promoToggle = page.getByRole('switch', {
    name: 'Allow users to enter promo codes',
  })
  await promoToggle.waitFor()
  const wasEnabled = (await promoToggle.getAttribute('aria-checked')) === 'true'
  await promoToggle.click()
  const flagSaved = page.waitForResponse(
    (response) =>
      response.request().method() === 'PUT' &&
      new URL(response.url()).pathname === '/api/option/'
  )
  await page.getByRole('button', { name: 'Save Changes', exact: true }).click()
  assert.equal((await (await flagSaved).json()).success, true)
  await page.reload()
  await promoToggle.waitFor()
  assert.equal(
    await promoToggle.getAttribute('aria-checked'),
    String(!wasEnabled)
  )
  await page
    .getByRole('button', { name: 'New promo code', exact: true })
    .click()
  const promoDialog = page.getByRole('dialog', {
    name: 'New promo code',
    exact: true,
  })
  await promoDialog.getByLabel('Code', { exact: true }).fill('BROWSER10')
  await promoDialog.getByLabel('Bonus %', { exact: true }).fill('10')
  await promoDialog.getByRole('button', { name: 'Save', exact: true }).click()
  await promoDialog.waitFor({ state: 'hidden' })
  const promoRow = page.getByRole('row').filter({ hasText: 'BROWSER10' })
  await promoRow.getByRole('button', { name: 'Edit', exact: true }).click()
  const editPromo = page.getByRole('dialog', {
    name: 'Edit promo code',
    exact: true,
  })
  await editPromo
    .getByLabel('Note', { exact: true })
    .fill('Persisted browser promo edit')
  await editPromo.getByRole('button', { name: 'Save', exact: true }).click()
  await editPromo.waitFor({ state: 'hidden' })
  const promoList = await api('get', '/api/adty/admin/promo-codes')
  assert.equal(
    promoList.items.find((code) => code.code === 'BROWSER10')?.note,
    'Persisted browser promo edit'
  )
  await api('put', '/api/option/', {
    key: 'Notice',
    value: 'Local system announcement remains available',
  })
  const noticeResponse = await fetch(base + '/api/notice')
  assert.equal(noticeResponse.status, 200)
  assert.equal(
    (await noticeResponse.json()).data,
    'Local system announcement remains available'
  )
  console.log(
    'PASS retired Marketing menu/page/API 404, billing promo flag save/reload, promo create/edit and preserved system announcement'
  )
  await page.goto(`${base}/wallet`)
  await page.getByLabel('Custom amount (USD)', { exact: true }).waitFor()
  await api('put', '/api/option/', {
    key: 'ServerAddress',
    value: base,
  })
  await api('put', '/api/option/', {
    key: 'CustomCallbackAddress',
    value: base,
  })
  const created = await api('post', '/api/adty/admin/epay/channels', {
    name: 'Local browser Epay validation',
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
  channelID = created.id
  await api('post', `/api/adty/admin/epay/channels/${channelID}/status`, {
    status: 'enabled',
  })
  const listed = await api('get', '/api/adty/admin/epay/channels')
  assert.equal(listed.find((c) => c.id === channelID).has_key, true)
  assert.equal('merchant_key' in listed.find((c) => c.id === channelID), false)
  await page.reload()
  await page.getByRole('button', { name: 'Alipay', exact: true }).waitFor()
  const before = await api('get', '/api/user/self')
  const beforeQuota = before.quota
  console.log('BEGIN wallet quota', beforeQuota, 'channel', channelID)
  async function createOrder(amount) {
    await page
      .getByLabel('Custom amount (USD)', { exact: true })
      .fill(String(amount))
    await page.getByRole('button', { name: 'Alipay', exact: true }).click()
    const pay = page.getByRole('button', { name: 'Pay now', exact: true })
    await pay.waitFor()
    await page.waitForFunction(() =>
      [...document.querySelectorAll('button')].some(
        (b) => b.textContent.trim() === 'Pay now' && !b.disabled
      )
    )
    const [response] = await Promise.all([
      page.waitForResponse(
        (r) =>
          new URL(r.url()).pathname === '/api/adty/epay/pay' &&
          r.request().method() === 'POST'
      ),
      pay.click(),
    ])
    const body = await response.json()
    assert.equal(body.success, true, JSON.stringify(body))
    assert.equal(body.data.status, 'pending')
    await page
      .getByRole('heading', { name: 'Pay with Alipay', exact: true })
      .waitFor()
    await page.locator('[data-testid=epay-qr]').waitFor()
    return body.data
  }
  const first = await createOrder(5)
  assert.equal(gatewayCreateCount, 1)
  assert.ok(gatewayOrders.has(first.trade_no))
  const checkout = await page.context().newPage()
  await checkout.goto(first.ext.pay_url)
  await checkout
    .getByRole('heading', { name: 'Local mock checkout', exact: true })
    .waitFor()
  await checkout.close()
  await page.keyboard.press('Escape')
  await page.reload()
  await page
    .getByRole('button', { name: 'Continue payment', exact: true })
    .click()
  await page
    .getByRole('heading', { name: 'Pay with Alipay', exact: true })
    .waitFor()
  assert.equal(gatewayCreateCount, 1)
  assert.equal(
    (await api('get', `/api/adty/topup/orders/${first.trade_no}`)).status,
    'pending'
  )
  console.log(
    'PASS native UI creates order and resumes same pending order after reload',
    first.trade_no
  )
  async function notify(
    order,
    { wrongAmount = false, wrongSignature = false } = {}
  ) {
    const record = gatewayOrders.get(order.trade_no)
    const params = {
      pid: merchant,
      type: 'alipay',
      out_trade_no: order.trade_no,
      trade_no: record.tradeID,
      trade_status: 'TRADE_SUCCESS',
      money: wrongAmount
        ? (Number(record.params.money) + 0.01).toFixed(2)
        : record.params.money,
      name: record.params.name,
    }
    params.sign = wrongSignature
      ? '00000000000000000000000000000000'
      : sign(params)
    params.sign_type = 'MD5'
    const response = await fetch(`${base}/api/user/epay/notify/${channelID}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams(params),
    })
    return { status: response.status, text: await response.text(), params }
  }
  const badSignature = await notify(first, { wrongSignature: true })
  assert.equal(badSignature.text, 'fail')
  assert.equal((await api('get', '/api/user/self')).quota, beforeQuota)
  assert.equal(
    (await api('get', `/api/adty/topup/orders/${first.trade_no}`)).status,
    'pending'
  )
  const badAmount = await notify(first, { wrongAmount: true })
  assert.equal(badAmount.text, 'success')
  assert.equal((await api('get', '/api/user/self')).quota, beforeQuota)
  assert.equal(
    (await api('get', `/api/adty/topup/orders/${first.trade_no}`)).status,
    'pending'
  )
  console.log(
    'PASS wrong signature and signed wrong amount leave quota unchanged'
  )
  const paid = await notify(first)
  assert.equal(paid.text, 'success')
  gatewayOrders.get(first.trade_no).paid = true
  await page.getByText(/^Credited /).waitFor({ timeout: 15000 })
  const after = await api('get', '/api/user/self')
  assert.equal(after.quota, beforeQuota + first.ext.credit_quota)
  const repeat = await notify(first)
  assert.equal(repeat.text, 'success')
  assert.equal((await api('get', '/api/user/self')).quota, after.quota)
  await page.keyboard.press('Escape')
  const firstHistory = page
    .getByRole('table', { name: 'Top-up history' })
    .getByRole('row')
    .filter({ hasText: first.trade_no })
  await firstHistory.getByText('Credited', { exact: true }).waitFor()
  console.log(
    'PASS signed notify, frontend poll, wallet/history credit and duplicate idempotency',
    JSON.stringify({
      trade: first.trade_no,
      face: first.ext.face_usd,
      creditQuota: first.ext.credit_quota,
      before: beforeQuota,
      after: after.quota,
      payCNY: first.ext.pay_amount,
    })
  )
  const second = await createOrder(6)
  await Promise.all([
    page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname ===
        `/api/adty/topup/orders/${second.trade_no}/cancel`
    ),
    page.getByRole('button', { name: 'Cancel order', exact: true }).click(),
  ])
  await page
    .getByRole('heading', { name: 'Pay with Alipay', exact: true })
    .waitFor({ state: 'hidden' })
  assert.equal(
    (await api('get', `/api/adty/topup/orders/${second.trade_no}`)).status,
    'canceled'
  )
  assert.equal((await api('get', '/api/user/self')).quota, after.quota)
  await page.reload()
  await page.getByLabel('Custom amount (USD)', { exact: true }).waitFor()
  await page
    .getByRole('table', { name: 'Top-up history' })
    .getByRole('row')
    .filter({ hasText: second.trade_no })
    .getByText('Canceled', { exact: true })
    .waitFor()
  assert.equal((await api('get', '/api/adty/topup/info')).pending, null)
  console.log(
    'PASS native cancellation persists, clears pending recovery, and preserves balance',
    second.trade_no
  )
  const audit = await api(
    'get',
    `/api/user/admin/finance/recharge-audit?keyword=${
      first.trade_no
    }&p=1&page_size=20`
  )
  const audited = audit.items.find((o) => o.trade_no === first.trade_no)
  assert.equal(audited.status, 'success')
  assert.equal(Number(audited.paid_amount), Number(first.ext.pay_amount))
  const orphan = await api(
    'get',
    '/api/user/admin/finance/payment-orphans?status=pending_review&p=1&page_size=20'
  )
  const reviewEvents = orphan.items.filter(
    (event) => event.reference_id === first.trade_no
  )
  assert.ok(reviewEvents.length > 0)
  assert.ok(reviewEvents.every((event) => event.status === 'pending_review'))
  assert.ok(
    reviewEvents.some((event) => event.error.includes('amount mismatch'))
  )
  console.log(
    'AUDIT',
    JSON.stringify({
      trade: audited.trade_no,
      status: audited.status,
      paidAmount: audited.paid_amount,
      paidCurrency: audited.paid_currency,
      reviewEvents: reviewEvents.map((e) => ({
        id: e.id,
        kind: e.event_type,
        status: e.status,
        reason: e.reason,
        error: e.error,
      })),
    })
  )
  await page.getByRole('link', { name: 'Recharge Audit', exact: true }).click()
  const eventRow = page
    .getByRole('table', { name: 'Payments needing review' })
    .getByRole('row')
    .filter({ hasText: first.trade_no })
  await eventRow.getByRole('button', { name: 'Review', exact: true }).click()
  const reviewDialog = page.getByRole('dialog', {
    name: 'Review payment event',
  })
  await reviewDialog
    .getByLabel('Cumulative external reversal amount')
    .fill('0.01')
  await reviewDialog.getByLabel('External reversal currency').fill('CNY')
  await reviewDialog
    .getByLabel('Proof (transaction hash or platform order number)')
    .fill('LOCAL_GATEWAY_AUDIT')
  await reviewDialog
    .getByLabel('Note', { exact: true })
    .fill('Local browser review evidence')
  const [reviewResponse] = await Promise.all([
    page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname ===
          `/api/user/admin/finance/payment-orphans/${reviewEvents[0].id}/resolve` &&
        response.request().method() === 'POST'
    ),
    reviewDialog
      .getByRole('button', { name: 'Save review', exact: true })
      .click(),
  ])
  const reviewResult = await reviewResponse.json()
  assert.equal(reviewResult.success, true, JSON.stringify(reviewResult))
  assert.equal(reviewResult.data.status, 'pending_review')
  assert.equal(reviewResult.data.trade_no, first.trade_no)
  assert.equal(reviewResult.data.external_amount, '0.01')
  assert.equal(reviewResult.data.refund_ledger_id, 0)
  await reviewDialog.waitFor({ state: 'hidden' })
  assert.equal((await api('get', '/api/user/self')).quota, after.quota)
  await page.reload()
  await eventRow.getByRole('button', { name: 'Review', exact: true }).click()
  const snapshot = reviewDialog.getByRole('region', {
    name: 'Latest recorded review',
  })
  await snapshot.getByText('0.01 CNY', { exact: true }).waitFor()
  await snapshot
    .getByText('Local browser review evidence', { exact: true })
    .waitFor()
  await snapshot
    .getByText(
      'No linked refund; cash coverage and exposure have not been calculated.'
    )
    .waitFor()
  await reviewDialog
    .getByRole('button', { name: 'Cancel', exact: true })
    .click()
  console.log(
    'PASS native review evidence save, pending reconciliation, persisted snapshot and unchanged wallet'
  )
  const readerUsername = `reader${randomBytes(3).toString('hex')}`
  const readerPassword = `LocalGate!${randomBytes(12).toString('hex')}`
  await api('post', '/api/user/', {
    username: readerUsername,
    password: readerPassword,
    role: 1,
  })
  const readerLogin = await fetch(`${base}/api/user/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: readerUsername,
      password: readerPassword,
    }),
  })
  const readerBundle = await readerLogin.json()
  assert.equal(readerBundle.success, true)
  const globalVisibility = (
    await api('get', '/api/adty/admin/reseller/settings')
  ).visibility
  await api('put', '/api/adty/admin/reseller/settings', {
    enabled: true,
    root_domains: ['sites.example.test'],
    cost_factor: 0.8,
    min_cost_factor: 0.5,
    max_markup: 3,
    max_site_bonus_extra: 0,
    trial_grant_max_usd: 5,
    trial_grant_daily_usd: 20,
  })
  const syntheticSite = await api('post', '/api/adty/admin/sites', {
    user_id: readerBundle.data.user.id,
    subdomain: 'browser-site',
    root_domain: 'sites.example.test',
  })
  const sitePath = `/api/adty/admin/sites/${syntheticSite.id}`
  const siteHost = 'browser-site.sites.example.test'
  await page.getByRole('link', { name: 'Reseller sites', exact: true }).click()
  const siteRow = page
    .getByRole('table', { name: 'Reseller sites' })
    .getByRole('row')
    .filter({ hasText: siteHost })
  await siteRow.getByRole('button', { name: 'Details', exact: true }).click()
  const siteDialog = page.getByRole('dialog', { name: siteHost, exact: true })
  const visibilityLabels = [
    'Customer email',
    'Customer balance',
    'Customer top-ups',
    'Customer usage',
    'Request logs',
    'Customer API keys',
  ]
  for (const label of visibilityLabels) {
    assert.equal(
      await siteDialog.getByLabel(label, { exact: true }).inputValue(),
      ''
    )
  }
  const saveVisibility = async (expected, reason) => {
    await siteDialog.getByLabel('Reason', { exact: true }).fill(reason)
    const [response] = await Promise.all([
      page.waitForResponse(
        (response) =>
          new URL(response.url()).pathname === sitePath &&
          response.request().method() === 'PUT'
      ),
      siteDialog
        .getByRole('button', { name: 'Save site settings', exact: true })
        .click(),
    ])
    const request = response.request().postDataJSON()
    assert.deepEqual(Object.keys(request).sort(), ['reason', 'visibility'])
    assert.equal(request.reason, reason)
    assert.deepEqual(JSON.parse(request.visibility), expected)
    const result = await response.json()
    assert.equal(result.success, true, JSON.stringify(result))
    assert.deepEqual(JSON.parse(result.data.visibility), expected)
    await page.waitForFunction(
      () => document.querySelector('#site-reason')?.value === ''
    )
    const readback = await api('get', sitePath)
    assert.deepEqual(JSON.parse(readback.site.visibility), expected)
    assert.deepEqual(
      (await api('get', '/api/adty/admin/reseller/settings')).visibility,
      globalVisibility
    )
  }
  await siteDialog
    .getByLabel('Customer email', { exact: true })
    .selectOption('full')
  await siteDialog
    .getByLabel('Customer API keys', { exact: true })
    .selectOption('off')
  await saveVisibility(
    { customer_email: 'full', customer_tokens: 'off' },
    'Local browser owner visibility evidence'
  )
  await siteDialog.getByRole('button', { name: 'Close', exact: true }).click()
  await siteDialog.waitFor({ state: 'hidden' })
  await page.reload()
  await siteRow.getByRole('button', { name: 'Details', exact: true }).click()
  assert.equal(
    await siteDialog.getByLabel('Customer email', { exact: true }).inputValue(),
    'full'
  )
  assert.equal(
    await siteDialog
      .getByLabel('Customer API keys', { exact: true })
      .inputValue(),
    'off'
  )
  await siteDialog
    .getByLabel('Customer email', { exact: true })
    .selectOption('')
  await saveVisibility(
    { customer_tokens: 'off' },
    'Local browser restore email inheritance'
  )
  await siteDialog
    .getByLabel('Customer API keys', { exact: true })
    .selectOption('')
  await saveVisibility({}, 'Local browser restore all inheritance')
  await siteDialog.getByRole('button', { name: 'Close', exact: true }).click()
  await siteDialog.waitFor({ state: 'hidden' })
  console.log(
    'PASS native six site visibility controls, reasoned override-only save, reload persistence, individual and full inheritance, unchanged global visibility'
  )
  const readerHeaders = {
    Authorization: `Bearer ${readerBundle.data.access_token}`,
    'New-Api-User': String(readerBundle.data.user.id),
  }
  const readerSelf = await fetch(`${base}/api/user/self`, {
    headers: readerHeaders,
  })
  assert.equal((await readerSelf.json()).success, true)
  const otherOrder = await fetch(
    `${base}/api/adty/topup/orders/${first.trade_no}`,
    { headers: readerHeaders }
  )
  const otherBody = await otherOrder.json()
  assert.equal(otherBody.success, false)
  assert.ok(!otherBody.data)
  const readerAdmin = await fetch(`${base}/api/adty/admin/epay/channels`, {
    headers: readerHeaders,
  })
  assert.equal(readerAdmin.status, 403)
  const unauth = await fetch(`${base}/api/adty/topup/orders/${first.trade_no}`)
  assert.equal(unauth.status, 401)
  const unauthAdmin = await fetch(`${base}/api/adty/admin/epay/channels`)
  assert.equal(unauthAdmin.status, 401)

  await page.goto(base + '/site-admin/settings')
  const cloudflareToken = page.getByLabel('Cloudflare API token', {
    exact: true,
  })
  await cloudflareToken.waitFor()
  const token = `browser-token-${randomBytes(16).toString('hex')}`
  const widgetSecret = `browser-widget-${randomBytes(16).toString('hex')}`
  await cloudflareToken.fill(token)
  await page.getByRole('button', { name: 'Add widget', exact: true }).click()
  await page
    .getByLabel('Widget ID of widget 1', { exact: true })
    .fill('browser-widget')
  await page
    .getByLabel('Site key of widget 1', { exact: true })
    .fill('browser-site-key')
  await page
    .getByLabel('Secret of widget 1', { exact: true })
    .fill(widgetSecret)
  const secretSaved = page.waitForResponse(
    (response) =>
      response.request().method() === 'PUT' &&
      new URL(response.url()).pathname === '/api/adty/admin/reseller/settings'
  )
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  const secretResult = await (await secretSaved).json()
  assert.equal(secretResult.success, true)
  assert.equal(secretResult.data.cloudflare_token_configured, true)
  assert.equal(
    secretResult.data.turnstile_secret_configured['browser-widget'],
    true
  )
  for (const secret of [token, widgetSecret]) {
    assert.ok(!JSON.stringify(secretResult).includes(secret))
  }
  assert.ok(process.env.ADTY_BROWSER_SQL_DSN)
  const secretRowsQuery = `SELECT json_object_agg(key, value) FROM options WHERE key IN ('adty_reseller_setting.cloudflare_apiToken', 'adty_reseller_setting.turnstile_widget_secret')`
  const durableSecrets = JSON.parse(postgresEvidence(secretRowsQuery))
  assert.equal(Object.keys(durableSecrets).length, 2)
  for (const value of Object.values(durableSecrets)) {
    assert.ok(value.startsWith('adty:secret:v1:'))
    assert.ok(!value.includes(token) && !value.includes(widgetSecret))
  }
  const auditEvidence = postgresEvidence(
    `SELECT json_agg(row_to_json(a)) FROM audit_logs a WHERE action = 'adty.site.settings' AND success = true`
  )
  assert.ok(JSON.parse(auditEvidence).length > 0)
  assert.ok(auditEvidence.includes('cloudflare_apiToken'))
  assert.ok(
    !auditEvidence.includes(token) && !auditEvidence.includes(widgetSecret)
  )
  await page.reload()
  await cloudflareToken.waitFor()
  assert.equal(await cloudflareToken.inputValue(), '')
  assert.equal(await cloudflareToken.getAttribute('type'), 'password')
  assert.equal(
    await cloudflareToken.getAttribute('placeholder'),
    'Saved. Enter a new value to replace it'
  )
  assert.equal(
    await page.getByLabel('Secret of widget 1', { exact: true }).inputValue(),
    ''
  )
  const secretView = JSON.stringify(await api('get', '/api/option/'))
  for (const secret of [
    token,
    widgetSecret,
    ...Object.values(durableSecrets),
  ]) {
    assert.ok(!secretView.includes(secret))
  }
  await page
    .getByRole('checkbox', { name: 'Remove the saved token', exact: true })
    .check()
  const secretCleared = page.waitForResponse(
    (response) =>
      response.request().method() === 'PUT' &&
      new URL(response.url()).pathname === '/api/adty/admin/reseller/settings'
  )
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  assert.equal(
    (await (await secretCleared).json()).data.cloudflare_token_configured,
    false
  )
  await api('put', '/api/adty/admin/reseller/settings', {
    clear_secrets: ['turnstile_widget_secret'],
  })
  const clearedRows = JSON.parse(postgresEvidence(secretRowsQuery))
  assert.equal(clearedRows['adty_reseller_setting.cloudflare_apiToken'], '')
  assert.notEqual(
    clearedRows['adty_reseller_setting.turnstile_widget_secret'],
    durableSecrets['adty_reseller_setting.turnstile_widget_secret']
  )
  const clearedView = await api('get', '/api/adty/admin/reseller/settings')
  assert.equal(clearedView.cloudflare_token_configured, false)
  assert.deepEqual(clearedView.turnstile_secret_configured, {
    'browser-widget': false,
  })
  await page.reload()
  await cloudflareToken.waitFor()
  assert.equal(await cloudflareToken.getAttribute('placeholder'), '')
  assert.equal(
    await page
      .getByLabel('Secret of widget 1', { exact: true })
      .getAttribute('placeholder'),
    ''
  )
  await api('put', '/api/adty/admin/reseller/settings', {
    turnstile_widgets: [],
  })
  console.log(
    'PASS native Root token/widget save, encrypted PostgreSQL rows, reload without disclosure, key-only audit and explicit token clear'
  )
  assert.deepEqual(errors, [])
  console.log(
    'PASS different-user order isolation, user/admin permissions, unauthenticated denial, real audit and zero pageerrors'
  )
  console.log(
    'MOCK boundary',
    JSON.stringify({
      createRequests: gatewayCreateCount,
      queryRequests: gatewayQueryCount,
    })
  )
} catch (error) {
  console.error(error)
  if (page) {
    console.log('failureurl', page.url())
    console.log((await page.locator('body').innerText()).slice(-12000))
  }
  process.exitCode = 1
} finally {
  for (const [key, value] of [
    ['adty_marketing_setting.promo_enabled', originalPromoEnabled],
    ['Notice', originalNotice],
  ]) {
    if (value !== undefined && authHeaders) {
      try {
        await api('put', '/api/option/', { key, value })
      } catch (e) {
        console.error('cleanup option', key, String(e))
      }
    }
  }
  if (originalChats !== undefined && authHeaders) {
    try {
      await api('put', '/api/option/', { key: 'Chats', value: originalChats })
    } catch (e) {
      console.error('cleanup chats', String(e))
    }
  }
  if (channelID && authHeaders) {
    try {
      await api('post', `/api/adty/admin/epay/channels/${channelID}/status`, {
        status: 'disabled',
      })
    } catch (e) {
      console.error('cleanup channel', String(e))
    }
  }
  if (originalCallback !== undefined && authHeaders) {
    try {
      await api('put', '/api/option/', {
        key: 'CustomCallbackAddress',
        value: originalCallback,
      })
    } catch (e) {
      console.error('cleanup callback', String(e))
    }
  }
  if (originalServerAddress !== undefined && authHeaders) {
    try {
      await api('put', '/api/option/', {
        key: 'ServerAddress',
        value: originalServerAddress,
      })
    } catch (e) {
      console.error('cleanup server address', String(e))
    }
  }
  await browser?.close()
  mock.closeAllConnections()
  await new Promise((resolve) => mock.close(resolve))
}
