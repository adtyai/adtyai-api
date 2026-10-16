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
 * Display names for raw A43 alert kinds (setting.DefaultAlertKindEnabled) and
 * A45 threshold keys (model.AlertA45ThresholdKeys). Values are English source
 * keys in the locale files; unknown values return null so callers render the
 * raw value unchanged.
 */
export const alertKindLabelKeys: Readonly<Record<string, string>> = {
  site_core_db: 'Site core database failure',
  payment_down: 'Payment channel down',
  reconcile_mismatch: 'Ledger reconciliation mismatch',
  pay_unknown_escalate: 'Unknown payment status escalation',
  hourly_rollup_missing: 'Hourly rollup missing',
  sensitive_burst: 'Sensitive operation burst',
  new_ip_sensitive: 'Sensitive operation from new IP',
  chatwoot_down: 'Support chat (Chatwoot) down',
  self_approve_over_threshold: 'Self-approval over threshold',
  refund_urgent: 'Refund over threshold',
  epay_demote: 'Epay channel demoted',
  channel_disable: 'Channel disabled',
  withdraw: 'Withdrawal request',
  usdt_underpay: 'USDT underpayment',
  build_fail: 'Build failed',
  balance_adjust: 'Balance adjustment',
  self_approve: 'Self-approval',
  refund_each: 'Every refund',
  aff_negative: 'Negative commission balance',
  hourly_rollup_stale: 'Hourly rollup delayed',
  sensitive_403_daily: 'Daily sensitive 403 summary',
  ops_job_stuck: 'Ops job stuck',
  disk: 'Disk usage',
  channel_balance: 'Low channel balance',
  alert_test: 'Test alert',
}

export const alertThresholdLabelKeys: Readonly<Record<string, string>> = {
  SelfApproveAlertAmount: 'Self-approval alert amount',
  PayUnknownEscalateHours: 'Unknown payment escalation (hours)',
  AlertMergeMinutes: 'Alert merge window (minutes)',
  RefundAlertAmount: 'Refund alert amount',
  RefundAlertDailyCount: 'Refund alert daily count',
  DiskAlertWarnPct: 'Disk warning threshold (%)',
  DiskAlertCritPct: 'Disk critical threshold (%)',
  ChannelBalanceAlertUsd: 'Channel balance alert (USD)',
  DailyReportTime: 'Daily report time',
  BackupRetentionDays: 'Backup retention (days)',
  EpayFailureDemoteThreshold: 'Epay failure demotion threshold',
}

function lookup(map: Readonly<Record<string, string>>, value: string) {
  return Object.hasOwn(map, value) ? map[value] : null
}

export function alertKindLabelKey(kind: string): string | null {
  return lookup(alertKindLabelKeys, kind)
}

export function alertThresholdLabelKey(key: string): string | null {
  return lookup(alertThresholdLabelKeys, key)
}
