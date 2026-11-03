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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  addDomain,
  addSenderDomain,
  getSenderDomain,
  removeDomain,
  removeSenderDomain,
  saveOwnerSite,
  siteOwnerKeys,
  uploadLogo,
} from '../api'
import { useVerifiedOwnerMutation } from '../hooks/use-verified-owner-mutation'
import { parseSenderRecords } from '../lib/site-owner'
import type { OwnerSite, SenderDomain, SiteBrand, SiteDomain } from '../types'

const BRAND_LIMITS: Record<Exclude<keyof SiteBrand, 'logo_url'>, number> = {
  name: 40,
  tagline: 80,
  notice: 500,
  support_contact: 200,
}

function domainVariant(status: SiteDomain['status']) {
  if (status === 'active') return 'success' as const
  if (status === 'failed') return 'danger' as const
  return 'warning' as const
}

function domainLabel(t: TFunction, status: SiteDomain['status']): string {
  if (status === 'active') return t('Active')
  if (status === 'failed') return t('Failed')
  if (status === 'removed') return t('Removed')
  return t('Waiting for DNS')
}

function BrandCard(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [brand, setBrand] = useState<SiteBrand>(props.site.brand)
  useEffect(() => setBrand(props.site.brand), [props.site.brand])
  const upload = useMutation({
    mutationFn: (file: File) => uploadLogo(file),
    onSuccess: (url) => setBrand((prev) => ({ ...prev, logo_url: url })),
    onError: (err) => handleServerError(err),
  })
  const save = useMutation({
    mutationFn: () => saveOwnerSite({ brand }),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const field = (
    key: keyof typeof BRAND_LIMITS,
    label: string,
    multiline = false
  ) => {
    const id = `brand-${key}`
    const inputProps = {
      id,
      maxLength: BRAND_LIMITS[key],
      value: brand[key],
      onChange: (event: { target: { value: string } }) =>
        setBrand((prev) => ({ ...prev, [key]: event.target.value })),
    }
    return (
      <div className='space-y-2'>
        <Label htmlFor={id}>{label}</Label>
        {multiline ? <Textarea {...inputProps} /> : <Input {...inputProps} />}
        <p className='text-muted-foreground text-xs'>
          {t('{{count}} / {{max}} characters', {
            count: [...brand[key]].length,
            max: BRAND_LIMITS[key],
          })}
        </p>
      </div>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Brand')}</CardTitle>
      </CardHeader>
      <CardContent className='grid gap-3'>
        {field('name', t('Site name'))}
        {field('tagline', t('Tagline'))}
        {field('notice', t('Notice'), true)}
        {field('support_contact', t('Support contact'))}
        <div className='space-y-2'>
          <Label htmlFor='brand-logo'>{t('Logo')}</Label>
          <Input
            id='brand-logo'
            type='file'
            accept='image/png,image/jpeg,image/svg+xml'
            disabled={upload.isPending}
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) upload.mutate(file)
            }}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'PNG, JPEG or SVG without scripts, up to 200 KB. Save the brand to use it.'
            )}
          </p>
          {brand.logo_url && (
            <div className='flex items-center gap-3'>
              <img
                src={brand.logo_url}
                alt={t('Logo')}
                className='h-12 w-12 rounded border object-contain'
              />
              <Button
                size='sm'
                variant='outline'
                onClick={() => setBrand((prev) => ({ ...prev, logo_url: '' }))}
              >
                {t('Remove logo')}
              </Button>
            </div>
          )}
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('Brand text is shown as plain text.')}
        </p>
        <div>
          <Button disabled={save.isPending} onClick={() => save.mutate()}>
            {t('Save brand')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function DomainsCard(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [hostname, setHostname] = useState('')
  const [deleting, setDeleting] = useState<SiteDomain | null>(null)
  const add = useMutation({
    mutationFn: () => addDomain(hostname.trim()),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setHostname('')
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const remove = useVerifiedOwnerMutation<number>({
    action: 'domain.delete',
    target: (id) => id,
    run: (id, proof) => removeDomain(id, proof),
    successMessage: t('Deleted successfully'),
    onSuccess: () => setDeleting(null),
  })
  const domains = props.site.domains ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Custom domains')}</CardTitle>
      </CardHeader>
      <CardContent className='grid gap-3'>
        <p className='text-sm'>
          {t('Platform address')}:{' '}
          <span className='font-mono'>{props.site.host}</span>{' '}
          <CopyButton value={props.site.base_url} />
        </p>
        {!props.site.custom_domain_enabled ? (
          <p className='text-muted-foreground text-sm'>
            {t('Custom domains are not available yet.')}
          </p>
        ) : (
          <>
            {domains.map((domain) => (
              <div key={domain.id} className='space-y-1 rounded-md border p-3'>
                <div className='flex items-center justify-between gap-2'>
                  <span className='font-mono text-sm'>{domain.hostname}</span>
                  <div className='flex items-center gap-2'>
                    <StatusBadge
                      variant={domainVariant(domain.status)}
                      label={domainLabel(t, domain.status)}
                    />
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setDeleting(domain)}
                    >
                      {t('Delete')}
                    </Button>
                  </div>
                </div>
                {domain.status !== 'active' && domain.verify_records && (
                  <pre className='bg-muted overflow-x-auto rounded p-2 text-xs'>
                    {domain.verify_records}
                  </pre>
                )}
                {domain.last_error && (
                  <p className='text-destructive text-xs'>
                    {domain.last_error}
                  </p>
                )}
              </div>
            ))}
            <div className='flex items-end gap-2'>
              <div className='space-y-2'>
                <Label htmlFor='new-domain'>{t('Add a domain')}</Label>
                <Input
                  id='new-domain'
                  className='w-72'
                  placeholder='api.example.com'
                  value={hostname}
                  onChange={(event) => setHostname(event.target.value)}
                />
              </div>
              <Button
                disabled={!hostname.trim() || add.isPending}
                onClick={() => add.mutate()}
              >
                {t('Add')}
              </Button>
            </div>
          </>
        )}
        <ConfirmDialog
          open={deleting !== null}
          onOpenChange={(open) => !open && setDeleting(null)}
          title={t('Delete domain')}
          desc={t('Customers can no longer reach your site on {{host}}.', {
            host: deleting?.hostname ?? '',
          })}
          destructive
          isLoading={remove.isPending}
          handleConfirm={() => deleting && remove.mutate(deleting.id)}
        />
      </CardContent>
    </Card>
  )
}

function SearchCard(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const seo = useMutation({
    mutationFn: (value: boolean) => saveOwnerSite({ seo_index: value }),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all }),
    onError: (err) => handleServerError(err),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Search engines')}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className='flex items-center justify-between gap-4'>
          <Label htmlFor='seo-index'>
            {t('Let search engines index my site')}
          </Label>
          <Switch
            id='seo-index'
            checked={props.site.seo_index}
            disabled={seo.isPending}
            onCheckedChange={(checked) => seo.mutate(checked)}
          />
        </div>
      </CardContent>
    </Card>
  )
}

function senderLabel(t: TFunction, status: SenderDomain['status']): string {
  if (status === 'verified') return t('Verified')
  if (status === 'failed') return t('Verification failed')
  if (status === 'disabled') return t('Turned off')
  return t('Waiting for DNS')
}

function senderVariant(status: SenderDomain['status']) {
  if (status === 'verified') return 'success' as const
  if (status === 'failed') return 'danger' as const
  if (status === 'disabled') return 'neutral' as const
  return 'warning' as const
}

/** Sender domain (task 11.3): DNS records to add and the verification status. */
function SenderDomainCard(props: { site: OwnerSite }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [domain, setDomain] = useState('')
  const [deleting, setDeleting] = useState(false)
  const query = useQuery({
    queryKey: siteOwnerKeys.list('sender-domain', {}),
    queryFn: getSenderDomain,
    enabled: props.site.sender_domain_enabled,
  })
  const add = useMutation({
    mutationFn: () => addSenderDomain(domain.trim()),
    onSuccess: () => {
      toast.success(t('Saved successfully'))
      setDomain('')
      void queryClient.invalidateQueries({ queryKey: siteOwnerKeys.all })
    },
    onError: (err) => handleServerError(err),
  })
  const remove = useVerifiedOwnerMutation<void>({
    action: 'sender.delete',
    target: () => props.site.id,
    run: (_vars, proof) => removeSenderDomain(proof),
    successMessage: t('Deleted successfully'),
    onSuccess: () => setDeleting(false),
  })
  const current =
    query.data && query.data.status !== 'disabled' ? query.data : null
  const records = parseSenderRecords(current?.records)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Sender domain')}</CardTitle>
      </CardHeader>
      <CardContent className='grid gap-3'>
        {!props.site.sender_domain_enabled ? (
          <p className='text-muted-foreground text-sm'>
            {t('Not available yet')}
          </p>
        ) : (
          <>
            <p className='text-muted-foreground text-sm'>
              {t(
                'System emails to your customers are sent from your own domain once it is verified. Until then they come from the platform address with your site name.'
              )}
            </p>
            {query.data?.status === 'disabled' &&
              query.data.disabled_by > 0 && (
                <p className='text-muted-foreground text-sm'>
                  {t('The platform turned off your sender domain.')}
                </p>
              )}
            {current ? (
              <div className='space-y-2 rounded-md border p-3'>
                <div className='flex items-center justify-between gap-2'>
                  <span className='font-mono text-sm'>{current.domain}</span>
                  <div className='flex items-center gap-2'>
                    <StatusBadge
                      variant={senderVariant(current.status)}
                      label={senderLabel(t, current.status)}
                    />
                    <Button
                      size='sm'
                      variant='outline'
                      onClick={() => setDeleting(true)}
                    >
                      {t('Delete')}
                    </Button>
                  </div>
                </div>
                {current.status !== 'verified' && records.length > 0 && (
                  <StaticDataTable
                    tableProps={{ 'aria-label': t('DNS records to add') }}
                    data={records}
                    getRowKey={(row) => `${row.type}-${row.name}-${row.value}`}
                    columns={[
                      {
                        id: 'type',
                        header: t('Type'),
                        cell: (row) => row.type,
                      },
                      {
                        id: 'name',
                        header: t('Name'),
                        cell: (row) => (
                          <span className='font-mono text-xs'>{row.name}</span>
                        ),
                      },
                      {
                        id: 'value',
                        header: t('Value'),
                        cell: (row) => (
                          <span className='flex items-center gap-1'>
                            <span className='max-w-64 truncate font-mono text-xs'>
                              {row.value}
                            </span>
                            <CopyButton value={row.value} />
                          </span>
                        ),
                      },
                      {
                        id: 'priority',
                        header: t('Priority'),
                        cell: (row) => row.priority ?? '—',
                      },
                    ]}
                  />
                )}
              </div>
            ) : (
              <div className='flex items-end gap-2'>
                <div className='space-y-2'>
                  <Label htmlFor='sender-domain'>{t('Domain')}</Label>
                  <Input
                    id='sender-domain'
                    className='w-72'
                    placeholder='mail.example.com'
                    value={domain}
                    onChange={(event) => setDomain(event.target.value)}
                  />
                </div>
                <Button
                  disabled={!domain.trim() || add.isPending}
                  onClick={() => add.mutate()}
                >
                  {t('Add')}
                </Button>
              </div>
            )}
          </>
        )}
        <ConfirmDialog
          open={deleting}
          onOpenChange={setDeleting}
          title={t('Delete sender domain')}
          desc={t(
            'Emails go back to the platform address with your site name.'
          )}
          destructive
          isLoading={remove.isPending}
          handleConfirm={() => remove.mutate()}
        />
      </CardContent>
    </Card>
  )
}

/** Site settings: brand, custom domains, sender domain and SEO (task 11.6). */
export function SiteSettingsPanel(props: { site: OwnerSite }) {
  return (
    <div className='grid gap-4'>
      <BrandCard site={props.site} />
      <DomainsCard site={props.site} />
      <SenderDomainCard site={props.site} />
      <SearchCard site={props.site} />
    </div>
  )
}
