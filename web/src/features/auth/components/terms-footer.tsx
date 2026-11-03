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
import { useTranslation } from 'react-i18next'

import { useAdtySite } from '@/features/adty-site/api'
import { cn } from '@/lib/utils'

import type { SystemStatus } from '../types'

interface TermsFooterProps {
  variant?: 'sign-in' | 'sign-up'
  className?: string
  status?: SystemStatus | null
}

export function TermsFooter({
  variant = 'sign-in',
  className,
  status,
}: TermsFooterProps) {
  const { t } = useTranslation()
  const text =
    variant === 'sign-in'
      ? 'By clicking sign in, you agree to our'
      : 'By creating an account, you agree to our'

  const hasUserAgreement = Boolean(status?.user_agreement_enabled)
  const hasPrivacyPolicy = Boolean(status?.privacy_policy_enabled)
  // adty: the refund policy link follows the site setting refund_policy_enabled;
  // like the consent checkbox, it never shows without an agreement.
  const hasRefundPolicy = useAdtySite().data?.refund_policy_enabled === true

  const activeLinks = [
    hasUserAgreement && { label: 'User Agreement', href: '/user-agreement' },
    hasPrivacyPolicy && { label: 'Privacy Policy', href: '/privacy-policy' },
    hasRefundPolicy && { label: 'Refund Policy', href: '/refund-policy' },
  ].filter((link) => link !== false)

  if (!hasUserAgreement && !hasPrivacyPolicy) {
    return null
  }

  return (
    <p className={cn('text-muted-foreground text-center text-xs', className)}>
      {text}{' '}
      {activeLinks.map((link, index) => (
        <span key={link.href}>
          {index > 0 && index === activeLinks.length - 1 && ` ${t('and')} `}
          {index > 0 && index < activeLinks.length - 1 && ', '}
          <a
            href={link.href}
            className='hover:text-primary underline underline-offset-4'
          >
            {link.label}
          </a>
        </span>
      ))}
      .
    </p>
  )
}
