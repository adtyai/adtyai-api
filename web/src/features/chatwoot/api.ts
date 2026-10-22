import { api } from '@/lib/api'

export type ChatwootPublicConfig = {
  enabled: boolean
  base_url: string
  website_token: string
  identity_verification?: boolean
  backup_contact_type?: string
  backup_contact?: string
}

export type SupportContact = {
  type: string
  contact: string
}

export type ChatwootIdentity = {
  identifier: string
  identifier_hash?: string
  /** false when backend has no HMAC secret: do not pass email/hash. */
  identity_verified?: boolean
  name?: string
  email?: string
}

const soft = { skipBusinessError: true, skipErrorHandler: true } as const

export function isSafeChatwootBaseUrl(raw: string): boolean {
  const value = raw.trim()
  if (!value || /[\s\\]/.test(value)) return false
  let parsed: URL
  try {
    parsed = new URL(value)
  } catch {
    return false
  }
  if (parsed.username || parsed.password) return false
  if (parsed.protocol === 'https:') return Boolean(parsed.hostname)
  if (parsed.protocol === 'http:') {
    const host = parsed.hostname.replaceAll(/^\[|\]$/g, '').toLowerCase()
    return host === 'localhost' || host === '127.0.0.1' || host === '::1'
  }
  return false
}

export async function fetchSupportContact(): Promise<SupportContact | null> {
  try {
    const res = await api.get('/api/chatwoot/config', soft)
    const d = res.data?.data
    const contact = String(d?.backup_contact || '').trim()
    if (!res.data?.success || !contact) return null
    return {
      type: String(d.backup_contact_type || ''),
      contact,
    }
  } catch {
    return null
  }
}

export async function fetchChatwootConfig(): Promise<ChatwootPublicConfig | null> {
  try {
    const res = await api.get('/api/chatwoot/config', soft)
    const d = res.data?.data
    if (
      res.data?.success &&
      d?.enabled &&
      isSafeChatwootBaseUrl(d.base_url) &&
      d.website_token
    ) {
      return d as ChatwootPublicConfig
    }
    return null
  } catch {
    return null
  }
}

export async function fetchChatwootIdentity(): Promise<ChatwootIdentity | null> {
  try {
    const res = await api.get('/api/chatwoot/identity', soft)
    const d = res.data?.data
    return res.data?.success && d?.identifier ? (d as ChatwootIdentity) : null
  } catch {
    return null
  }
}

export function identityAttrs(id: ChatwootIdentity): Record<string, unknown> {
  const attrs: Record<string, unknown> = {}
  if (id.name) {
    attrs.name = id.name
  }
  // R-P2-9: unverified identity → never hand email/hash to the widget.
  if (id.identity_verified === false) {
    return attrs
  }
  if (id.email) {
    attrs.email = id.email
  }
  if (id.identifier_hash) {
    attrs.identifier_hash = id.identifier_hash
  }
  return attrs
}
