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

it('preserves an inactive provider draft when refreshed settings include another provider save', async () => {
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
  await userEvent.click(screen.getByRole('tab', { name: 'Stripe' }))
  expect(screen.getByLabelText('Price ID')).toHaveValue('price_remote')
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

it.each([
  ['stripe', 'Stripe'],
  ['creem', 'Creem'],
  ['waffo-pancake', 'Waffo Pancake'],
  ['waffo', 'Waffo'],
])('opens the %s provider from its deep link', async (tab, name) => {
  await mount(tab)
  expect(screen.getByRole('tab', { name })).toHaveAttribute(
    'aria-selected',
    'true'
  )
})

it('keeps Stripe selected when switching from Epay and restores the previous tab on history back', async () => {
  const router = await mount('epay')
  await userEvent.click(screen.getByRole('tab', { name: 'Stripe' }))
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Stripe' })).toHaveAttribute(
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

it('saves Stripe while an unconfigured Waffo setting is invalid without saving another provider', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  await mount('general', true)
  await userEvent.click(screen.getByRole('tab', { name: 'Stripe' }))
  const price = screen.getByLabelText('Price ID')
  await userEvent.type(price, 'price_synthetic')
  await userEvent.click(
    screen.getByRole('button', { name: /Save (all settings|Changes)/ })
  )
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/option/', {
      key: 'StripePriceId',
      value: 'price_synthetic',
    })
  )
  expect(put).toHaveBeenCalledTimes(1)
})

it('defaults Pancake to production and saves the explicitly selected environment with its binding', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => ({
    data: {
      message: 'success',
      data: String(url).endsWith('/save')
        ? { store_id: 'store_saved', product_id: 'prod_saved' }
        : {
            stores: [
              {
                id: 'store_saved',
                name: 'Saved store',
                onetimeProducts: [{ id: 'prod_saved', name: 'Saved product' }],
              },
            ],
          },
    },
  }))
  await mount('waffo-pancake', false, true)
  const environment = screen.getByLabelText('Environment')
  expect(environment).toHaveValue('prod')
  await userEvent.selectOptions(environment, 'test')
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh saved Stripe' })
  )
  expect(environment).toHaveValue('test')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/option/waffo-pancake/save', {
      merchant_id: 'MER_saved',
      private_key: '',
      return_url: '',
      store_id: 'store_saved',
      product_id: 'prod_saved',
      environment: 'test',
    })
  )
})

it('preserves a Pancake product draft when the saved binding refreshes', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      message: 'success',
      data: {
        stores: [
          {
            id: 'store_saved',
            name: 'Saved store',
            onetimeProducts: [
              { id: 'prod_saved', name: 'Saved product' },
              { id: 'prod_draft', name: 'Draft product' },
              { id: 'prod_remote', name: 'Remote product' },
            ],
          },
        ],
      },
    },
  })
  await mount('waffo-pancake', false, true)
  const product = await screen.findByPlaceholderText('Select a product')
  await userEvent.click(product)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Draft product (prod_draft)' })
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh provider bindings' })
  )
  expect(product).toHaveValue('Draft product (prod_draft)')
})

it('preserves a deleted Waffo payment-method draft when saved methods refresh', async () => {
  await mount('waffo')
  await userEvent.click(screen.getByRole('button', { name: 'Open menu' }))
  await userEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  expect(screen.getByText('No payment methods configured')).toBeVisible()
  await userEvent.click(
    screen.getByRole('button', { name: 'Refresh provider bindings' })
  )
  expect(screen.getByText('No payment methods configured')).toBeVisible()
  expect(screen.queryByText('Remote method')).not.toBeInTheDocument()
})
