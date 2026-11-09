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

import { chromium } from 'playwright'

const base = process.env.ADTY_BROWSER_BASE_URL
const username = process.env.ADTY_BROWSER_USERNAME
const password = process.env.ADTY_BROWSER_PASSWORD
assert.ok(
  base && username && password,
  'An isolated backend and synthetic root are required'
)
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(base).hostname))
const browser = await chromium.launch({
  headless: true,
  ...(process.env.ADTY_CHROMIUM_PATH
    ? { executablePath: process.env.ADTY_CHROMIUM_PATH }
    : {}),
})
const context = await browser.newContext({
  locale: 'en-US',
  viewport: { width: 1440, height: 1000 },
})
const page = await context.newPage()
const errors = []
let authHeaders
page.on('pageerror', (error) => errors.push(error.message))
page.on('request', (request) => {
  const headers = request.headers()
  if (headers.authorization) {
    authHeaders = { authorization: headers.authorization }
  }
})
async function api(method, path) {
  const response = await page.request[method](base + path, {
    headers: authHeaders,
  })
  assert.equal(response.status(), 200, path)
  const body = await response.json()
  assert.equal(body.success, true, path)
  return body.data
}
try {
  await page.goto(base + '/sign-in')
  await page.getByLabel('Username or Email').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('heading', { name: 'Overview', exact: true }).waitFor()
  assert.ok(authHeaders?.authorization)
  const deploymentRequests = []
  const trackDeployment = (request) => {
    const url = new URL(request.url())
    if (
      url.pathname.startsWith('/api/deployments') ||
      url.hostname.endsWith('io.net')
    ) {
      deploymentRequests.push(request.url())
    }
  }
  page.on('request', trackDeployment)
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.goto(base + '/models/deployments')
    await page.waitForURL('**/models/metadata')
    await page
      .getByRole('heading', { name: 'Model management', exact: true })
      .waitFor()
    await page.getByRole('tab', { name: 'Models', exact: true }).waitFor()
    await page.getByRole('tab', { name: 'Vendors', exact: true }).waitFor()
    assert.equal(
      await page.getByRole('tab', { name: 'Deployments', exact: true }).count(),
      0
    )
    await page.goto(base + '/system-settings/models/model-deployment')
    await page.waitForURL('**/system-settings/models/global')
    assert.equal(
      await page.getByText('io.net Deployments', { exact: true }).count(),
      0
    )
  }
  await page.goto(base + '/console/deployment')
  await page.waitForURL('**/models/metadata')
  await page.goto(base + '/console/setting?tab=model-deployment')
  await page.waitForURL('**/system-settings/models/global?tab=model-deployment')
  page.off('request', trackDeployment)
  assert.deepEqual(
    deploymentRequests,
    [],
    'retired UI must not contact a deployment service'
  )
  assert.ok(await api('get', '/api/models/'))
  assert.ok(await api('get', '/api/channel/'))
  assert.equal(
    (await api('get', '/api/option/')).some((option) =>
      option.key.startsWith('model_deployment.')
    ),
    false
  )
  for (const headers of [{}, authHeaders]) {
    for (const path of [
      '/api/deployments',
      '/api/deployments/settings',
      '/api/deployments/old/containers',
    ]) {
      const response = await fetch(base + path, { headers })
      assert.equal(response.status, 404, path)
      assert.ok(
        response.headers.get('content-type')?.includes('application/json')
      )
    }
  }
  console.log(
    'PASS retired deployment desktop/mobile tabs and settings, legacy fallback, no deployment calls, API JSON404 and retained model/channel reads'
  )
  assert.deepEqual(errors, [])
  console.log(
    'PASS deployment retirement zero pageerrors with default security rate limits'
  )
} catch (error) {
  console.error(error)
  console.log('failureurl', page.url())
  process.exitCode = 1
} finally {
  await browser.close()
}
