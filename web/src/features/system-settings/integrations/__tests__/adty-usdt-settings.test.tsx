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
import { describe, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AdtyBonusRatesEditor } from '../adty-bonus-rates-editor'
import { AdtyEpaySettingsSection } from '../adty-epay-settings-section'
import { BEpusdtSettingsSection } from '../bepusdt-settings-section'

const mutateAsync = vi.hoisted(() => vi.fn())
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

function BonusFixture(props: { onChange: (value: string) => void }) {
  const [value, setValue] = useState('{"10":1,"30":1.5}')
  return (
    <AdtyBonusRatesEditor
      value={value}
      onChange={(next) => {
        setValue(next)
        props.onChange(next)
      }}
    />
  )
}

function UsdtFixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <BEpusdtSettingsSection
        defaultValues={{
          enabled: true,
          baseUrl: 'https://pay.example',
          tradeTypes: '["usdt.trc20"]',
          orderMinutes: 15,
        }}
      />
    </SettingsPageProvider>
  )
}

describe('AdtyBonusRatesEditor', () => {
  it('saves tiers as a JSON object of amount to percent and accepts 8', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<BonusFixture onChange={onChange} />)
    await user.click(screen.getByRole('button', { name: 'Add bonus tier' }))
    await user.type(screen.getByLabelText('Recharge Amount (USD)'), '100000')
    await user.type(screen.getByLabelText('Bonus %'), '8')
    await user.click(screen.getByRole('button', { name: 'Add' }))
    expect(JSON.parse(onChange.mock.calls.at(-1)?.[0] as string)).toEqual({
      '10': 1,
      '30': 1.5,
      '100000': 8,
    })
  })

  it('saves an empty object after the last tier is deleted', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<BonusFixture onChange={onChange} />)
    for (const _ of [0, 1]) {
      await user.click(screen.getAllByRole('button', { name: 'Open menu' })[0])
      await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
    }
    expect(onChange.mock.calls.at(-1)?.[0]).toBe('{}')
    expect(screen.getByText('No bonus tiers configured')).toBeInTheDocument()
  })

  it('rejects a bonus above 100', async () => {
    const user = userEvent.setup()
    render(<BonusFixture onChange={() => {}} />)
    await user.click(screen.getByRole('button', { name: 'Add bonus tier' }))
    await user.type(screen.getByLabelText('Recharge Amount (USD)'), '10')
    await user.type(screen.getByLabelText('Bonus %'), '101')
    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
  })
})

describe('BEpusdtSettingsSection', () => {
  it('keeps the stored token when the field is empty and saves chains as IDs', async () => {
    mutateAsync.mockReset()
    const user = userEvent.setup()
    render(<UsdtFixture />)
    await user.click(screen.getByRole('checkbox', { name: 'Polygon' }))
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    const keys = mutateAsync.mock.calls.map((call) => call[0].key)
    expect(keys).not.toContain('adty_bepusdt.secret')
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'adty_bepusdt.trade_types',
      value: '["usdt.trc20","usdt.polygon"]',
    })
  })

  it('requires at least one chain', async () => {
    mutateAsync.mockReset()
    const user = userEvent.setup()
    render(<UsdtFixture />)
    await user.click(screen.getByRole('checkbox', { name: 'TRC20' }))
    expect(screen.getByText('Choose at least one chain')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})

function EpayFixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <AdtyEpaySettingsSection
        defaultValues={{
          orderMinutes: 10,
          channelTimeoutSeconds: 5,
          totalTimeoutSeconds: 12,
          queryAfterSeconds: 120,
          expiredQuerySeconds: 86400,
          unpaidThreshold: 10,
          probeEnabled: false,
          probeIntervalMinutes: 5,
        }}
      />
    </SettingsPageProvider>
  )
}

describe('AdtyEpaySettingsSection', () => {
  it('saves the total before the per-channel timeout when both grow', async () => {
    mutateAsync.mockReset()
    const user = userEvent.setup()
    render(<EpayFixture />)
    const channel = screen.getByLabelText('Per-channel timeout (seconds)')
    const total = screen.getByLabelText('Total ordering timeout (seconds)')
    await user.clear(channel)
    await user.type(channel, '15')
    await user.clear(total)
    await user.type(total, '20')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync.mock.calls.map((call) => call[0])).toEqual([
      { key: 'adty_epay_setting.total_timeout_seconds', value: 20 },
      { key: 'adty_epay_setting.channel_timeout_seconds', value: 15 },
    ])
  })

  it('blocks saving a per-channel timeout above the total', async () => {
    mutateAsync.mockReset()
    const user = userEvent.setup()
    render(<EpayFixture />)
    const channel = screen.getByLabelText('Per-channel timeout (seconds)')
    await user.clear(channel)
    await user.type(channel, '13')
    await user.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(mutateAsync).not.toHaveBeenCalled()
    expect(
      screen.getByLabelText('Total ordering timeout (seconds)')
    ).toHaveAttribute('aria-invalid', 'true')
  })
})
