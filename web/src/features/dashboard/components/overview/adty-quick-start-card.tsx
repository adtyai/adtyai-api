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
import { Link } from '@tanstack/react-router'
import { ArrowRightLeft, KeyRound } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useApiBaseUrl } from '@/features/adty-site/api'
import {
  API_KEY_PLACEHOLDER,
  buildClientSnippet,
  CLIENT_SNIPPET_LABELS,
  CLIENT_SNIPPET_LANGUAGES,
  DEFAULT_SNIPPET_MODEL,
  type ClientSnippetId,
} from '@/features/adty-site/client-snippets'
import { fetchTokenKey } from '@/features/keys/api'
import { CCSwitchDialog } from '@/features/keys/components/dialogs/cc-switch-dialog'
import type { ApiKey } from '@/features/keys/types'
import { handleServerError } from '@/lib/handle-server-error'

/**
 * Always-visible connection card on the overview (task 8.7): the public Base
 * URL, per-client config snippets and the CC Switch import.
 */
export function AdtyQuickStartCard(props: {
  apiKey: ApiKey | null
  model?: string
}) {
  const { t } = useTranslation()
  const baseUrl = useApiBaseUrl()
  const [client, setClient] = useState<ClientSnippetId>('claude-code')
  const [ccSwitchKey, setCCSwitchKey] = useState('')
  const [ccSwitchOpen, setCCSwitchOpen] = useState(false)
  const [isLoadingKey, setIsLoadingKey] = useState(false)

  const snippet = buildClientSnippet(
    client,
    baseUrl,
    props.model || DEFAULT_SNIPPET_MODEL,
    {
      provider: t('Provider type'),
      address: t('API address'),
      key: t('API Key'),
      model: t('Model'),
    }
  )

  const handleOpenCCSwitch = async () => {
    if (!props.apiKey || isLoadingKey) return
    setIsLoadingKey(true)
    try {
      const result = await fetchTokenKey(props.apiKey.id)
      const key = result.success && result.data?.key ? result.data.key : ''
      if (!key) {
        handleServerError(result, t('Failed to load the API key'))
        return
      }
      setCCSwitchKey(key)
      setCCSwitchOpen(true)
    } catch (error) {
      handleServerError(error, t('Failed to load the API key'))
    } finally {
      setIsLoadingKey(false)
    }
  }

  return (
    <section
      aria-labelledby='adty-quick-start-title'
      className='bg-card flex flex-col gap-4 rounded-2xl border p-4 shadow-xs sm:p-5'
    >
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3
          id='adty-quick-start-title'
          className='text-sm font-semibold sm:text-base'
        >
          {t('Quick start')}
        </h3>
        <div className='flex flex-wrap items-center gap-2'>
          {props.apiKey && (
            <Button
              variant='outline'
              size='sm'
              disabled={isLoadingKey}
              onClick={handleOpenCCSwitch}
            >
              <ArrowRightLeft data-icon='inline-start' />
              {t('Import to CC Switch')}
            </Button>
          )}
          <Button
            size='sm'
            variant={props.apiKey ? 'outline' : 'default'}
            render={<Link to='/keys' />}
          >
            <KeyRound data-icon='inline-start' />
            {t('Create API Key')}
          </Button>
        </div>
      </div>

      <div className='flex flex-col gap-1.5'>
        <div className='bg-muted/50 flex items-center gap-3 rounded-lg border px-3 py-2'>
          <span className='text-muted-foreground shrink-0 text-xs font-medium'>
            {t('Base URL')}
          </span>
          <code
            data-testid='adty-base-url'
            className='min-w-0 flex-1 truncate font-mono text-sm'
          >
            {baseUrl}
          </code>
          <CopyButton
            value={baseUrl}
            size='sm'
            variant='ghost'
            aria-label={t('Copy Base URL')}
          />
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('Compatible protocols')}: OpenAI /v1 · Anthropic /v1/messages ·
          Gemini /v1beta
        </p>
      </div>

      <div className='flex flex-col gap-2'>
        <Tabs
          value={client}
          onValueChange={(value) => setClient(value as ClientSnippetId)}
        >
          <TabsList className='bg-muted/40 h-8 max-w-full overflow-x-auto p-0.5'>
            {(Object.keys(CLIENT_SNIPPET_LABELS) as ClientSnippetId[]).map(
              (key) => (
                <TabsTrigger
                  key={key}
                  value={key}
                  className='h-7 px-2.5 text-xs'
                >
                  {CLIENT_SNIPPET_LABELS[key]}
                </TabsTrigger>
              )
            )}
          </TabsList>
        </Tabs>
        <CodeBlock
          code={snippet}
          language={CLIENT_SNIPPET_LANGUAGES[client]}
          title={CLIENT_SNIPPET_LABELS[client]}
          enableCollapse={false}
        >
          <CodeBlockCopyButton />
        </CodeBlock>
        <p className='text-muted-foreground text-xs'>
          {t('Replace')}{' '}
          <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
            {API_KEY_PLACEHOLDER}
          </code>{' '}
          {t('with the API key from your token settings.')}
        </p>
      </div>

      {props.apiKey && (
        <CCSwitchDialog
          open={ccSwitchOpen}
          onOpenChange={(open) => {
            setCCSwitchOpen(open)
            // Do not keep the real key around after the dialog closes.
            if (!open) setCCSwitchKey('')
          }}
          tokenKey={ccSwitchKey}
          endpoint={baseUrl}
        />
      )}
    </section>
  )
}
