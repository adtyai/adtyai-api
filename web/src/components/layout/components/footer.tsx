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
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { useAdtySite } from '@/features/adty-site/api'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { ADTY_BRAND_NAME } from '@/lib/adty-brand'
import { DEFAULT_LOGO } from '@/lib/constants'
import { cn } from '@/lib/utils'

interface FooterLink {
  text: string
  href: string
}

interface FooterColumnProps {
  title: string
  links: FooterLink[]
}

interface FooterProps {
  logo?: string
  name?: string
  columns?: FooterColumnProps[]
  copyright?: string
  className?: string
}

function FooterLinkItem(props: { link: FooterLink }) {
  const { t } = useTranslation()
  const isExternal = props.link.href.startsWith('http')
  const label = t(props.link.text)
  const className =
    'text-muted-foreground hover:text-foreground text-[13px] transition-colors duration-200'

  if (props.link.href.startsWith('mailto:')) {
    return (
      <a href={props.link.href} className={className}>
        {label}
      </a>
    )
  }

  if (isExternal) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        className={className}
      >
        {label}
      </a>
    )
  }

  return (
    <Link to={props.link.href} className={className}>
      {label}
    </Link>
  )
}

function LegalLinks(props: { leadingSeparator?: boolean }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const refundPolicyEnabled = useAdtySite().data?.refund_policy_enabled === true
  const items: { key: string; label: string; href: string }[] = []
  if (status?.user_agreement_enabled) {
    items.push({
      key: 'user-agreement',
      label: t('User Agreement'),
      href: '/user-agreement',
    })
  }
  if (status?.privacy_policy_enabled) {
    items.push({
      key: 'privacy-policy',
      label: t('Privacy Policy'),
      href: '/privacy-policy',
    })
  }
  if (refundPolicyEnabled) {
    items.push({
      key: 'refund-policy',
      label: t('Refund Policy'),
      href: '/refund-policy',
    })
  }
  if (items.length === 0) {
    return null
  }
  return (
    <>
      {items.map((item, index) => (
        <Fragment key={item.key}>
          {(props.leadingSeparator || index > 0) && (
            <span aria-hidden='true' className='text-border px-0.5'>
              ·
            </span>
          )}
          <Link
            to={item.href}
            className='text-muted-foreground hover:text-foreground transition-colors duration-200'
          >
            {item.label}
          </Link>
        </Fragment>
      ))}
    </>
  )
}

export function Footer(props: FooterProps) {
  const { t } = useTranslation()
  const {
    systemName,
    logo: systemLogo,
    footerHtml,
    demoSiteEnabled,
  } = useSystemConfig()

  const displayLogo = systemLogo || props.logo || '/logo.png'
  const displayName = systemName || props.name || ADTY_BRAND_NAME
  const isDemoSiteMode = Boolean(demoSiteEnabled)
  const currentYear = new Date().getFullYear()
  const displayColumns = props.columns ?? []

  return (
    <footer
      className={cn('border-border bg-background border-t', props.className)}
    >
      <div className='mx-auto w-full max-w-[1180px] px-5 py-12 sm:px-10 md:py-14'>
        <div className='flex flex-col justify-between gap-10 md:flex-row md:items-start'>
          <div className='shrink-0'>
            <Link to='/' className='group flex items-center gap-2.5'>
              {/* The default ADTY artwork already carries the wordmark, so the
                  name is not repeated next to it; a site-configured logo keeps
                  the accessible name through `alt`. */}
              {displayLogo === DEFAULT_LOGO ? (
                <>
                  <img
                    src={displayLogo}
                    alt={displayName}
                    className='h-7 w-auto max-w-none object-contain dark:hidden'
                  />
                  <img
                    src='/logo-dark.png'
                    alt=''
                    aria-hidden='true'
                    className='dark:block hidden h-7 w-auto max-w-none object-contain'
                  />
                </>
              ) : (
                <img
                  src={displayLogo}
                  alt={displayName}
                  className='h-7 w-auto max-w-none object-contain dark:bg-white/10 dark:rounded-md dark:px-1 dark:py-0.5'
                />
              )}
            </Link>
            <p className='text-muted-foreground mt-3 max-w-[260px] text-[13px] leading-relaxed'>
              {t('One API key for every model')}
            </p>
          </div>

          {isDemoSiteMode && displayColumns.length > 0 && (
            <div className='grid grid-cols-2 gap-8 sm:grid-cols-3 md:gap-16'>
              {displayColumns.map((column) => (
                <div key={column.title}>
                  <p className='text-muted-foreground mb-3 text-[11px] font-semibold tracking-[0.08em] uppercase'>
                    {t(column.title)}
                  </p>
                  <ul className='space-y-2.5'>
                    {column.links.map((link) => (
                      <li key={`${link.href}:${link.text}`}>
                        <FooterLinkItem link={link} />
                      </li>
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>

        {footerHtml && (
          <div
            className='custom-footer text-muted-foreground mt-8 text-[13px]'
            dangerouslySetInnerHTML={{ __html: footerHtml }}
          />
        )}

        <div className='border-border text-muted-foreground mt-10 flex flex-col gap-2 border-t pt-6 text-xs sm:flex-row sm:items-center sm:justify-between'>
          <span>
            &copy; {currentYear} {displayName}.{' '}
            {props.copyright ?? t('footer.defaultCopyright')}
          </span>
          <div className='flex flex-wrap items-center gap-y-1'>
            <LegalLinks />
          </div>
        </div>
      </div>
    </footer>
  )
}
