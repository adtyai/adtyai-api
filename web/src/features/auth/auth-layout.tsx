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
import { Link, useRouterState } from '@tanstack/react-router'
import { Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { useFirstTopupBonusPercent } from '@/features/home/hooks/use-first-topup-bonus'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

function AuthBrandPanel({ systemName }: { systemName: string }) {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const isRegister = pathname.includes('sign-up')
  const bonusPercent = useFirstTopupBonusPercent()

  const headline = isRegister
    ? t('Invite friends to use AI together')
    : t('Unified access to global LLMs')
  const sub = isRegister
    ? t('Earn commission from invitees paid amount via invite link')
    : t('OpenAI-compatible · USD billing · Transparent multipliers')

  return (
    <div className='relative hidden flex-col justify-between overflow-hidden bg-gradient-to-br from-indigo-600 via-indigo-600 to-violet-600 p-10 text-white lg:flex lg:min-h-svh'>
      <div className='pointer-events-none absolute -top-24 -right-24 size-96 rounded-full bg-white/10' />
      <div className='pointer-events-none absolute right-40 bottom-24 size-64 rounded-full bg-white/5' />

      <Link
        to='/'
        className='relative z-10 flex items-center gap-2.5 transition-opacity hover:opacity-90'
      >
        <span className='grid size-9 place-items-center rounded-lg bg-white/15'>
          <Zap className='size-5' strokeWidth={1.8} />
        </span>
        <span className='text-xl font-bold tracking-tight'>{systemName}</span>
      </Link>

      <div className='relative z-10 max-w-lg'>
        <h2 className='text-4xl leading-tight font-bold'>{headline}</h2>
        <p className='mt-3 text-base text-white/80'>{sub}</p>
        <ul className='mt-8 space-y-2 text-sm text-white/85'>
          <li>· {t('OpenAI-compatible')}</li>
          <li>· {t('Pay as you go')}</li>
          <li>
            ·{' '}
            {bonusPercent == null
              ? t('First top-up bonus')
              : t('First top-up bonus {{percent}}%', {
                  percent: bonusPercent,
                })}
          </li>
        </ul>
      </div>

      <div className='relative z-10 text-xs text-white/60'>
        © {new Date().getFullYear()} {systemName}
      </div>
    </div>
  )
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='bg-muted/40 grid min-h-svh lg:grid-cols-[1fr_minmax(28rem,36rem)]'>
      <AuthBrandPanel systemName={systemName} />

      <div className='relative flex flex-col bg-background'>
        {/* Mobile / narrow: compact brand row (desktop brand lives on the left panel) */}
        <Link
          to='/'
          className='absolute top-4 left-4 z-10 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-6 sm:left-6 lg:hidden'
        >
          <div className='relative size-8'>
            {loading ? (
              <Skeleton className='absolute inset-0 rounded-lg' />
            ) : (
              <img
                src={logo}
                alt={t('Logo')}
                className='size-8 rounded-lg object-cover'
              />
            )}
          </div>
          {loading ? (
            <Skeleton className='h-6 w-24' />
          ) : (
            <span className='text-lg font-semibold tracking-tight'>
              {systemName}
            </span>
          )}
        </Link>

        <div className='flex flex-1 items-center justify-center px-4 py-16 sm:px-8 lg:px-10 lg:py-10'>
          <div className='mx-auto w-full max-w-[400px]'>{children}</div>
        </div>
      </div>
    </div>
  )
}
