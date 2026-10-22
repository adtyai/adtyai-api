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

/** Checkout networks. Trade types match BEpusdt and setting/usdt_chain_adty.go. */
export const USDT_CHAIN_CATALOG = [
  {
    id: 'trc20',
    tradeType: 'usdt.trc20',
    label: 'USDT-TRC20',
    network: 'TRON (TRC20)',
  },
  {
    id: 'bep20',
    tradeType: 'usdt.bep20',
    label: 'USDT-BEP20',
    network: 'BNB Smart Chain (BEP20)',
  },
  {
    id: 'polygon',
    tradeType: 'usdt.polygon',
    label: 'USDT-Polygon',
    network: 'Polygon',
  },
  {
    id: 'xlayer',
    tradeType: 'usdt.xlayer',
    label: 'USDT-X Layer',
    network: 'X Layer',
  },
] as const

export type UsdtChainId = (typeof USDT_CHAIN_CATALOG)[number]['id']

const CHAIN_IDS = new Set<string>(USDT_CHAIN_CATALOG.map((chain) => chain.id))

export function isUsdtChainId(id: string): id is UsdtChainId {
  return CHAIN_IDS.has(id)
}

export function defaultEnabledUsdtChains(): UsdtChainId[] {
  return USDT_CHAIN_CATALOG.map((chain) => chain.id)
}

/** Keep catalog order and drop unknown ids. */
export function normalizeEnabledUsdtChains(raw: unknown): UsdtChainId[] {
  const wanted = new Set<string>()
  if (Array.isArray(raw)) {
    for (const item of raw) {
      const id = String(item).trim().toLowerCase()
      if (isUsdtChainId(id)) wanted.add(id)
    }
  }
  return USDT_CHAIN_CATALOG.map((chain) => chain.id).filter((id) =>
    wanted.has(id)
  )
}

export function parseEnabledUsdtChainsOption(
  raw: string | undefined | null
): UsdtChainId[] {
  if (raw == null || raw.trim() === '') return defaultEnabledUsdtChains()
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return normalizeEnabledUsdtChains(parsed)
  } catch {
    return []
  }
}

export function usdtChainById(id: string | undefined | null) {
  const key = (id ?? '').trim().toLowerCase()
  return USDT_CHAIN_CATALOG.find((chain) => chain.id === key)
}

const TRON_PAYOUT_ADDRESS = /^T[1-9A-HJ-NP-Za-km-z]{33}$/
const EVM_PAYOUT_ADDRESS = /^0x[0-9a-fA-F]{40}$/

/** null when the address matches the selected catalog chain. */
export function validateUsdtPayoutAddress(
  chain: string,
  address: string
): 'invalid_usdt_chain' | 'withdraw_address_invalid' | null {
  const id = chain.trim().toLowerCase()
  const value = address.trim()
  if (!isUsdtChainId(id)) return 'invalid_usdt_chain'
  const ok =
    id === 'trc20'
      ? TRON_PAYOUT_ADDRESS.test(value)
      : EVM_PAYOUT_ADDRESS.test(value)
  return ok ? null : 'withdraw_address_invalid'
}
