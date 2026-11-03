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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getReferralPublicSettings,
  referralQueryKeys,
} from '@/features/referral/api'
import { useAuthStore } from '@/stores/auth-store'

import { AffiliateDetails } from './components/affiliate-details'

/** Public affiliate program page (adty task 8.6). Numbers come from the referral settings. */
export function AffiliateProgram() {
  const { t } = useTranslation()
  const signedIn = useAuthStore((state) => Boolean(state.auth.user))
  const settings = useQuery({
    queryKey: referralQueryKeys.publicSettings,
    queryFn: getReferralPublicSettings,
    staleTime: 5 * 60 * 1000,
    meta: { errorToast: false },
  })
  const data = settings.data
  // A program without a rate pays nothing yet, so it is shown as not open (adty convention I).
  const rate = data?.enabled ? data.default_rate : null
  const open = data !== undefined && rate !== null

  const cta = signedIn ? (
    <Button size='lg' render={<Link to='/referral' />}>
      {t('Get my referral link')}
    </Button>
  ) : (
    <Button size='lg' render={<Link to='/sign-up' />}>
      {t('Sign up to start earning')}
    </Button>
  )

  return (
    <PublicLayout>
      <div className='mx-auto max-w-5xl space-y-14 py-10'>
        <section className='space-y-5 text-center'>
          <h1 className='text-4xl font-semibold tracking-tight'>
            {t('Affiliate Program')}
          </h1>
          {settings.isPending && (
            <Skeleton className='mx-auto h-6 w-72' aria-hidden />
          )}
          {!settings.isPending && open && (
            <p className='text-muted-foreground mx-auto max-w-2xl text-lg'>
              {t(
                'Earn {{rate}}% of every top-up made by the people you invite, for as long as they stay.',
                { rate }
              )}
            </p>
          )}
          {!settings.isPending && !open && (
            <p className='text-muted-foreground mx-auto max-w-2xl text-lg'>
              {t('The affiliate program is not open yet. Check back soon.')}
            </p>
          )}
          {open && <div className='flex justify-center'>{cta}</div>}
        </section>

        {data !== undefined && rate !== null && (
          <AffiliateDetails settings={{ ...data, default_rate: rate }} />
        )}
      </div>
    </PublicLayout>
  )
}
