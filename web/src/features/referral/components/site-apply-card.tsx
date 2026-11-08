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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getLegalDocument } from '@/features/legal/api'
import { toContentLang } from '@/lib/adty-lang'
import { api } from '@/lib/api'
import { isHttpUrl } from '@/lib/content-format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

type SiteConfig = {
  success?: boolean
  data?: { site_agreement_version?: number }
}

/** Apply to open a reseller site: POST /api/adty/site/apply. */
export function SiteApplyCard() {
  const { t, i18n } = useTranslation()
  const lang = toContentLang(i18n.resolvedLanguage || i18n.language)
  const config = useQuery({
    queryKey: ['adty-site-public'],
    queryFn: async () => {
      const res = await api.get<SiteConfig>('/api/adty/site')
      return requireServerSuccess(res.data)
    },
  })
  const document = useQuery({
    queryKey: ['legal', 'site-agreement', lang],
    queryFn: async () =>
      requireServerSuccess(await getLegalDocument('site-agreement', lang)),
    staleTime: 10 * 60 * 1000,
  })
  const text = document.data?.data?.trim() ?? ''
  const version = config.data?.data?.site_agreement_version
  const agreementReady =
    config.isSuccess &&
    document.isSuccess &&
    text !== '' &&
    typeof version === 'number' &&
    Number.isSafeInteger(version) &&
    version > 0
  const [subdomain, setSubdomain] = useState('')
  const [contact, setContact] = useState('')
  const [invite, setInvite] = useState('')
  const [agreed, setAgreed] = useState(false)
  const apply = useMutation({
    mutationFn: async () => {
      if (!agreementReady || !agreed) {
        throw new Error(t('Accept the current site agreement first'))
      }
      const res = await api.post<{ success: boolean; message?: string }>(
        '/api/adty/site/apply',
        {
          subdomain: subdomain.trim(),
          contact_info: contact.trim(),
          invite_code: invite.trim(),
          agree_version: String(version),
        }
      )
      return requireServerSuccess(res.data)
    },
    onSuccess: () => {
      toast.success(
        invite.trim()
          ? t(
              'Site application sent. An invite code opens the site immediately when it is valid.'
            )
          : t('Site application sent. The platform will review it.')
      )
      setSubdomain('')
      setContact('')
      setInvite('')
      setAgreed(false)
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Apply for a reseller site')}</CardTitle>
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <Label htmlFor='site-subdomain'>{t('Subdomain')}</Label>
        <Input
          id='site-subdomain'
          value={subdomain}
          onChange={(event) => setSubdomain(event.target.value)}
        />
        <Label htmlFor='site-contact'>{t('Contact')}</Label>
        <Input
          id='site-contact'
          value={contact}
          onChange={(event) => setContact(event.target.value)}
        />
        <Label htmlFor='site-invite'>{t('Invite code')}</Label>
        <Input
          id='site-invite'
          value={invite}
          onChange={(event) => setInvite(event.target.value)}
        />
        <section aria-label={t('Site agreement')} className='space-y-3'>
          {(config.isLoading || document.isLoading) && <LoadingState />}
          {(config.isError || document.isError) && (
            <ErrorState
              onRetry={() => {
                void config.refetch()
                void document.refetch()
              }}
            />
          )}
          {document.isSuccess && text === '' && (
            <p>{t('The site agreement has not been published yet')}</p>
          )}
          {text !== '' &&
            (isHttpUrl(text) ? (
              <Button
                variant='outline'
                render={
                  <a href={text} target='_blank' rel='noopener noreferrer' />
                }
              >
                {t('View document')}
              </Button>
            ) : (
              <RichContent content={text} />
            ))}
        </section>
        <div className='flex items-center gap-2 text-sm'>
          <Checkbox
            id='site-apply-agreed'
            checked={agreed}
            onCheckedChange={setAgreed}
          />
          <Label htmlFor='site-apply-agreed'>
            {t('I agree to the reseller site agreement')}
          </Label>
        </div>
        <Button
          disabled={
            !agreementReady ||
            !agreed ||
            subdomain.trim() === '' ||
            apply.isPending
          }
          onClick={() => apply.mutate()}
        >
          {t('Apply for a reseller site')}
        </Button>
      </CardContent>
    </Card>
  )
}
