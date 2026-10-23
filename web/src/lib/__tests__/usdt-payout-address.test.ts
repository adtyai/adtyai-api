import { describe, expect, it } from 'vitest'

import { validateUsdtPayoutAddress } from '@/lib/usdt-chains'

describe('validateUsdtPayoutAddress', () => {
  const tron = 'T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb'
  const evm = '0x0000000000000000000000000000000000000001'

  it('accepts a TRON address only on TRC20', () => {
    expect(validateUsdtPayoutAddress(' trc20 ', ` ${tron} `)).toBeNull()
    expect(validateUsdtPayoutAddress('trc20', evm)).toBe(
      'withdraw_address_invalid'
    )
  })

  it('accepts an EVM address on the other three chains and rejects a TRON address', () => {
    for (const chain of ['bep20', 'polygon', 'xlayer']) {
      expect(validateUsdtPayoutAddress(chain, evm)).toBeNull()
      expect(validateUsdtPayoutAddress(chain, tron)).toBe(
        'withdraw_address_invalid'
      )
    }
  })

  it('rejects a missing or unknown chain', () => {
    expect(validateUsdtPayoutAddress('', tron)).toBe('invalid_usdt_chain')
    expect(validateUsdtPayoutAddress('erc20', tron)).toBe('invalid_usdt_chain')
  })
})
