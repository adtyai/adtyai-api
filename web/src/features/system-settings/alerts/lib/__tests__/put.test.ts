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

import { describe, expect, it } from 'vitest'

import type { AlertsConfigView, AlertsFormValues } from '../../types'
import {
  ALERTS_FORM_DEFAULTS,
  buildAlertsPut,
  configToForm,
  formatTokenFingerprint,
  isTelegramPendingConfigured,
} from '../put'

const base: AlertsFormValues = {
  ...ALERTS_FORM_DEFAULTS,
  urgent_chat_id: '-1001',
  pending_chat_id: '-1002',
  kind_enabled: { disk: true, refund_urgent: true },
  merge_minutes: 10,
}

describe('configToForm', () => {
  it('never copies bot token plaintext into the form', () => {
    const view: AlertsConfigView = {
      bot_token: {
        secret_set: true,
        fingerprint: 'hm_abc',
        option_key: 'AdtyTelegramBotToken',
        note: 'note',
      },
      urgent_chat_id: '-1',
      pending_chat_id: '-2',
      urgent_route_enabled: true,
      pending_route_enabled: false,
      kind_enabled: { disk: false },
      merge_minutes: 15,
      thresholds: { AlertMergeMinutes: '15' },
      thresholds_note: 'A45',
    }
    const form = configToForm(view)
    expect(form.bot_token).toBe('')
    expect(form.urgent_chat_id).toBe('-1')
    expect(form.pending_route_enabled).toBe(false)
    expect(form.merge_minutes).toBe(15)
    expect(form.kind_enabled).toEqual({ disk: false })
  })
})

describe('buildAlertsPut', () => {
  it('returns null when nothing changed (empty token omitted)', () => {
    expect(buildAlertsPut({ ...base, bot_token: '   ' }, base)).toBeNull()
  })

  it('includes bot_token only when non-empty', () => {
    expect(buildAlertsPut({ ...base, bot_token: 'new-secret' }, base)).toEqual({
      bot_token: 'new-secret',
    })
  })

  it('emits only changed fields (pointer semantics)', () => {
    const next: AlertsFormValues = {
      ...base,
      bot_token: '',
      urgent_chat_id: '-2001',
      urgent_route_enabled: false,
      merge_minutes: 20,
      kind_enabled: { disk: false, refund_urgent: true },
    }
    expect(buildAlertsPut(next, base)).toEqual({
      urgent_chat_id: '-2001',
      urgent_route_enabled: false,
      kind_enabled: { disk: false, refund_urgent: true },
      merge_minutes: 20,
    })
  })

  it('does not PUT empty token even when other fields change', () => {
    const put = buildAlertsPut(
      { ...base, bot_token: '', pending_chat_id: '-9' },
      base
    )
    expect(put).toEqual({ pending_chat_id: '-9' })
    expect(put).not.toHaveProperty('bot_token')
  })
})

describe('formatTokenFingerprint', () => {
  it('shows dash when missing', () => {
    expect(formatTokenFingerprint('')).toBe('-')
    expect(formatTokenFingerprint(null)).toBe('-')
    expect(formatTokenFingerprint('hm_x')).toBe('hm_x')
  })
})

describe('isTelegramPendingConfigured', () => {
  it('requires secret_set, pending chat id, and pending route', () => {
    expect(isTelegramPendingConfigured(undefined)).toBe(false)
    expect(
      isTelegramPendingConfigured({
        bot_token: {
          secret_set: true,
          option_key: 'AdtyTelegramBotToken',
          note: '',
        },
        pending_chat_id: '-100',
        pending_route_enabled: true,
      })
    ).toBe(true)
    expect(
      isTelegramPendingConfigured({
        bot_token: {
          secret_set: false,
          option_key: 'AdtyTelegramBotToken',
          note: '',
        },
        pending_chat_id: '-100',
        pending_route_enabled: true,
      })
    ).toBe(false)
    expect(
      isTelegramPendingConfigured({
        bot_token: {
          secret_set: true,
          option_key: 'AdtyTelegramBotToken',
          note: '',
        },
        pending_chat_id: '  ',
        pending_route_enabled: true,
      })
    ).toBe(false)
    expect(
      isTelegramPendingConfigured({
        bot_token: {
          secret_set: true,
          option_key: 'AdtyTelegramBotToken',
          note: '',
        },
        pending_chat_id: '-100',
        pending_route_enabled: false,
      })
    ).toBe(false)
  })
})
