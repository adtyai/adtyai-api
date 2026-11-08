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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { expect, it, vi } from 'vitest'

import { DeploymentAccessGuard } from '../components/deployment-access-guard'

type GuardProps = ComponentProps<typeof DeploymentAccessGuard>

async function renderDeploymentGuard(overrides: Partial<GuardProps> = {}) {
  const props: GuardProps = {
    children: <button type='button'>Deployment content</button>,
    loading: false,
    isEnabled: true,
    connectionLoading: false,
    connectionOk: true,
    connectionError: null,
    onRetry: vi.fn(),
    ...overrides,
  }
  const root = createRootRoute()
  const index = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => (
      <main>
        <DeploymentAccessGuard {...props} />
      </main>
    ),
  })
  const settings = createRoute({
    getParentRoute: () => root,
    path: 'system-settings/models/$section',
    component: () => <h1>Deployment settings</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([index, settings]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  await screen.findByRole('main')
  return { router, props }
}

it.each([
  { condition: 'null', children: null, expected: '' },
  { condition: 'false', children: false, expected: '' },
  { condition: 'zero', children: 0, expected: '0' },
  { condition: 'a string', children: 'Ready', expected: 'Ready' },
])(
  'passes through $condition children when deployment access is ready',
  async ({ children, expected }) => {
    await renderDeploymentGuard({ children })

    expect(screen.getByRole('main').textContent).toBe(expected)
  }
)

it('keeps interactive array children available when deployment access is ready', async () => {
  const onClick = vi.fn()
  const user = userEvent.setup()
  await renderDeploymentGuard({
    children: [
      <span key='label'>Ready</span>,
      <button key='action' type='button' onClick={onClick}>
        Deploy
      </button>,
    ],
  })

  expect(screen.getByText('Ready')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Deploy' }))

  expect(onClick).toHaveBeenCalledTimes(1)
})

it.each([
  {
    condition: 'settings are loading',
    props: { loading: true, isEnabled: false },
  },
  {
    condition: 'the connection is loading',
    props: { connectionLoading: true },
  },
])('keeps deployment content hidden while $condition', async ({ props }) => {
  await renderDeploymentGuard(props)

  expect(screen.getByText('Loading configuration')).toBeVisible()
  expect(screen.getByText('Checking connection')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Deployment content' })
  ).not.toBeInTheDocument()
})

it('keeps deployment content hidden and links to settings when the service is disabled', async () => {
  const user = userEvent.setup()
  const { router } = await renderDeploymentGuard({ isEnabled: false })

  expect(
    screen.getByRole('heading', {
      name: 'Model deployment service is disabled',
    })
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Deployment content' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Go to settings' }))

  expect(
    await screen.findByRole('heading', { name: 'Deployment settings' })
  ).toBeVisible()
  expect(router.state.location.pathname).toBe(
    '/system-settings/models/model-deployment'
  )
})

it('keeps deployment content hidden and retries when the connection fails', async () => {
  const user = userEvent.setup()
  const onRetry = vi.fn()
  await renderDeploymentGuard({
    connectionOk: false,
    connectionError: 'Connection unavailable',
    onRetry,
  })

  expect(
    screen.getByRole('heading', { name: 'Connection failed' })
  ).toBeVisible()
  expect(screen.getByText('Connection unavailable')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Deployment content' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))

  expect(onRetry).toHaveBeenCalledTimes(1)
})
