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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ComponentProps } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { PaymentSettingsSection } from '../payment-settings-section'

const clients: QueryClient[] = []
const defaults: ComponentProps<typeof PaymentSettingsSection> = {
  defaultValues: {
    PayAddress: '',
    EpayId: '',
    EpayKey: '',
    Price: 1,
    MinTopUp: 1,
    CustomCallbackAddress: '',
    PayMethods: '[]',
    AmountOptions: '[10]',
    BonusRates: '{}',
    StripeApiSecret: '',
    StripeWebhookSecret: '',
    StripePriceId: '',
    StripeUnitPrice: 1,
    StripeMinTopUp: 1,
    StripePromotionCodesEnabled: false,
    CreemApiKey: '',
    CreemWebhookSecret: '',
    CreemTestMode: false,
    CreemProducts: '[]',
  },
  waffoDefaultValues: {
    WaffoEnabled: false,
    WaffoApiKey: '',
    WaffoPrivateKey: '',
    WaffoPublicCert: '',
    WaffoSandboxPublicCert: '',
    WaffoSandboxApiKey: '',
    WaffoSandboxPrivateKey: '',
    WaffoSandbox: false,
    WaffoMerchantId: '',
    WaffoCurrency: 'USD',
    WaffoUnitPrice: 1,
    WaffoMinTopUp: 1,
    WaffoNotifyUrl: '',
    WaffoReturnUrl: '',
    WaffoPayMethods: '[]',
  },
  waffoPancakeDefaultValues: {
    WaffoPancakeMerchantID: '',
    WaffoPancakePrivateKey: '',
    WaffoPancakeReturnURL: '',
    WaffoPancakeEnvironment: 'prod',
  },
  epayDefaultValues: {
    orderMinutes: 10,
    channelTimeoutSeconds: 5,
    totalTimeoutSeconds: 12,
    queryAfterSeconds: 120,
    expiredQuerySeconds: 86400,
    unpaidThreshold: 10,
    probeEnabled: false,
    probeIntervalMinutes: 5,
  },
  usdtDefaultValues: {
    enabled: false,
    baseUrl: '',
    tradeTypes: '["usdt.trc20"]',
    orderMinutes: 15,
  },
}

function Fixture(props: {
  invalidOtherProvider?: boolean
  pancakeConfigured?: boolean
}) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const [savedStripe, setSavedStripe] = useState('')
  const [refreshedBindings, setRefreshedBindings] = useState(false)
  const savedProduct = refreshedBindings ? 'prod_remote' : 'prod_saved'
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <button type='button' onClick={() => setSavedStripe('price_remote')}>
        Refresh saved Stripe
      </button>
      <button type='button' onClick={() => setRefreshedBindings(true)}>
        Refresh provider bindings
      </button>
      <PaymentSettingsSection
        {...defaults}
        defaultValues={{
          ...defaults.defaultValues,
          StripePriceId: savedStripe,
        }}
        waffoDefaultValues={{
          ...defaults.waffoDefaultValues,
          WaffoMinTopUp: props.invalidOtherProvider ? 0 : 1,
          WaffoPayMethods: JSON.stringify([
            {
              name: refreshedBindings ? 'Remote method' : 'Saved method',
              icon: '',
              payMethodType: 'card',
              payMethodName: 'card',
            },
          ]),
        }}
        waffoPancakeDefaultValues={{
          ...defaults.waffoPancakeDefaultValues,
          WaffoPancakeMerchantID: props.pancakeConfigured ? 'MER_saved' : '',
        }}
        waffoPancakeProvisionedStoreID={
          props.pancakeConfigured ? 'store_saved' : ''
        }
        waffoPancakeProvisionedProductID={
          props.pancakeConfigured ? savedProduct : ''
        }
      />
    </SettingsPageProvider>
  )
}

async function mount(
  tab: string,
  invalidOtherProvider = false,
  pancakeConfigured = false
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute({
    component: () => (
      <Fixture
        invalidOtherProvider={invalidOtherProvider}
        pancakeConfigured={pancakeConfigured}
      />
    ),
  })
  const route = createRoute({
    getParentRoute: () => root,
    path: '/system-settings/billing/$section',
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({
      initialEntries: [`/system-settings/billing/payment?tab=${tab}`],
    }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('tab', { name: 'General' })
  return router
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
})

it('exposes dynamic payment scene management on the Root general tab', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'scene-root', role: 100 })
  vi.mocked(api.get).mockImplementation(async (path) => ({
    data: {
      success: true,
      data:
        path === '/api/adty/admin/payment-scenes'
          ? { version: 1, scenes: [] }
          : [],
    },
  }))
  await mount('general')
  expect(
    await screen.findByRole('button', { name: 'New payment scene' })
  ).toBeVisible()
})

it('preserves an Epay draft when refreshed settings include another provider save', async () => {
  await mount('epay')
  await userEvent.type(
    screen.getByLabelText('Epay merchant ID'),
    'draft_merchant'
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh saved Stripe' })
  )
  expect(screen.getByLabelText('Epay merchant ID')).toHaveValue(
    'draft_merchant'
  )
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it.each(['stripe', 'creem', 'waffo-pancake', 'waffo'])(
  'does not offer retired %s checkout as an operable admin tab',
  async (tab) => {
    await mount(tab)
    expect(screen.getByRole('tab', { name: 'General' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      screen.queryByRole('tab', { name: 'Stripe' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Creem' })).not.toBeInTheDocument()
    expect(
      screen.queryByRole('tab', { name: 'Waffo Pancake' })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Waffo' })).not.toBeInTheDocument()
    expect(
      screen.getByText('Hosted checkout channels are retired')
    ).toBeVisible()
  }
)

it('keeps USDT selected when switching from Epay and restores the previous tab on history back', async () => {
  const router = await mount('epay')
  await userEvent.click(screen.getByRole('tab', { name: 'USDT' }))
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'USDT' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
  router.history.back()
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Epay' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  )
})

it('saves official Epay settings and never writes retired hosted payment keys', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  await mount('epay')
  await userEvent.type(
    screen.getByLabelText('Epay merchant ID'),
    'epay_official'
  )
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Save Changes' })[0]
  )
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'EpayId',
      value: 'epay_official',
    })
  )
  expect(
    put.mock.calls.some((call) => {
      const option = call[1] as { key?: string } | undefined
      return /^(Stripe|Creem|Waffo)/.test(String(option?.key ?? ''))
    })
  ).toBe(false)
})
