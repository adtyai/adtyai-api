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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { getLegalDocument } from '@/features/legal/api'
import { api } from '@/lib/api'
import { toContentLang } from '@/lib/adty-lang'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

/** Replaces the console until a site customer accepts the current terms. */
export function CustomerTermsGate(props: { children: ReactNode }) {
  const user = useAuthStore((state) => state.auth.user)
  if (!user?.site_terms_required || user.adty_site_owner) {
    return props.children
  }
  return <CustomerTermsPanel version={user.site_terms_version ?? ''} />
}

function CustomerTermsPanel(props: { version: string }) {
  const { t, i18n } = useTranslation()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const document = useQuery({
    queryKey: ['legal', 'site-customer-terms', lang],
    queryFn: () => getLegalDocument('site-customer-terms', lang),
    staleTime: 10 * 60 * 1000,
  })
  const accept = useMutation({
    mutationFn: async () => {
      const res = await api.post('/api/adty/site/terms/accept', {
        version: props.version,
      })
      requireServerSuccess(res.data)
    },
    onSuccess: () => {
      const current = useAuthStore.getState().auth.user
      if (current) {
        useAuthStore.getState().auth.setUser({
          ...current,
          site_terms_required: false,
        })
      }
      toast.success(t('Saved successfully'))
    },
    onError: (err) => handleServerError(err),
  })
  const text = document.data?.data?.trim() ?? ''

  return (
    <div className='mx-auto flex w-full max-w-3xl flex-col gap-4 p-4'>
      <p className='text-sm'>{t('Accept the current site customer terms first')}</p>
      {document.isLoading && <Skeleton className='h-40 w-full' />}
      {text !== '' && <RichContent content={text} />}
      <Button
        disabled={props.version.trim() === '' || accept.isPending}
        onClick={() => accept.mutate()}
      >
        {t('I agree to the site customer terms')}
      </Button>
    </div>
  )
}
