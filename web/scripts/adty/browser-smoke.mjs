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
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { resolve } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright'

const webRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)))
const chromiumPath = process.env.ADTY_CHROMIUM_PATH

function captureOutput(stream, output) {
  stream.on('data', (chunk) => {
    output.text = `${output.text}${chunk}`.slice(-4000)
  })
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolveListen, rejectListen) => {
    server.once('error', rejectListen)
    server.listen(0, '127.0.0.1', resolveListen)
  })
  const address = server.address()
  if (!address || typeof address === 'string') {
    throw new Error('Could not reserve a local browser-smoke port')
  }
  await new Promise((resolveClose, rejectClose) => {
    server.close((error) => (error ? rejectClose(error) : resolveClose()))
  })
  return address.port
}

async function waitForPreview(url, child, output) {
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`Production preview exited early:\n${output.text}`)
    }
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1000) })
      if (response.ok) return
    } catch {
      // Keep waiting while the isolated preview server starts.
    }
    await delay(200)
  }
  throw new Error(`Production preview did not become ready:\n${output.text}`)
}

async function stopPreview(child) {
  if (child.exitCode !== null) return
  child.kill('SIGTERM')
  await Promise.race([
    new Promise((resolveExit) => child.once('exit', resolveExit)),
    delay(5000).then(() => child.kill('SIGKILL')),
  ])
}

const port = await reserveLocalPort()
const baseURL = `http://127.0.0.1:${port}`
const output = { text: '' }
const preview = spawn(
  process.env.ADTY_BUN_PATH ?? 'bun',
  [
    'run',
    'preview',
    '--host',
    '127.0.0.1',
    '--port',
    String(port),
    '--strictPort',
  ],
  { cwd: webRoot, stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true }
)
captureOutput(preview.stdout, output)
captureOutput(preview.stderr, output)

let browser
try {
  await waitForPreview(baseURL, preview, output)
  browser = await chromium.launch({
    headless: true,
    ...(chromiumPath ? { executablePath: chromiumPath } : {}),
  })
  const page = await browser.newPage({
    locale: 'en-US',
    viewport: { width: 1280, height: 800 },
  })
  const pageErrors = []
  page.on('pageerror', (error) => pageErrors.push(error.message))

  const response = await page.goto(baseURL, { waitUntil: 'domcontentloaded' })
  if (response?.status() !== 200) {
    throw new Error(`Production homepage returned HTTP ${response?.status()}`)
  }
  await page
    .getByRole('heading', {
      level: 1,
      name: 'Claude, GPT and Gemini with one API key',
    })
    .waitFor({ state: 'visible', timeout: 30_000 })
  const pricingLink = page.getByRole('link', { name: 'View pricing' })
  await pricingLink.waitFor({ state: 'visible' })
  if ((await pricingLink.getAttribute('href')) !== '/pricing') {
    throw new Error('Production homepage pricing link did not render correctly')
  }
  if (pageErrors.length > 0) {
    throw new Error(`Browser runtime errors:\n${pageErrors.join('\n')}`)
  }

  console.log('browser smoke: production homepage rendered in Chromium')
} finally {
  await browser?.close()
  await stopPreview(preview)
}
