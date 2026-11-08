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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import type { MarketingAudience } from '../api'
import { CampaignEditor } from '../components/campaign-editor'
import { emptyCampaign } from '../lib/campaign'

const api = vi.hoisted(() => ({
  estimateAudience: vi.fn(),
  saveCampaign: vi.fn(),
  getCampaignPlan: vi.fn(),
  sendCampaign: vi.fn(),
  sendCampaignTest: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  ...api,
}))

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{node}</QueryClientProvider>
}

function renderEditor(onClose = vi.fn()) {
  const initial = emptyCampaign(false)
  initial.name = 'Launch'
  initial.subjects = { en: 'Hello' }
  initial.bodies = { en: '<p>Hi {{username}}</p>' }
  render(
    wrap(
      <CampaignEditor
        campaignId={null}
        initial={initial}
        canCreate
        canWrite
        canSend
        onClose={onClose}
      />
    )
  )
  return onClose
}

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 10,
    username: 'campaign-admin',
    role: 10,
    permissions: {
      admin_permissions: {
        adty_marketing: {
          read: true,
          create: true,
          write: true,
          delete: true,
          send: true,
        },
      },
    },
  })
  vi.clearAllMocks()
  api.estimateAudience.mockImplementation(
    async (audience: MarketingAudience) => ({
      total: audience.include_site_customers ? 12 : 5,
      by_lang: {},
    })
  )
  api.saveCampaign.mockResolvedValue({ id: 42 })
  api.getCampaignPlan.mockResolvedValue({
    total: 5,
    versions: { en: 3, 'zh-CN': 2 },
    sites: 1,
  })
  api.sendCampaign.mockResolvedValue({ total: 5, versions: {}, sites: 1 })
})

afterEach(() => useAuthStore.getState().auth.reset())

describe('CampaignEditor', () => {
  it('updates the estimated recipients when the filters change', async () => {
    const user = userEvent.setup()
    renderEditor()
    const estimate = screen.getByTestId('audience-estimate')
    await waitFor(() => expect(estimate).toHaveTextContent('5'))
    await user.click(
      screen.getByRole('checkbox', {
        name: 'Include sub-site customers (each site is sent separately)',
      })
    )
    await waitFor(() => expect(estimate).toHaveTextContent('12'))
    expect(api.estimateAudience).toHaveBeenLastCalledWith(
      expect.objectContaining({ include_site_customers: true })
    )
  })

  it('sends only after the confirmation shows recipients and language versions', async () => {
    const user = userEvent.setup()
    const onClose = renderEditor()
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    const plan = await screen.findByTestId('send-plan')
    expect(plan).toHaveTextContent('5 recipients across 1 site(s).')
    expect(plan).toHaveTextContent('English: 3')
    expect(plan).toHaveTextContent('简体中文: 2')
    expect(api.sendCampaign).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Send' }))
    await waitFor(() => expect(api.sendCampaign).toHaveBeenCalledWith(42, 5))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('does not send when the confirmation is dismissed', async () => {
    const user = userEvent.setup()
    renderEditor()
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    await screen.findByTestId('send-plan')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() =>
      expect(screen.queryByTestId('send-plan')).not.toBeInTheDocument()
    )
    expect(api.sendCampaign).not.toHaveBeenCalled()
  })

  it('cannot send without an English version', async () => {
    render(
      wrap(
        <CampaignEditor
          campaignId={null}
          initial={{
            ...emptyCampaign(false),
            name: 'x',
            subjects: { fr: 'Salut' },
          }}
          canCreate
          canWrite
          canSend
          onClose={vi.fn()}
        />
      )
    )
    expect(screen.getByRole('button', { name: 'Send…' })).toBeDisabled()
    expect(
      screen.getByText('The English version is required; it is the fallback.')
    ).toBeInTheDocument()
  })
})
