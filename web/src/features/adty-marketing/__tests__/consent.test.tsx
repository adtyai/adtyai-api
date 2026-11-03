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
import { useState, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { MarketingSettings } from '../api'
import { MarketingConsentCheckbox } from '../components/marketing-consent-checkbox'
import { SettingsPanel } from '../components/settings-panel'
import {
  buildMarketingSettingsUpdate,
  toMarketingDrafts,
} from '../lib/settings'

const api = vi.hoisted(() => ({
  getMarketingConsentDefault: vi.fn(),
  getMarketingSettings: vi.fn(),
  saveMarketingSettings: vi.fn(),
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

function ConsentFixture(props: { onChange?: (value: boolean) => void }) {
  const [checked, setChecked] = useState(false)
  return (
    <MarketingConsentCheckbox
      checked={checked}
      onCheckedChange={(value) => {
        setChecked(value)
        props.onChange?.(value)
      }}
    />
  )
}

const settings: MarketingSettings = {
  enabled: false,
  from_address: '',
  resend_segment_prefix: 'adty',
  consent_default_non_eu: false,
  consent_version: '1',
  eu_countries: 'DE,FR',
  footer_company: '',
  footer_address: '',
  open_tracking: true,
  click_tracking: true,
  sync_batch_size: 100,
  sync_interval_seconds: 60,
  api_rate_per_second: 8,
  include_site_customers_default: false,
  promo_enabled: false,
  webhook_secret_configured: false,
  resend_key_configured: false,
  missing: [
    'from_address',
    'footer_company',
    'footer_address',
    'webhook_secret',
    'resend_api_key',
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('MarketingConsentCheckbox', () => {
  it('stays hidden while marketing emails are off', async () => {
    api.getMarketingConsentDefault.mockResolvedValue({
      show: false,
      checked: true,
    })
    render(wrap(<ConsentFixture />))
    await waitFor(() =>
      expect(api.getMarketingConsentDefault).toHaveBeenCalled()
    )
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('starts unticked when the server says the visitor may be in the EU', async () => {
    api.getMarketingConsentDefault.mockResolvedValue({
      show: true,
      checked: false,
    })
    render(wrap(<ConsentFixture />))
    const box = await screen.findByRole('checkbox')
    expect(box).not.toBeChecked()
  })

  it('starts ticked outside the EU when configured, and the user can untick it', async () => {
    api.getMarketingConsentDefault.mockResolvedValue({
      show: true,
      checked: true,
    })
    const onChange = vi.fn()
    render(wrap(<ConsentFixture onChange={onChange} />))
    const box = await screen.findByRole('checkbox')
    await waitFor(() => expect(box).toBeChecked())
    await userEvent.setup().click(box)
    expect(box).not.toBeChecked()
    expect(onChange).toHaveBeenLastCalledWith(false)
  })
})

describe('Marketing SettingsPanel', () => {
  it('marks required blanks as not configured and keeps the switch off', async () => {
    api.getMarketingSettings.mockResolvedValue(settings)
    render(wrap(<SettingsPanel canWrite />))
    const toggle = await screen.findByRole('switch', {
      name: 'Marketing emails',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('switch', { name: 'Promo codes' })
    ).not.toBeChecked()
    // sender, two footer fields, webhook secret and Resend API key
    expect(screen.getAllByText('Not configured')).toHaveLength(5)
  })

  it('allows the switch once every required field is filled', async () => {
    api.getMarketingSettings.mockResolvedValue({
      ...settings,
      resend_key_configured: true,
      webhook_secret_configured: true,
      missing: ['from_address', 'footer_company', 'footer_address'],
    })
    const user = userEvent.setup()
    render(wrap(<SettingsPanel canWrite />))
    const toggle = await screen.findByRole('switch', {
      name: 'Marketing emails',
    })
    await user.type(
      screen.getByLabelText('Marketing sender address'),
      'ADTY AI <news@news.example.com>'
    )
    await user.type(
      screen.getByLabelText('Sender name in the email footer'),
      'ADTY Ltd'
    )
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.type(
      screen.getByLabelText('Contact address in the email footer'),
      '1 Example Road'
    )
    expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('is read-only without the edit permission', async () => {
    api.getMarketingSettings.mockResolvedValue(settings)
    render(wrap(<SettingsPanel canWrite={false} />))
    expect(
      await screen.findByLabelText('Marketing sender address')
    ).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Save Changes' })
    ).not.toBeInTheDocument()
  })
})

describe('buildMarketingSettingsUpdate', () => {
  it('sends only changed fields and leaves a blank secret alone', () => {
    const drafts = toMarketingDrafts(settings)
    drafts.footer_company = ' ADTY Ltd '
    drafts.sync_batch_size = '50'
    drafts.open_tracking = false
    expect(buildMarketingSettingsUpdate(settings, drafts)).toEqual({
      footer_company: 'ADTY Ltd',
      sync_batch_size: 50,
      open_tracking: false,
    })
  })

  it('sends the promo code switch without turning marketing emails on', () => {
    const drafts = toMarketingDrafts(settings)
    drafts.promo_enabled = true
    expect(buildMarketingSettingsUpdate(settings, drafts)).toEqual({
      promo_enabled: true,
    })
  })

  it('does not send the webhook secret with the other settings', () => {
    const drafts = toMarketingDrafts(settings)
    drafts.webhook_secret = 'whsec_abc'
    expect(buildMarketingSettingsUpdate(settings, drafts)).toEqual({})
  })
})
