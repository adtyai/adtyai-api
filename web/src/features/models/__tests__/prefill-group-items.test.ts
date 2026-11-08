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
import { expect, it } from 'vitest'

import { parseEndpointKeys } from '../components/prefill-group-shared'

it('keeps endpoint names and duplicates while discarding entries without a string name', () => {
  expect(
    parseEndpointKeys(
      '["chat",{"name":"responses"},null,0,false,{}, {"name":7},"",{"name":""},"  ","chat"]'
    )
  ).toEqual(['chat', 'responses', '  ', 'chat'])
})

it('keeps whitespace and duplicate names from a direct array', () => {
  expect(parseEndpointKeys(['chat', '', ' ', 'chat'])).toEqual([
    'chat',
    ' ',
    'chat',
  ])
})

it('returns object keys in JavaScript key order without changing their text', () => {
  expect(parseEndpointKeys('{"z":{},"2":{},"a":{},"1":{}," ":{}}')).toEqual([
    '1',
    '2',
    'z',
    'a',
    ' ',
  ])
})

it.each(['', '{invalid', 'null', '7', '"chat"'])(
  'returns no endpoints for noncollection input %s',
  (items) => {
    expect(parseEndpointKeys(items)).toEqual([])
  }
)
