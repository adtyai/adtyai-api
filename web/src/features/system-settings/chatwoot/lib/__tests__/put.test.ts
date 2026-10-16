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

import type { ChatwootConfigView, ChatwootFormValues } from '../../types'
import {
  CHATWOOT_FORM_DEFAULTS,
  buildChatwootPut,
  chatwootPutNeedsConfirm,
  configToForm,
  formatHmacFingerprint,
} from '../put'

const base: ChatwootFormValues = {
  ...CHATWOOT_FORM_DEFAULTS,
  script: '<script>cw</script>',
  website_token: 'tok-1',
  base_url: 'https://cw.example.com',
}

describe('configToForm', () => {
  it('never copies hmac secret plaintext into the form', () => {
    const view: ChatwootConfigView = {
      script: 's',
      website_token: 'wt',
      base_url: 'https://x',
      hmac_secret_set: true,
      hmac_secret_fingerprint: 'hm_abc',
      backup_contact_type: 'telegram',
      backup_contact: '@ops',
      backup_contact_note: 'A45',
    }
    const form = configToForm(view)
    expect(form.hmac_secret).toBe('')
    expect(form.script).toBe('s')
    expect(form.website_token).toBe('wt')
    expect(form.base_url).toBe('https://x')
  })
})

describe('buildChatwootPut', () => {
  it('returns null when nothing changed (empty hmac omitted)', () => {
    expect(buildChatwootPut({ ...base, hmac_secret: '   ' }, base)).toBeNull()
  })

  it('includes hmac_secret only when non-empty', () => {
    expect(
      buildChatwootPut({ ...base, hmac_secret: 'new-secret' }, base)
    ).toEqual({
      hmac_secret: 'new-secret',
    })
  })

  it('emits only changed fields (pointer semantics)', () => {
    const next: ChatwootFormValues = {
      ...base,
      hmac_secret: '',
      website_token: 'tok-2',
      base_url: 'https://new.example.com',
    }
    expect(buildChatwootPut(next, base)).toEqual({
      website_token: 'tok-2',
      base_url: 'https://new.example.com',
    })
  })

  it('includes script when changed (including clear to empty)', () => {
    expect(buildChatwootPut({ ...base, script: '' }, base)).toEqual({
      script: '',
    })
    expect(
      buildChatwootPut({ ...base, script: '<script>v2</script>' }, base)
    ).toEqual({
      script: '<script>v2</script>',
    })
  })

  it('does not PUT empty hmac even when other fields change', () => {
    const put = buildChatwootPut(
      { ...base, hmac_secret: '', website_token: 'tok-9' },
      base
    )
    expect(put).toEqual({ website_token: 'tok-9' })
    expect(put).not.toHaveProperty('hmac_secret')
  })
})

describe('chatwootPutNeedsConfirm', () => {
  it('requires confirm when script is present', () => {
    expect(chatwootPutNeedsConfirm({ script: 'x' })).toBe(true)
    expect(chatwootPutNeedsConfirm({ script: '' })).toBe(true)
  })

  it('requires confirm when hmac_secret is non-empty', () => {
    expect(chatwootPutNeedsConfirm({ hmac_secret: 'sec' })).toBe(true)
  })

  it('does not require confirm for token/base_url only', () => {
    expect(
      chatwootPutNeedsConfirm({
        website_token: 't',
        base_url: 'https://a',
      })
    ).toBe(false)
  })
})

describe('formatHmacFingerprint', () => {
  it('shows dash when missing', () => {
    expect(formatHmacFingerprint('')).toBe('-')
    expect(formatHmacFingerprint(null)).toBe('-')
    expect(formatHmacFingerprint('hm_x')).toBe('hm_x')
  })
})
