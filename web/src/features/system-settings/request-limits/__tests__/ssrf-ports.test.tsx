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
import { useState } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { SSRFSection } from '../ssrf-section'

const storedPorts = ['80', '443', '1-65535']

function Fixture() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <SSRFSection
          defaultValues={{
            'fetch_setting.enable_ssrf_protection': true,
            'fetch_setting.allow_private_ip': false,
            'fetch_setting.domain_filter_mode': false,
            'fetch_setting.ip_filter_mode': false,
            'fetch_setting.domain_list': [],
            'fetch_setting.ip_list': [],
            'fetch_setting.allowed_ports': storedPorts,
            'fetch_setting.apply_ip_filter_for_domain': false,
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test('saving the loaded SSRF form keeps port ranges as strings', async () => {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Fixture />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  const ports = await screen.findByRole('textbox', { name: 'Allowed Ports' })
  await user.clear(ports)
  await user.type(ports, '80, 443, 8000-9000')
  await user.click(
    screen.getByRole('button', { name: 'Save SSRF settings' })
  )

  await waitFor(() => expect(api.put).toHaveBeenCalled())
  const payload = vi.mocked(api.put).mock.calls.find((call) =>
    String(call[1]?.key).includes('allowed_ports')
  )?.[1]
  expect(JSON.parse(String(payload?.value))).toEqual([
    '80',
    '443',
    '8000-9000',
  ])
})
