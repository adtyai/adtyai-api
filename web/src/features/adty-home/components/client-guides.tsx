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
import { useTranslation } from 'react-i18next'

import { LandingSection } from './landing-section'

/**
 * Setup facts already documented under /docs. Product names stay untranslated.
 * Nothing here may touch window, document or localStorage.
 */
export function ClientGuides() {
  const { t } = useTranslation()
  const guides = [
    {
      name: 'Claude Code',
      href: '/docs/claude-code',
      body: t(
        'Set ANTHROPIC_BASE_URL to this site, without /v1, and use your key.'
      ),
    },
    {
      name: 'Codex CLI',
      href: '/docs/codex-cli',
      body: t('Add this site as a provider. The base URL ends with /v1.'),
    },
    {
      name: 'Cherry Studio',
      href: '/docs/cherry-studio',
      body: t('Add an OpenAI provider, then fill in the API host and your key.'),
    },
    {
      name: 'OpenAI SDK',
      href: '/docs/openai-sdk',
      body: t(
        'Set base_url to this site with /v1. The model can be Claude, GPT or Gemini.'
      ),
    },
  ]
  const more = [
    { name: t('Quick start'), href: '/docs/quickstart' },
    { name: t('Errors and limits'), href: '/docs/errors-and-limits' },
  ]

  return (
    <LandingSection
      id='setup-guides'
      title={t('Setup guides')}
      description={t(
        'Claude Code, Codex CLI and the OpenAI SDK work as they are. Set the base URL and the key.'
      )}
    >
      <ul className='grid gap-4 sm:grid-cols-2'>
        {guides.map((guide) => (
          <li key={guide.href} className='bg-card rounded-xl border p-5'>
            <h3 className='mb-1.5 text-sm font-semibold'>
              <a href={guide.href} className='hover:underline'>
                {guide.name}
              </a>
            </h3>
            <p className='text-muted-foreground text-sm'>{guide.body}</p>
          </li>
        ))}
      </ul>
      <p className='text-muted-foreground mt-4 flex flex-wrap gap-x-4 gap-y-2 text-sm'>
        {more.map((item) => (
          <a key={item.href} href={item.href} className='hover:underline'>
            {item.name}
          </a>
        ))}
      </p>
    </LandingSection>
  )
}
