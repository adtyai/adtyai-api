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
import type { IconBaseProps, IconType } from 'react-icons'
import { LuCreditCard } from 'react-icons/lu'
import { SiAlipay, SiTether, SiWechat } from 'react-icons/si'

// Payment method icons are a fixed set: Alipay, WeChat Pay, USDT and a
// generic card. Other stored names render nothing.
const PAYMENT_ICONS: Record<string, IconType> = {
  SiAlipay,
  SiWechat,
  SiTether,
  LuCreditCard,
}

type ReactIconByNameProps = IconBaseProps & {
  name?: string | null
}

export function ReactIconByName({ name, ...props }: ReactIconByNameProps) {
  const Icon = name ? PAYMENT_ICONS[name.trim()] : undefined
  return Icon ? <Icon {...props} /> : null
}
