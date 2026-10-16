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
import { Fragment, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
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

  if (isExternal) {
    return (
      <a
        href={props.link.href}
        target='_blank'
        rel='noopener noreferrer'
        className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
      >
        {label}
      </a>
    )
  }

  return (
    <Link
      to={props.link.href}
      className='text-muted-foreground hover:text-foreground text-sm transition-colors duration-200'
    >
      {label}
    </Link>
  )
}

// Renders User Agreement / Privacy Policy links inline with the parent's
// copyright row when either is configured in System Settings → Site. Emits
// fragmented siblings so the parent flex container's gap controls spacing.
function LegalLinks(props: { leadingSeparator?: boolean }) {
  const { t } = useTranslation()
  const { status } = useStatus()
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
  if (items.length === 0) {
    return null
  }
  return (
    <>
      {items.map((item, index) => (
        <Fragment key={item.key}>
          {(props.leadingSeparator || index > 0) && (
            <span aria-hidden='true' className='text-muted-foreground/30'>
              ·
            </span>
          )}
          <Link
            to={item.href}
            className='hover:text-foreground transition-colors duration-200'
          >
            {item.label}
          </Link>
        </Fragment>
      ))}
    </>
  )
}

/**
 * AGPL upstream attribution for the About compliance zone only.
 * Do NOT render this in the site footer next to the ADTY AI copyright —
 * that produced a dual "© … New API" line users saw as brand residue.
 */
export function ProjectAttribution(props: {
  currentYear?: number
  inline?: boolean
}) {
  const { t } = useTranslation()
  void props.currentYear
  const content = (
    <span className='text-muted-foreground/60'>
      {t(
        'Open-source notice: this product is based on the AGPL-licensed upstream project'
      )}{' '}
      <a
        href='https://github.com/QuantumNous/new-api'
        target='_blank'
        rel='noopener noreferrer'
        className='hover:text-muted-foreground underline-offset-2 hover:underline'
      >
        QuantumNous/new-api
      </a>
      .
    </span>
  )
  if (props.inline) {
    return content
  }
  return (
    <div className='text-muted-foreground/60 text-center text-[11px] leading-relaxed'>
      {content}
    </div>
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
  const { status } = useStatus()
  const docsLink =
    typeof status?.docs_link === 'string' ? status.docs_link : ''

  const displayLogo = systemLogo || props.logo || '/logo.png'
  const displayName = systemName || props.name || 'ADTY AI'
  const isDemoSiteMode = Boolean(demoSiteEnabled)
  const currentYear = new Date().getFullYear()

  // Demo-site fallback columns: keep structure, no docs.newapi.pro defaults.
  const fallbackColumns = useMemo<FooterColumnProps[]>(
    () => [
      {
        title: t('footer.columns.about.title'),
        links: [
          {
            text: t('footer.columns.about.links.aboutProject'),
            href: '/about',
          },
          {
            text: t('footer.columns.about.links.contact'),
            href: '/about',
          },
          {
            text: t('footer.columns.about.links.features'),
            href: '/pricing',
          },
        ],
      },
      {
        title: t('footer.columns.docs.title'),
        links: [
          {
            text: t('footer.columns.docs.links.quickStart'),
            href: docsLink || '/about',
          },
          {
            text: t('footer.columns.docs.links.installation'),
            href: docsLink || '/about',
          },
          {
            text: t('footer.columns.docs.links.apiDocs'),
            href: docsLink || '/about',
          },
        ],
      },
      {
        title: t('footer.columns.related.title'),
        links: [
          {
            text: t('About'),
            href: '/about',
          },
          {
            text: t('Model Square'),
            href: '/pricing',
          },
        ],
      },
    ],
    [t, docsLink]
  )

  const displayColumns = props.columns ?? fallbackColumns

  if (footerHtml) {
    return (
      <footer
        className={cn(
          'border-border/40 relative z-10 border-t',
          props.className
        )}
      >
        <div className='mx-auto w-full max-w-6xl px-6 py-5'>
          <div className='bg-muted/20 border-border/50 flex flex-col items-center justify-between gap-4 rounded-2xl border px-4 py-4 backdrop-blur-sm sm:flex-row sm:px-5'>
            <div
              className='custom-footer text-muted-foreground min-w-0 text-center text-sm sm:text-left'
              dangerouslySetInnerHTML={{ __html: footerHtml }}
            />
            <div className='border-border/60 text-muted-foreground/45 flex w-full flex-wrap items-center justify-center gap-x-3 gap-y-1 border-t pt-4 text-xs sm:w-auto sm:justify-end sm:border-t-0 sm:border-l sm:pt-0 sm:pl-5'>
              <LegalLinks />
            </div>
          </div>
        </div>
      </footer>
    )
  }

  // adtyai：默认四列页脚（品牌 / 产品 / 支持 / 条款）；演示站模式仍用官方列。
  const productLinks: FooterLink[] = [
    { text: t('Model Square'), href: '/pricing' },
    ...(docsLink ? [{ text: t('Docs'), href: docsLink }] : []),
  ]
  const supportLinks: FooterLink[] = [{ text: t('About'), href: '/about' }]
  const legalLinks: FooterLink[] = [
    ...(status?.user_agreement_enabled
      ? [{ text: t('User Agreement'), href: '/user-agreement' }]
      : []),
    ...(status?.privacy_policy_enabled
      ? [{ text: t('Privacy Policy'), href: '/privacy-policy' }]
      : []),
  ]
  const adtyColumns: FooterColumnProps[] = [
    { title: t('Product'), links: productLinks },
    { title: t('footer.supportTitle'), links: supportLinks },
    ...(legalLinks.length > 0
      ? [{ title: t('Terms'), links: legalLinks }]
      : []),
  ]
  const columnsToShow = isDemoSiteMode ? displayColumns : adtyColumns

  return (
    <footer
      className={cn('border-border/60 relative z-10 border-t', props.className)}
    >
      <div className='mx-auto max-w-6xl px-6 py-12 md:py-14'>
        <div className='grid grid-cols-2 gap-8 md:grid-cols-4'>
          <div className='col-span-2 md:col-span-1'>
            <Link to='/' className='group flex items-center gap-2.5'>
              <img
                src={displayLogo}
                alt={displayName}
                className='size-7 rounded-lg object-contain'
              />
              <span className='text-base font-bold tracking-tight'>
                {displayName}
              </span>
            </Link>
            <p className='text-muted-foreground mt-3 max-w-[220px] text-sm leading-relaxed'>
              {t('One key to access all mainstream models')}
            </p>
          </div>
          {columnsToShow.map((column) => (
            <div key={column.title}>
              <p className='mb-3 text-sm font-semibold'>{t(column.title)}</p>
              <ul className='space-y-2.5'>
                {column.links.map((link) => (
                  <li key={link.href}>
                    <FooterLinkItem link={link} />
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className='border-border/40 mt-10 flex flex-col items-center justify-center gap-x-3 gap-y-2 border-t pt-6 sm:flex-row'>
          <div className='text-muted-foreground/70 text-xs'>
            &copy; {currentYear} {displayName}.{' '}
            {props.copyright ?? t('footer.defaultCopyright')}
          </div>
        </div>
      </div>
    </footer>
  )
}
