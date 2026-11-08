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
import {
  ChevronRight,
  Gauge,
  KeyRound,
  ScrollText,
  Sigma,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { BundledLanguage } from 'shiki/bundle/web'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import {
  StaticDataTable,
  staticDataTableClassNames as tableStyles,
} from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Empty, EmptyDescription, EmptyHeader } from '@/components/ui/empty'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useStatus } from '@/hooks/use-status'

import {
  buildSupportedParameters,
  type SupportedParameter,
} from '../lib/mock-stats'
import { replaceModelInPath } from '../lib/model-helpers'
import type { PricingModel } from '../types'

// ---------------------------------------------------------------------------
// Code-sample registry
// ---------------------------------------------------------------------------
//
// Each sample is keyed by language and endpoint type. The endpoint type comes
// from the model's `supported_endpoint_types`; we render samples only for the
// types the model actually supports. This keeps copy-pasted code accurate and
// provider-shaped (OpenAI vs Anthropic vs Gemini, etc.).

type Lang = 'curl' | 'python' | 'typescript' | 'javascript'

const LANG_LABELS: Record<Lang, string> = {
  curl: 'cURL',
  python: 'Python',
  typescript: 'TypeScript',
  javascript: 'JavaScript',
}

const LANG_HIGHLIGHT: Record<Lang, BundledLanguage> = {
  curl: 'bash',
  python: 'python',
  typescript: 'typescript',
  javascript: 'javascript',
}

type SampleContext = {
  baseUrl: string
  apiKeyEnv: string
  modelName: string
  endpointType: string
  endpointPath: string
  method: string
}

function buildSampleBody(ctx: SampleContext): Record<string, unknown> | null {
  const message = 'Explain quantum entanglement in one paragraph.'
  switch (ctx.endpointType) {
    case 'openai':
      return {
        model: ctx.modelName,
        messages: [{ role: 'user', content: message }],
      }
    case 'openai-response':
      return { model: ctx.modelName, input: message }
    case 'anthropic':
      return {
        model: ctx.modelName,
        max_tokens: 1024,
        messages: [{ role: 'user', content: message }],
      }
    case 'gemini':
      return { contents: [{ parts: [{ text: message }] }] }
    case 'embeddings':
      return {
        model: ctx.modelName,
        input: 'The food was delicious and the waiter…',
      }
    case 'jina-rerank':
      return {
        model: ctx.modelName,
        query: 'What is the capital of France?',
        documents: [
          'Paris is the capital of France.',
          'Berlin is the capital of Germany.',
        ],
      }
    case 'image-generation':
      return {
        model: ctx.modelName,
        prompt: 'A serene koi pond at sunset, ukiyo-e style.',
        size: '1024x1024',
        n: 1,
      }
    default:
      return null
  }
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`
}

function buildSample(lang: Lang, ctx: SampleContext): string | null {
  const body = buildSampleBody(ctx)
  if (!body) return null

  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const bodyJson = JSON.stringify(body, null, 2)
  let authHeader = 'Authorization'
  let authPrefix = 'Bearer '
  if (ctx.endpointType === 'anthropic') {
    authHeader = 'x-api-key'
    authPrefix = ''
  } else if (ctx.endpointType === 'gemini') {
    authHeader = 'x-goog-api-key'
    authPrefix = ''
  }
  const extraHeaders: Record<string, string> = {}
  if (ctx.endpointType === 'anthropic') {
    extraHeaders['anthropic-version'] = '2023-06-01'
  }

  if (lang === 'curl') {
    return [
      `curl --request ${shellQuote(ctx.method)} ${shellQuote(url)} \\`,
      '  -H "Content-Type: application/json" \\',
      `  -H "${authHeader}: ${authPrefix}$${ctx.apiKeyEnv}" \\`,
      ...Object.entries(extraHeaders).map(
        ([name, value]) => `  -H ${shellQuote(`${name}: ${value}`)} \\`
      ),
      `  --data-raw ${shellQuote(bodyJson)}`,
    ].join('\n')
  }

  if (lang === 'python') {
    const authValue = authPrefix
      ? `${JSON.stringify(authPrefix)} + api_key`
      : 'api_key'
    return [
      'import json',
      'import os',
      'from urllib.request import Request, urlopen',
      '',
      `api_key = os.environ["${ctx.apiKeyEnv}"]`,
      `payload = ${bodyJson}`,
      '',
      'request = Request(',
      `    ${JSON.stringify(url)},`,
      '    data=json.dumps(payload).encode("utf-8"),',
      '    headers={',
      '        "Content-Type": "application/json",',
      `        "${authHeader}": ${authValue},`,
      ...Object.entries(extraHeaders).map(
        ([name, value]) => `        "${name}": ${JSON.stringify(value)},`
      ),
      '    },',
      `    method=${JSON.stringify(ctx.method)},`,
      ')',
      'with urlopen(request) as response:',
      '    print(json.load(response))',
    ].join('\n')
  }

  const authValue = authPrefix ? `\`${authPrefix}\${apiKey}\`` : 'apiKey'
  return [
    `const apiKey = process.env.${ctx.apiKeyEnv}`,
    'if (!apiKey) throw new Error("Set ADTYAI_API_KEY to your API key")',
    '',
    `const response = await fetch(${JSON.stringify(url)}, {`,
    `  method: ${JSON.stringify(ctx.method)},`,
    '  headers: {',
    '    "Content-Type": "application/json",',
    `    "${authHeader}": ${authValue},`,
    ...Object.entries(extraHeaders).map(
      ([name, value]) => `    "${name}": ${JSON.stringify(value)},`
    ),
    '  },',
    `  body: JSON.stringify(${bodyJson}),`,
    '})',
    '',
    'if (!response.ok) {',
    '  throw new Error(`HTTP ${response.status}: ${await response.text()}`)',
    '}',
    'console.log(await response.json())',
  ].join('\n')
}

// ---------------------------------------------------------------------------
// Code samples section
// ---------------------------------------------------------------------------

function CodeSamplesSection(props: {
  model: PricingModel
  endpointMap: Record<string, { path?: string; method?: string }>
}) {
  const { t } = useTranslation()
  const { status } = useStatus()

  const baseUrl = useMemo(() => {
    const candidate =
      (status as Record<string, unknown> | null)?.server_address ??
      (status as Record<string, unknown> | null)?.serverAddress ??
      (status?.data as Record<string, unknown> | undefined)?.server_address ??
      (status?.data as Record<string, unknown> | undefined)?.serverAddress
    if (candidate && typeof candidate === 'string') {
      return candidate.replace(/\/$/, '')
    }
    if (typeof window !== 'undefined') return window.location.origin
    return 'https://api.example.com'
  }, [status])

  const endpoints = useMemo(() => {
    const types = props.model.supported_endpoint_types || []
    return types
      .map((type) => {
        const info = props.endpointMap[type] || {}
        let path = info.path || ''
        if (path && path.includes('{model}')) {
          path = replaceModelInPath(path, props.model.model_name || '')
        }
        return { type, path, method: info.method || 'POST' }
      })
      .filter((e) => Boolean(e.path))
  }, [props.model, props.endpointMap])

  const [endpointType, setEndpointType] = useState<string>(
    endpoints[0]?.type ?? ''
  )
  const [lang, setLang] = useState<Lang>('curl')

  const activeEndpoint = useMemo(() => {
    return endpoints.find((e) => e.type === endpointType) ?? endpoints[0]
  }, [endpointType, endpoints])

  if (endpoints.length === 0 || !activeEndpoint) {
    return null
  }

  const code = buildSample(lang, {
    baseUrl,
    apiKeyEnv: 'ADTYAI_API_KEY',
    modelName: props.model.model_name || '',
    endpointType: activeEndpoint.type,
    endpointPath: activeEndpoint.path,
    method: activeEndpoint.method,
  })

  return (
    <section>
      <SectionTitle icon={ScrollText}>{t('Code samples')}</SectionTitle>

      <div className='flex flex-wrap items-center gap-2'>
        {endpoints.length > 1 && (
          <Tabs value={endpointType} onValueChange={setEndpointType}>
            <TabsList className='bg-muted/40 h-8 p-0.5'>
              {endpoints.map((ep) => (
                <TabsTrigger
                  key={ep.type}
                  value={ep.type}
                  className='h-7 px-2.5 text-xs'
                >
                  {ep.type}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        )}

        <Tabs
          value={lang}
          onValueChange={(v) => setLang(v as Lang)}
          className='ml-auto'
        >
          <TabsList className='bg-muted/40 h-8 p-0.5'>
            {(Object.keys(LANG_LABELS) as Lang[]).map((l) => (
              <TabsTrigger key={l} value={l} className='h-7 px-2.5 text-xs'>
                {LANG_LABELS[l]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      <div className='mt-3'>
        {code ? (
          <CodeBlock code={code} language={LANG_HIGHLIGHT[lang]}>
            <CodeBlockCopyButton />
          </CodeBlock>
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyDescription>
                {t(
                  'A request example is not available for this endpoint. Refer to the endpoint documentation.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>

      {code && (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t(
            'Set the ADTYAI_API_KEY environment variable to the API key from your token settings.'
          )}
        </p>
      )}
    </section>
  )
}

// ---------------------------------------------------------------------------
// Supported parameters table
// ---------------------------------------------------------------------------

function SupportedParametersSection(props: { model: PricingModel }) {
  const { t } = useTranslation()
  const params = useMemo(
    () => buildSupportedParameters(props.model),
    [props.model]
  )

  if (params.length === 0) return null

  return (
    <section>
      <SectionTitle icon={Sigma}>{t('Supported parameters')}</SectionTitle>
      <StaticDataTable
        className={tableStyles.sectionContainer}
        headerRowClassName={tableStyles.mutedHeaderRow}
        data={params}
        getRowKey={(param) => param.name}
        getRowClassName={() => 'hover:bg-muted/20'}
        columns={[
          {
            id: 'parameter',
            header: t('Parameter'),
            className: 'h-9 w-44',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <div className='flex items-center gap-1.5'>
                <code className='font-mono text-sm font-medium'>{p.name}</code>
                {p.required && (
                  <Badge
                    variant='outline'
                    className='h-6 border-rose-500/40 px-2 text-sm text-rose-600 dark:text-rose-400'
                  >
                    {t('required')}
                  </Badge>
                )}
              </div>
            ),
          },
          {
            id: 'type',
            header: t('Type'),
            className: 'h-9 w-24',
            cellClassName: tableStyles.topCell,
            cell: (p) => (
              <Badge
                variant='secondary'
                className='h-7 rounded-full px-2.5 font-mono text-sm font-normal'
              >
                {p.type}
              </Badge>
            ),
          },
          {
            id: 'range',
            header: t('Default / range'),
            className: 'h-9 w-32',
            cellClassName: tableStyles.topCell,
            cell: (p) => <ParamRangeCell param={p} />,
          },
          {
            id: 'description',
            header: t('Description'),
            className: 'h-9',
            cellClassName: tableStyles.topMutedCell,
            cell: (p) => t(p.descriptionKey),
          },
        ]}
      />
    </section>
  )
}

function ParamRangeCell(props: { param: SupportedParameter }) {
  const { defaultValue, range, enumValues } = props.param
  if (defaultValue !== undefined) {
    return (
      <div className='flex flex-wrap items-center gap-1'>
        <span className='text-muted-foreground text-sm'>=</span>
        <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-sm'>
          {String(defaultValue)}
        </code>
        {range && (
          <span className='text-muted-foreground text-sm'>{range}</span>
        )}
      </div>
    )
  }
  if (range) {
    return (
      <span className='text-muted-foreground font-mono text-sm'>{range}</span>
    )
  }
  if (enumValues && enumValues.length > 0) {
    return (
      <div className='flex flex-wrap gap-0.5'>
        {enumValues.map((v) => (
          <code
            key={v}
            className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 font-mono text-sm'
          >
            {v}
          </code>
        ))}
      </div>
    )
  }
  return <span className='text-muted-foreground/60 text-sm'>—</span>
}

// ---------------------------------------------------------------------------
// Rate-limits table
// ---------------------------------------------------------------------------

function RateLimitsSection() {
  const { t } = useTranslation()
  return (
    <section>
      <SectionTitle icon={Gauge}>{t('Rate limits')}</SectionTitle>
      <Alert>
        <AlertDescription>
          <p>
            {t('Actual rate limits are not available in the model catalog.')}
          </p>
          <p>
            {t(
              'Check the limits for your account and API key, or ask your administrator.'
            )}
          </p>
        </AlertDescription>
      </Alert>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Authentication preview
// ---------------------------------------------------------------------------

function AuthSection() {
  const { t } = useTranslation()
  return (
    <section>
      <SectionTitle icon={KeyRound}>{t('Authentication')}</SectionTitle>
      <div className='border-border/60 bg-muted/20 flex items-start gap-2 rounded-lg border p-3'>
        <ChevronRight className='text-muted-foreground mt-0.5 size-3.5 shrink-0' />
        <div className='space-y-1.5 text-xs leading-relaxed'>
          <p>
            {t(
              'Use the authentication header shown in the request example for this endpoint.'
            )}
          </p>
          <p className='text-muted-foreground'>
            {t(
              'Generate tokens from the Tokens page; you can scope them to specific models, groups, IPs, and rate-limits.'
            )}
          </p>
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Composite API tab
// ---------------------------------------------------------------------------

export function ModelDetailsApi(props: {
  model: PricingModel
  endpointMap: Record<string, { path?: string; method?: string }>
}) {
  return (
    <div className='space-y-6'>
      <CodeSamplesSection model={props.model} endpointMap={props.endpointMap} />
      <AuthSection />
      <SupportedParametersSection model={props.model} />
      <RateLimitsSection />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Local UI helpers
// ---------------------------------------------------------------------------

function SectionTitle(props: {
  children: React.ReactNode
  icon: React.ComponentType<{ className?: string }>
}) {
  const Icon = props.icon
  return (
    <h3 className='text-foreground mb-3 flex items-center gap-1.5 text-sm font-semibold'>
      <Icon className='text-muted-foreground/70 size-3.5' />
      {props.children}
    </h3>
  )
}

// Re-export so the parent can keep its own SectionTitle if it wants:
export { Zap as ApiTabIcon }
