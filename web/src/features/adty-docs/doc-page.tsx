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
import { FileQuestion } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { useApiBaseUrl } from '@/features/adty-site/api'
import { toContentLang } from '@/lib/adty-lang'
import { cn } from '@/lib/utils'

import {
  DEFAULT_DOC_SLUG,
  DOC_PAGES,
  fillDocPlaceholders,
  getDocMarkdown,
  isDocSlug,
  type DocSlug,
} from './content'

/** One integration guide with the doc navigation beside it. */
export function DocPage(props: { slug: string }) {
  const { t, i18n } = useTranslation()
  const baseUrl = useApiBaseUrl()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  // Client and SDK names are product names and stay untranslated.
  const titles: Record<DocSlug, string> = {
    quickstart: t('Quick start'),
    models: t('Models and groups'),
    roles: t('Role guides'),
    'claude-code': 'Claude Code',
    'codex-cli': 'Codex CLI',
    'cherry-studio': 'Cherry Studio',
    'openai-sdk': 'OpenAI SDK',
    agents: t('Agents'),
    'errors-and-limits': t('Errors and limits'),
    faq: t('FAQ'),
  }
  const slug = props.slug
  const known = isDocSlug(slug)
  const markdown = isDocSlug(slug)
    ? fillDocPlaceholders(getDocMarkdown(slug, lang), baseUrl)
    : ''

  return (
    <PublicLayout>
      <div className='mx-auto flex w-full max-w-[1180px] flex-col gap-10 py-2 md:flex-row md:gap-14 md:py-6'>
        <nav
          aria-label={t('Documentation')}
          className='md:w-[180px] md:shrink-0'
        >
          <ul className='flex gap-1 overflow-x-auto md:sticky md:top-[104px] md:flex-col md:gap-0.5 md:overflow-visible'>
            {DOC_PAGES.map((page) => {
              const active = page === props.slug
              return (
                <li key={page} className='shrink-0'>
                  <Link
                    to='/docs/$slug'
                    params={{ slug: page }}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'text-muted-foreground block rounded-md px-3 py-1.5 text-[13px] whitespace-nowrap transition-colors',
                      'md:rounded-none md:border-l-2 md:px-0 md:py-2 md:pl-3.5',
                      active
                        ? 'bg-muted text-foreground font-medium md:border-l-brand md:bg-transparent'
                        : 'hover:text-foreground md:hover:border-border'
                    )}
                  >
                    {titles[page]}
                  </Link>
                </li>
              )
            })}
          </ul>
        </nav>
        <article className='max-w-[680px] min-w-0 flex-1'>
          {known ? (
            <RichContent
              mode='markdown'
              content={markdown}
              className='prose-neutral dark:prose-invert max-w-none [&_li]:leading-[1.8] [&_p]:leading-[1.8]'
            />
          ) : (
            <EmptyState
              icon={FileQuestion}
              title={t('Page not found')}
              description={t('This document does not exist.')}
              action={
                <Button
                  render={
                    <Link
                      to='/docs/$slug'
                      params={{ slug: DEFAULT_DOC_SLUG }}
                    />
                  }
                >
                  {t('Quick start')}
                </Button>
              }
            />
          )}
        </article>
      </div>
    </PublicLayout>
  )
}
