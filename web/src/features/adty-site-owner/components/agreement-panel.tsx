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
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { RichContent } from '@/components/rich-content'
import { Skeleton } from '@/components/ui/skeleton'
import { getLegalDocument } from '@/features/legal/api'
import { toContentLang } from '@/lib/adty-lang'
import { formatTimestamp } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { OwnerSite } from '../types'

/** The site agreement the owner accepted, read-only (task 11.1). */
export function AgreementPanel(props: { site: OwnerSite }) {
  const { t, i18n } = useTranslation()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const query = useQuery({
    queryKey: ['legal', 'site-agreement', lang],
    queryFn: async () =>
      requireServerSuccess(await getLegalDocument('site-agreement', lang)),
    staleTime: 10 * 60 * 1000,
  })
  const text = query.data?.data?.trim() ?? ''

  return (
    <div className='flex flex-col gap-3'>
      <p className='text-muted-foreground text-sm'>
        {props.site.agreement_at > 0
          ? t('You accepted version {{version}} on {{time}}.', {
              version: props.site.agreement_version,
              time: formatTimestamp(props.site.agreement_at),
            })
          : t('This site was opened by the platform.')}
      </p>
      {query.isLoading && <Skeleton className='h-40 w-full' />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.isSuccess && !text && (
        <EmptyState
          className='min-h-32'
          title={t('The site agreement has not been published yet')}
        />
      )}
      {text && <RichContent content={text} />}
    </div>
  )
}
