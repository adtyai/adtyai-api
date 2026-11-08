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
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useOAuthLogin } from '../use-oauth-login'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  sessionStorage.clear()
})

it.each(['github', 'discord', 'linuxdo', 'custom'])(
  'uses the server callback and checked terms when starting %s on a site',
  async (provider) => {
    const canonical = `https://platform.example/oauth/${provider}`
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/user/auth/logout') return { data: { success: true } }
      if (url === '/api/oauth/state') {
        return {
          data: {
            success: true,
            data: { flow_token: 'site-flow', redirect_uri: canonical },
          },
        }
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const custom = {
      id: 1,
      name: 'Custom',
      slug: 'custom',
      icon: '',
      client_id: 'custom-client',
      authorization_endpoint: 'https://issuer.example/authorize',
      scopes: 'profile',
    }
    const { result } = renderHook(() =>
      useOAuthLogin(
        {
          github_client_id: 'github-client',
          discord_client_id: 'discord-client',
          linuxdo_client_id: 'linuxdo-client',
        },
        undefined,
        '4'
      )
    )
    await act(async () => {
      if (provider === 'github') await result.current.handleGitHubLogin()
      if (provider === 'discord') await result.current.handleDiscordLogin()
      if (provider === 'linuxdo') await result.current.handleLinuxDOLogin()
      if (provider === 'custom') {
        await result.current.handleCustomOAuthLogin(custom)
      }
    })

    expect(post).toHaveBeenCalledWith(
      '/api/oauth/state',
      expect.objectContaining({
        provider,
        intent: 'login',
        site_terms_version: '4',
      }),
      expect.anything()
    )
    expect(open).toHaveBeenCalled()
    const destination = new URL(String(open.mock.calls[0][0]))
    expect(destination.searchParams.get('redirect_uri')).toBe(canonical)
    expect(destination.searchParams.get('state')).toBe('site-flow')
  }
)
