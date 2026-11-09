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
import { createHash, randomBytes } from 'node:crypto'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'

export async function verifyOrderTrace(context) {
  const {
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
  } = context
  const artifacts = process.env.ADTY_BROWSER_ARTIFACT_DIR
  if (artifacts) {
    await mkdir(artifacts, { recursive: true })
  }
  const secret = randomBytes(24).toString('hex')
  await success(root, 'PUT', '/api/option/', {
    key: 'adty_bepusdt.secret',
    value: secret,
  })
  for (const [trade, provider, method, currency] of [
    ['m03-callback-order', 'bepusdt', 'usdt.trc20', 'USDT'],
    ['m03-manual-order', 'stripe', 'stripe', 'USD'],
  ]) {
    postgres(
      `INSERT INTO top_ups (user_id, amount, money, trade_no, payment_provider, payment_method, create_time, complete_time, status) VALUES (${buyer}, 2, 2, '${trade}', '${provider}', '${method}', 1767225600, 0, 'pending')`
    )
    postgres(
      `INSERT INTO adty_topup_orders (trade_no, user_id, site_id, provider, method, face_usd, bonus_usd, credit_quota, pay_amount, pay_currency, topup_group_ratio, created_at) VALUES ('${trade}', ${buyer}, ${north.id}, '${provider}', '${method}', 2, 0.2, 1100000, 2, '${currency}', 1, 1767225600)`
    )
  }
  const callback = async (status) => {
    const body = {
      order_id: 'm03-callback-order',
      status,
      actual_amount: '2',
      amount: '2',
      block_transaction_id: 'synthetic-tx-m03',
      secret_marker: 'private-provider-payload',
    }
    const canonical = Object.keys(body)
      .sort()
      .map((key) => `${key}=${body[key]}`)
      .join('&')
    body.signature = createHash('md5')
      .update(canonical + secret)
      .digest('hex')
    const result = await root.page.evaluate(async (body) => {
      const response = await fetch('/api/user/bepusdt/notify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      return { status: response.status, text: await response.text() }
    }, body)
    assert.deepEqual(result, { status: 200, text: 'ok' })
  }
  // Real signed pending notifications exceed one page without changing funds.
  for (let i = 0; i < 17; i++) await callback(1)
  await callback(2)
  await callback(2)
  assert.equal(
    Number(postgres(`SELECT quota FROM users WHERE id = ${buyer}`)),
    1100000
  )
  const proof = `private-manual-order-proof-${'x'.repeat(180)}`
  await success(
    root,
    'POST',
    '/api/adty/admin/topup/m03-manual-order/complete',
    { proof, note: 'private-manual-note' }
  )
  const preview = await success(
    root,
    'GET',
    '/api/adty/admin/topup/m03-callback-order/refund-preview'
  )
  await success(
    root,
    'POST',
    '/api/adty/admin/topup/m03-callback-order/refund',
    {
      reason: 'private-refund-note',
      idempotency_key: 'm03-browser-refund',
      refund_quota: preview.refund_quota,
    }
  )
  const detail = await success(
    root,
    'GET',
    '/api/user/admin/finance/recharge-audit/m03-callback-order?section=payments&page_size=20'
  )
  assert.equal(detail.total, 21)
  assert.equal(detail.items.length, 20)
  assert.equal(detail.order.site_id, north.id)
  assert.equal(detail.order.status, 'refunded')
  assert.equal(detail.refund.refund_amount, '2')
  assert.equal(detail.refund.reason, 'private-refund-note')
  const second = await success(
    root,
    'GET',
    '/api/user/admin/finance/recharge-audit/m03-callback-order?section=payments&p=2&page_size=20'
  )
  assert.equal(second.total, 21)
  assert.equal(second.items.length, 1)
  for (const section of [
    'payments',
    'wallet',
    'bonuses',
    'site_wallet',
    'exceptions',
    'reviews',
  ]) {
    const scoped = await success(
      owner,
      'GET',
      `/api/adty/site/orders/m03-callback-order?section=${section}&site_id=${south.id}`
    )
    assert.equal(scoped.order.site_id, north.id)
    const serialized = JSON.stringify(scoped)
    for (const privateValue of [
      secret,
      'private-provider-payload',
      'private-refund-note',
    ]) {
      assert.ok(!serialized.includes(privateValue))
    }
    assert.equal(scoped.refund.reason, undefined)
    if (section === 'wallet') {
      assert.equal(scoped.total, 2)
      assert.ok(scoped.items.some((entry) => entry.delta_quota === 1100000))
      assert.ok(scoped.items.some((entry) => entry.delta_quota === -1100000))
      assert.ok(
        scoped.items.every(
          (entry) =>
            entry.balance_before === undefined &&
            entry.balance_after === undefined
        )
      )
    }
  }
  const manual = await success(
    owner,
    'GET',
    '/api/adty/site/orders/m03-manual-order'
  )
  assert.equal(manual.order.paid_tx_hash, '')
  assert.ok(!JSON.stringify(manual).includes(proof))
  assert.equal(manual.items[0].source, 'manual')
  for (const trade of [
    'm02-south-order',
    'm02-main-order',
    'm02-legacy-order',
    'missing-order',
  ]) {
    assert.equal(
      (await api(owner, 'GET', `/api/adty/site/orders/${trade}`)).status,
      404
    )
  }
  for (const width of [1440, 390]) {
    for (const state of [root, owner]) {
      await state.page.setViewportSize({ width, height: 1000 })
      await state.page.goto(
        `${base}/${state === root ? 'recharge-audit' : 'site-owner/orders'}`
      )
      await orderRow(state.page, 'm03-callback-order')
        .getByRole('button', { name: 'm03-callback-order', exact: true })
        .click()
      const dialog = state.page.getByRole('dialog', {
        name: 'Order details',
        exact: true,
      })
      await dialog
        .getByText(
          'Earlier payment attempts may not have been recorded. The order snapshot is not a complete callback history.',
          { exact: true }
        )
        .waitFor()
      await dialog
        .getByRole('table', { name: 'Trace records', exact: true })
        .waitFor()
      await dialog.getByRole('button', { name: 'Next', exact: true }).click()
      await dialog.getByText('2 / 2 · 21', { exact: true }).waitFor()
      assert.equal(
        await dialog
          .getByRole('button', { name: 'Next', exact: true })
          .isDisabled(),
        true
      )
      await dialog
        .getByRole('combobox', { name: 'Trace records', exact: true })
        .selectOption('wallet')
      await dialog
        .getByText('Quota change: -1,100,000', { exact: true })
        .waitFor()
      if (state === root) {
        await dialog
          .getByText(/private-refund-note/)
          .first()
          .waitFor()
      } else {
        assert.ok(!(await dialog.textContent()).includes('private-refund-note'))
        assert.equal(
          await dialog.locator('option[value="commissions"]').count(),
          0
        )
      }
      assert.equal(
        await state.page.evaluate(
          () => document.documentElement.scrollWidth > window.innerWidth
        ),
        false
      )
      if (artifacts) {
        await state.page.screenshot({
          animations: 'disabled',
          path: join(
            artifacts,
            `${state === root ? 'root' : 'owner'}-${width}-refund.png`
          ),
        })
      }
      await state.page.keyboard.press('Escape')
      await dialog.waitFor({ state: 'hidden' })
      await orderRow(state.page, 'm03-manual-order')
        .getByRole('button', { name: 'm03-manual-order', exact: true })
        .click()
      const manualDialog = state.page.getByRole('dialog', {
        name: 'Order details',
        exact: true,
      })
      await manualDialog
        .getByRole('table', { name: 'Trace records', exact: true })
        .waitFor()
      if (state === root) {
        await manualDialog
          .getByText(`Payment proof: ${proof}`, { exact: true })
          .waitFor()
      } else {
        assert.ok(!(await manualDialog.textContent()).includes(proof))
      }
      assert.equal(
        await state.page.evaluate(
          () => document.documentElement.scrollWidth > window.innerWidth
        ),
        false
      )
      if (artifacts) {
        await state.page.screenshot({
          animations: 'disabled',
          path: join(
            artifacts,
            `${state === root ? 'root' : 'owner'}-${width}-manual.png`
          ),
        })
      }
      await state.page.keyboard.press('Escape')
      await manualDialog.waitFor({ state: 'hidden' })
    }
    console.log(
      `order-trace: ${width}px detail, payment pagination, callback/credit/refund chain, manual-proof redaction and owner isolation passed`
    )
  }
  assert.equal(
    Number(
      postgres(
        `SELECT COUNT(*) FROM adty_balance_ledgers WHERE ref_type = 'topup' AND ref_id = 'm03-callback-order' AND type = 'topup'`
      )
    ),
    1
  )
  console.log(
    'order-trace: real signed callbacks and replay credited once; refund and 21 payment records persisted'
  )
}
