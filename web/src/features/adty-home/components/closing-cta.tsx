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

const CTA_BUTTON = 'h-[50px] rounded-[11px] px-6 text-[15.5px] font-semibold'

/**
 * Closing card: black block with its own dot pattern, one white button and one
 * outlined button. Ends the page before the footer.
 */
export function ClosingCta(props: { signedIn: boolean }) {
  const { t } = useTranslation()

  return (
    <section
      aria-labelledby='closing-title'
      className='px-6 pb-28 md:px-10 md:pb-32'
    >
      <div className='dark bg-background text-foreground relative isolate mx-auto max-w-[1180px] overflow-hidden rounded-[24px] border px-6 py-20 text-center md:px-14 md:py-24'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10 [background-image:radial-gradient(var(--foreground)_1px,transparent_1px)] [mask-image:radial-gradient(60%_82%_at_50%_0%,#000,transparent_74%)] [background-size:22px_22px] opacity-10'
        />
        <h2
          id='closing-title'
          className='text-[clamp(2.1rem,4.4vw,3.6rem)] leading-[1.06] font-extrabold tracking-[-0.04em] [text-wrap:balance] [word-break:keep-all]'
        >
          <span className='block'>{t('Start with one request.')}</span>
          <span className='text-brand block'>
            {t('Decide about the invoice later.')}
          </span>
        </h2>
        <p className='text-muted-foreground mx-auto mt-5 max-w-[32.5rem] text-base leading-[1.7] text-pretty'>
          {t('A key, an endpoint, and pricing you can read before you commit.')}
        </p>
        <div className='mt-10 flex flex-wrap justify-center gap-3.5'>
          <a
            href={props.signedIn ? '/dashboard' : '/sign-up'}
            className={cn(
              buttonVariants({ size: 'lg' }),
              CTA_BUTTON,
              'bg-foreground text-background hover:bg-foreground/90'
            )}
          >
            {props.signedIn ? t('Go to console') : t('Get a key')}
          </a>
          <a
            href='/pricing'
            className={cn(
              buttonVariants({ variant: 'outline', size: 'lg' }),
              CTA_BUTTON,
              'border-foreground/25 text-foreground hover:bg-foreground/10 bg-transparent'
            )}
          >
            {t('See pricing')}
          </a>
        </div>
      </div>
    </section>
  )
}
