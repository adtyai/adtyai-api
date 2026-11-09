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
  args: ['--host-resolver-rules=MAP *.sites.example.test 127.0.0.1'],
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
    if (request.headers().authorization) {
      session.headers = { authorization: request.headers().authorization }
    }
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
  async function api(session, method, path, data) {
    const response = await session.page.request[method](base + path, {
      headers: session.headers,
      ...(data === undefined ? {} : { data }),
    })
    assert.equal(response.status(), 200, path)
    const body = await response.json()
    assert.equal(body.success, true, body.message || path)
    return body.data
  }
  await api(root, 'put', '/api/adty/admin/reseller/settings', {
    enabled: true,
    root_domains: ['sites.example.test'],
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
      JSON.stringify({
        en: 'Synthetic site agreement for isolated browser verification.',
      }),
    ],
    ['adty_legal_setting.site_agreement_version', '1'],
  ]) {
    await api(root, 'put', '/api/option/', { key, value })
  }
  const ownerPassword = `LocalGate!${randomBytes(12).toString('hex')}`
  const sites = []
  for (const name of ['brandowner', 'brandother']) {
    await api(root, 'post', '/api/user/', {
      username: name,
      password: ownerPassword,
      role: 1,
    })
    const userID = Number(
      postgres(`SELECT id FROM users WHERE username = '${name}'`)
    )
    assert.ok(Number.isSafeInteger(userID) && userID > 0)
    sites.push(
      await api(root, 'post', '/api/adty/admin/sites', {
        user_id: userID,
        subdomain: name,
        root_domain: 'sites.example.test',
      })
    )
  }
  const owner = await signIn('brandowner', ownerPassword)
  const page = owner.page
  await page.goto(`${base}/site-owner/settings`)
  await page
    .getByRole('button', { name: 'Accept site agreement', exact: true })
    .click()
  await page.getByLabel('Website title', { exact: true }).waitFor()
  const publicContext = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 1000 },
  })
  const publicPage = await publicContext.newPage()
  publicPage.on('pageerror', (error) => errors.push(error.message))
  const siteURL = `http://brandowner.sites.example.test:${new URL(base).port}`
  const otherURL = `http://brandother.sites.example.test:${new URL(base).port}`
  const image = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jv2kAAAAASUVORK5CYII=',
    'base64'
  )
  const faviconImage = Buffer.from(
    'UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA',
    'base64'
  )
  async function save(expectedSuccess = true) {
    const pending = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === '/api/adty/site/self' &&
        response.request().method() === 'PUT'
    )
    await page.getByRole('button', { name: 'Save brand', exact: true }).click()
    const response = await pending
    assert.equal(response.status(), 200)
    const body = await response.json()
    assert.equal(body.success, expectedSuccess, body.message)
    return body.data
  }
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await publicPage.setViewportSize({ width, height: 1000 })
    const name = `Brand ${width}`
    const title = `Workspace ${width}`
    const color = width === 1440 ? '#225588' : '#446622'
    await page.getByLabel('Site name', { exact: true }).fill(name)
    await page.getByLabel('Website title', { exact: true }).fill(title)
    for (const label of ['Logo', 'Favicon']) {
      const upload = page.waitForResponse(
        (response) => new URL(response.url()).pathname === '/api/adty/site/logo'
      )
      await page.getByLabel(label, { exact: true }).setInputFiles({
        name: label === 'Favicon' ? 'favicon.webp' : 'logo.png',
        mimeType: label === 'Favicon' ? 'image/webp' : 'image/png',
        buffer: label === 'Favicon' ? faviconImage : image,
      })
      const body = await (await upload).json()
      assert.equal(body.success, true, body.message)
      await page
        .getByRole('tabpanel', { name: 'Site settings', exact: true })
        .getByAltText(label, { exact: true })
        .waitFor()
      await page.waitForFunction(
        (url) => document.querySelector(`img[src="${url}"]`)?.naturalWidth > 0,
        body.data.url
      )
    }
    await page.getByLabel('Theme color', { exact: true }).fill('#GGGGGG')
    await save(false)
    await page
      .getByText('Theme color must use #RRGGBB', { exact: true })
      .waitFor()
    await page.getByLabel('Theme color', { exact: true }).fill(color)
    const saved = await save()
    assert.equal(saved.brand.title, title)
    assert.equal(saved.brand.theme_color, color)
    assert.notEqual(saved.brand.favicon_url, saved.brand.logo_url)
    assert.equal(
      JSON.parse(
        postgres(`SELECT brand FROM adty_sites WHERE id = ${sites[0].id}`)
      ).favicon_url,
      saved.brand.favicon_url
    )
    await page.reload()
    await page.getByLabel('Website title', { exact: true }).waitFor()
    assert.equal(
      await page.getByLabel('Website title', { exact: true }).inputValue(),
      title
    )
    await publicPage.goto(siteURL)
    await publicPage.waitForFunction(
      (value) =>
        document.title === value &&
        document.documentElement.style.getPropertyValue('--primary') !== '',
      title
    )
    const branding = await publicPage.evaluate(() => ({
      title: document.title,
      favicon: document
        .querySelector('link[rel~="icon"]')
        ?.getAttribute('href'),
      color: getComputedStyle(document.documentElement)
        .getPropertyValue('--primary')
        .trim(),
      meta: document
        .querySelector('meta[name="theme-color"]')
        ?.getAttribute('content'),
    }))
    assert.deepEqual(branding, {
      title,
      favicon: saved.brand.favicon_url,
      color,
      meta: color,
    })
    const asset = await publicPage.evaluate(async (url) => {
      const response = await fetch(url)
      return {
        status: response.status,
        contentType: response.headers.get('content-type'),
        contentOptions: response.headers.get('x-content-type-options'),
      }
    }, saved.brand.favicon_url)
    assert.deepEqual(asset, {
      status: 200,
      contentType: 'image/webp',
      contentOptions: 'nosniff',
    })
    await publicPage.reload()
    await publicPage.waitForFunction(
      (value) =>
        document.querySelector('link[rel~="icon"]')?.getAttribute('href') ===
        value,
      saved.brand.favicon_url
    )
    assert.equal(await publicPage.title(), title)
    await publicPage.goto(otherURL)
    await publicPage.waitForFunction(() => document.title === 'brandother')
    assert.equal(
      await publicPage.evaluate(() =>
        document.documentElement.style.getPropertyValue('--primary')
      ),
      ''
    )
    assert.equal(
      postgres(
        `SELECT COALESCE(brand, '') FROM adty_sites WHERE id = ${sites[1].id}`
      ),
      ''
    )
    await page.getByLabel('Website title', { exact: true }).fill('')
    await page.getByLabel('Theme color', { exact: true }).fill('')
    await page
      .getByRole('button', { name: 'Remove favicon', exact: true })
      .click()
    await page.getByRole('button', { name: 'Remove logo', exact: true }).click()
    await save()
    await publicPage.goto(siteURL)
    await publicPage.waitForFunction(
      (value) =>
        document.title === value &&
        document.querySelector('link[rel~="icon"]')?.getAttribute('href') ===
          '/favicon.ico',
      name
    )
    assert.equal(
      await publicPage.evaluate(() =>
        document.documentElement.style.getPropertyValue('--primary')
      ),
      ''
    )
    console.log(
      `PASS ${width}px independent image uploads, invalid-color retry, saved brand/SQL, public title/favicon/theme, reload, other Host and clear`
    )
  }
  assert.deepEqual(errors, [])
  console.log(
    'PASS site-brand production browser checks; pageerrors=0; all requests used the real isolated backend'
  )
} finally {
  await browser.close()
}
