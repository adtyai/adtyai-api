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
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AdtyLegalSection, type AdtyLegalDefaults } from '../adty-legal-section'

const mutateAsync = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

const saved: AdtyLegalDefaults = {
  user_agreement: '{"en":"English terms","zh-CN":"中文协议"}',
  privacy_policy: '{}',
  refund_policy: '{"en":"Old refund"}',
}

function Fixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <AdtyLegalSection defaultValues={saved} />
    </SettingsPageProvider>
  )
}

afterEach(() => {
  mutateAsync.mockReset()
})

describe('AdtyLegalSection', () => {
  it('shows the saved text for the chosen document and language', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    expect(screen.getByRole('textbox')).toHaveValue('English terms')
    await user.click(screen.getByRole('tab', { name: '简体中文' }))
    expect(screen.getByRole('textbox')).toHaveValue('中文协议')
  })

  it('saves only the changed document as language to Markdown, dropping empty languages', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    await user.selectOptions(screen.getByLabelText('Document'), 'refund_policy')
    await user.clear(screen.getByRole('textbox'))
    await user.click(screen.getByRole('tab', { name: 'Français' }))
    await user.type(screen.getByRole('textbox'), 'Remboursement')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(mutateAsync).toHaveBeenCalledTimes(1)
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_legal_setting.refund_policy',
      value: '{"fr":"Remboursement"}',
    })
  })

  it('saves nothing when no text changed', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
