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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { ModelDetailsApi } from '../components/model-details-api'
import type { PricingModel } from '../types'

const clients: QueryClient[] = []
const baseUrl = 'https://gateway.example/proxy'
const apiKey = 'sk-test-project-key'
const message = 'Explain quantum entanglement in one paragraph.'

function renderApi(
  endpointType: string,
  path: string,
  modelName = 'test-model'
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(STATUS_QUERY_KEY, {
    server_address: `${baseUrl}/`,
    system_name: 'ADTY AI',
  })
  const model: PricingModel = {
    id: 1,
    model_name: modelName,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default', 'premium'],
    supported_endpoint_types: [endpointType],
  }
  return render(
    <QueryClientProvider client={client}>
      <ModelDetailsApi
        model={model}
        endpointMap={{ [endpointType]: { path, method: 'POST' } }}
      />
    </QueryClientProvider>
  )
}

async function copySample(language: string) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: language }))
  await user.click(screen.getByRole('button', { name: 'Copy code' }))
  return navigator.clipboard.readText()
}

type CapturedRequest = {
  url: string
  method: string | undefined
  headers: Record<string, string>
  body: Record<string, unknown>
}

async function runFetchSample(code: string): Promise<CapturedRequest[]> {
  const requests: CapturedRequest[] = []
  const execute = new Function(
    'fetch',
    'process',
    'console',
    `return (async () => { ${code}\n})()`
  )
  await execute(
    async (url: string, init: RequestInit) => {
      requests.push({
        url,
        method: init.method,
        headers: Object.fromEntries(new Headers(init.headers).entries()),
        body: JSON.parse(String(init.body)) as Record<string, unknown>,
      })
      return {
        ok: true,
        json: async () => ({
          id: 'test-response',
          choices: [
            { message: { role: 'assistant', content: 'test response' } },
          ],
          output_text: 'test response',
          content: [{ type: 'text', text: 'test response' }],
          candidates: [{ content: { parts: [{ text: 'test response' }] } }],
          data: [{ embedding: [0.1, 0.2] }],
        }),
      }
    },
    { env: { ADTYAI_API_KEY: apiKey } },
    { log: () => undefined }
  )
  return requests
}

afterEach(() => {
  for (const client of clients.splice(0)) client.clear()
})

describe('model catalog rate limits', () => {
  it('explains unavailable effective limits instead of inventing per-group allowances', () => {
    renderApi('openai', '/v1/chat/completions')
    const notice = screen.getByRole('alert')
    expect(notice).toHaveTextContent(/rate limits.*not available/i)
    expect(notice).toHaveTextContent(/account and API key/i)
    expect(
      screen.queryByRole('columnheader', { name: 'RPM' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'TPM' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'RPD' })
    ).not.toBeInTheDocument()
  })
})

describe('copied HTTP examples', () => {
  const cases = [
    {
      type: 'openai',
      path: '/v1/chat/completions',
      body: {
        model: 'test-model',
        messages: [{ role: 'user', content: message }],
      },
    },
    {
      type: 'openai-response',
      path: '/v1/responses',
      body: { model: 'test-model', input: message },
    },
    {
      type: 'anthropic',
      path: '/v1/messages',
      body: {
        model: 'test-model',
        max_tokens: 1024,
        messages: [{ role: 'user', content: message }],
      },
    },
    {
      type: 'gemini',
      path: '/v1beta/models/{model}:generateContent',
      resolvedPath: '/v1beta/models/test-model:generateContent',
      body: { contents: [{ parts: [{ text: message }] }] },
    },
    {
      type: 'embeddings',
      path: '/v1/embeddings',
      body: {
        model: 'test-model',
        input: 'The food was delicious and the waiter…',
      },
    },
    {
      type: 'jina-rerank',
      path: '/v1/rerank',
      body: {
        model: 'test-model',
        query: 'What is the capital of France?',
        documents: [
          'Paris is the capital of France.',
          'Berlin is the capital of Germany.',
        ],
      },
    },
    {
      type: 'image-generation',
      path: '/v1/images/generations',
      body: {
        model: 'test-model',
        prompt: 'A serene koi pond at sunset, ukiyo-e style.',
        size: '1024x1024',
        n: 1,
      },
    },
  ]

  it.each(cases)(
    'sends the $type JavaScript example to this gateway with the correct contract',
    async (fixture) => {
      renderApi(fixture.type, fixture.path)
      const code = await copySample('JavaScript')
      const requests = await runFetchSample(code)
      expect(requests).toHaveLength(1)
      expect(requests[0]).toMatchObject({
        url: `${baseUrl}${fixture.resolvedPath ?? fixture.path}`,
        method: 'POST',
        body: fixture.body,
      })
      expect(requests[0].headers['content-type']).toBe('application/json')
      if (fixture.type === 'gemini') {
        expect(requests[0].headers['x-goog-api-key']).toBe(apiKey)
      } else if (fixture.type === 'anthropic') {
        expect(requests[0].headers['x-api-key']).toBe(apiKey)
        expect(requests[0].headers['anthropic-version']).toBe('2023-06-01')
      } else {
        expect(requests[0].headers.authorization).toBe(`Bearer ${apiKey}`)
      }
    }
  )

  it.each(['TypeScript', 'JavaScript'])(
    'keeps Gemini key interpolation valid in %s',
    async (language) => {
      renderApi('gemini', '/v1beta/models/{model}:generateContent')
      const code = await copySample(language)
      expect(code).toContain(
        `${baseUrl}/v1beta/models/test-model:generateContent`
      )
      const requests = await runFetchSample(code)
      expect(requests[0].url).toBe(
        `${baseUrl}/v1beta/models/test-model:generateContent`
      )
      expect(requests[0].headers['x-goog-api-key']).toBe(apiKey)
    }
  )

  it('uses an expandable shell header for Gemini instead of a single-quoted key query', async () => {
    renderApi('gemini', '/v1beta/models/{model}:generateContent')
    const code = await copySample('cURL')
    expect(code).toContain(
      `${baseUrl}/v1beta/models/test-model:generateContent`
    )
    expect(code).toMatch(/-H "x-goog-api-key: \$\{?ADTYAI_API_KEY\}?"/)
    expect(code).not.toContain('?key=')
  })

  it.each(['cURL', 'Python', 'TypeScript'])(
    'copies rerank query and documents in the %s HTTP example',
    async (language) => {
      renderApi('jina-rerank', '/v1/rerank')
      const code = await copySample(language)
      expect(code).toContain(`${baseUrl}/v1/rerank`)
      expect(code).toContain('"query"')
      expect(code).toContain('"documents"')
      expect(code).not.toMatch(/embeddings\.create|"input"/)
      expect(code).toContain('ADTYAI_API_KEY')
    }
  )

  it('uses the configured gateway in Python Gemini requests', async () => {
    renderApi('gemini', '/v1beta/models/{model}:generateContent')
    const code = await copySample('Python')
    expect(code).toContain(
      `${baseUrl}/v1beta/models/test-model:generateContent`
    )
    expect(code).toContain('os.environ["ADTYAI_API_KEY"]')
    expect(code).toContain('x-goog-api-key')
  })

  it('does not fabricate a chat payload for an endpoint without a supported example', () => {
    renderApi('openai-video', '/v1/videos')
    expect(
      screen.queryByRole('button', { name: 'Copy code' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(/request example is not available for this endpoint/i)
    ).toBeVisible()
  })
})
