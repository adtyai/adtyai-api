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
// ADTY task 9.3: build-time prerender of the home page.
//
//   bun scripts/adty/prerender-home.tsx     (run by `bun run build` after rsbuild)
//
// Renders the landing page in the seven languages and writes
// dist/adty-home.json ({"en": "<html>", "zh-CN": "<html>", ...}). The Go
// server inserts the page's language before #root (router/adty_seo.go).
import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { renderAllAdtyHomes } from '../../src/features/adty-home/prerender'

const out = resolve(import.meta.dirname, '../../dist/adty-home.json')
const pages = await renderAllAdtyHomes()
writeFileSync(out, JSON.stringify(pages))
console.log(
  `prerender-home: wrote ${Object.keys(pages).length} languages to dist/adty-home.json`
)
