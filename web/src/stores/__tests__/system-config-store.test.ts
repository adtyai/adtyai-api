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
import { beforeEach, describe, expect, test } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

describe('system config store', () => {
  beforeEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  test('keeps the same config object when a status refresh changes nothing', () => {
    const before = useSystemConfigStore.getState().config
    useSystemConfigStore.getState().setConfig({
      systemName: before.systemName,
      logo: before.logo,
      footerHtml: before.footerHtml,
      currency: { ...before.currency },
    })
    expect(useSystemConfigStore.getState().config).toBe(before)
  })

  test('replaces the config object when a displayed field changes', () => {
    const before = useSystemConfigStore.getState().config
    useSystemConfigStore.getState().setConfig({ systemName: 'Renamed' })
    const after = useSystemConfigStore.getState().config
    expect(after).not.toBe(before)
    expect(after.systemName).toBe('Renamed')
  })
})
