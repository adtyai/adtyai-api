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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { CONTENT_LANG_LABELS, CONTENT_LANGS } from '@/lib/adty-lang'
import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AdtyLegalSection, type AdtyLegalDefaults } from '../adty-legal-section'

const saved = {
  user_agreement: '{"en":"Original agreement"}',
  privacy_policy: '{}',
  refund_policy: '{"en":"Original refund"}',
  site_agreement: '{}',
  site_customer_terms: '{}',
  site_agreement_version: 0,
  site_customer_terms_version: 0,
}
const clients: QueryClient[] = []

function Fixture(props: { values: typeof saved; client: QueryClient }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <QueryClientProvider client={props.client}>
      <SettingsPageProvider actionsContainer={actions}>
        <div ref={setActions} />
        <AdtyLegalSection defaultValues={props.values as AdtyLegalDefaults} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  return {
    client,
    view: render(<Fixture values={saved} client={client} />),
    user: userEvent.setup(),
  }
}

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

it.each(['site_agreement', 'site_customer_terms'])(
  'publishes all eight languages and the version for %s through legal options',
  async (field) => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user } = mount()
    await user.selectOptions(screen.getByLabelText('Document'), field)
    const texts: Record<string, string> = {}
    for (const lang of CONTENT_LANGS) {
      await user.click(
        screen.getByRole('tab', { name: CONTENT_LANG_LABELS[lang] })
      )
      texts[lang] = `Legal text ${lang}`
      await user.type(screen.getByRole('textbox'), texts[lang])
    }
    await user.clear(screen.getByLabelText('Document version'))
    await user.type(screen.getByLabelText('Document version'), '3')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
    expect(put).toHaveBeenNthCalledWith(1, '/api/option/', {
      key: `adty_legal_setting.${field}`,
      value: JSON.stringify(texts),
    })
    expect(put).toHaveBeenNthCalledWith(2, '/api/option/', {
      key: `adty_legal_setting.${field}_version`,
      value: '3',
    })
  }
)

it('keeps another document draft when a saved document refreshes', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const { user, client, view } = mount()
  await user.selectOptions(screen.getByLabelText('Document'), 'refund_policy')
  await user.clear(screen.getByRole('textbox'))
  await user.type(screen.getByRole('textbox'), 'Unsaved refund')
  await user.selectOptions(screen.getByLabelText('Document'), 'user_agreement')
  await user.clear(screen.getByRole('textbox'))
  await user.type(screen.getByRole('textbox'), 'Saved agreement')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put).toHaveBeenCalledWith('/api/option/', {
    key: 'adty_legal_setting.user_agreement',
    value: '{"en":"Saved agreement"}',
  })
  view.rerender(
    <Fixture
      values={{ ...saved, user_agreement: '{"en":"Saved agreement"}' }}
      client={client}
    />
  )
  await user.selectOptions(screen.getByLabelText('Document'), 'refund_policy')
  expect(screen.getByRole('textbox')).toHaveValue('Unsaved refund')
})

it('preserves edits made while a document save is pending', async () => {
  let resolveSave: (value: unknown) => void = () => {}
  vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveSave = resolve
      })
  )
  const { user, client, view } = mount()
  await user.clear(screen.getByRole('textbox'))
  await user.type(screen.getByRole('textbox'), 'Submitted agreement')
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await user.type(screen.getByRole('textbox'), ' plus newer draft')
  resolveSave({ data: { success: true } })
  await screen.findByRole('button', { name: 'Save Changes' })
  view.rerender(
    <Fixture
      values={{ ...saved, user_agreement: '{"en":"Submitted agreement"}' }}
      client={client}
    />
  )
  expect(screen.getByRole('textbox')).toHaveValue(
    'Submitted agreement plus newer draft'
  )
})

it.each(['-1', '1.5'])(
  'rejects invalid agreement version %s',
  async (version) => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user } = mount()
    await user.selectOptions(
      screen.getByLabelText('Document'),
      'site_agreement'
    )
    await user.clear(screen.getByLabelText('Document version'))
    await user.type(screen.getByLabelText('Document version'), version)
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Version must be a non-negative integer.'
    )
    expect(put).not.toHaveBeenCalled()
  }
)
