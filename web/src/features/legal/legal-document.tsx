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
import { useQuery } from '@tanstack/react-query'
import { FileWarning } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toContentLang } from '@/lib/adty-lang'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'

import { getLegalDocument, type LegalDoc } from './api'

type LegalDocumentProps = {
  title: string
  doc: LegalDoc
  emptyMessage: string
  /** adty: the site turned this document off; show the empty message without fetching. */
  hidden?: boolean
  /** adty: still deciding whether the document is shown. */
  pending?: boolean
}

export function LegalDocument({
  title,
  doc,
  emptyMessage,
  hidden = false,
  pending = false,
}: LegalDocumentProps) {
  const { t, i18n } = useTranslation()
  // adty: the text depends on the language, so a language switch refetches.
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const { data, isLoading } = useQuery({
    queryKey: ['legal', doc, lang],
    queryFn: () => getLegalDocument(doc, lang),
    staleTime: 10 * 60 * 1000,
    enabled: !hidden && !pending,
  })

  const rawContent = hidden ? '' : (data?.data?.trim() ?? '')
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)
  const success = !hidden && (data?.success ?? false)

  if (isLoading || pending) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  if (!success || !hasContent) {
    return (
      <PublicLayout>
        <div className='mx-auto max-w-2xl py-12'>
          <Card className='border-dashed'>
            <CardHeader className='flex flex-row items-center gap-4'>
              <div className='bg-muted rounded-lg p-2'>
                <FileWarning className='text-muted-foreground h-5 w-5' />
              </div>
              <div className='space-y-1'>
                <CardTitle className='text-lg font-semibold'>{title}</CardTitle>
                <p className='text-muted-foreground text-sm'>
                  {(!hidden && data?.message) || emptyMessage}
                </p>
              </div>
            </CardHeader>
          </Card>
        </div>
      </PublicLayout>
    )
  }

  if (isUrl) {
    return (
      <PublicLayout>
        <div className='mx-auto max-w-2xl py-12'>
          <Card>
            <CardHeader>
              <CardTitle>{title}</CardTitle>
            </CardHeader>
            <CardContent className='space-y-4'>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'The administrator configured an external link for this document.'
                )}
              </p>
              <Button
                render={
                  <a
                    href={rawContent}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
              >
                {t('View document')}
              </Button>
            </CardContent>
          </Card>
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={!contentIsHtml}>
      {contentIsHtml ? (
        <RichContent mode='html' htmlVariant='isolated' content={rawContent} />
      ) : (
        <div className='mx-auto max-w-4xl space-y-6 py-12'>
          <div className='space-y-2'>
            <h1 className='text-3xl font-semibold tracking-tight'>{title}</h1>
          </div>

          <RichContent
            mode='markdown'
            content={rawContent}
            className='prose-neutral dark:prose-invert max-w-none'
          />
        </div>
      )}
    </PublicLayout>
  )
}
