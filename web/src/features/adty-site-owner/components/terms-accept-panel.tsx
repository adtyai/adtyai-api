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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { getLegalDocument } from '@/features/legal/api'
import { toContentLang } from '@/lib/adty-lang'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { acceptSiteTerms } from '../api'

/** Blocks the owner center until the current site agreement is accepted. */
export function TermsAcceptPanel(props: { version: string }) {
  const { t, i18n } = useTranslation()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const document = useQuery({
    queryKey: ['legal', 'site-agreement', lang],
    queryFn: () => getLegalDocument('site-agreement', lang),
    staleTime: 10 * 60 * 1000,
  })
  const accept = useMutation({
    mutationFn: () => acceptSiteTerms(props.version),
    onSuccess: () => {
      const user = useAuthStore.getState().auth.user
      if (user) {
        useAuthStore.getState().auth.setUser({
          ...user,
          site_terms_required: false,
        })
      }
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  const text = document.data?.data?.trim() ?? ''

  return (
    <div className='flex max-w-3xl flex-col gap-4'>
      <p className='text-sm'>
        {t('Accept the current site agreement first')}
      </p>
      {document.isLoading && <Skeleton className='h-40 w-full' />}
      {text !== '' && <RichContent content={text} />}
      <Button
        disabled={props.version.trim() === '' || accept.isPending}
        onClick={() => accept.mutate()}
      >
        {t('Accept site agreement')}
      </Button>
    </div>
  )
}
