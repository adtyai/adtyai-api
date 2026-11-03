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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  buildClientSnippet,
  CLIENT_SNIPPET_LABELS,
  DEFAULT_SNIPPET_MODEL,
  type ClientSnippetId,
} from '@/features/adty-site/client-snippets'

const LANDING_CLIENTS: ClientSnippetId[] = [
  'claude-code',
  'codex',
  'openai-sdk',
  'cherry-studio',
]

/**
 * Dark code card of the hero: the same snippets as the console quick-start
 * card. Plain <pre> instead of the highlighted CodeBlock, which highlights in
 * an effect and would leave the build-time render empty.
 */
export function CodeCard(props: { baseUrl: string }) {
  const { t } = useTranslation()
  const [client, setClient] = useState<ClientSnippetId>('claude-code')
  const snippet = buildClientSnippet(
    client,
    props.baseUrl,
    DEFAULT_SNIPPET_MODEL,
    {
      provider: t('Provider type'),
      address: t('API address'),
      key: t('API Key'),
      model: t('Model'),
    }
  )

  return (
    <div className='dark bg-background text-foreground overflow-hidden rounded-xl border shadow-lg'>
      <Tabs
        value={client}
        onValueChange={(value) => setClient(value as ClientSnippetId)}
      >
        <div className='flex items-center justify-between gap-2 border-b px-3 py-2'>
          <TabsList className='h-8 max-w-full overflow-x-auto p-0.5'>
            {LANDING_CLIENTS.map((id) => (
              <TabsTrigger key={id} value={id} className='h-7 px-2.5 text-xs'>
                {CLIENT_SNIPPET_LABELS[id]}
              </TabsTrigger>
            ))}
          </TabsList>
          <CopyButton
            value={snippet}
            size='sm'
            variant='ghost'
            aria-label={t('Copy code')}
          />
        </div>
      </Tabs>
      <pre
        data-testid='landing-code'
        className='overflow-x-auto p-4 font-mono text-xs leading-relaxed'
      >
        <code>{snippet}</code>
      </pre>
    </div>
  )
}
