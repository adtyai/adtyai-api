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

import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** Hero buttons take the sample's 50px height; `lg` is 40px. */
const HERO_BUTTON = 'h-[50px] rounded-[11px] px-6 text-[15.5px] font-semibold'

/**
 * First screen (design sample: hero). One headline broken into two lines, a
 * two-sentence lead, two buttons and a small note — nothing else. The grid
 * and the warm corner light are the only decoration.
 */
export function Hero(props: { signedIn: boolean }) {
  const { t } = useTranslation()

  return (
    <section
      id='hero'
      aria-labelledby='hero-title'
      className='relative isolate flex min-h-svh flex-col justify-center overflow-hidden px-6 pt-32 pb-24 sm:px-8 sm:pt-36 md:px-10'
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10 [background-image:linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(70%_76%_at_78%_22%,#000_0%,transparent_72%)] [background-size:72px_72px] opacity-60'
      />
      <div
        aria-hidden
        className='pointer-events-none absolute -top-[25rem] -right-[17.5rem] -z-10 size-[53.75rem] rounded-full bg-[radial-gradient(circle,var(--brand-soft),transparent_64%)]'
      />
      <div className='relative z-10 mx-auto w-full max-w-[1180px]'>
        <h1
          id='hero-title'
          className='text-[clamp(2.6rem,6vw,5.2rem)] leading-[1.02] font-extrabold tracking-[-0.045em] [text-wrap:balance] [word-break:keep-all]'
        >
          <span className='block'>{t('One endpoint.')}</span>
          <span className='text-neutral block'>{t('Every model.')}</span>
        </h1>
        <p className='text-muted-foreground mt-8 max-w-[37.5rem] text-[17px] leading-[1.7] text-pretty'>
          {t(
            'Claude, GPT, Gemini, DeepSeek and the rest — behind the API you already integrate with.'
          )}
        </p>
        <div className='mt-10 flex flex-wrap gap-3.5'>
          <a
            href={props.signedIn ? '/dashboard' : '/sign-up'}
            className={cn(
              buttonVariants({ variant: 'brand', size: 'lg' }),
              HERO_BUTTON
            )}
          >
            {props.signedIn ? t('Go to console') : t('Get a key')}
          </a>
          <a
            href='/docs'
            className={cn(
              buttonVariants({ variant: 'outline', size: 'lg' }),
              HERO_BUTTON
            )}
          >
            {t('Read the docs')}
          </a>
        </div>
        <p className='text-neutral mt-6 text-[13.5px]'>
          {t('No card required to start. Billed in USD, per request.')}
        </p>
      </div>
      <div
        aria-hidden
        className='text-neutral absolute inset-x-0 bottom-7 hidden justify-center md:flex'
      >
        <svg
          width='22'
          height='22'
          viewBox='0 0 24 24'
          fill='none'
          stroke='currentColor'
          strokeWidth='1.7'
          strokeLinecap='round'
          strokeLinejoin='round'
        >
          <path d='m6 9 6 6 6-6' />
        </svg>
      </div>
    </section>
  )
}
