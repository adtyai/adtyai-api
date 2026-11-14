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
import { expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

const enPack = en.translation as Record<string, string>
const zhPack = zh.translation as Record<string, string>

const officialTerms: Array<[string, string]> = [
  ['Principal', '本金'],
  ['Permanent bonus', '永久赠金'],
  ['Frozen amount', '冻结金额'],
  ['Available Balance', '可用余额'],
  ['Earnings', '收益'],
  ['Deposit', '保证金'],
  ['Manual refund', '人工退款'],
  ['Commission release', '佣金释放'],
  ['Clawback', '冲回'],
  ['Recovery', '追偿'],
]

test('official zh and en packs name the A.20.2 glossary terms', () => {
  expect(Object.keys(zhPack).sort()).toEqual(Object.keys(enPack).sort())
  for (const [english, chinese] of officialTerms) {
    expect(enPack[english]).toBe(english)
    expect(zhPack[english]).toBe(chinese)
  }
})

test('fund and audit copy uses official permanent bonus, frozen amount, manual refund, and commission-state terms', () => {
  expect(enPack['Bonus used']).toBe('Permanent bonus used')
  expect(zhPack['Bonus used']).toBe('已用永久赠金')
  expect(enPack['Bonus remaining']).toBe('Remaining permanent bonus')
  expect(zhPack['Bonus remaining']).toBe('剩余永久赠金')
  expect(zhPack['Remaining permanent bonus']).toBe('剩余永久赠金')

  expect(enPack['Frozen amount']).toBe('Frozen amount')
  expect(zhPack['Frozen amount']).toBe('冻结金额')

  expect(enPack['Manual refund']).toBe('Manual refund')
  expect(zhPack['Manual refund']).toBe('人工退款')
  expect(enPack['Refund order']).toBe('Manual refund')
  expect(zhPack['Refund order']).toBe('人工退款')

  expect(zhPack['Commission clawback']).toBe('佣金冲回')
  expect(zhPack['Partial commission clawback']).toBe('佣金部分冲回')
  expect(zhPack['Clawed back']).toBe('已冲回')
  expect(zhPack['Commission voided']).toBe('佣金释放')
  expect(enPack['Commission voided']).toBe('Commission release')
  expect(zhPack['Claw back unsettled and withdrawable earnings']).toBe(
    '冲回待结算和可提现的收益'
  )

  const refunded = 'Refunded top-ups cancel or take back their commission.'
  expect(enPack[refunded]).toBe(
    'Refunded top-ups are handled as commission release, clawback, or recovery.'
  )
  expect(zhPack[refunded]).toBe(
    '充值被退款时，对应的佣金会按状态释放、冲回或追偿。'
  )

  const settles =
    'Commission settles {{days}} days after the top-up. Refunded top-ups void or claw back their commission, and the balance can go below zero.'
  expect(enPack[settles]).toContain('commission release, clawback, or recovery')
  expect(zhPack[settles]).toContain('释放、冲回或追偿')
  expect(zhPack[settles]).not.toContain('扣回')
  expect(zhPack[settles]).not.toContain('追回')
  expect(zhPack[refunded]).not.toContain('扣回')
  expect(zhPack[refunded]).not.toContain('追回')

  // Intentionally left alone: wallet take-back and unrecoverable transfers
  // are not commission 冲回 / 追偿.
  expect(zhPack['Wallet credit taken back']).toBe('钱包已扣回额度')
  expect(zhPack['Principal credit taken back']).toBe('已扣回本金额度')
  expect(zhPack['A wrong network or address cannot be recovered.']).toBe(
    '链或地址填错无法追回。'
  )
  expect(
    zhPack[
      'Only the {{chain}} network is accepted. Transfers on other networks cannot be recovered.'
    ]
  ).toBe('只认 {{chain}} 网络，转到其他网络无法追回。')
  expect(zhPack['Gift reserve']).toBe('赠金保障预留')
  expect(zhPack['A team top-up does not include a payment-method gift.']).toBe(
    '团队充值不包含支付方式赠金。'
  )
})
