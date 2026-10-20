/*
 * Audit N-P2-10: load the Chatwoot live-chat widget and bind the signed-in
 * user identity (HMAC identifier_hash from the backend). Soft-fail: any error
 * simply leaves the widget unloaded.
 */
import { useEffect } from 'react'

import { useAuthStore } from '@/stores/auth-store'

import {
  fetchChatwootConfig,
  fetchChatwootIdentity,
  identityAttrs,
  isSafeChatwootBaseUrl,
  type ChatwootPublicConfig,
} from './api'

type ChatwootGlobal = {
  setUser: (id: string, attrs: Record<string, unknown>) => void
  reset: () => void
}

declare global {
  interface Window {
    chatwootSDK?: {
      run: (opts: { websiteToken: string; baseUrl: string }) => void
    }
    $chatwoot?: ChatwootGlobal
  }
}

const SCRIPT_ID = 'chatwoot-sdk'

function loadSdk(cfg: ChatwootPublicConfig): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!isSafeChatwootBaseUrl(cfg.base_url)) {
      reject(new Error('unsafe chatwoot base_url'))
      return
    }
    if (document.querySelector(`#${SCRIPT_ID}`)) {
      resolve()
      return
    }
    const s = document.createElement('script')
    s.id = SCRIPT_ID
    s.src = `${cfg.base_url}/packs/js/sdk.js`
    s.async = true
    s.defer = true
    s.addEventListener('load', () => {
      try {
        window.chatwootSDK?.run({
          websiteToken: cfg.website_token,
          baseUrl: cfg.base_url,
        })
        resolve()
      } catch (error) {
        reject(error)
      }
    })
    s.addEventListener('error', () =>
      reject(new Error('chatwoot sdk load failed'))
    )
    document.body.append(s)
  })
}

export function ChatwootWidget() {
  const userId = useAuthStore((s) => s.auth.user?.id)

  useEffect(() => {
    let cancelled = false
    const applyUser = async () => {
      if (!userId) {
        window.$chatwoot?.reset()
        return
      }
      const id = await fetchChatwootIdentity()
      if (!cancelled && id && window.$chatwoot) {
        window.$chatwoot.setUser(id.identifier, identityAttrs(id))
      }
    }
    const onReady = () => void applyUser()
    void (async () => {
      const cfg = await fetchChatwootConfig()
      if (!cfg || cancelled) {
        return
      }
      window.addEventListener('chatwoot:ready', onReady)
      try {
        await loadSdk(cfg)
        if (window.$chatwoot) {
          await applyUser()
        }
      } catch {
        /* soft-fail: widget stays hidden */
      }
    })()
    return () => {
      cancelled = true
      window.removeEventListener('chatwoot:ready', onReady)
    }
  }, [userId])

  return null
}
