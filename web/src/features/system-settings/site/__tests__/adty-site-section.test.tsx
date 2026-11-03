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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { getOptionValue } from '../../hooks/use-system-options'
import type { SiteSettings } from '../../types'
import { ADTY_SITE_OPTION_DEFAULTS } from '../adty-site-defaults'
import { getSiteSectionContent, SITE_SECTION_IDS } from '../section-registry'

const mutateAsync = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

// What GET /api/option/ returns after the settings were saved once.
const savedOptions = [
  { key: 'adty_site_setting.api_base_url', value: 'https://api.example.com' },
  { key: 'adty_site_setting.support_email', value: 'help@example.com' },
  { key: 'adty_site_setting.telegram_url', value: 'https://t.me/example' },
  { key: 'adty_site_setting.support_hours', value: 'Daily 10:00-24:00' },
  { key: 'adty_site_setting.tawk_property_id', value: 'prop-1' },
  { key: 'adty_site_setting.tawk_widget_id', value: 'widget-1' },
  { key: 'adty_site_setting.refund_policy_enabled', value: 'true' },
]

function Fixture(props: { children: ReactNode }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      {props.children}
    </SettingsPageProvider>
  )
}

function renderSection(options: Array<{ key: string; value: string }>) {
  // site/index.tsx spreads ADTY_SITE_OPTION_DEFAULTS into the page defaults;
  // the SiteSettings type makes the build fail if that spread goes missing.
  const settings = getOptionValue(
    options,
    ADTY_SITE_OPTION_DEFAULTS
  ) as SiteSettings
  render(<Fixture>{getSiteSectionContent('adty-site', settings)}</Fixture>)
}

afterEach(() => {
  mutateAsync.mockReset()
})

describe('ADTY AI site settings section', () => {
  it('is listed in the site settings', () => {
    expect(SITE_SECTION_IDS).toContain('adty-site')
  })

  it('reads back the saved values', () => {
    renderSection(savedOptions)

    expect(screen.getByLabelText('API base URL')).toHaveValue(
      'https://api.example.com'
    )
    expect(screen.getByLabelText('Support email')).toHaveValue(
      'help@example.com'
    )
    expect(screen.getByLabelText('Telegram link')).toHaveValue(
      'https://t.me/example'
    )
    expect(screen.getByLabelText('Support hours')).toHaveValue(
      'Daily 10:00-24:00'
    )
    expect(screen.getByLabelText('Tawk.to property ID')).toHaveValue('prop-1')
    expect(screen.getByLabelText('Tawk.to widget ID')).toHaveValue('widget-1')
    expect(
      screen.getByRole('switch', { name: 'Show the refund policy' })
    ).toBeChecked()
  })

  it('saves only the changed fields under adty_site_setting', async () => {
    const user = userEvent.setup()
    renderSection(savedOptions)

    await user.clear(screen.getByLabelText('Support hours'))
    await user.type(screen.getByLabelText('Support hours'), '  24/7  ')
    await user.click(
      screen.getByRole('switch', { name: 'Show the refund policy' })
    )
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mutateAsync).toHaveBeenCalledTimes(2)
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_site_setting.support_hours',
      value: '24/7',
    })
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_site_setting.refund_policy_enabled',
      value: false,
    })
  })

  it('does not save an API address that is not an http(s) URL', async () => {
    const user = userEvent.setup()
    renderSection([])

    await user.type(screen.getByLabelText('API base URL'), 'api.example.com')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(
      screen.getByText('Enter a full URL starting with http:// or https://')
    ).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
