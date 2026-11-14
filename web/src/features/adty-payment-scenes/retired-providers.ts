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
const RETIRED_HOSTED_PAYMENT_PROVIDERS = new Set([
  'stripe',
  'creem',
  'waffo',
  'waffo_pancake',
])

export function isRetiredHostedPaymentProvider(provider: string) {
  return RETIRED_HOSTED_PAYMENT_PROVIDERS.has(provider.trim().toLowerCase())
}

export function dropRetiredHostedPaymentProviders<
  T extends { providers?: string[] },
>(scene: T): T {
  return {
    ...scene,
    providers: (scene.providers ?? []).filter(
      (provider) => !isRetiredHostedPaymentProvider(provider)
    ),
  }
}

export function dropRetiredHostedPaymentProvidersFromScenes<
  T extends { providers?: string[] },
>(scenes: T[]): T[] {
  return scenes.map((scene) => dropRetiredHostedPaymentProviders(scene))
}
