/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

import { describe, it, expect } from 'vitest';

import { addValueMapField, buildValueMapRows, setValueMapEntry } from './value-maps';

describe('addValueMapField', () => {
  it('appends a field last, since the first field with a value wins, and never twice', () => {
    const one = addValueMapField([], { id: 18, name: 'Bug Severity' });
    const two = addValueMapField(one, { id: 43, name: 'Ticket Severity' });
    expect(two.map((f) => f.field.id)).toEqual([18, 43]);
    expect(addValueMapField(two, { id: 18, name: 'Bug Severity' })).toBe(two);
  });
});

describe('buildValueMapRows', () => {
  it('lists the known values with their entries, then entries for values nobody lists', () => {
    const rows = buildValueMapRows(
      [
        { value: 'PROD', to: 'PRODUCTION' },
        { value: 'env:prod-client', label: 'observed registry key', to: 'PRODUCTION' },
      ],
      [
        { value: 'PROD', label: 'PROD', source: 'live' },
        { value: 'ACC', label: 'ACC', source: 'live' },
      ],
    );
    expect(rows).toEqual([
      { value: 'PROD', label: 'PROD', to: 'PRODUCTION', known: true },
      { value: 'ACC', label: 'ACC', to: undefined, known: true },
      { value: 'env:prod-client', label: 'observed registry key', to: 'PRODUCTION', known: false },
    ]);
  });
});

describe('setValueMapEntry', () => {
  it('stores the label only when it differs from the value', () => {
    expect(setValueMapEntry([], '1', 'Critical', 'Critical')).toEqual([
      { value: '1', label: 'Critical', to: 'Critical' },
    ]);
    expect(setValueMapEntry([], 'PROD', 'PROD', 'PRODUCTION')).toEqual([{ value: 'PROD', to: 'PRODUCTION' }]);
  });

  it('replaces an entry in place and removes it when the target is emptied', () => {
    const entries = [
      { value: 'a', to: 'X' },
      { value: 'b', to: 'Y' },
    ];
    expect(setValueMapEntry(entries, 'a', undefined, 'Z')).toEqual([
      { value: 'a', to: 'Z' },
      { value: 'b', to: 'Y' },
    ]);
    expect(setValueMapEntry(entries, 'a', undefined, '  ')).toEqual([{ value: 'b', to: 'Y' }]);
  });
});
