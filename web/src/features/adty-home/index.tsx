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
import { useRemovePrerenderOnMount } from '@/features/adty-seo/prerender-cleanup'
import { useApiBaseUrl } from '@/features/adty-site/api'
import { useAuthStore } from '@/stores/auth-store'

import { AffiliateBanner } from './components/affiliate-banner'
import { ClientGuides } from './components/client-guides'
import { Faq } from './components/faq'
import { Hero } from './components/hero'
import { PaymentStrip } from './components/payment-strip'
import { PopularModels } from './components/popular-models'
import { SellingPoints } from './components/selling-points'
import { Steps } from './components/steps'
import { useLandingData, type LandingData } from './hooks/use-landing-data'

/**
 * The landing page blocks from the hero down, built only from props. The
 * build-time prerender (task 9.3) renders this same component without data,
 * so nothing here may touch window, document or localStorage, and links are
 * plain anchors.
 */
export function LandingSections(props: {
  signedIn: boolean
  baseUrl: string
  data: LandingData
}) {
  return (
    <>
      <Hero
        signedIn={props.signedIn}
        baseUrl={props.baseUrl}
        prices={props.data.prices}
        minTopUp={props.data.minTopUp}
      />
      <PaymentStrip supportHours={props.data.supportHours} />
      <PopularModels prices={props.data.prices} />
      <Steps />
      <ClientGuides />
      <SellingPoints />
      <AffiliateBanner
        open={props.data.affiliate.open}
        rate={props.data.affiliate.rate}
      />
      <Faq />
    </>
  )
}

/**
 * Default home page (adty task 8.3); the public layout adds header and
 * footer. On mount it removes the server-inserted prerendered copy (9.3).
 */
export function AdtyLanding() {
  useRemovePrerenderOnMount()
  const user = useAuthStore((state) => state.auth.user)
  const baseUrl = useApiBaseUrl()
  const data = useLandingData({ signedIn: !!user, userGroup: user?.group })
  return <LandingSections signedIn={!!user} baseUrl={baseUrl} data={data} />
}
