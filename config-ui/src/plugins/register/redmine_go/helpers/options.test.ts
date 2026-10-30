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

import { mergeCustomFields, mergeStatuses, mergeTrackers, mergeValueOptions, valueOptionLabel } from './options';
import { DEFAULT_STATUSES, DEFAULT_TRACKERS, DEV_CUSTOM_FIELDS } from '../test-utils';

describe('mergeTrackers', () => {
  it('lets the live answer win for an id present on both sides and appends observed-only trackers by name', () => {
    const merged = mergeTrackers(DEFAULT_TRACKERS, [
      { id: 1, name: 'Old bug name' },
      { id: 30, name: 'Zebra' },
      { id: 31, name: 'Alpha' },
    ]);
    expect(merged.map((t) => [t.id, t.name, t.source])).toEqual([
      [1, 'Bug', 'live'],
      [2, 'Feature', 'live'],
      [3, 'Support', 'live'],
      [31, 'Alpha', 'observed'],
      [30, 'Zebra', 'observed'],
    ]);
    expect(merged[0].defaultStatusId).toBe(1);
  });

  it('falls back to the observed trackers when the live read failed', () => {
    expect(mergeTrackers(null, [{ id: 7, name: 'Task' }])).toEqual([{ id: 7, name: 'Task', source: 'observed' }]);
  });
});

describe('mergeStatuses', () => {
  it('keeps the closed flag of live statuses and of observed-only ones', () => {
    const merged = mergeStatuses(DEFAULT_STATUSES, [
      { id: 5, name: 'Renamed', isClosed: false },
      { id: 40, name: 'Archived', isClosed: true },
    ]);
    expect(merged.find((s) => s.id === 5)).toMatchObject({ name: 'Closed', isClosed: true, source: 'live' });
    expect(merged.find((s) => s.id === 40)).toMatchObject({ name: 'Archived', isClosed: true, source: 'observed' });
    expect(merged).toHaveLength(7);
  });
});

describe('mergeValueOptions', () => {
  it('unions possible values with observed ones, live label winning, free-form values appended', () => {
    const merged = mergeValueOptions(
      [
        { value: '1', label: 'Critical' },
        { value: '2', label: 'Normal' },
      ],
      [
        { value: '1', label: 'critical (old)' },
        { value: 'env:prod-client', label: 'env:prod-client' },
        { value: '17' },
      ],
    );
    expect(merged).toEqual([
      { value: '1', label: 'Critical', source: 'live' },
      { value: '2', label: 'Normal', source: 'live' },
      { value: 'env:prod-client', label: 'env:prod-client', source: 'observed' },
      { value: '17', label: '17', source: 'observed' },
    ]);
  });
});

describe('mergeCustomFields', () => {
  it('offers issue custom fields only and merges observed values into a live field', () => {
    const merged = mergeCustomFields(DEV_CUSTOM_FIELDS, [
      {
        id: 21,
        name: 'Discovery Environment',
        fieldFormat: 'list',
        multiple: false,
        values: [
          { value: 'PROD', label: 'PROD' },
          { value: 'env:prod-client', label: 'env:prod-client' },
        ],
      },
    ]);
    expect(merged.map((f) => f.id)).toEqual([21, 43, 18, 31]);
    expect(merged[0].values.map((v) => [v.value, v.source])).toEqual([
      ['PROD', 'live'],
      ['ACC', 'live'],
      ['UAT', 'live'],
      ['DEV', 'live'],
      ['env:prod-client', 'observed'],
    ]);
  });

  it('is built from observed values alone for a non-admin key, sorted by name', () => {
    const merged = mergeCustomFields(null, [
      {
        id: 43,
        name: 'Ticket Severity',
        fieldFormat: 'list',
        multiple: false,
        values: [{ value: '1', label: 'Critical' }],
      },
      { id: 18, name: 'Bug Severity', fieldFormat: 'list', multiple: true, values: [] },
    ]);
    expect(merged.map((f) => [f.id, f.source, f.multiple])).toEqual([
      [18, 'observed', true],
      [43, 'observed', false],
    ]);
    expect(merged[1].values).toEqual([{ value: '1', label: 'Critical', source: 'observed' }]);
  });
});

describe('valueOptionLabel', () => {
  it('shows the raw value next to a label that differs', () => {
    expect(valueOptionLabel({ value: '1', label: 'Critical' })).toBe('Critical (1)');
    expect(valueOptionLabel({ value: 'PROD', label: 'PROD' })).toBe('PROD');
    expect(valueOptionLabel({ value: 'x' })).toBe('x');
  });
});
