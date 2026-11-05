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
    'text-sm text-white/55 transition-colors duration-200 hover:text-white'

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
            <span aria-hidden='true' className='text-white/25'>
              ·
            </span>
          )}
          <Link
            to={item.href}
            className='text-white/55 transition-colors duration-200 hover:text-white'
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
      className={cn('relative z-10 bg-[#0d0d0d] text-white', props.className)}
    >
      <div className='mx-auto max-w-6xl px-6 py-12 md:py-16'>
        <div className='flex flex-col justify-between gap-10 md:flex-row md:items-start'>
          <div className='shrink-0'>
            <Link to='/' className='group flex items-center gap-2.5'>
              <img
                src={displayLogo}
                alt={displayName}
                className='size-7 rounded-lg object-contain'
              />
              <span className='text-sm font-semibold tracking-tight text-white'>
                {displayName}
              </span>
            </Link>
            <p className='mt-3 max-w-[240px] text-xs leading-relaxed text-white/45'>
              {t('One API key for every model')}
            </p>
          </div>

          {isDemoSiteMode && displayColumns.length > 0 && (
            <div className='grid grid-cols-3 gap-8 md:gap-16'>
              {displayColumns.map((column) => (
                <div key={column.title}>
                  <p className='mb-3 text-xs font-medium tracking-wider text-white/40 uppercase'>
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
            className='custom-footer mt-8 text-sm text-white/55'
            dangerouslySetInnerHTML={{ __html: footerHtml }}
          />
        )}

        <div className='mt-12 flex flex-col items-center justify-between gap-x-3 gap-y-2 border-t border-white/10 pt-6 sm:flex-row'>
          <div className='flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-xs text-white/40 sm:justify-start'>
            <span>
              &copy; {currentYear} {displayName}.{' '}
              {props.copyright ?? t('footer.defaultCopyright')}
            </span>
            <LegalLinks leadingSeparator />
          </div>
        </div>
      </div>
    </footer>
  )
}
