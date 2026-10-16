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
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import {
  adtyBackendMessageCodes,
  adtyErrorCodeKeys,
  adtyHttpStatusKeys,
  resolveAdtyErrorKey,
  resolveAdtyHttpStatusKey,
} from './adty-error-codes'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from './server-error-message'

const zhT = (zh as { translation: Record<string, string> }).translation
const enT = (en as { translation: Record<string, string> }).translation

// Every key each resolver can produce (with and without optional params).
function allKeys(): string[] {
  const full = {
    key: 'k',
    value: 'v',
    min: 1,
    max: 2,
    amount: 1,
    available: 1,
    account: 'x',
    amount_quota: 1,
    balance: 1,
    level: 'x',
  }
  const variants = [{}, full, { key: 'k', min: 1 }, { key: 'k', max: 2 }]
  const keys = new Set<string>()
  for (const code of Object.keys(adtyErrorCodeKeys)) {
    if (adtyBackendMessageCodes.has(code)) continue
    for (const p of variants) {
      keys.add(resolveAdtyErrorKey(code, p)?.key ?? code)
    }
  }
  return [...keys]
}

describe('adty error codes', () => {
  beforeAll(async () => {
    await i18next.init({
      lng: 'zh',
      resources: { zh: { translation: zhT }, en: { translation: enT } },
      interpolation: { escapeValue: false },
      nsSeparator: false,
    })
  })

  test('every code has zh and en text; zh has no English sentence or code', () => {
    for (const key of allKeys()) {
      expect(enT[key], key).toBeTruthy()
      const z = zhT[key]
      expect(z, key).toBeTruthy()
      const stripped = z.replaceAll(/\{\{\w+\}\}/g, '')
      expect(stripped, key).not.toMatch(/[A-Za-z]{3,}/)
    }
    for (const code of Object.keys(adtyErrorCodeKeys)) {
      for (const key of allKeys()) expect(zhT[key]).not.toContain(code)
    }
  })

  test('looks up a code from a business response', () => {
    expect(
      getServerErrorMessage({
        success: false,
        code: 'confirm_required',
        message: 'confirm=true required',
      })
    ).toBe('需要二次确认')
    expect(
      getServerErrorMessage({
        response: {
          data: { success: false, code: 'invalid_id', message: 'invalid id' },
        },
      })
    ).toBe('无效的 ID')
  })

  test('interpolates params', () => {
    expect(
      getServerErrorMessage({
        success: false,
        code: 'out_of_range',
        message: 'value out of range',
        params: { key: 'AffMinWithdrawAmount', value: '0', min: 1, max: 100 },
      })
    ).toBe('参数 最低提现金额 超出范围（1 ~ 100）')
    // Unmapped key keeps the raw key.
    expect(
      getServerErrorMessage({
        success: false,
        code: 'out_of_range',
        message: 'value out of range',
        params: { key: 'SomeNewKey', value: '0', min: 1, max: 100 },
      })
    ).toBe('参数 SomeNewKey 超出范围（1 ~ 100）')
    expect(
      getServerErrorMessage({
        success: false,
        code: 'refund_exceeds_available',
        message: 'x',
        params: { amount: 50, available: 20 },
      })
    ).toBe('退款金额 50 超过可退金额 20')
    expect(
      getServerErrorMessage({
        success: false,
        code: 'below_min',
        message: 'x',
        params: { amount_quota: 5, min: 10 },
      })
    ).toBe('提现金额 5 低于最低提现额 10')
  })

  test('picks the param-less text when optional params are missing', () => {
    expect(
      getServerErrorMessage({
        success: false,
        code: 'out_of_range',
        message: 'x',
      })
    ).toBe('取值超出范围')
    expect(
      getServerErrorMessage({
        success: false,
        code: 'out_of_range',
        message: 'x',
        params: { key: 'K', min: 3 },
      })
    ).toBe('参数 K 不能小于 3')
  })

  test('falls back to message for an unknown code, then to the fallback', () => {
    expect(
      getServerErrorMessage({
        success: false,
        code: 'brand_new_code',
        message: '原始信息',
      })
    ).toBe('原始信息')
    expect(
      getServerErrorMessage({ success: false, message: 'plain message' })
    ).toBe('plain message')
    expect(
      getServerErrorMessage(
        { success: false, code: 'brand_new_code' },
        '保存失败'
      )
    ).toBe('保存失败')
    expect(resolveAdtyErrorKey('toString')).toBeNull()
  })

  test('reads the code through an Error cause', () => {
    const err = new Error('forbidden', {
      cause: { success: false, code: 'forbidden', message: 'forbidden' },
    })
    expect(getServerErrorMessage(err)).toBe('你没有执行此操作的权限')
  })

  test('payment_maintenance keeps the backend message', () => {
    expect(resolveAdtyErrorKey('payment_maintenance')).toBeNull()
    expect(
      getServerErrorMessage({
        response: {
          status: 503,
          data: {
            success: false,
            code: 'payment_maintenance',
            message: '支付通道维护中，预计 10 分钟恢复',
          },
        },
      })
    ).toBe('支付通道维护中，预计 10 分钟恢复')
  })
})

describe('adty HTTP status fallback', () => {
  const axiosLike = (status: number, data: unknown) =>
    Object.assign(new Error(`Request failed with status code ${status}`), {
      isAxiosError: true,
      response: { status, data },
    })

  test('status keys have zh and en text; zh has no English', () => {
    for (const key of Object.values(adtyHttpStatusKeys)) {
      expect(enT[key], key).toBeTruthy()
      expect(zhT[key], key).toBeTruthy()
      expect(zhT[key], key).not.toMatch(/[A-Za-z]{3,}/)
    }
    expect(resolveAdtyHttpStatusKey(500)).toBeNull()
    expect(resolveAdtyHttpStatusKey('401')).toBeNull()
  })

  test('401/403/429 without code show localized text, not the English body', () => {
    expect(
      getServerErrorMessage(
        axiosLike(401, {
          success: false,
          message: 'Unauthorized, not logged in and no access token provided',
        })
      )
    ).toBe('请先登录')
    expect(
      getServerErrorMessage(
        axiosLike(403, {
          success: false,
          message: 'Unauthorized, insufficient permissions',
        })
      )
    ).toBe('没有权限')
    expect(getServerErrorMessage(axiosLike(429, 'Too Many Requests'))).toBe(
      '请求过于频繁，请稍后再试'
    )
  })

  test('unknown code falls back by status; known code wins', () => {
    expect(
      getServerErrorMessage(
        axiosLike(403, { success: false, code: 'brand_new', message: 'nope' })
      )
    ).toBe('没有权限')
    expect(
      getServerErrorMessage(
        axiosLike(403, { success: false, code: 'root_required', message: 'x' })
      )
    ).toBe('仅超级管理员可执行此操作')
  })

  test('other statuses keep message then fallback', () => {
    expect(
      getServerErrorMessage(axiosLike(400, { success: false, message: '原文' }))
    ).toBe('原文')
    expect(
      getServerErrorMessage({ success: false, code: 'brand_new' }, '保存失败')
    ).toBe('保存失败')
  })

  test('localized message_key takes precedence over the status fallback', () => {
    expect(
      getServerErrorMessage(
        axiosLike(401, {
          success: false,
          message_key: 'Session expired!',
          message: 'Session expired!',
        })
      )
    ).toBe('会话已过期！')
  })

  test('en locale', async () => {
    await i18next.changeLanguage('en')
    try {
      expect(getServerErrorMessage(axiosLike(401, {}))).toBe(
        'Please sign in first'
      )
      expect(getServerErrorMessage(axiosLike(403, {}))).toBe(
        "You don't have permission"
      )
    } finally {
      await i18next.changeLanguage('zh')
    }
  })
})

describe('B-07 TopupBonusTiers codes', () => {
  const err = (code: string, params: Record<string, unknown>) =>
    getServerErrorMessage({ success: false, code, message: 'x', params })
  test('topup_tiers_count_mismatch', () => {
    expect(
      err('topup_tiers_count_mismatch', {
        key: 'TopupBonusTiers',
        expected_count: 13,
        actual_count: 12,
      })
    ).toBe('参数 充值档位与赠送阶梯 需要 13 档比例，实际填了 12 档')
  })
  test('topup_tiers_not_ascending with index → 第 index+1 档', () => {
    expect(
      err('topup_tiers_not_ascending', {
        key: 'TopupBonusTiers',
        index: 3,
        value: 2,
        prev_value: 5,
      })
    ).toBe('参数 充值档位与赠送阶梯 第 4 档（2）低于上一档（5），各档比例不能递减')
    expect(err('topup_tiers_not_ascending', { key: 'TopupBonusTiers' })).toBe(
      '参数 充值档位与赠送阶梯 各档比例不能递减'
    )
  })
  test('out_of_range with index shows tier; index 0 → 第 1 档', () => {
    expect(
      err('out_of_range', {
        key: 'TopupBonusTiers',
        index: 0,
        value: 120,
        min: 0,
        max: 100,
      })
    ).toBe('参数 充值档位与赠送阶梯 第 1 档超出范围（0 ~ 100）')
  })
  test('out_of_range without index unchanged', () => {
    expect(
      err('out_of_range', {
        key: 'AffMinWithdrawAmount',
        value: 0,
        min: 1,
        max: 100,
      })
    ).toBe('参数 最低提现金额 超出范围（1 ~ 100）')
  })
})

describe('B-08 param_rule_violation and /api/option path', () => {
  const err = (code: string, params: Record<string, unknown>) =>
    getServerErrorMessage({ success: false, code, message: 'x', params })
  test('greater_than / less_than with both keys localized', () => {
    expect(
      err('param_rule_violation', {
        key: 'DiskAlertCritPct',
        rule: 'greater_than',
        other_key: 'DiskAlertWarnPct',
      })
    ).toBe('参数 磁盘紧急线 % 必须大于 磁盘告警线 %')
    expect(
      err('param_rule_violation', {
        key: 'DiskAlertWarnPct',
        rule: 'less_than',
        other_key: 'DiskAlertCritPct',
      })
    ).toBe('参数 磁盘告警线 % 必须小于 磁盘紧急线 %')
  })
  test('unknown rule → generic; unknown keys stay raw', () => {
    expect(
      err('param_rule_violation', {
        key: 'NewA',
        rule: 'not_equal',
        other_key: 'NewB',
      })
    ).toBe('参数 NewA 与 NewB 的取值冲突')
    expect(err('param_rule_violation', { rule: 'x' })).toBe(
      '参数取值相互冲突，请检查输入'
    )
  })
  test('AdtyTelegramBotToken key shows Chinese label, never raw key', () => {
    expect(err('unknown_key', { key: 'AdtyTelegramBotToken' })).toBe(
      '未知参数：Telegram 机器人令牌'
    )
    expect(
      err('param_rule_violation', {
        key: 'AdtyTelegramBotToken',
        rule: 'not_equal',
        other_key: 'AdtyTelegramBotToken',
      })
    ).not.toContain('AdtyTelegramBotToken')
  })
  test('unknown_key from batch save shows {key}', () => {
    expect(err('unknown_key', { key: 'NoSuchParam' })).toBe(
      '未知参数：NoSuchParam'
    )
  })
  test('/api/option HTTP 200 success:false with code goes through requireServerSuccess → code text', () => {
    let caught: unknown
    try {
      requireServerSuccess({
        success: false,
        message: 'value out of range',
        code: 'out_of_range',
        params: { key: 'TopupBonusTiers', index: 2, value: 120, min: 0, max: 100 },
      })
    } catch (e) {
      caught = e
    }
    expect(getServerErrorMessage(caught, 'fallback')).toBe(
      '参数 充值档位与赠送阶梯 第 3 档超出范围（0 ~ 100）'
    )
  })
  test('/api/option without code keeps upstream message', () => {
    let caught: unknown
    try {
      requireServerSuccess({ success: false, message: 'upstream says no' })
    } catch (e) {
      caught = e
    }
    expect(getServerErrorMessage(caught, 'fallback')).toBe('upstream says no')
  })
})
