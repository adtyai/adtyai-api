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
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { LanguageSwitcher } from '@/components/language-switcher'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()
  // adty: brand panel on the left, form on the right (task 8.7)
  const highlights = [
    t('Pay as you go, top up from $1'),
    t('USDT, Alipay and WeChat Pay'),
    t('Works with Claude Code, Codex and OpenAI SDKs'),
  ]

  return (
    <div className='relative grid min-h-svh max-w-none lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'>
      <Link
        to='/'
        className='absolute top-4 left-4 z-10 flex items-center gap-2 transition-opacity hover:opacity-80 sm:top-8 sm:left-8'
      >
        <div className='relative h-8 w-8'>
          {loading ? (
            <Skeleton className='absolute inset-0 rounded-lg' />
          ) : (
            <img
              src={logo}
              alt={t('Logo')}
              className='h-8 w-8 rounded-lg object-cover'
            />
          )}
        </div>
        {loading ? (
          <Skeleton className='h-6 w-24' />
        ) : (
          <h1 className='text-xl font-medium'>{systemName}</h1>
        )}
      </Link>
      <div className='absolute top-4 right-4 z-10 sm:top-8 sm:right-8'>
        <LanguageSwitcher />
      </div>
      <aside
        aria-label={t('About this site')}
        className='bg-muted/40 hidden flex-col justify-center border-r px-12 py-24 lg:flex xl:px-20'
      >
        <div className='flex max-w-md flex-col gap-6'>
          <p className='text-3xl leading-tight font-semibold tracking-tight xl:text-4xl'>
            {t('Claude, GPT and Gemini with one API key')}
          </p>
          <p className='text-muted-foreground text-base'>
            {t(
              'Change the base URL and keep your code. Billed per request, no subscription.'
            )}
          </p>
          <ul className='flex flex-col gap-3 text-sm'>
            {highlights.map((item) => (
              <li key={item} className='flex items-center gap-2'>
                <Check className='text-success size-4' aria-hidden='true' />
                {item}
              </li>
            ))}
          </ul>
        </div>
      </aside>
      <main className='flex items-center pt-16 sm:pt-0'>
        <div className='mx-auto flex w-full flex-col justify-center space-y-2 px-4 py-8 sm:w-[480px] sm:p-8'>
          {children}
        </div>
      </main>
    </div>
  )
}
