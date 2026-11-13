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
import { createFileRoute, notFound, redirect } from '@tanstack/react-router'

import { prepareLangHome } from '@/features/adty-seo/home-language'
import { Home } from '@/features/home'

/**
 * Official non-English home page is `/zh-cn`. Leftover unofficial segments
 * (`/zh-tw`, `/fr`, `/ja`, `/ko`, `/ru`, `/vi`) stay registered so old links
 * do not 404, but they resolve to the official language and redirect. Any
 * other single segment is a 404. Static routes such as /pricing and /docs
 * win over this one.
 */
export const Route = createFileRoute('/$lang')({
  beforeLoad: async ({ params, location }) => {
    const prepared = await prepareLangHome(params.lang)
    if (!prepared) throw notFound()
    if (prepared.redirectTo) {
      throw redirect({
        to: prepared.redirectTo,
        search: location.search,
        replace: true,
      })
    }
  },
  component: Home,
})
