import { api } from '@/lib/api'

export type ChatwootPublicConfig = {
  enabled: boolean
  base_url: string
  website_token: string
  identity_verification?: boolean
}

export type ChatwootIdentity = {
  identifier: string
  identifier_hash?: string
  name?: string
  email?: string
}

const soft = { skipBusinessError: true, skipErrorHandler: true } as const

export async function fetchChatwootConfig(): Promise<ChatwootPublicConfig | null> {
  try {
    const res = await api.get('/api/chatwoot/config', soft)
    const d = res.data?.data
    if (res.data?.success && d?.enabled && d.base_url && d.website_token) {
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
  if (id.email) {
    attrs.email = id.email
  }
  if (id.identifier_hash) {
    attrs.identifier_hash = id.identifier_hash
  }
  return attrs
}
