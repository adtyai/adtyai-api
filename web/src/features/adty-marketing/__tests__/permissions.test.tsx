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
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { AdtyMarketing } from '..'
import {
  marketingKeys,
  type CampaignInput,
  type MarketingCampaign,
  type MarketingSettings,
  type PromoCode,
  type PromoCodeInput,
} from '../api'

type Grants = Partial<Record<'create' | 'write' | 'delete' | 'send', boolean>>
type Request = { method: string; url: string; body?: unknown }
type CampaignResponse = { data: { success: boolean; data: MarketingCampaign } }
const base = '/api/adty/admin/marketing'
const requests: Request[] = []
const clients: QueryClient[] = []
const initialAuth = useAuthStore.getState()
let pendingUpdate: Promise<CampaignResponse> | undefined

const campaign: MarketingCampaign = {
  id: 41,
  name: 'Existing campaign',
  is_template: false,
  status: 'draft',
  subjects: { en: 'Hello' },
  bodies: { en: '<p>Hello</p>' },
  audience: { include_site_customers: false },
  scheduled_at: 0,
  recipients: 0,
  error: '',
  created_at: 0,
  sent_at: 0,
}
const template: MarketingCampaign = {
  ...campaign,
  id: 42,
  name: 'Existing template',
  is_template: true,
}
const scheduled: MarketingCampaign = {
  ...campaign,
  id: 43,
  name: 'Scheduled campaign',
  status: 'scheduled',
}
const promo: PromoCode = {
  id: 51,
  code: 'EXISTING10',
  kind: 'percent',
  value: '10',
  min_face_usd: '0',
  max_uses: 0,
  per_user_limit: 1,
  used_count: 0,
  starts_at: 0,
  expires_at: 0,
  new_users_only: false,
  site_id: 0,
  funder: 'platform',
  enabled: true,
  note: '',
}
const settings: MarketingSettings = {
  enabled: true,
  from_address: 'Marketing <marketing@example.com>',
  resend_segment_prefix: '',
  consent_default_non_eu: false,
  consent_version: '1',
  eu_countries: '',
  footer_company: 'Example',
  footer_address: 'Example address',
  open_tracking: false,
  click_tracking: false,
  sync_batch_size: 100,
  sync_interval_seconds: 60,
  api_rate_per_second: 1,
  include_site_customers_default: false,
  promo_enabled: true,
  webhook_secret_configured: false,
  resend_key_configured: true,
  missing: [],
}
const plan = { total: 5, versions: { en: 5 }, sites: 1 }
const response = <T,>(data: T) => ({ data: { success: true, data } })

function setGrants(grants: Grants) {
  useAuthStore.getState().auth.setUser({
    id: 10,
    username: 'marketing-admin',
    role: 10,
    permissions: {
      admin_permissions: { adty_marketing: { read: true, ...grants } },
    },
  })
}

beforeEach(() => {
  requests.length = 0
  pendingUpdate = undefined
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === `${base}/settings`) {
      return response(settings)
    }
    if (url === `${base}/campaigns`) {
      const params = config?.params as { is_template?: boolean } | undefined
      const items = params?.is_template ? [template] : [campaign, scheduled]
      return response({ items, total: items.length, page: 1 })
    }
    if (url === `${base}/promo-codes`) {
      return response({ items: [promo], total: 1, page: 1 })
    }
    if (url.endsWith('/plan')) {
      return response(plan)
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url === `${base}/audience/estimate`) {
      return response({ total: 5, by_lang: { en: 5 } })
    }
    requests.push({ method: 'POST', url, body })
    if (url === `${base}/campaigns`) {
      return response({ ...campaign, ...(body as CampaignInput), id: 99 })
    }
    if (url === `${base}/promo-codes`) {
      return response({ ...promo, ...(body as PromoCodeInput), id: 88 })
    }
    if (url.endsWith('/send')) {
      return response(plan)
    }
    if (url.endsWith('/test') || url.endsWith('/cancel')) {
      return response(null)
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  vi.spyOn(api, 'put').mockImplementation(async (url, body) => {
    requests.push({ method: 'PUT', url, body })
    if (url === `${base}/campaigns/41` || url === `${base}/campaigns/42`) {
      if (pendingUpdate) {
        return pendingUpdate
      }
      return response({
        ...campaign,
        ...(body as CampaignInput),
        id: Number(url.split('/').at(-1)),
      })
    }
    if (url === `${base}/promo-codes/51`) {
      return response({ ...promo, ...(body as PromoCodeInput) })
    }
    throw new Error(`Unexpected PUT ${url}`)
  })
  vi.spyOn(api, 'delete').mockImplementation(async (url) => {
    requests.push({ method: 'DELETE', url })
    return response(null)
  })
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(initialAuth)
  localStorage.clear()
})

async function renderMarketing(
  grants: Grants,
  tab = 'Campaigns',
  providedClient?: QueryClient
) {
  setGrants(grants)
  const client =
    providedClient ??
    new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
  clients.push(client)
  client.setQueryData(marketingKeys.settings, settings)
  client.setQueryData(marketingKeys.campaigns({ p: 1, is_template: false }), {
    items: [campaign, scheduled],
    total: 2,
    page: 1,
  })
  client.setQueryData(marketingKeys.campaigns({ p: 1, is_template: true }), {
    items: [template],
    total: 1,
    page: 1,
  })
  client.setQueryData(marketingKeys.promoCodes(1), {
    items: [promo],
    total: 1,
    page: 1,
  })
  render(
    <QueryClientProvider client={client}>
      <AdtyMarketing />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  if (tab !== 'Campaigns') {
    await user.click(screen.getByRole('tab', { name: tab }))
  }
  return user
}

describe.each([
  {
    tab: 'Campaigns',
    newLabel: 'New campaign',
    saveLabel: 'Save draft',
    id: 41,
    name: 'Existing campaign',
  },
  {
    tab: 'Templates',
    newLabel: 'New template',
    saveLabel: 'Save template',
    id: 42,
    name: 'Existing template',
  },
])('$tab action permissions', (kind) => {
  it('creates with Create alone and cannot reuse that grant for a later update', async () => {
    const user = await renderMarketing({ create: true }, kind.tab)
    await user.click(screen.getByRole('button', { name: kind.newLabel }))
    await user.type(
      screen.getByLabelText('Name (only admins see it)'),
      'New entry'
    )
    await user.click(screen.getByRole('button', { name: kind.saveLabel }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      url: `${base}/campaigns`,
      body: expect.objectContaining({
        name: 'New entry',
        is_template: kind.tab === 'Templates',
      }),
    })
    await waitFor(() =>
      expect(screen.getByLabelText('Name (only admins see it)')).toBeDisabled()
    )
    expect(
      screen.queryByRole('button', { name: kind.saveLabel })
    ).not.toBeInTheDocument()
  })

  it('uses Write only for an existing entry and hides creation and deletion', async () => {
    const user = await renderMarketing({ write: true }, kind.tab)
    expect(
      screen.queryByRole('button', { name: kind.newLabel })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Use template' })
    ).not.toBeInTheDocument()
    await user.click(
      within(
        screen.getByRole('row', { name: new RegExp(kind.name) })
      ).getByRole('button', { name: 'Edit' })
    )
    const name = screen.getByLabelText('Name (only admins see it)')
    await user.clear(name)
    await user.type(name, 'Updated entry')
    await user.click(screen.getByRole('button', { name: kind.saveLabel }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'PUT',
      url: `${base}/campaigns/${kind.id}`,
      body: expect.objectContaining({ name: 'Updated entry' }),
    })
  })

  it('deletes with Delete alone without granting editing or creation', async () => {
    const user = await renderMarketing({ delete: true }, kind.tab)
    expect(
      screen.queryByRole('button', { name: kind.newLabel })
    ).not.toBeInTheDocument()
    const row = within(screen.getByRole('row', { name: new RegExp(kind.name) }))
    expect(row.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument()
    await user.click(row.getByRole('button', { name: 'Delete' }))

    await waitFor(() =>
      expect(requests).toEqual([
        { method: 'DELETE', url: `${base}/campaigns/${kind.id}` },
      ])
    )
  })
})

describe('campaign authorization at request boundaries', () => {
  it('preserves an edited campaign when switching to another tab and back', async () => {
    const user = await renderMarketing({ create: true, write: true })
    await user.click(screen.getByRole('button', { name: 'New campaign' }))
    await user.type(
      screen.getByLabelText('Name (only admins see it)'),
      'Unsaved campaign'
    )
    await user.click(screen.getByRole('tab', { name: 'Templates' }))
    await user.click(screen.getByRole('tab', { name: 'Campaigns' }))

    expect(screen.getByLabelText('Name (only admins see it)')).toHaveValue(
      'Unsaved campaign'
    )
  })

  it('confirms before discarding an edited campaign through Back', async () => {
    const user = await renderMarketing({ create: true })
    await user.click(screen.getByRole('button', { name: 'New campaign' }))
    await user.type(
      screen.getByLabelText('Name (only admins see it)'),
      'Unsaved campaign'
    )
    await user.click(screen.getByRole('button', { name: 'Back' }))
    await user.click(await screen.findByRole('button', { name: 'Stay' }))
    expect(screen.getByLabelText('Name (only admins see it)')).toHaveValue(
      'Unsaved campaign'
    )
    await user.click(screen.getByRole('button', { name: 'Back' }))
    await user.click(await screen.findByRole('button', { name: 'Leave' }))

    expect(
      screen.getByRole('button', { name: 'New campaign' })
    ).toBeInTheDocument()
  })

  it('offers cancellation for a failed campaign and confirms before dispatch', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    const user = await renderMarketing({ send: true }, 'Campaigns', client)
    act(() =>
      client.setQueryData(
        marketingKeys.campaigns({ p: 1, is_template: false }),
        {
          items: [
            { ...scheduled, status: 'failed', name: 'Interrupted campaign' },
          ],
          total: 1,
          page: 1,
        }
      )
    )
    const row = within(
      await screen.findByRole('row', { name: /Interrupted campaign/ })
    )
    await user.click(row.getByRole('button', { name: 'Cancel' }))
    expect(requests).toEqual([])
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }))

    await waitFor(() =>
      expect(requests).toEqual([
        { method: 'POST', url: `${base}/campaigns/43/cancel`, body: {} },
      ])
    )
  })

  it('uses a template to create a new campaign with Create but no Write', async () => {
    const user = await renderMarketing({ create: true }, 'Templates')
    await user.click(screen.getByRole('button', { name: 'Use template' }))
    await user.click(screen.getByRole('button', { name: 'Save draft' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      url: `${base}/campaigns`,
      body: expect.objectContaining({
        is_template: false,
        name: 'Existing template',
        audience: { include_site_customers: false },
      }),
    })
  })

  it.each([
    { name: 'Create', grants: { create: true } },
    { name: 'Send', grants: { send: true } },
  ])(
    'does not grant existing edits or saves through $name alone',
    async (mode) => {
      const user = await renderMarketing(mode.grants)
      await user.click(
        within(
          screen.getByRole('row', { name: /Existing campaign/ })
        ).getByRole('button', { name: 'View' })
      )

      expect(screen.getByLabelText('Name (only admins see it)')).toBeDisabled()
      expect(
        screen.queryByRole('button', { name: 'Save draft' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Send…' })
      ).not.toBeInTheDocument()
      expect(requests).toEqual([])
    }
  )

  it('saves an existing campaign before planning and confirming Send', async () => {
    const user = await renderMarketing({ write: true, send: true })
    await user.click(
      within(screen.getByRole('row', { name: /Existing campaign/ })).getByRole(
        'button',
        { name: 'Edit' }
      )
    )
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Send this campaign?',
    })
    expect(api.get).toHaveBeenCalledWith(`${base}/campaigns/41/plan`)
    expect(requests).toEqual([
      {
        method: 'PUT',
        url: `${base}/campaigns/41`,
        body: expect.objectContaining({ name: 'Existing campaign' }),
      },
    ])
    await user.click(within(dialog).getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(requests).toHaveLength(2))
    expect(requests[1]).toEqual({
      method: 'POST',
      url: `${base}/campaigns/41/send`,
      body: { confirm: true, recipients: 5 },
    })
  })

  it('creates before sending a new campaign without granting later Write access', async () => {
    const user = await renderMarketing({ create: true, send: true })
    await user.click(screen.getByRole('button', { name: 'New campaign' }))
    await user.type(
      screen.getByLabelText('Name (only admins see it)'),
      'New send'
    )
    await user.type(screen.getByLabelText('Subject in English'), 'Hello')
    await user.type(
      screen.getByLabelText('Body in English (HTML)'),
      '<p>Hello</p>'
    )
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Send this campaign?',
    })
    expect(api.get).toHaveBeenCalledWith(`${base}/campaigns/99/plan`)
    expect(requests).toEqual([
      {
        method: 'POST',
        url: `${base}/campaigns`,
        body: expect.objectContaining({ name: 'New send' }),
      },
    ])
    await user.click(within(dialog).getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(requests).toHaveLength(2))
    expect(requests[1]).toEqual({
      method: 'POST',
      url: `${base}/campaigns/99/send`,
      body: { confirm: true, recipients: 5 },
    })
    expect(requests.some((request) => request.method === 'PUT')).toBe(false)
  })

  it('blocks an open send confirmation after Send permission is revoked', async () => {
    const user = await renderMarketing({ write: true, send: true })
    await user.click(
      within(screen.getByRole('row', { name: /Existing campaign/ })).getByRole(
        'button',
        { name: 'Edit' }
      )
    )
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Send this campaign?',
    })

    act(() => setGrants({ write: true }))
    const send = within(dialog).getByRole('button', { name: 'Send' })
    await user.click(send)

    expect(requests).toHaveLength(1)
    expect(send).toBeDisabled()
  })

  it('does not send a test after Send is revoked while the required save is pending', async () => {
    let finishSave = (_response: CampaignResponse) => {}
    pendingUpdate = new Promise<CampaignResponse>((resolve) => {
      finishSave = resolve
    })
    const user = await renderMarketing({ write: true, send: true })
    await user.click(
      within(screen.getByRole('row', { name: /Existing campaign/ })).getByRole(
        'button',
        { name: 'Edit' }
      )
    )
    await user.type(
      screen.getByLabelText('Send a test to'),
      'reader@example.com'
    )
    await user.click(screen.getByRole('button', { name: 'Send test' }))
    await waitFor(() => expect(requests).toHaveLength(1))

    act(() => setGrants({ write: true }))
    await act(async () => {
      finishSave(response(campaign))
    })

    expect(requests).toEqual([
      { method: 'PUT', url: `${base}/campaigns/41`, body: expect.any(Object) },
    ])
  })

  it('does not request a send plan after Send is revoked during saving', async () => {
    let finishSave = (_response: CampaignResponse) => {}
    pendingUpdate = new Promise<CampaignResponse>((resolve) => {
      finishSave = resolve
    })
    const user = await renderMarketing({ write: true, send: true })
    await user.click(
      within(screen.getByRole('row', { name: /Existing campaign/ })).getByRole(
        'button',
        { name: 'Edit' }
      )
    )
    await user.click(screen.getByRole('button', { name: 'Send…' }))
    await waitFor(() => expect(requests).toHaveLength(1))

    act(() => setGrants({ write: true }))
    await act(async () => {
      finishSave(response(campaign))
    })

    expect(api.get).not.toHaveBeenCalledWith(`${base}/campaigns/41/plan`)
    expect(
      screen.queryByRole('alertdialog', { name: 'Send this campaign?' })
    ).not.toBeInTheDocument()
  })

  it('blocks an open scheduled-campaign cancellation after Send is revoked', async () => {
    const user = await renderMarketing({ send: true })
    await user.click(
      within(screen.getByRole('row', { name: /Scheduled campaign/ })).getByRole(
        'button',
        { name: 'Cancel' }
      )
    )
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Cancel this campaign?',
    })

    act(() => setGrants({}))
    const confirm = within(dialog).getByRole('button', { name: 'Continue' })
    await user.click(confirm)

    expect(requests).toEqual([])
    expect(confirm).toBeDisabled()
  })
})

describe('promo code action permissions', () => {
  it('creates a promo code with Create and no Write', async () => {
    const user = await renderMarketing({ create: true }, 'Promo codes')
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'New promo code' }))
    await user.type(screen.getByLabelText('Code'), 'spring10')
    await user.type(screen.getByLabelText('Bonus %'), '10')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'POST',
      url: `${base}/promo-codes`,
      body: expect.objectContaining({ code: 'SPRING10', value: '10' }),
    })
  })

  it('updates an existing promo code with Write and no Create', async () => {
    const user = await renderMarketing({ write: true }, 'Promo codes')
    expect(
      screen.queryByRole('button', { name: 'New promo code' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Edit' }))
    const bonus = screen.getByLabelText('Bonus %')
    await user.clear(bonus)
    await user.type(bonus, '15')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).toEqual({
      method: 'PUT',
      url: `${base}/promo-codes/51`,
      body: expect.objectContaining({ code: 'EXISTING10', value: '15' }),
    })
  })

  it('does not offer promo mutations with no Create or Write action', async () => {
    await renderMarketing({ delete: true, send: true }, 'Promo codes')

    expect(
      screen.queryByRole('button', { name: 'New promo code' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete' })
    ).not.toBeInTheDocument()
    expect(requests).toEqual([])
  })

  it.each([
    {
      creating: true,
      grants: { create: true, write: true },
      label: 'New promo code',
    },
    { creating: false, grants: { write: true }, label: 'Edit' },
  ])(
    'blocks an open promo save after its required action is revoked ($creating)',
    async (mode) => {
      const user = await renderMarketing(mode.grants, 'Promo codes')
      await user.click(screen.getByRole('button', { name: mode.label }))
      if (mode.creating) {
        await user.type(screen.getByLabelText('Code'), 'spring10')
        await user.type(screen.getByLabelText('Bonus %'), '10')
      }
      const save = screen.getByRole('button', { name: 'Save' })

      act(() => setGrants(mode.creating ? { write: true } : { create: true }))
      await user.click(save)

      expect(requests).toEqual([])
      expect(save).toBeDisabled()
    }
  )
})

describe('unmounted marketing request permissions', () => {
  it.each([
    { kind: 'campaign create', grants: { create: true } },
    { kind: 'campaign update', grants: { write: true } },
    { kind: 'campaign delete', grants: { delete: true } },
    { kind: 'campaign cancel', grants: { send: true } },
    { kind: 'campaign send', grants: { write: true, send: true } },
    { kind: 'promo create', grants: { create: true } },
    { kind: 'promo update', grants: { write: true } },
  ])('rejects queued $kind after unmount and revocation', async (mode) => {
    let held = false
    let resume = () => {}
    const gate = new Promise<void>((resolve) => {
      resume = resolve
    })
    const entered = vi.fn()
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity },
        mutations: {
          onMutate: async () => {
            if (held) {
              entered()
              await gate
            }
          },
        },
      },
    })
    const user = await renderMarketing(
      mode.grants,
      mode.kind.startsWith('promo') ? 'Promo codes' : 'Campaigns',
      client
    )
    let dispatch: () => Promise<void>
    if (mode.kind === 'campaign create') {
      await user.click(screen.getByRole('button', { name: 'New campaign' }))
      await user.type(
        screen.getByLabelText('Name (only admins see it)'),
        'Queued campaign'
      )
      dispatch = () =>
        user.click(screen.getByRole('button', { name: 'Save draft' }))
    } else if (
      mode.kind === 'campaign update' ||
      mode.kind === 'campaign send'
    ) {
      await user.click(
        within(
          screen.getByRole('row', { name: /Existing campaign/ })
        ).getByRole('button', { name: 'Edit' })
      )
      if (mode.kind === 'campaign send') {
        await user.click(screen.getByRole('button', { name: 'Send…' }))
        const dialog = await screen.findByRole('alertdialog', {
          name: 'Send this campaign?',
        })
        dispatch = () =>
          user.click(within(dialog).getByRole('button', { name: 'Send' }))
      } else {
        dispatch = () =>
          user.click(screen.getByRole('button', { name: 'Save draft' }))
      }
    } else if (mode.kind === 'campaign delete') {
      dispatch = () =>
        user.click(
          within(
            screen.getByRole('row', { name: /Existing campaign/ })
          ).getByRole('button', { name: 'Delete' })
        )
    } else if (mode.kind === 'campaign cancel') {
      await user.click(
        within(
          screen.getByRole('row', { name: /Scheduled campaign/ })
        ).getByRole('button', { name: 'Cancel' })
      )
      const dialog = await screen.findByRole('alertdialog', {
        name: 'Cancel this campaign?',
      })
      dispatch = () =>
        user.click(within(dialog).getByRole('button', { name: 'Continue' }))
    } else {
      await user.click(
        screen.getByRole('button', {
          name: mode.kind === 'promo create' ? 'New promo code' : 'Edit',
        })
      )
      if (mode.kind === 'promo create') {
        await user.type(screen.getByLabelText('Code'), 'queued10')
        await user.type(screen.getByLabelText('Bonus %'), '10')
      }
      dispatch = () => user.click(screen.getByRole('button', { name: 'Save' }))
    }
    const before = [...requests]
    held = true
    await dispatch()
    await waitFor(() => expect(entered).toHaveBeenCalledTimes(1))
    act(() => {
      cleanup()
      setGrants({})
    })
    await act(async () => {
      resume()
    })
    await waitFor(() => expect(client.isMutating()).toBe(0))
    expect(requests).toEqual(before)
  })

  it.each(['test', 'plan'] as const)(
    'does not continue %s after save resolves following unmount and Send revocation',
    async (action) => {
      let finishSave = (_response: CampaignResponse) => {}
      pendingUpdate = new Promise<CampaignResponse>((resolve) => {
        finishSave = resolve
      })
      const user = await renderMarketing({ write: true, send: true })
      await user.click(
        within(
          screen.getByRole('row', { name: /Existing campaign/ })
        ).getByRole('button', { name: 'Edit' })
      )
      if (action === 'test') {
        await user.type(
          screen.getByLabelText('Send a test to'),
          'reader@example.com'
        )
      }
      await user.click(
        screen.getByRole('button', {
          name: action === 'test' ? 'Send test' : 'Send…',
        })
      )
      await waitFor(() => expect(requests).toHaveLength(1))
      const client = clients.at(-1)
      if (!client) throw new Error('Marketing query client is missing')
      act(() => {
        cleanup()
        setGrants({ write: true })
      })
      await act(async () => {
        finishSave(response(campaign))
      })
      await waitFor(() => expect(client.isMutating()).toBe(0))
      expect(requests).toEqual([
        {
          method: 'PUT',
          url: `${base}/campaigns/41`,
          body: expect.any(Object),
        },
      ])
      expect(api.get).not.toHaveBeenCalledWith(`${base}/campaigns/41/plan`)
    }
  )
})
