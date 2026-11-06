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
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { AuthLayout } from '../auth-layout'

afterEach(() => {
  cleanup()
})

async function renderLayout() {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <AuthLayout>
          <form aria-label='Sign in form' />
        </AuthLayout>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('auth layout', () => {
  it('puts the brand panel before the form and the form inside the main landmark', async () => {
    await renderLayout()

    const brand = await screen.findByRole('complementary', {
      name: 'About this site',
    })
    const main = screen.getByRole('main')
    expect(main).toContainElement(
      screen.getByRole('form', { name: 'Sign in form' })
    )
    expect(brand).not.toContainElement(main)
    expect(
      brand.compareDocumentPosition(main) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('offers a language switch on the sign-in layout', async () => {
    await renderLayout()

    expect(
      await screen.findByRole('button', { name: 'Change language' })
    ).toBeInTheDocument()
  })

  it('does not name USDT, Alipay or WeChat when those methods are not the enabled list', async () => {
    await renderLayout()

    expect(
      await screen.findByRole('complementary', { name: 'About this site' })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('USDT, Alipay and WeChat Pay')
    ).not.toBeInTheDocument()
  })

  it('hides the brand panel on small screens so the form keeps one column', async () => {
    await renderLayout()

    const brand = await screen.findByRole('complementary', {
      name: 'About this site',
    })
    expect(brand).toHaveClass('hidden', 'lg:flex')
  })
})
