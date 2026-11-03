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
import { zodResolver } from '@hookform/resolvers/zod'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  within,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { z } from 'zod'

import { GroupDisplaySync } from '@/features/adty-groups/components/group-display-sync'
import { useGroupDisplayStore } from '@/stores/adty-group-display-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { positiveIntegerSchema } from '../../utils/numeric-field'
import { GroupRatioForm } from '../group-ratio-form'

const groupApi = vi.hoisted(() => ({
  getAdminGroups: vi.fn(),
  getGroupDisplay: vi.fn(),
  createAdminGroup: vi.fn(),
  updateAdminGroup: vi.fn(),
  migrateAdminGroup: vi.fn(),
  deleteAdminGroup: vi.fn(),
}))
vi.mock('@/features/adty-groups/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/adty-groups/api')>()),
  ...groupApi,
}))

function adminGroup(id: string, en: string) {
  return {
    id,
    names: { en },
    descs: {},
    refs: { users: 0, tokens: 0, channels: 0 },
  }
}

beforeEach(() => {
  useGroupDisplayStore.setState({ groups: [] })
  const groups = [adminGroup('default', 'Default'), adminGroup('vip', 'VIP')]
  groupApi.getAdminGroups.mockResolvedValue(groups)
  groupApi.getGroupDisplay.mockResolvedValue(
    groups.map((g) => ({ ...g, usable: true, visible: true }))
  )
})

const defaults = {
  GroupRatio: '{"default":1,"vip":0.8}',
  TopupGroupRatio: '{"vip":1.2}',
  UserUsableGroups: '{"default":"Standard access","vip":"Premium access"}',
  GroupGroupRatio: '{}',
  AutoGroups: '["default","vip"]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
}

const schema = z.object({
  GroupRatio: z.string(),
  TopupGroupRatio: z.string(),
  UserUsableGroups: z.string(),
  GroupGroupRatio: z.string(),
  AutoGroups: z.string(),
  MaxTokenAutoGroups: positiveIntegerSchema('Enter a positive integer'),
  DefaultUseAutoGroup: z.boolean(),
  GroupSpecialUsableGroup: z.string(),
})

function Fixture(props: {
  onSave?: (values: typeof defaults) => Promise<void>
  initial?: Partial<typeof defaults>
  isSaving?: boolean
}) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
  )
  const form = useForm({
    defaultValues: { ...defaults, ...props.initial },
    resolver: zodResolver(schema),
  })
  return (
    <QueryClientProvider client={client}>
      <GroupDisplaySync />
      <SettingsPageProvider actionsContainer={actions}>
        <div ref={setActions} />
        <GroupRatioForm
          form={form}
          onSave={props.onSave ?? (async () => {})}
          isSaving={props.isSaving ?? false}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

describe('group settings workspace', () => {
  it('filters by description and clears search without discarding edits', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    const search = screen.getByRole('textbox', {
      name: 'Search groups by name or description',
    })
    await user.type(search, 'PREMIUM')
    expect(screen.getAllByTestId('group-id').map((c) => c.textContent)).toEqual(
      ['vip']
    )
    await user.clear(screen.getByRole('spinbutton', { name: 'Ratio' }))
    await user.type(screen.getByRole('spinbutton', { name: 'Ratio' }), '0.6')
    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(screen.getAllByTestId('group-id')).toHaveLength(2)
    expect(screen.getAllByRole('spinbutton', { name: 'Ratio' })[1]).toHaveValue(
      0.6
    )
    await user.type(search, 'missing')
    expect(screen.getByText('No results found')).toBeVisible()
  })

  it('finds a group by its display name', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    await screen.findByRole('button', { name: 'Edit names of vip' })
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Edit names of vip' })
      ).toHaveTextContent('VIP')
    )
    await user.type(
      screen.getByRole('textbox', {
        name: 'Search groups by name or description',
      }),
      'Default'
    )
    expect(screen.getAllByTestId('group-id').map((c) => c.textContent)).toEqual(
      ['default']
    )
  })

  it('adds a group with the ID returned by the server and keeps the top-up ratios', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(async (_values: typeof defaults) => {})
    groupApi.createAdminGroup.mockResolvedValue(
      adminGroup('gk3x9', 'New group')
    )
    render(<Fixture onSave={onSave} />)

    await user.click(screen.getByRole('button', { name: 'Add group' }))
    // The name dialog opens for the new group; close it before saving.
    expect(
      await screen.findByRole('dialog', { name: 'Edit group names' })
    ).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() =>
      expect(screen.getAllByTestId('group-id')[0]).toHaveTextContent('gk3x9')
    )
    await user.click(
      screen.getByRole('button', { name: 'Save group settings' })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    const saved = onSave.mock.calls[0][0]
    expect(JSON.parse(saved.GroupRatio)).toEqual({
      gk3x9: 1,
      default: 1,
      vip: 0.8,
    })
    expect(JSON.parse(saved.UserUsableGroups)).toEqual({
      gk3x9: '',
      default: 'Standard access',
      vip: 'Premium access',
    })
    expect(JSON.parse(saved.TopupGroupRatio)).toEqual({ vip: 1.2 })
    expect(saved.AutoGroups).toBe(defaults.AutoGroups)
  })

  it('shows IDs as text, hides the top-up ratio and counts references', async () => {
    groupApi.getAdminGroups.mockResolvedValue([
      {
        ...adminGroup('default', 'Default'),
        refs: { users: 3, tokens: 5, channels: 2 },
      },
      adminGroup('vip', 'VIP'),
    ])
    render(<Fixture />)
    expect(
      screen.queryByRole('textbox', { name: 'Group name' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('columnheader', { name: 'Top-up ratio' })
    ).not.toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getAllByTestId('group-refs')[0]).toHaveTextContent(
        '3 users · 5 keys · 2 channels'
      )
    )
  })

  it('only allows deleting an unused group that is not default', async () => {
    groupApi.getAdminGroups.mockResolvedValue([
      adminGroup('default', 'Default'),
      {
        ...adminGroup('vip', 'VIP'),
        refs: { users: 1, tokens: 0, channels: 0 },
      },
      adminGroup('gfree', 'Free'),
    ])
    render(
      <Fixture
        initial={{
          GroupRatio: '{"default":1,"vip":0.8,"gfree":1}',
        }}
      />
    )
    await waitFor(() =>
      expect(screen.getAllByTestId('group-id')).toHaveLength(3)
    )
    const buttons = await screen.findAllByRole('button', {
      name: 'Delete group',
    })
    await waitFor(() => expect(buttons[2]).toBeEnabled())
    expect(buttons[0]).toBeDisabled()
    expect(buttons[1]).toBeDisabled()
  })

  it('keeps unfinished visibility rules while switching sections and supports keyboard tabs', async () => {
    const user = userEvent.setup()
    render(
      <Fixture
        initial={{
          GroupSpecialUsableGroup: '{"vip":{"+:default":"Standard"}}',
        }}
      />
    )
    const pricing = screen.getByRole('tab', { name: 'Pricing groups' })
    pricing.focus()
    await user.keyboard('{ArrowRight}{Enter}')
    expect(
      screen.getByRole('tab', { name: 'Special ratio rules' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(
      screen.queryByRole('table', { name: 'Pricing groups' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Group visibility' }))
    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    expect(
      screen.getAllByRole('combobox', { name: 'Group name' })
    ).toHaveLength(2)
    await user.click(pricing)
    await user.click(screen.getByRole('tab', { name: 'Group visibility' }))
    expect(
      screen.getAllByRole('combobox', { name: 'Group name' })
    ).toHaveLength(2)
    const toggle = screen.getByRole('button', { name: 'Rules for vip' })
    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
  })

  it('reorders and removes auto groups with named controls and saves across sections', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(async (_values: typeof defaults) => {})
    render(<Fixture onSave={onSave} />)
    await user.click(screen.getByRole('tab', { name: 'Auto group order' }))
    const list = screen.getByRole('list', { name: 'Auto group order' })
    expect(
      screen.getByRole('button', { name: 'Move default up' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Move vip down' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Move vip up' }))
    expect(within(list).getAllByRole('listitem')[0]).toHaveTextContent('vip')
    await user.click(
      screen.getByRole('switch', { name: 'Default to auto groups' })
    )
    await user.click(screen.getByRole('button', { name: 'Remove default' }))
    expect(screen.getByRole('button', { name: 'Move vip up' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Move vip down' })).toBeDisabled()
    await user.click(screen.getByRole('tab', { name: 'Pricing groups' }))
    await user.click(
      screen.getByRole('button', { name: 'Save group settings' })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(onSave.mock.calls[0]?.[0]).toEqual(
      expect.objectContaining({
        AutoGroups: '[\n  "vip"\n]',
        DefaultUseAutoGroup: true,
      })
    )
  })

  it('reorders from the drag handle with arrow keys, preserves unknown groups and saves the new order', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(async (_values: typeof defaults) => {})
    render(
      <Fixture
        onSave={onSave}
        initial={{
          AutoGroups: '["default","vip","retired"]',
          MaxTokenAutoGroups: 1,
        }}
      />
    )
    await user.click(screen.getByRole('tab', { name: 'Auto group order' }))
    const list = screen.getByRole('list', { name: 'Auto group order' })
    const handle = screen.getByRole('button', {
      name: 'Drag default to reorder',
    })
    handle.focus()
    await user.keyboard('{ArrowUp}')
    expect(within(list).getAllByRole('listitem')[0]).toHaveTextContent(
      'default'
    )
    await user.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}')
    expect(within(list).getAllByRole('listitem')[2]).toHaveTextContent(
      'default'
    )
    expect(handle).toHaveFocus()
    await user.keyboard('{ArrowUp}')
    expect(within(list).getAllByRole('listitem')[1]).toHaveTextContent(
      'default'
    )
    expect(screen.getByText('Not in pricing table')).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Pricing groups' }))
    await user.click(
      screen.getByRole('button', { name: 'Save group settings' })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalled())
    expect(JSON.parse(onSave.mock.calls[0][0].AutoGroups)).toEqual([
      'vip',
      'default',
      'retired',
    ])
  })

  it('keeps a single auto group unchanged when using its drag handle keyboard controls', async () => {
    const user = userEvent.setup()
    render(<Fixture initial={{ AutoGroups: '["default"]' }} />)
    await user.click(screen.getByRole('tab', { name: 'Auto group order' }))
    const handle = screen.getByRole('button', {
      name: 'Drag default to reorder',
    })
    handle.focus()
    await user.keyboard('{ArrowUp}{ArrowDown}')
    const list = screen.getByRole('list', { name: 'Auto group order' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(1)
    expect(within(list).getByRole('listitem')).toHaveTextContent('default')
    expect(handle).toHaveFocus()
    expect(
      screen.getByRole('button', { name: 'Move default up' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Move default down' })
    ).toBeDisabled()
  })

  it('opens the auto section and focuses its invalid limit when saving from another section', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(async (_values: typeof defaults) => {})
    render(<Fixture onSave={onSave} />)
    await user.click(screen.getByRole('tab', { name: 'Auto group order' }))
    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: 'Maximum custom groups per token',
      }),
      { target: { value: '0' } }
    )
    await user.click(screen.getByRole('tab', { name: 'Pricing groups' }))
    await user.click(
      screen.getByRole('button', { name: 'Save group settings' })
    )
    await waitFor(() =>
      expect(
        screen.getByRole('tab', { name: 'Auto group order' })
      ).toHaveAttribute('aria-selected', 'true')
    )
    const input = screen.getByRole('spinbutton', {
      name: 'Maximum custom groups per token',
    })
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(input).toHaveFocus())
    expect(screen.getByText('Enter a positive integer')).toBeVisible()
    expect(onSave).not.toHaveBeenCalled()
  })

  it('shows empty state guidance and disables save during a pending request', async () => {
    const user = userEvent.setup()
    groupApi.getAdminGroups.mockResolvedValue([])
    render(
      <Fixture
        isSaving
        initial={{
          GroupRatio: '{}',
          TopupGroupRatio: '{}',
          UserUsableGroups: '{}',
          AutoGroups: '[]',
        }}
      />
    )
    expect(
      screen.getByText('No groups yet. Add a group to get started.')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
    await user.click(screen.getByRole('tab', { name: 'Special ratio rules' }))
    expect(
      screen.getByText('Base group ratios apply until you add an override.')
    ).toBeVisible()
    await user.click(screen.getByRole('tab', { name: 'Auto group order' }))
    expect(screen.getByText('No auto groups configured')).toBeVisible()
  })

  it('offers no JSON editor so group IDs are only edited in the table', () => {
    render(<Fixture />)
    expect(
      screen.queryByRole('button', { name: 'Switch to JSON' })
    ).not.toBeInTheDocument()
  })
})
