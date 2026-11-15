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
import type { AuthBundle } from '@/stores/auth-store'

export type VerificationMethod =
  | '2fa'
  | 'passkey'
  | 'password'
  | 'email_code'
  | 'oauth'
  | 'session'
  | 'root_exempt'
export type SecurityProofScope =
  | 'channel.key.read'
  | 'passkey.register'
  | 'passkey.delete'
  | '2fa.setup'
  | '2fa.disable'
  | '2fa.backup_codes.regenerate'
  | 'access_token.generate'
  | 'access_token.update'
  | 'access_token.revoke'
  | 'account.binding.bind'
  | 'account.binding.unbind'
  | 'account.password.set'
  | 'account.password.change'
  | 'account.delete'
  | 'admin.user.create'
  | 'admin.user.update'
  | 'admin.user.delete'
  | 'admin.user.manage'
  | 'admin.user.passkey.reset'
  | 'admin.user.2fa.disable'
  | 'admin.user.binding.clear'
  | 'site.owner.manage'
  | 'site.staff.customer.manage'
  | 'adty.notice.configure'
  // 资金写操作与查询的二次验证 scope（service/adty_topup_verification.go）。
  // 后端早已校验这些字符串，前端联合类型一直没跟上，导致调用点只能报类型错。
  | 'adty.topup.refund'
  | 'adty.topup.manual_refund_step'
  | 'adty.topup.manual_complete'
  | 'adty.payment_orphan.credit'
  | 'adty.payment_orphan.resolve'
  | 'adty.topup.query'
  | 'adty.site_earning.review'

/** Site owner actions that need step-up (service/adty_site_verification.go). */
export type SiteOwnerAction =
  | 'pricing'
  | 'bonus'
  | 'trial'
  | 'domain.delete'
  | 'promo'
  | 'sender.delete'
  | 'staff.create'
  | 'staff.update'
  | 'staff.password'
  | 'staff.delete'

/** ManageUser actions that change a user's status or role and need step-up. */
export type AdminUserManageAction = 'disable' | 'enable' | 'promote' | 'demote'

export type VerificationOperation =
  | { scope: 'channel.key.read'; context: { channel_id: number } }
  | {
      scope: 'account.binding.bind'
      context: { provider: string; email?: string; code?: string }
    }
  | { scope: 'account.binding.unbind'; context: { provider_id: number } }
  | { scope: 'admin.user.create'; context: { role: number } }
  | {
      scope:
        | 'admin.user.update'
        | 'admin.user.delete'
        | 'admin.user.passkey.reset'
        | 'admin.user.2fa.disable'
      context: { user_id: number }
    }
  | {
      scope: 'admin.user.manage'
      context: { user_id: number; action: AdminUserManageAction }
    }
  | {
      scope: 'admin.user.binding.clear'
      context:
        | { user_id: number; binding_type: string }
        | { user_id: number; provider_id: number }
    }
  | {
      scope: 'access_token.generate'
      context: { scopes: string[]; expires_at: number }
    }
  | {
      scope: 'access_token.update'
      context: { token_id: number; scopes: string[] }
    }
  | {
      scope: 'access_token.revoke'
      context: { token_id: number } | { legacy: true }
    }
  | {
      scope: 'site.owner.manage'
      context: { action: SiteOwnerAction; target: number }
    }
  | {
      scope: 'site.staff.customer.manage'
      context: {
        action: 'customer.disable'
        target: number
      }
    }
  | {
      scope: 'adty.notice.configure'
      context: {
        action: 'telegram.save' | 'telegram.test' | 'translation.save' | 'translation.test'
        target: string
      }
    }
  | {
      // 资金类 scope 的上下文由各自的处理函数解释：退款带 trade_no / reason /
      // refund_usd，退款步骤带 refund_id / step / payout_*，孤儿事件带事件 id。
      // 这里只声明「这些 scope 允许自由上下文」，字段名不去猜后端校验。
      scope:
        | 'adty.topup.refund'
        | 'adty.topup.manual_refund_step'
        | 'adty.topup.manual_complete'
        | 'adty.payment_orphan.credit'
        | 'adty.payment_orphan.resolve'
        | 'adty.topup.query'
        | 'adty.site_earning.review'
      context?: Record<string, unknown>
    }
  | {
      scope: Exclude<
        SecurityProofScope,
        | 'channel.key.read'
        | 'account.binding.bind'
        | 'account.binding.unbind'
        | 'access_token.generate'
        | 'access_token.update'
        | 'access_token.revoke'
        | `admin.user.${string}`
        | 'site.owner.manage'
        | 'site.staff.customer.manage'
        | 'adty.notice.configure'
        | 'adty.topup.refund'
        | 'adty.topup.manual_refund_step'
        | 'adty.topup.manual_complete'
        | 'adty.payment_orphan.credit'
        | 'adty.payment_orphan.resolve'
        | 'adty.topup.query'
        | 'adty.site_earning.review'
      >
      context?: Record<string, never>
    }

export interface SecurityProof {
  proof_token: string
  expires_at: number
  method: VerificationMethod
  scope: SecurityProofScope
}

export interface VerificationRequirements {
  scope: SecurityProofScope | 'auth.login'
  methods: { method: VerificationMethod; available: boolean; reason?: string }[]
  oauth_providers: { slug: string; name: string }[]
  password_encryption_enabled: boolean
}

export type VerificationInput =
  | { method: '2fa'; code: string }
  | { method: 'password'; password: string }
  | { method: 'email_code'; code: string }
  | { method: 'passkey'; rpID?: string }
  | { method: 'oauth'; provider: string }
  | { method: 'session' }
  | { method: 'root_exempt' }

export type RequestVerificationOptions = VerificationOperation & {
  title?: string
  description?: string
}

export interface LoginChallenge {
  require_verification: true
  flow_token: string
  expires_at: number
  methods: VerificationRequirements['methods']
}

export interface RequestLoginVerificationOptions {
  scope: 'auth.login'
  challenge: LoginChallenge
  title?: string
  description?: string
}

export type VerificationRequest =
  | RequestVerificationOptions
  | RequestLoginVerificationOptions
export type LoginResult = AuthBundle | LoginChallenge

export type SecureVerificationState =
  | { phase: 'idle' }
  | { phase: 'loading'; request: VerificationRequest }
  | { phase: 'error'; request: VerificationRequest; error: string }
  | {
      phase: 'ready' | 'verifying'
      request: VerificationRequest
      requirements: VerificationRequirements
      input: VerificationInput | null
      error?: string
    }
