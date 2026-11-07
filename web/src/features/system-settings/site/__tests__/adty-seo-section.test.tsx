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
import { ADTY_SEO_OPTION_DEFAULTS } from '../adty-seo-defaults'
import { getSiteSectionContent, SITE_SECTION_IDS } from '../section-registry'

const mutateAsync = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

// What GET /api/option/ returns after the SEO settings were saved once.
const savedOptions = [
  {
    key: 'adty_seo_setting.home_title',
    value: '{"en":"ADTY AI - one key","zh-CN":"ADTY AI 一个密钥"}',
  },
  {
    key: 'adty_seo_setting.home_description',
    value: '{"en":"English description"}',
  },
  { key: 'adty_seo_setting.og_image', value: '/custom-og.png' },
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
  const settings = getOptionValue(
    options,
    ADTY_SEO_OPTION_DEFAULTS
  ) as SiteSettings
  render(<Fixture>{getSiteSectionContent('adty-seo', settings)}</Fixture>)
}

afterEach(() => {
  mutateAsync.mockReset()
})

describe('home page SEO settings section', () => {
  it('is listed in the site settings', () => {
    expect(SITE_SECTION_IDS).toContain('adty-seo')
  })

  it('describes all eight home language URLs', () => {
    renderSection([])

    expect(screen.getByText(/eight home page URLs/)).toBeInTheDocument()
  })

  it('reads back the saved English title, description and share image', () => {
    renderSection(savedOptions)

    expect(screen.getByLabelText('Title (English)')).toHaveValue(
      'ADTY AI - one key'
    )
    expect(screen.getByLabelText('Description (English)')).toHaveValue(
      'English description'
    )
    expect(screen.getByLabelText('Share image')).toHaveValue('/custom-og.png')
  })

  it('reads back another language from its tab', async () => {
    const user = userEvent.setup()
    renderSection(savedOptions)

    await user.click(screen.getByRole('tab', { name: '简体中文' }))

    expect(screen.getByLabelText('Title (简体中文)')).toHaveValue(
      'ADTY AI 一个密钥'
    )
    expect(screen.getByLabelText('Description (简体中文)')).toHaveValue('')
  })

  it('saves the changed language map as JSON without empty languages', async () => {
    const user = userEvent.setup()
    renderSection(savedOptions)

    await user.click(screen.getByRole('tab', { name: 'Français' }))
    await user.type(screen.getByLabelText('Title (Français)'), 'ADTY AI FR')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mutateAsync).toHaveBeenCalledTimes(1)
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_seo_setting.home_title',
      value:
        '{"en":"ADTY AI - one key","zh-CN":"ADTY AI 一个密钥","fr":"ADTY AI FR"}',
    })
  })

  it('uses the saved values as the baseline for later saves', async () => {
    const user = userEvent.setup()
    renderSection(savedOptions)

    await user.click(screen.getByRole('tab', { name: /Fran/ }))
    await user.type(
      screen.getByRole('textbox', { name: /Title/ }),
      'ADTY AI FR'
    )
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    mutateAsync.mockClear()
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).not.toHaveBeenCalled()

    await user.clear(screen.getByLabelText('Share image'))
    await user.type(screen.getByLabelText('Share image'), '/new-og.png')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).toHaveBeenCalledTimes(1)
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_seo_setting.og_image',
      value: '/new-og.png',
    })
  })

  it('warns about a title over 60 characters but still saves it', async () => {
    const user = userEvent.setup()
    renderSection([])

    await user.type(screen.getByLabelText('Title (English)'), 'x'.repeat(61))

    expect(
      screen.getByText('61 characters; 60 or fewer is recommended.')
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_seo_setting.home_title',
      value: JSON.stringify({ en: 'x'.repeat(61) }),
    })
  })
})
