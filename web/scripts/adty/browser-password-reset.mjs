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
import { randomBytes } from 'node:crypto'
import test from 'node:test'

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
assert.ok(base)
assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const pageErrors = []

try {
  await test('reset documents do not disclose credentials through resource referrers', async () => {
    const context = await browser.newContext()
    try {
      const page = await context.newPage()
      const requests = []
      page.on('pageerror', (error) => pageErrors.push(error.message))
      page.on('request', (request) => {
        if (!request.isNavigationRequest()) requests.push(request)
      })
      const response = await page.goto(
        `${base}/user/reset?email=synthetic%40example.test&token=synthetic-reset-proof`
      )
      assert.equal(response.headers()['referrer-policy'], 'no-referrer')
      await page
        .getByRole('heading', { name: 'Reset password', exact: true })
        .waitFor()
      assert.ok(requests.length > 0)
      for (const request of requests) {
        const referer = (await request.allHeaders()).referer
        if (referer) {
          // Fonts can name their stylesheet as referrer; the reset document
          // and its credentials must never be sent as that referrer.
          assert.notEqual(new URL(referer).pathname, '/user/reset')
          assert.equal(referer.includes('synthetic-reset-proof'), false)
        }
      }
    } finally {
      await context.close()
    }
  })

  for (const format of ['fragment', 'legacy-query']) {
    for (const width of [1440, 390]) {
      await test(`${format} ${width}: clear URL, retain submission, discard on reload`, async () => {
        const context = await browser.newContext({
          locale: 'en-US',
          viewport: { width, height: 950 },
        })
        try {
          const page = await context.newPage()
          page.on('pageerror', (error) => pageErrors.push(error.message))
          const email = 'reset+browser@example.test'
          const proof = randomBytes(24).toString('hex')
          const credentials = new URLSearchParams({ email, token: proof })
          const separator = format === 'fragment' ? '#' : '?'
          await page.goto(`${base}/user/reset${separator}${credentials}`)
          await page
            .getByRole('heading', { name: 'Reset password', exact: true })
            .waitFor()
          assert.equal(page.url(), `${base}/user/reset`)
          assert.equal(
            await page.getByLabel('Email', { exact: true }).inputValue(),
            email
          )
          assert.equal(
            await page.getByLabel('New password', { exact: true }).isEnabled(),
            true
          )
          const stored = await page.evaluate(() => ({
            local: Object.values(localStorage).join(' '),
            session: Object.values(sessionStorage).join(' '),
            history: JSON.stringify(history.state),
            readerPresent: '__adtyReadPasswordResetLink' in window,
          }))
          assert.equal(stored.readerPresent, false)
          for (const value of [stored.local, stored.session, stored.history]) {
            assert.equal(value.includes(proof) || value.includes(email), false)
          }
          const newPassword = `LocalReset!${randomBytes(12).toString('hex')}`
          await page
            .getByLabel('New password', { exact: true })
            .fill(newPassword)
          const pending = page.waitForResponse(
            (response) =>
              response.url() === `${base}/api/user/reset` &&
              response.request().method() === 'POST'
          )
          await page
            .getByRole('button', {
              name: 'Confirm reset password',
              exact: true,
            })
            .click()
          const response = await pending
          assert.deepEqual(response.request().postDataJSON(), {
            email,
            token: proof,
            password: newPassword,
          })
          assert.equal(
            (await response.request().allHeaders()).referer,
            undefined
          )
          assert.equal(
            (await response.json()).success,
            false,
            'the real backend must reject the synthetic proof'
          )
          await page.reload()
          await page
            .getByText(
              'Invalid reset link, please request a new password reset.',
              { exact: true }
            )
            .waitFor()
          assert.equal(
            await page.getByLabel('New password', { exact: true }).isEnabled(),
            false
          )
        } finally {
          await context.close()
        }
      })
    }
  }
  assert.deepEqual(pageErrors, [])
} finally {
  await browser.close()
}
