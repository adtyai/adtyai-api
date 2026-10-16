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
/**
 * Stable `code` (+ optional snake_case `params`) returned by the adtyai admin
 * APIs (controller/*_adty.go, business_params.go → adtyFail). Each code maps to
 * an English source key in the locale files; params are interpolated with
 * i18next `{{key}}`. Unknown codes fall back to the backend `message`.
 */
type Params = Record<string, unknown>
type KeyResolver = string | ((params: Params) => string)

const has = (params: Params, key: string) =>
  params[key] !== undefined && params[key] !== null && params[key] !== ''

export const adtyErrorCodeKeys: Record<string, KeyResolver> = {
  // Generic
  invalid_id: 'Invalid ID',
  invalid_parameters: 'Invalid parameters, please check your input',
  invalid_params: 'Invalid input, please check the fields',
  not_found: 'The requested record was not found',
  forbidden: 'You do not have permission to perform this action',
  unauthorized: 'Not signed in or the session has expired',
  bad_request: 'Invalid request',
  internal_error: 'Internal server error, please try again later',
  // Confirmation / permission
  confirm_required: 'Second confirmation is required',
  root_required: 'Only Root can perform this action',
  self_approve_forbidden: 'You cannot approve a request you submitted',
  role_not_allowed: 'Only the Admin role can be created',
  username_taken: 'Username already exists',
  db_error: 'Failed to create administrator, please try again later',
  create_failed: 'Failed to create administrator, please try again later',
  feature_frozen: 'This feature is not available yet',
  // State
  status_invalid: 'The current status does not allow this action',
  order_status_invalid: 'The current order status does not allow this action',
  already_decided: 'This request has already been processed',
  risk_review_order_refunded: (p) =>
    has(p, 'order_no')
      ? 'Order {{order_no}} is fully refunded and cannot be credited'
      : 'This order is fully refunded and cannot be credited',
  retry_forbidden: 'This withdrawal cannot be retried',
  // Business params
  unknown_key: (p) =>
    has(p, 'key') ? 'Unknown parameter: {{key}}' : 'Unknown parameter',
  invalid_value: (p) =>
    has(p, 'key')
      ? 'Invalid value for {{key}}: {{value}}'
      : 'Invalid parameter value',
  out_of_range: (p) => {
    // TopupBonusTiers 逐档校验带 index（0 起），显示成「第 N 档」
    if (has(p, 'index') && has(p, 'min') && has(p, 'max')) {
      return 'Tier {{tier}} of {{key}} is out of range ({{min}} to {{max}})'
    }
    if (has(p, 'min') && has(p, 'max')) {
      return 'Value for {{key}} is out of range ({{min}} to {{max}})'
    }
    if (has(p, 'min')) return 'Value for {{key}} cannot be less than {{min}}'
    if (has(p, 'max')) return 'Value for {{key}} cannot be greater than {{max}}'
    return 'Value is out of range'
  },
  // B-08 跨字段规则 {key, rule, other_key}
  param_rule_violation: (p) => {
    if (p.rule === 'greater_than') return '{{key}} must be greater than {{other_key}}'
    if (p.rule === 'less_than') return '{{key}} must be less than {{other_key}}'
    return has(p, 'other_key')
      ? '{{key}} conflicts with {{other_key}}'
      : 'Parameter values conflict, please check your input'
  },
  // B-07 TopupBonusTiers
  topup_tiers_count_mismatch:
    '{{key}} needs {{expected_count}} tier rates, got {{actual_count}}',
  topup_tiers_not_ascending: (p) =>
    has(p, 'index')
      ? 'Tier {{tier}} of {{key}} ({{value}}) is lower than the previous tier ({{prev_value}}); rates must be non-decreasing'
      : 'Rates of {{key}} must be non-decreasing',
  validation_failed: 'Validation failed, please check your input',
  // Refunds / orders / payments
  order_no_required: 'Order number is required',
  request_id_required: 'Request ID is required',
  reason_required: 'Reason is required',
  amount_invalid: 'Amount must be greater than 0',
  refund_idempotent: 'This refund request has already been submitted',
  refund_exceeds_available: (p) =>
    has(p, 'available')
      ? 'Refund amount {{amount}} exceeds the refundable amount {{available}}'
      : 'Refund amount exceeds the refundable amount',
  refund_failed: 'Refund failed, please try again later',
  trade_no_duplicate: 'This transaction number has already been used',
  invalid_tier_amount: 'Amount must be one of the fixed top-up tiers',
  payment_maintenance: 'Payment is under maintenance, please try again later',
  epay_snapshot_failed: 'Failed to bind the payment snapshot',
  use_disable_endpoint: 'Use the disable action to disable this instance',
  // Balance adjustments / ledger
  invalid_account: (p) =>
    has(p, 'account')
      ? 'Invalid account type: {{account}} (must be paid or bonus balance)'
      : 'Invalid account type (must be paid or bonus balance)',
  insufficient_balance: 'Insufficient balance',
  adjust_failed: 'Balance adjustment failed, please try again later',
  invalid_created_from: 'Invalid start time',
  invalid_created_to: 'Invalid end time',
  // Affiliate withdrawals
  below_min: (p) =>
    has(p, 'min')
      ? 'Withdrawal amount {{amount_quota}} is below the minimum {{min}}'
      : 'Withdrawal amount is below the minimum',
  insufficient_aff_quota: (p) =>
    has(p, 'balance')
      ? 'Insufficient commission balance: requested {{amount_quota}}, available {{balance}}'
      : 'Insufficient commission balance',
  // Enterprise pricing
  bad_discount: 'Invalid discount value',
  conflict: 'A rule for the same user, group and model already exists',
  bad_user: 'The user is missing or does not exist',
  // Group display names
  code_immutable: 'The group code cannot be changed',
  invalid_code: 'Invalid group code',
  display_name_required: 'Display name is required',
  exists: 'This group already exists',
  // Alerts / health
  invalid_level: (p) =>
    has(p, 'level') ? 'Invalid alert level: {{level}}' : 'Invalid alert level',
  not_configured: 'Alert channel is not configured',
  send_failed: 'Failed to send test alert',
  health_probe_failed: 'Health probe failed',
}

export interface AdtyErrorText {
  key: string
  params?: Params
}

/** Resolve a backend code to its locale key; null when the code is unknown. */
export function resolveAdtyErrorKey(
  code: unknown,
  params?: unknown
): AdtyErrorText | null {
  if (
    typeof code !== 'string' ||
    !Object.hasOwn(adtyErrorCodeKeys, code) ||
    adtyBackendMessageCodes.has(code)
  ) {
    return null
  }
  const p: Params =
    params && typeof params === 'object' && !Array.isArray(params)
      ? (params as Params)
      : {}
  const entry = adtyErrorCodeKeys[code]
  const key = typeof entry === 'function' ? entry(p) : entry
  return { key, params: p }
}

/**
 * Codes whose backend `message` is the user-facing text (e.g. operator-written
 * maintenance notice); never replace it with the generic locale string.
 */
export const adtyBackendMessageCodes: ReadonlySet<string> = new Set([
  'payment_maintenance',
])

/**
 * HTTP status fallback for responses without a recognized `code` (e.g. the
 * upstream auth middleware's 401/403, rate limiter 429), so the zh console
 * never shows the raw English sentence.
 */
export const adtyHttpStatusKeys: Readonly<Record<number, string>> = {
  401: 'Please sign in first',
  403: "You don't have permission",
  429: 'Too many requests, please try again later',
}

export function resolveAdtyHttpStatusKey(status: unknown): string | null {
  if (typeof status !== 'number') return null
  return Object.hasOwn(adtyHttpStatusKeys, status)
    ? adtyHttpStatusKeys[status]
    : null
}
