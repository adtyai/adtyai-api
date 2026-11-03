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
import { createFileRoute, notFound } from '@tanstack/react-router'

import { prepareLangHome } from '@/features/adty-seo/home-language'
import { Home } from '@/features/home'

/**
 * Home page in one of the six non-English languages (adty task 9.3):
 * /zh-cn, /zh-tw, /fr, /ja, /ru, /vi. Any other single segment is a 404.
 * Static routes such as /pricing and /docs win over this one.
 */
export const Route = createFileRoute('/$lang')({
  beforeLoad: async ({ params }) => {
    const lang = await prepareLangHome(params.lang)
    if (!lang) throw notFound()
  },
  component: Home,
})
